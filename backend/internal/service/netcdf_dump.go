package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/batchatco/go-native-netcdf/netcdf"
	"github.com/batchatco/go-native-netcdf/netcdf/api"
)

// MaxNetCDFDumpBytes 限制摘要输出大小，避免超大文件拖慢服务。
const MaxNetCDFDumpBytes = 512 * 1024

var (
	// ErrNetCDFNotApplicable 表示文件扩展名不是 .nc，不应走 NetCDF 摘要。
	ErrNetCDFNotApplicable = errors.New("not a netcdf file")
)

// NetCDFDumpAttr 为 NetCDF 属性的键值（值为与 ncdump 相近的可读字符串）。
type NetCDFDumpAttr struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// NetCDFDumpDim 为维度名与长度。
type NetCDFDumpDim struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	// Range 为该维度对应坐标变量的取值范围（首值 → 末值）：时间坐标为可读时刻，
	// lat/lon 等数值坐标为原始数值。
	Range string `json:"range,omitempty"`
	// TimeUnits 为时间坐标变量原始 units 属性，仅在时间维时返回。
	TimeUnits string `json:"time_units,omitempty"`
	// TimeCalendar 为非标准历法名（360_day / noleap 等），标准历法为空。
	TimeCalendar string `json:"time_calendar,omitempty"`
}

// NetCDFDumpVar 为变量的类型、维度、形状与属性（不含数据值）。
type NetCDFDumpVar struct {
	Name            string           `json:"name"`
	Type            string           `json:"type"`
	Dimensions      []string         `json:"dimensions,omitempty"`
	Shape           []int64          `json:"shape,omitempty"`
	Attributes      []NetCDFDumpAttr `json:"attributes,omitempty"`
	Unreadable      bool             `json:"unreadable,omitempty"`
	Values          []string         `json:"values,omitempty"`
	ValuesTruncated bool             `json:"values_truncated,omitempty"`
	// TimeValues 与 Values 一一对应：CF 时间变量解析出的可读时间，无法解析的项为空串。
	TimeValues []string `json:"time_values,omitempty"`
	// TimeUnits / TimeCalendar 为时间变量的原始 units 与（非标准）历法名。
	TimeUnits    string `json:"time_units,omitempty"`
	TimeCalendar string `json:"time_calendar,omitempty"`
}

// NetCDFDumpGroup 为单个组（含根组）的结构化摘要。
type NetCDFDumpGroup struct {
	Path             string            `json:"path"`
	GlobalAttributes []NetCDFDumpAttr  `json:"global_attributes,omitempty"`
	Dimensions       []NetCDFDumpDim   `json:"dimensions,omitempty"`
	Variables        []NetCDFDumpVar   `json:"variables,omitempty"`
	Subgroups        []NetCDFDumpGroup `json:"subgroups,omitempty"`
}

// PrepareNetCDFDump 打开托管卷内 .nc 文件，生成类似 ncdump -h 的文本摘要（不含变量具体取值），
// 并返回便于前端渲染的结构化树（structure）。
// go-native-netcdf 对部分 HDF5 属性遇内部 assert 会 panic，此处统一 recover 并转为错误。
func (s *PublicDownloadService) PrepareNetCDFDump(ctx context.Context, fileID string) (out string, structure *NetCDFDumpGroup, truncated bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			out = ""
			structure = nil
			truncated = false
			err = fmt.Errorf("读取 NetCDF 结构失败（文件格式或属性类型可能超出当前解析库支持）: %v", r)
		}
	}()

	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return "", nil, false, ErrDownloadFileNotFound
	}
	file, err := s.repository.FindManagedFileByID(ctx, fileID)
	if err != nil {
		return "", nil, false, fmt.Errorf("find file for netcdf dump: %w", err)
	}
	if file == nil {
		return "", nil, false, ErrDownloadFileNotFound
	}

	ext := strings.ToLower(strings.TrimPrefix(file.Extension, "."))
	if ext != "nc" {
		return "", nil, false, ErrNetCDFNotApplicable
	}

	allowed, err := s.EffectiveDownloadAllowedForFile(ctx, file)
	if err != nil {
		return "", nil, false, err
	}
	if !allowed && !inlinePlaybackAllowedWhenDownloadForbidden(file.MimeType, file.Name) {
		return "", nil, false, ErrDownloadForbidden
	}

	diskPath, err := s.resolveManagedFilePath(ctx, file)
	if err != nil {
		return "", nil, false, err
	}

	g, err := netcdf.Open(diskPath)
	if err != nil {
		return "", nil, false, fmt.Errorf("open netcdf: %w", err)
	}
	defer g.Close()

	root := &NetCDFDumpGroup{Path: "/"}
	var sb strings.Builder
	truncFlag := false
	dumpErr := writeNetCDFGroup(&sb, &truncFlag, "", g, 0, 24, root)
	if dumpErr != nil {
		return "", nil, truncFlag, dumpErr
	}
	out = sb.String()
	structure = root
	if truncFlag {
		return out, structure, true, nil
	}
	return out, structure, false, nil
}

