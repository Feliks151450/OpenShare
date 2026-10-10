package service

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/batchatco/go-native-netcdf/netcdf/api"
)

// CF 时间编码解析：把 `<unit> since <base>` 形式的 units 属性配上 calendar 属性，
// 还原成可读的日历时间。除标准历法外，另支持气候模式常用的 noleap / 360_day 等
// 人工历法——这些历法不能用 time.Time 直接加减（年长不固定），因此这里按
// 「绝对日序」自行做历法算术。
//
// 说明：standard/gregorian 按 proleptic Gregorian 处理（不实现 1582 年儒略→格里
// 历切换），对 1582 年之前的历史时段会有一天级差异；此时展示的时间仍可作为
// 近似读数，原始数值与 units 原文都会一并呈现。

// cfTimeUnitsPattern 匹配 CF 时间单位，如 "days since 1970-01-01"。
var cfTimeUnitsPattern = regexp.MustCompile(`(?i)^\s*(milliseconds|seconds|minutes|hours|days)\s+since\s+(.+?)\s*$`)

// cfBaseTimePattern 解析基准时刻的日期与可选时间部分。
var cfBaseTimePattern = regexp.MustCompile(
	`^(\d{1,4})[-/](\d{1,2})(?:[-/](\d{1,2}))?(?:[ T](\d{1,2}):(\d{2})(?::(\d{2}))?(?:\.(\d{1,9}))?)?`,
)

// cfBaseTimeZonePattern 基准时刻尾部的时区标记；按墙上时间解析，故仅剥离不换算。
var cfBaseTimeZonePattern = regexp.MustCompile(`(?i)\s*(?:utc|gmt|z|[+-]\d{2}:?\d{2})\s*$`)

const (
	cfNanosPerSecond = int64(1e9)
	cfNanosPerMinute = int64(60) * cfNanosPerSecond
	cfNanosPerHour   = int64(60) * cfNanosPerMinute
	cfNanosPerDay    = int64(24) * cfNanosPerHour
)

// cfMaxTimeSpanNanos 限制可解析的时间跨度（约 10 万年），用于挡掉 _FillValue
// 之类的哨兵值（如 -1e34）导致年份循环失控。
const cfMaxTimeSpanNanos = int64(3155695200000000000)

// cfDate 为某历法下的墙上日期（不含时区）。
type cfDate struct {
	year  int64
	month int
	day   int
}

// cfCalendar 为一种历法：年长/月长规则由 name 决定。
type cfCalendar struct {
	name string
}

var (
	cfCalendarStandard    = cfCalendar{name: "standard"}
	cfCalendarNoleap      = cfCalendar{name: "noleap"}
	cfCalendarAllLeap     = cfCalendar{name: "366_day"}
	cfCalendarDay360      = cfCalendar{name: "360_day"}
	cfCalendarJulian      = cfCalendar{name: "julian"}
	cfCalendarUnsupported = cfCalendar{name: ""}
)

var cfCommonMonthLengths = [12]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

// resolveCFCalendar 归一化 CF calendar 属性名。未识别的历法返回不支持，
// 调用方据此放弃解析而不是猜一个错误日期。
func resolveCFCalendar(raw string) cfCalendar {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return cfCalendarStandard
	case "standard", "gregorian", "proleptic_gregorian":
		return cfCalendarStandard
	case "noleap", "365_day":
		return cfCalendarNoleap
	case "all_leap", "366_day":
		return cfCalendarAllLeap
	case "360_day":
		return cfCalendarDay360
	case "julian":
		return cfCalendarJulian
	default:
		return cfCalendarUnsupported
	}
}

func (c cfCalendar) supported() bool { return c.name != "" }

