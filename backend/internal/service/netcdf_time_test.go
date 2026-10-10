package service

import (
	"math"
	"reflect"
	"testing"
)

func mustCodec(t *testing.T, units, calendar string) *cfTimeCodec {
	t.Helper()
	codec, ok := newCFTimeCodec(units, calendar)
	if !ok {
		t.Fatalf("expected units %q (calendar %q) to parse", units, calendar)
	}
	return codec
}

func TestCFTimeCodecStandardCalendar(t *testing.T) {
	cases := []struct {
		units    string
		calendar string
		value    float64
		want     string
	}{
		{"days since 1970-01-01", "", 0, "1970-01-01"},
		{"days since 1970-01-01", "", 20, "1970-01-21"},
		{"days since 1970-01-01", "", -1, "1969-12-31"},
		{"days since 1970-01-01", "", 19.5, "1970-01-20T12:00:00"},
		{"hours since 1970-01-01", "standard", 25, "1970-01-02T01:00:00"},
		{"minutes since 1970-01-01 06:00", "", 90, "1970-01-01T07:30:00"},
		{"seconds since 2000-01-01 00:00:00", "", 3600, "2000-01-01T01:00:00"},
		{"seconds since 2000-01-01 00:00:00", "", 1.5, "2000-01-01T00:00:01.500"},
		{"milliseconds since 1970-01-01", "", 1500, "1970-01-01T00:00:01.500"},
		// 格里历闰年规则：2000 闰、1900 不闰
		{"days since 2000-02-28", "", 1, "2000-02-29"},
		{"days since 1900-02-28", "", 1, "1900-03-01"},
		// 基准可省略「日」，也可带时区标记（按墙上时间解析）
		{"days since 2010-6", "", 1, "2010-06-02"},
		{"seconds since 1970-01-01 00:00:00 UTC", "", 86400, "1970-01-02"},
		{"seconds since 1970-01-01T00:00:00Z", "", 86400, "1970-01-02"},
		// 儒略历下 1900 是闰年，与格里历不同
		{"days since 1900-02-28", "julian", 1, "1900-02-29"},
	}
	for _, tc := range cases {
		codec := mustCodec(t, tc.units, tc.calendar)
		if got := codec.label(tc.value); got != tc.want {
			t.Errorf("units=%q calendar=%q value=%v: got %q, want %q", tc.units, tc.calendar, tc.value, got, tc.want)
		}
	}
}

func TestCFTimeCodecArtificialCalendars(t *testing.T) {
	cases := []struct {
		units    string
		calendar string
		value    float64
		want     string
	}{
		// noleap / 365_day：不存在 2 月 29
		{"days since 2000-02-27", "noleap", 1, "2000-02-28"},
		{"days since 2000-02-27", "noleap", 2, "2000-03-01"},
		{"days since 2000-02-27", "365_day", 365, "2001-02-27"},
		// all_leap / 366_day：每年都有 2 月 29
		{"days since 2000-02-27", "all_leap", 2, "2000-02-29"},
		{"days since 2000-02-27", "366_day", 3, "2000-03-01"},
		// 360_day：每月 30 天、每年 360 天
		{"days since 2000-01-01", "360_day", 29, "2000-01-30"},
		{"days since 2000-01-01", "360_day", 30, "2000-02-01"},
		{"days since 2000-01-01", "360_day", 359, "2000-12-30"},
		{"days since 2000-01-01", "360_day", 360, "2001-01-01"},
		// 人工历法下 2 月同样没有 29 日之外的越界写法
		{"hours since 2000-01-01", "360_day", 24*30 + 1, "2000-02-01T01:00:00"},
	}
	for _, tc := range cases {
		codec := mustCodec(t, tc.units, tc.calendar)
		if got := codec.label(tc.value); got != tc.want {
			t.Errorf("units=%q calendar=%q value=%v: got %q, want %q", tc.units, tc.calendar, tc.value, got, tc.want)
		}
	}
}

func TestCFTimeCodecRejects(t *testing.T) {
	if _, ok := newCFTimeCodec("1", ""); ok {
		t.Errorf("expected unitless units to be rejected")
	}
	if _, ok := newCFTimeCodec("kelvin since 1970-01-01", ""); ok {
		t.Errorf("expected unknown unit to be rejected")
	}
	if _, ok := newCFTimeCodec("days since", ""); ok {
		t.Errorf("expected missing base time to be rejected")
	}
	if _, ok := newCFTimeCodec("days since 1970-13-01", ""); ok {
		t.Errorf("expected invalid base month to be rejected")
	}
	if _, ok := newCFTimeCodec("days since 1970-01-01", "gnu"); ok {
		t.Errorf("expected unknown calendar to be rejected instead of guessing")
	}
}

func TestCFTimeCodecSkipsFillValues(t *testing.T) {
	codec := mustCodec(t, "days since 1970-01-01", "")
	if got := codec.label(math.NaN()); got != "" {
		t.Errorf("NaN should yield no label, got %q", got)
	}
	// _FillValue 常见的 -1e34 会算出荒谬年份，必须被跨度上限挡住
	if got := codec.label(-1e34); got != "" {
		t.Errorf("fill value should yield no label, got %q", got)
	}
	if got := codec.label(1e34); got != "" {
		t.Errorf("huge value should yield no label, got %q", got)
	}
}

func TestCFTimeCodecLabelRange(t *testing.T) {
	codec := mustCodec(t, "days since 1970-01-01", "")
	if got := codec.labelRange(0, 10); got != "1970-01-01 → 1970-01-11" {
		t.Errorf("unexpected range text %q", got)
	}
	if got := codec.labelRange(0, 0); got != "1970-01-01" {
		t.Errorf("single-step range should collapse, got %q", got)
	}
	if got := codec.labelRange(math.NaN(), math.NaN()); got != "" {
		t.Errorf("unparsable range should be empty, got %q", got)
	}
}

func TestCFCalendarDisplay(t *testing.T) {
	if got := resolveCFCalendar("standard").display(); got != "" {
		t.Errorf("standard calendar should not be labelled, got %q", got)
	}
	if got := resolveCFCalendar("360_day").display(); got != "360_day" {
		t.Errorf("expected 360_day label, got %q", got)
	}
	if resolveCFCalendar("unknown").supported() {
		t.Errorf("unknown calendar must be unsupported")
	}
}

func TestNetCDFNumberValue(t *testing.T) {
	if value, ok := netCDFNumberValue([]float64{1.25, 9}); !ok || value != 1.25 {
		t.Errorf("float slice: got %v/%v", value, ok)
	}
	if value, ok := netCDFNumberValue([]int32{-7}); !ok || value != -7 {
		t.Errorf("int slice: got %v/%v", value, ok)
	}
	if _, ok := netCDFNumberValue([]string{"2010-01-01"}); ok {
		t.Errorf("string values are not CF numeric times")
	}
	if _, ok := netCDFNumberValue(reflect.Value{}); ok {
		t.Errorf("invalid reflect value must be rejected")
	}
}