func writeNetCDFGroup(sb *strings.Builder, trunc *bool, indent string, g api.Group, depth, maxDepth int, node *NetCDFDumpGroup) error {
	if depth > maxDepth {
		fmt.Fprintf(sb, "%s// … 子组嵌套过深，已省略\n", indent)
		return nil
	}
	writeAttrs(sb, trunc, indent, "// global attributes:\n", g.Attributes(), &node.GlobalAttributes)

	dims := g.ListDimensions()
	if len(dims) > 0 {
		tryWrite(sb, trunc, fmt.Sprintf("%s// dimensions:\n", indent))
		for _, name := range dims {
			size, ok := g.GetDimension(name)
			if !ok {
				continue
			}
			entry := NetCDFDumpDim{Name: name, Size: int64(size)}
			entry.Range, entry.TimeUnits, entry.TimeCalendar = dimensionRangeInfo(g, name)
			comment := ""
			if entry.Range != "" {
				comment = fmt.Sprintf(" // %s", entry.Range)
			}
			tryWrite(sb, trunc, fmt.Sprintf("%s%s = %d ;%s\n", indent, name, size, comment))
			node.Dimensions = append(node.Dimensions, entry)
		}
		tryWrite(sb, trunc, "\n")
	}

	vars := g.ListVariables()
	if len(vars) > 0 {
		tryWrite(sb, trunc, fmt.Sprintf("%s// variables:\n", indent))
		for _, name := range vars {
			vg, err := g.GetVarGetter(name)
			if err != nil || vg == nil {
				tryWrite(sb, trunc, fmt.Sprintf("%s// %s: (无法读取变量信息)\n", indent, name))
				node.Variables = append(node.Variables, NetCDFDumpVar{Name: name, Unreadable: true})
				continue
			}
			vDims := vg.Dimensions()
			shape := vg.Shape()
			typeStr := vg.Type()
			tryWrite(sb, trunc, fmt.Sprintf("%s%s %s ", indent, typeStr, name))
			if len(vDims) > 0 {
				tryWrite(sb, trunc, "(")
				for i, d := range vDims {
					if i > 0 {
						tryWrite(sb, trunc, ", ")
					}
					tryWrite(sb, trunc, d)
				}
				tryWrite(sb, trunc, ")")
			}
			if len(shape) > 0 {
				tryWrite(sb, trunc, fmt.Sprintf(" // shape: %v", shape))
			}
			tryWrite(sb, trunc, " ;\n")
			attrIndent := indent + "    "
			vEntry := NetCDFDumpVar{
				Name:       name,
				Type:       typeStr,
				Dimensions: append([]string(nil), vDims...),
				Shape:      append([]int64(nil), shape...),
			}
			writeAttrs(sb, trunc, attrIndent, "", vg.Attributes(), &vEntry.Attributes)
			// 一维变量：读取变量值供前端预览展示
			if len(vDims) == 1 {
				vals, numbers, vt := readOneDimValues(vg)
				vEntry.Values = vals
				vEntry.ValuesTruncated = vt
				if codec, ok := cfTimeCodecForVar(vg); ok {
					vEntry.TimeUnits = codec.unitsRaw
					vEntry.TimeCalendar = codec.calendar.display()
					labels := make([]string, len(numbers))
					for i, number := range numbers {
						labels[i] = codec.label(number)
					}
					vEntry.TimeValues = labels
				}
			}
			node.Variables = append(node.Variables, vEntry)
		}
		tryWrite(sb, trunc, "\n")
	}

	subs := g.ListSubgroups()
	for _, subName := range subs {
		sub, err := g.GetGroup(subName)
		if err != nil || sub == nil {
			tryWrite(sb, trunc, fmt.Sprintf("%s// group %q: (无法打开)\n", indent, subName))
			continue
		}
		childPath := subGroupPath(node.Path, subName)
		child := &NetCDFDumpGroup{Path: childPath}
		tryWrite(sb, trunc, fmt.Sprintf("%sgroup: %s {\n", indent, subName))
		nextIndent := indent + "  "
		if err := writeNetCDFGroup(sb, trunc, nextIndent, sub, depth+1, maxDepth, child); err != nil {
			sub.Close()
			return err
		}
		tryWrite(sb, trunc, fmt.Sprintf("%s}\n\n", indent))
		sub.Close()
		node.Subgroups = append(node.Subgroups, *child)
	}
	return nil
}