// display 为展示用的历法名；标准历法不额外标注。
func (c cfCalendar) display() string {
	if c.name == "standard" {
		return ""
	}
	return c.name
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func floorMod(a, b int64) int64 { return a - floorDiv(a, b)*b }

func (c cfCalendar) isLeap(year int64) bool {
	switch c.name {
	case cfCalendarNoleap.name:
		return false
	case cfCalendarAllLeap.name:
		return true
	case cfCalendarDay360.name:
		return false
	case cfCalendarJulian.name:
		return floorMod(year, 4) == 0
	default: // proleptic Gregorian
		return floorMod(year, 4) == 0 && (floorMod(year, 100) != 0 || floorMod(year, 400) == 0)
	}
}

func (c cfCalendar) monthLength(year int64, month int) int {
	if c.name == cfCalendarDay360.name {
		return 30
	}
	if month == 2 {
		if c.isLeap(year) {
			return 29
		}
		return 28
	}
	return cfCommonMonthLengths[month-1]
}

func (c cfCalendar) yearLength(year int64) int64 {
	switch c.name {
	case cfCalendarDay360.name:
		return 360
	case cfCalendarNoleap.name:
		return 365
	case cfCalendarAllLeap.name:
		return 366
	default:
		total := int64(0)
		for m := 1; m <= 12; m++ {
			total += int64(c.monthLength(year, m))
		}
		return total
	}
}

// daysBeforeYear 为公元元年（天文纪年，含 0 年）到 year-01-01 的天数解析式。
func (c cfCalendar) daysBeforeYear(year int64) int64 {
	switch c.name {
	case cfCalendarDay360.name:
		return year * 360
	case cfCalendarNoleap.name:
		return year * 365
	case cfCalendarAllLeap.name:
		return year * 366
	case cfCalendarJulian.name:
		return year*365 + floorDiv(year, 4)
	default:
		return year*365 + floorDiv(year, 4) - floorDiv(year, 100) + floorDiv(year, 400)
	}
}

func (c cfCalendar) toAbsDays(d cfDate) int64 {
	total := c.daysBeforeYear(d.year)
	for m := 1; m < d.month; m++ {
		total += int64(c.monthLength(d.year, m))
	}
	return total + int64(d.day-1)
}

func (c cfCalendar) fromAbsDays(abs int64) cfDate {
	avg := float64(c.yearLength(2000))
	year := int64(math.Floor(float64(abs) / avg))
	for c.daysBeforeYear(year) > abs {
		year--
	}
	for c.daysBeforeYear(year+1) <= abs {
		year++
	}
	rest := abs - c.daysBeforeYear(year)
	month := 1
	for month <= 12 {
		length := int64(c.monthLength(year, month))
		if rest < length {
			break
		}
		rest -= length
		month++
	}
	return cfDate{year: year, month: month, day: int(rest) + 1}
}

// cfTimeCodec 为一个 CF 时间变量的编码方式（单位 + 基准 + 历法）。
type cfTimeCodec struct {
	unit      string
	nanosPer  int64
	base      cfDate
	baseNanos int64
	calendar  cfCalendar
	unitsRaw  string
}

// newCFTimeCodec 解析 units / calendar 属性；不认识的格式返回 false。
func newCFTimeCodec(units, calendar string) (*cfTimeCodec, bool) {
	match := cfTimeUnitsPattern.FindStringSubmatch(units)
	if match == nil {
		return nil, false
	}

	calendarKind := resolveCFCalendar(calendar)
	if !calendarKind.supported() {
		return nil, false
	}

	baseText := cfBaseTimeZonePattern.ReplaceAllString(match[2], "")
	baseMatch := cfBaseTimePattern.FindStringSubmatch(strings.TrimSpace(baseText))
	if baseMatch == nil {
		return nil, false
	}

	year, err := strconv.ParseInt(baseMatch[1], 10, 64)
	if err != nil {
		return nil, false
	}
	month, err := strconv.Atoi(baseMatch[2])
	if err != nil || month < 1 || month > 12 {
		return nil, false
	}
	day := 1
	if baseMatch[3] != "" {
		parsed, err := strconv.Atoi(baseMatch[3])
		if err != nil || parsed < 1 || parsed > 31 {
			return nil, false
		}
		day = parsed
	}
	if max := calendarKind.monthLength(year, month); day > max {
		day = max
	}

	var nanos int64
	if baseMatch[4] != "" {
		hour, _ := strconv.Atoi(baseMatch[4])
		minute, _ := strconv.Atoi(baseMatch[5])
		second := 0
		if baseMatch[6] != "" {
			second, _ = strconv.Atoi(baseMatch[6])
		}
		if hour > 23 || minute > 59 || second > 61 {
			return nil, false
		}
		nanos = int64(hour)*cfNanosPerHour + int64(minute)*cfNanosPerMinute + int64(second)*cfNanosPerSecond
		if baseMatch[7] != "" {
			frac := baseMatch[7]
			for len(frac) < 9 {
				frac += "0"
			}
			if len(frac) > 9 {
				frac = frac[:9]
			}
			sub, err := strconv.ParseInt(frac, 10, 64)
			if err == nil {
				nanos += sub
			}
		}
	}

	unit := strings.ToLower(match[1])
	nanosPer, ok := cfUnitNanos(unit)
	if !ok {
		return nil, false
	}

	return &cfTimeCodec{
		unit:      unit,
		nanosPer:  nanosPer,
		base:      cfDate{year: year, month: month, day: day},
		baseNanos: nanos,
		calendar:  calendarKind,
		unitsRaw:  strings.TrimSpace(units),
	}, true
}

func cfUnitNanos(unit string) (int64, bool) {
	switch unit {
	case "milliseconds":
		return cfNanosPerSecond / 1000, true
	case "seconds":
		return cfNanosPerSecond, true
	case "minutes":
		return cfNanosPerMinute, true
	case "hours":
		return cfNanosPerHour, true
	case "days":
		return cfNanosPerDay, true
	default:
		return 0, false
	}
}

// label 把一个原始偏移量转成可读时间；越界或非数值返回空串。
func (c *cfTimeCodec) label(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return ""
	}
	span := value * float64(c.nanosPer)
	if math.IsNaN(span) || math.Abs(span) > float64(cfMaxTimeSpanNanos) {
		return ""
	}

	absDays := c.calendar.toAbsDays(c.base)
	totalNanos := c.baseNanos + int64(math.Round(span))
	days := floorDiv(totalNanos, cfNanosPerDay)
	rest := totalNanos - days*cfNanosPerDay
	return formatCFWallTime(c.calendar.fromAbsDays(absDays+days), rest)
}

