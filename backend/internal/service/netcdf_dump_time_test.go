package service

import (
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/batchatco/go-native-netcdf/netcdf"
	"github.com/batchatco/go-native-netcdf/netcdf/api"
	"github.com/batchatco/go-native-netcdf/netcdf/util"
)

// writeTestNetCDF 用解析库自带的写接口生成真实的 classic NetCDF 文件，
// 让维度范围解析在「真实文件 → 读取 → 摘要」全链路上得到验证。
func writeTestNetCDF(t *testing.T, calendar string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "sample.nc")
	w, err := netcdf.OpenWriter(path, netcdf.KindCDF)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}

	timeAttrs, err := util.NewOrderedMap([]string{"units", "calendar", "standard_name"}, map[string]any{
		"units":         "days since 1850-01-01",
		"calendar":      calendar,
		"standard_name": "time",
	})
	if err != nil {
		t.Fatalf("time attrs: %v", err)
	}
	// 360_day 历法下每月固定 30 天，12 个月正好一年。
	days := []float64{0, 29, 30, 359}
	if err := w.AddVar("time", api.Variable{Values: days, Dimensions: []string{"time"}, Attributes: timeAttrs}); err != nil {
		t.Fatalf("add time var: %v", err)
	}

	// float32 坐标：末位 0.1 用于验证提升为 float64 后不产生伪精度数字。
	latAttrs, err := util.NewOrderedMap([]string{"units"}, map[string]any{"units": "degrees_north"})
	if err != nil {
		t.Fatalf("lat attrs: %v", err)
	}
	if err := w.AddVar("lat", api.Variable{Values: []float32{-90, 45, 0.1}, Dimensions: []string{"lat"}, Attributes: latAttrs}); err != nil {
		t.Fatalf("add lat var: %v", err)
	}

	// lev 维没有同名坐标变量，范围应当留空。
	if err := w.AddVar("q", api.Variable{Values: []float32{1, 2, 3, 4}, Dimensions: []string{"lev"}, Attributes: nil}); err != nil {
		t.Fatalf("add q var: %v", err)
	}

	tAttrs, err := util.NewOrderedMap([]string{"long_name"}, map[string]any{"long_name": "sample"})
	if err != nil {
		t.Fatalf("t attrs: %v", err)
	}
	if err := w.AddVar("t", api.Variable{Values: []float32{1, 2, 3, 4}, Dimensions: []string{"time"}, Attributes: tAttrs}); err != nil {
		t.Fatalf("add t var: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return path
}

func openTestGroup(t *testing.T, path string) api.Group {
	t.Helper()
	g, err := netcdf.Open(path)
	if err != nil {
		t.Fatalf("open netcdf: %v", err)
	}
	t.Cleanup(g.Close)
	return g
}

func dimensionByName(root *NetCDFDumpGroup, name string) *NetCDFDumpDim {
	for i := range root.Dimensions {
		if root.Dimensions[i].Name == name {
			return &root.Dimensions[i]
		}
	}
	return nil
}

func TestDimensionRangeInfoTimeStandard(t *testing.T) {
	g := openTestGroup(t, writeTestNetCDF(t, "standard"))

	rangeText, units, calendar := dimensionRangeInfo(g, "time")
	if units != "days since 1850-01-01" {
		t.Errorf("units=%q", units)
	}
	if calendar != "" {
		t.Errorf("standard 历法不应带日历标注, got %q", calendar)
	}
	if rangeText != "1850-01-01 → 1850-12-26" {
		t.Errorf("range=%q, want 1850-01-01 → 1850-12-26", rangeText)
	}
}

func TestDimensionRangeInfoNumericCoordinate(t *testing.T) {
	g := openTestGroup(t, writeTestNetCDF(t, "standard"))

	// 数值坐标给出原始首末值，且不带时间单位/历法。
	rangeText, units, calendar := dimensionRangeInfo(g, "lat")
	if rangeText != "-90 → 0.1" {
		t.Errorf("lat range=%q, want -90 → 0.1", rangeText)
	}
	if units != "" || calendar != "" {
		t.Errorf("数值坐标不应有时间元数据, units=%q calendar=%q", units, calendar)
	}

	// 没有同名坐标变量的维度不编造范围。
	if r, _, _ := dimensionRangeInfo(g, "lev"); r != "" {
		t.Errorf("lev 维不应有范围, got %q", r)
	}
	if r, _, _ := dimensionRangeInfo(g, "nope"); r != "" {
		t.Errorf("未知维度不应有范围, got %q", r)
	}
}

func TestDimensionRangeInfo360Day(t *testing.T) {
	g := openTestGroup(t, writeTestNetCDF(t, "360_day"))

	rangeText, _, calendar := dimensionRangeInfo(g, "time")
	if calendar != "360_day" {
		t.Errorf("calendar=%q", calendar)
	}
	// 360_day 下每月 30 天：第 30 天是 02-01，第 359 天是年内最后一天 12-30。
	if rangeText != "1850-01-01 → 1850-12-30" {
		t.Errorf("range=%q, want 1850-01-01 → 1850-12-30", rangeText)
	}
}

func TestFormatCoordinateRange(t *testing.T) {
	for _, tc := range []struct {
		first, last float64
		cdlType     string
		want        string
	}{
		{-90, 0.1, "float", "-90 → 0.1"},
		{90, -90, "float", "90 → -90"},
		{5, 5, "int", "5"},
		{0.5, 0.25, "double", "0.5 → 0.25"},
		{1e20, 1, "double", "1e+20 → 1"},
		{math.NaN(), 1, "double", "1"},
		{math.NaN(), math.Inf(-1), "double", ""},
	} {
		if got := formatCoordinateRange(tc.first, tc.last, tc.cdlType); got != tc.want {
			t.Errorf("formatCoordinateRange(%v, %v, %q)=%q, want %q", tc.first, tc.last, tc.cdlType, got, tc.want)
		}
	}
}

func TestWriteNetCDFGroupIncludesDimensionRanges(t *testing.T) {
	for _, tc := range []struct {
		calendar  string
		wantDim   string
		wantValue string
	}{
		{"standard", "1850-01-01 → 1850-12-26", "1850-01-31"},
		{"360_day", "1850-01-01 → 1850-12-30", "1850-02-01"},
	} {
		t.Run(tc.calendar, func(t *testing.T) {
			g := openTestGroup(t, writeTestNetCDF(t, tc.calendar))
			root := &NetCDFDumpGroup{Path: "/"}
			var sb strings.Builder
			trunc := false
			if err := writeNetCDFGroup(&sb, &trunc, "", g, 0, 24, root); err != nil {
				t.Fatalf("dump: %v", err)
			}

			timeDim := dimensionByName(root, "time")
			if timeDim == nil || timeDim.Range != tc.wantDim {
				t.Errorf("time 维=%+v, want range %q", timeDim, tc.wantDim)
			}
			latDim := dimensionByName(root, "lat")
			if latDim == nil || latDim.Range != "-90 → 0.1" {
				t.Errorf("lat 维=%+v, want range \"-90 → 0.1\"", latDim)
			}
			levDim := dimensionByName(root, "lev")
			if levDim == nil || levDim.Range != "" {
				t.Errorf("lev 维=%+v, want 空 range", levDim)
			}

			text := sb.String()
			if !strings.Contains(text, "time = 4 ; // "+tc.wantDim) {
				t.Errorf("ncdump 文本缺少时间范围:\n%s", text)
			}
			if !strings.Contains(text, "lat = 3 ; // -90 → 0.1") {
				t.Errorf("ncdump 文本缺少 lat 范围:\n%s", text)
			}
			if !strings.Contains(text, "lev = 4 ;\n") {
				t.Errorf("lev 维不应带范围注释:\n%s", text)
			}

			var timeVar *NetCDFDumpVar
			for i := range root.Variables {
				if root.Variables[i].Name == "time" {
					timeVar = &root.Variables[i]
				}
			}
			if timeVar == nil {
				t.Fatal("未找到 time 变量")
			}
			if len(timeVar.TimeValues) != 4 || timeVar.TimeValues[2] != tc.wantValue {
				t.Errorf("time_values=%v, 第 3 项 want %q", timeVar.TimeValues, tc.wantValue)
			}
			if timeVar.TimeUnits != "days since 1850-01-01" || timeVar.TimeCalendar != displayOf(tc.calendar) {
				t.Errorf("time_units=%q time_calendar=%q", timeVar.TimeUnits, timeVar.TimeCalendar)
			}

			// 普通变量即使共用 time 维也不应带上时间标签。
			for _, v := range root.Variables {
				if v.Name == "t" && len(v.TimeValues) != 0 {
					t.Errorf("t 变量不应有 time_values: %v", v.TimeValues)
				}
			}
		})
	}
}

func displayOf(calendar string) string {
	return resolveCFCalendar(calendar).display()
}