func subGroupPath(parentPath, name string) string {
	p := strings.TrimSpace(parentPath)
	if p == "" || p == "/" {
		return "/" + name
	}
	return p + "/" + name
}

func writeAttrs(sb *strings.Builder, trunc *bool, indent, header string, attrs api.AttributeMap, out *[]NetCDFDumpAttr) {
	if attrs == nil {
		return
	}
	keys := attrs.Keys()
	if len(keys) == 0 {
		return
	}
	if header != "" {
		tryWrite(sb, trunc, fmt.Sprintf("%s%s", indent, header))
	}
	for _, k := range keys {
		raw, ok := attrs.Get(k)
		if !ok {
			continue
		}
		// 勿调用 attrs.GetType：部分 HDF5 全局属性会触发 go-native-netcdf 内部 panic（didn't find attribute type）。
		valStr := formatNetCDFAttrValue(raw)
		tryWrite(sb, trunc, fmt.Sprintf("%s%s = %s\n", indent, k, valStr))
		if out != nil {
			*out = append(*out, NetCDFDumpAttr{Key: k, Value: valStr})
		}
	}
	if len(keys) > 0 && header != "" {
		tryWrite(sb, trunc, "\n")
	}
}

// normalizeNetCDFAttrText 将属性字符串压成单行展示：去掉不必要的引号与 Go 风格转义（如 \n），
// 换行与制表改为空格并合并连续空白，便于在 ncdump 文本与 Markdown 表格中阅读。
func normalizeNetCDFAttrText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func formatNetCDFAttrValue(v any) string {
	switch x := v.(type) {
	case string:
		return normalizeNetCDFAttrText(x)
	case []string:
		if len(x) == 0 {
			return ""
		}
		if len(x) <= 8 {
			parts := make([]string, len(x))
			for i := range x {
				parts[i] = normalizeNetCDFAttrText(x[i])
			}
			return strings.Join(parts, ", ")
		}
		return fmt.Sprintf("%d strings（首项 %s …）", len(x), normalizeNetCDFAttrText(x[0]))
	case []byte:
		if len(x) == 0 {
			return ""
		}
		if len(x) > 64 {
			return fmt.Sprintf("<opaque %d bytes>", len(x))
		}
		if !utf8.ValidString(string(x)) {
			return fmt.Sprintf("<opaque %d bytes>", len(x))
		}
		return normalizeNetCDFAttrText(string(x))
	default:
		return fmt.Sprintf("%v", v)
	}
}