// labelRange 为「起点 → 终点」的紧凑展示。
func (c *cfTimeCodec) labelRange(first, last float64) string {
	start := c.label(first)
	end := c.label(last)
	switch {
	case start == "" && end == "":
		return ""
	case start == "":
		return end
	case end == "":
		return start
	case start == end:
		return start
	default:
		return start + " → " + end
	}
}

func formatCFWallTime(date cfDate, nanos int64) string {
	text := fmt.Sprintf("%04d-%02d-%02d", date.year, date.month, date.day)
	if nanos <= 0 {
		return text
	}
	hour := nanos / cfNanosPerHour
	nanos -= hour * cfNanosPerHour
	minute := nanos / cfNanosPerMinute
	nanos -= minute * cfNanosPerMinute
	second := nanos / cfNanosPerSecond
	nanos -= second * cfNanosPerSecond
	text += fmt.Sprintf("T%02d:%02d:%02d", hour, minute, second)
	if nanos > 0 {
		milli := nanos / (cfNanosPerSecond / 1000)
		text += fmt.Sprintf(".%03d", milli)
	}
	return text
}

// netCDFAttrString 读取字符串属性（首元素），读不到时返回空串。
// 注意：不要调用 AttributeMap.GetType，部分 HDF5 属性会触发解析库 panic。
func netCDFAttrString(attrs api.AttributeMap, key string) string {
	if attrs == nil {
		return ""
	}
	raw, ok := attrs.Get(key)
	if !ok || raw == nil {
		return ""
	}
	switch x := raw.(type) {
	case string:
		return strings.TrimSpace(x)
	case []string:
		if len(x) > 0 {
			return strings.TrimSpace(x[0])
		}
	}
	return ""
}

// netCDFNumberAt 读取变量第 index 个元素并转为 float64。
// GetSlice 的 end 为开区间，传 (index, 1) 会在 index>0 时越界失败。
func netCDFNumberAt(vg api.VarGetter, index int64) (float64, bool) {
	raw, err := vg.GetSlice(index, index+1)
	if err != nil || raw == nil {
		return 0, false
	}
	return netCDFNumberValue(raw)
}

// netCDFNumberValue 从 GetSlice 返回的切片（或标量）中取首个数值。
func netCDFNumberValue(raw any) (float64, bool) {
	rv := reflect.ValueOf(raw)
	for rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		if rv.Len() == 0 {
			return 0, false
		}
		rv = rv.Index(0)
	}
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		return rv.Float(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint()), true
	default:
		return 0, false
	}
}

// cfTimeCodecForVar 依据变量的 units / calendar 属性构造时间编码器。
func cfTimeCodecForVar(vg api.VarGetter) (*cfTimeCodec, bool) {
	if vg == nil {
		return nil, false
	}
	units := netCDFAttrString(vg.Attributes(), "units")
	if units == "" {
		return nil, false
	}
	return newCFTimeCodec(units, netCDFAttrString(vg.Attributes(), "calendar"))
}

// dimensionRangeInfo 解析「与维度同名的坐标变量」，得到该维的取值范围与单位。
// 时间坐标（CF units 可解析）给出可读时刻范围；lat/lon 等数值坐标给出首末原始值。
// 末值直接从文件末尾读取，不受变量值展示上限影响。
func dimensionRangeInfo(g api.Group, dimName string) (rangeText, units, calendar string) {
	if g == nil {
		return "", "", ""
	}
	vg, err := g.GetVarGetter(dimName)
	if err != nil || vg == nil {
		return "", "", ""
	}
	if dims := vg.Dimensions(); len(dims) != 1 || dims[0] != dimName {
		return "", "", ""
	}
	total := vg.Len()
	if total <= 0 {
		return "", "", ""
	}
	first, ok := netCDFNumberAt(vg, 0)
	if !ok {
		return "", "", ""
	}
	last := first
	if total > 1 {
		if value, ok := netCDFNumberAt(vg, total-1); ok {
			last = value
		}
	}

	if codec, ok := cfTimeCodecForVar(vg); ok {
		return codec.labelRange(first, last), codec.unitsRaw, codec.calendar.display()
	}
	return formatCoordinateRange(first, last, vg.Type()), "", ""
}

// formatCoordinateRange 按坐标变量的声明类型格式化首末原始值。
// bitSize 用 32 时可将 float32 提升为 float64 后产生的伪精度数字（0.30000001…）
// 还原成最短写法。
func formatCoordinateRange(first, last float64, cdlType string) string {
	format := func(f float64) string {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return ""
		}
		bitSize := 64
		if strings.EqualFold(strings.TrimSpace(cdlType), "float") {
			bitSize = 32
		}
		return strconv.FormatFloat(f, 'g', -1, bitSize)
	}
	start, end := format(first), format(last)
	switch {
	case start == "" && end == "":
		return ""
	case start == "" || start == end:
		return end
	case end == "":
		return start
	default:
		return start + " → " + end
	}
}
