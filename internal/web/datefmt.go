package web

import (
	"context"
	"strings"
	"sync/atomic"
	"time"
)

// Display formats for dates and times, chosen in Settings › Display.
// Stored as settings "date_format" (long | eu | iso | us) and "clock" (24 | 12).
type dateStyle struct {
	Date  string
	Clock string
}

var curStyle atomic.Value // dateStyle

func init() { curStyle.Store(dateStyle{Date: "long", Clock: "24"}) }

func style() dateStyle { return curStyle.Load().(dateStyle) }

var dateLayouts = map[string]string{
	"long": "Jan 2, 2006",
	"eu":   "02.01.2006",
	"iso":  "2006-01-02",
	"us":   "01/02/2006",
}

// DateFormats lists the choices for the settings page.
var DateFormats = []struct{ Key, Label string }{
	{"long", "Nov 16, 2026"},
	{"eu", "16.11.2026 (EU)"},
	{"iso", "2026-11-16 (ISO)"},
	{"us", "11/16/2026 (US)"},
}

func setStyle(date, clock string) {
	if _, ok := dateLayouts[date]; !ok {
		date = "long"
	}
	if clock != "12" {
		clock = "24"
	}
	curStyle.Store(dateStyle{Date: date, Clock: clock})
}

func loadStyle(ctx context.Context, get func(ctx context.Context, key, def string) (string, error)) {
	d, _ := get(ctx, "date_format", "long")
	c, _ := get(ctx, "clock", "24")
	setStyle(d, c)
}

func dayLayout() string { return dateLayouts[style().Date] }

func clockLayout() string {
	if style().Clock == "12" {
		return "3:04 PM"
	}
	return "15:04"
}

// fmtDay formats a time as a date in the chosen style.
func fmtDay(t time.Time) string { return t.Format(dayLayout()) }

// fmtDayTime formats date and time in the chosen style.
func fmtDayTime(t time.Time) string { return t.Format(dayLayout() + ", " + clockLayout()) }

// fmtClock formats only the time of day.
func fmtClock(t time.Time) string { return t.Format(clockLayout()) }

func parseAny(s string) (time.Time, bool, bool) {
	s = strings.TrimSpace(s)
	for _, l := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(l, s, time.UTC); err == nil {
			return t, true, true
		}
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, false, true
	}
	return time.Time{}, false, false
}

// day is the template func for date-only strings (YYYY-MM-DD or timestamps).
func day(s string) string {
	t, withTime, ok := parseAny(s)
	if !ok {
		return s
	}
	if withTime {
		t = t.Local()
	}
	return fmtDay(t)
}

// when is the template func for UTC timestamps: local date and time.
func when(s string) string {
	t, withTime, ok := parseAny(s)
	if !ok {
		return s
	}
	if !withTime {
		return fmtDay(t)
	}
	return fmtDayTime(t.Local())
}

// DateFmt and Clock expose the display style to the page (date pickers read them).
func (Layout) DateFmt() string { return style().Date }
func (Layout) Clock() string   { return style().Clock }