// PrepareNetCDFDumpFallback 使用系统 ncdump -h 获取 NetCDF 文件头部信息，用于 go-native-netcdf 库无法打开文件时的回退方案。
func (s *PublicDownloadService) PrepareNetCDFDumpFallback(ctx context.Context, fileID string) (text string, truncated bool, err error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return "", false, ErrDownloadFileNotFound
	}
	file, err := s.repository.FindManagedFileByID(ctx, fileID)
	if err != nil {
		return "", false, fmt.Errorf("find file for netcdf dump fallback: %w", err)
	}
	if file == nil {
		return "", false, ErrDownloadFileNotFound
	}

	ext := strings.ToLower(strings.TrimPrefix(file.Extension, "."))
	if ext != "nc" {
		return "", false, ErrNetCDFNotApplicable
	}

	diskPath, err := s.resolveManagedFilePath(ctx, file)
	if err != nil {
		return "", false, err
	}

	ncdumpPath, lookErr := exec.LookPath("ncdump")
	if lookErr != nil {
		return "", false, fmt.Errorf("ncdump not found: %w", lookErr)
	}

	cmd := exec.Command(ncdumpPath, "-h", diskPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if runErr := cmd.Run(); runErr != nil {
		return "", false, fmt.Errorf("ncdump -h failed: %w (stderr: %s)", runErr, strings.TrimSpace(stderr.String()))
	}

	raw := stdout.String()
	if len(raw) > MaxNetCDFDumpBytes {
		raw = raw[:MaxNetCDFDumpBytes] + "\n… 输出已超过长度上限，已截断 …\n"
		truncated = true
	}

	return raw, truncated, nil
}

// max1DValues 为单维变量展示的数值上限，避免变量过大时内存/输出溢出。
const max1DValues = 100

// readOneDimValues 从 VarGetter 读取一维变量的值并格式化为字符串切片。
// numbers 与 values 一一对应，保存原始数值（非数值元素为 NaN），供 CF 时间解析使用。
// 限量读取：仅物化前 max1DValues 个值。原实现 vg.Values() 会把整个一维变量
// 读进内存（1e8 元素 double ≈ 800MB 峰值）之后才截断，构成公开端点上的
// 内存放大 DoS 面。
func readOneDimValues(vg api.VarGetter) (values []string, numbers []float64, truncated bool) {
	total := vg.Len()
	if total <= 0 {
		return nil, nil, false
	}
	limit := int64(max1DValues)
	if total > limit {
		truncated = true
	} else {
		limit = total
	}
	raw, err := vg.GetSlice(0, limit)
	if err != nil || raw == nil {
		// GetSlice 不可用时退回全量读取（旧行为兜底）
		raw, err = vg.Values()
		if err != nil || raw == nil {
			return nil, nil, false
		}
	}
	rv := reflect.ValueOf(raw)
	if rv.Kind() != reflect.Slice {
		return nil, nil, false
	}
	n := rv.Len()
	if n == 0 {
		return nil, nil, false
	}
	if n > max1DValues {
		n = max1DValues
		truncated = true
	}
	values = make([]string, n)
	numbers = make([]float64, n)
	for i := 0; i < n; i++ {
		item := rv.Index(i)
		values[i] = formatAtom(item)
		numbers[i] = atomFloat(item)
	}
	return values, numbers, truncated
}

// atomFloat 将数值元素转为 float64，非数值返回 NaN。
func atomFloat(v reflect.Value) float64 {
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		return v.Float()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint())
	default:
		return math.NaN()
	}
}

// formatAtom 将 Go 基础数值/字符串转为可读字符串，特殊处理 NaN/Inf。
func formatAtom(v reflect.Value) string {
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsNaN(f) {
			return "NaN"
		}
		if math.IsInf(f, 1) {
			return "Infinity"
		}
		if math.IsInf(f, -1) {
			return "-Infinity"
		}
		// 保留最多 8 位小数，去除尾部零
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.8f", f), "0"), ".")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Uint())
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

func tryWrite(sb *strings.Builder, trunc *bool, s string) {
	if *trunc {
		return
	}
	if sb.Len()+len(s) > MaxNetCDFDumpBytes {
		sb.WriteString("\n… 输出已超过长度上限，已截断 …\n")
		*trunc = true
		return
	}
	sb.WriteString(s)
}
