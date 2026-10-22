// Package ics builds the notebook's personal reminder calendar (RFC 5545).
//
// Version 2.0 schedule (SPEC2 §6), recomputed from the saved settings (start
// date, cycle, holidays):
//   - Friday reading, every Friday 13:55, one weekly rule; holiday Fridays
//     are in EXDATE.
//   - Monthly risk check, the first Monday of each month 09:20 (next workday
//     when that Monday is a holiday), one single event per month.
//   - Cycle start and cool-down week start, Mondays 09:20 (next workday when
//     a holiday), single events.
//   - 3-month check and 6-month review, Monday 09:20 of the week that holds
//     week 0 + 3 / 6 months (next workday when a holiday), single events.
//
// The span runs from the Monday of week 0 to the Friday of the week that
// holds week 0 + 12 months. Only simple rules are used (one weekly RRULE, no
// RDATE, no multi-ordinal BYDAY) so Outlook and groupware calendars import
// them. No library is used.
package ics

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// KST is Asia/Seoul (+09:00, no DST). A fixed zone keeps output independent of TZ.
var KST = time.FixedZone("KST", 9*60*60)

const (
	// DefaultStartDate is used when settings are missing or invalid.
	DefaultStartDate = "2026-10-19"
	// DefaultHolidays mirrors the front end's default holiday list
	// (HOLIDAYS_DEFAULT).
	DefaultHolidays = "2026-12-25 성탄절\n2027-01-01 신정\n2027-02-08 설날 연휴\n2027-02-09 설날 대체공휴일\n2027-03-01 삼일절\n2027-05-03 노동절 대체공휴일\n2027-05-05 어린이날\n2027-05-13 부처님오신날\n2027-07-19 제헌절 대체공휴일\n2027-08-16 광복절 대체공휴일\n2027-09-14 추석 연휴\n2027-09-15 추석\n2027-09-16 추석 연휴\n2027-10-04 개천절 대체공휴일\n2027-10-11 한글날 대체공휴일\n2027-12-27 성탄절 대체공휴일"
	// NotebookURL is where the notebook runs on the user's PC.
	NotebookURL = "http://localhost:8080/"
	// Filename is the download name.
	Filename = "manager-notebook.ics"
	// uidSuffix ends every UID; "v2" keeps 2.0 events apart from 1.0 ones
	// that may already be in a calendar.
	uidSuffix = "-v2@manager-notebook"
)

// Event keys (the first part of each UID).
const (
	KeyFriday   = "friday-read"
	KeyRisk     = "risk-monthly"
	KeyCycle    = "cycle-start"
	KeyCooldown = "cooldown-start"
	KeyMonth3   = "check-3month"
	KeyMonth6   = "review-6month"
)

// Settings are the inputs that shape the calendar.
type Settings struct {
	Week0          time.Time // Monday of week 0, 00:00 KST
	CycleStartWeek int
	CycleWeeks     int
	CooldownWeeks  int
	Holidays       map[string]bool // "YYYY-MM-DD"
}

var dateRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

// ParseHolidays finds every valid YYYY-MM-DD in s (same rule as the front end).
func ParseHolidays(s string) map[string]bool {
	out := map[string]bool{}
	for _, m := range dateRe.FindAllString(s, -1) {
		if _, err := time.ParseInLocation("2006-01-02", m, KST); err == nil {
			out[m] = true
		}
	}
	return out
}

// FromDoc reads the settings object saved by the front end. nil or missing
// fields fall back to the defaults (holidays fall back only when absent/null).
func FromDoc(doc map[string]any) Settings {
	s := Settings{CycleStartWeek: 2, CycleWeeks: 6, CooldownWeeks: 1}
	start, _ := doc["startDate"].(string)
	d, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(start), KST)
	if err != nil {
		d, _ = time.ParseInLocation("2006-01-02", DefaultStartDate, KST)
	}
	s.Week0 = mondayOf(d)
	s.CycleStartWeek = intField(doc, "cycleStartWeek", 2, 1, 12)
	s.CycleWeeks = intField(doc, "cycleWeeks", 6, 1, 26)
	s.CooldownWeeks = intField(doc, "cooldownWeeks", 1, 1, 4)
	if h, ok := doc["holidays"]; ok && h != nil {
		s.Holidays = ParseHolidays(holidayText(h))
	} else {
		s.Holidays = ParseHolidays(DefaultHolidays)
	}
	return s
}

func intField(doc map[string]any, key string, def, lo, hi int) int {
	var n int
	switch v := doc[key].(type) {
	case float64:
		n = int(v)
	case int:
		n = v
	case string:
		n, _ = strconv.Atoi(strings.TrimSpace(v))
	}
	if n == 0 { // same as the front end's `Number(x) || def`
		return def
	}
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// holidayText accepts the holiday setting as one text (1.0 and 2.0 front
// end) or as a list of lines.
func holidayText(v any) string {
	switch h := v.(type) {
	case string:
		return h
	case []any:
		parts := make([]string, 0, len(h))
		for _, x := range h {
			if s, ok := x.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func mondayOf(t time.Time) time.Time {
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, KST)
	off := (int(t.Weekday()) + 6) % 7 // Monday=0
	return t.AddDate(0, 0, -off)
}

// Event is one VEVENT before serialisation.
type Event struct {
	Key         string // UID key
	UIDDate     string // date part of the UID
	Summary     string
	Description string
	Start       time.Time // KST wall time
	Duration    time.Duration
	RRule       string // without "RRULE:" prefix; empty for single events
	ExDates     []time.Time
}

// UID is the event's stable id.
func (e Event) UID() string { return e.Key + "-" + e.UIDDate + uidSuffix }

func at(day time.Time, hh, mm int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, KST)
}

func ymd(t time.Time) string { return t.Format("2006-01-02") }

func (s Settings) isWorkday(t time.Time) bool {
	wd := t.Weekday()
	return wd != time.Saturday && wd != time.Sunday && !s.Holidays[ymd(t)]
}

// workdayFrom returns t when it is a workday, otherwise the next workday.
func (s Settings) workdayFrom(t time.Time) time.Time {
	for i := 0; i < 60 && !s.isWorkday(t); i++ {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// week returns the given weekday (0 = Monday) of week n.
func (s Settings) week(n, day int) time.Time { return s.Week0.AddDate(0, 0, 7*n+day) }

// EndFriday is the last day of the calendar: the Friday of the week that
// holds week 0 + 12 months.
func (s Settings) EndFriday() time.Time {
	return mondayOf(s.Week0.AddDate(0, 12, 0)).AddDate(0, 0, 4)
}

// CycleStart is the first Monday of cycle k (k >= 1), before the holiday move
// (same as the front end's cycleStart).
func (s Settings) CycleStart(k int) time.Time {
	return s.week(s.CycleStartWeek+(k-1)*(s.CycleWeeks+s.CooldownWeeks), 0)
}

// Events computes every event for s, in a fixed order.
func (s Settings) Events() []Event {
	end := s.EndFriday()
	inSpan := func(t time.Time) bool { return !t.Before(s.Week0) && !t.After(end) }
	const dur = 30 * time.Minute
	var evs []Event
	single := func(key, summary, desc string, day time.Time) {
		evs = append(evs, Event{
			Key: key, UIDDate: ymd(day), Summary: summary, Description: desc,
			Start: at(day, 9, 20), Duration: dur,
		})
	}

	// 1. Friday reading: weekly from the week-0 Friday to the end Friday.
	firstFri := s.week(0, 4)
	until := at(end, 13, 55).UTC().Format("20060102T150405Z")
	var friEx []time.Time
	for d := firstFri; !d.After(end); d = d.AddDate(0, 0, 7) {
		if s.Holidays[ymd(d)] {
			friEx = append(friEx, at(d, 13, 55))
		}
	}
	evs = append(evs, Event{
		Key: KeyFriday, UIDDate: ymd(s.Week0),
		Summary: "[관리] 금요 읽기 — 진행 지도·주간 보고", Description: desc(descFriday),
		Start: at(firstFri, 13, 55), Duration: dur,
		RRule:   "FREQ=WEEKLY;BYDAY=FR;UNTIL=" + until,
		ExDates: friEx,
	})

	// 2. Monthly risk check: first Monday of each month, moved to the next
	// workday when it is a holiday.
	for m := time.Date(s.Week0.Year(), s.Week0.Month(), 1, 0, 0, 0, 0, KST); !m.After(end); m = m.AddDate(0, 1, 0) {
		first := m
		for first.Weekday() != time.Monday {
			first = first.AddDate(0, 0, 1)
		}
		day := s.workdayFrom(first)
		if !first.Before(s.Week0) && inSpan(day) {
			single(KeyRisk, "[관리] 월간 리스크 15분", desc(descRisk), day)
		}
	}

	// 3. and 4. Cycle start and cool-down week start.
	for k := 1; ; k++ {
		start := s.CycleStart(k)
		if start.After(end) {
			break
		}
		if day := s.workdayFrom(start); inSpan(day) {
			single(KeyCycle, fmt.Sprintf("[관리] 사이클 %d 시작 — 자리 계산·지침 네 칸", k), desc(descCycle), day)
		}
		if day := s.workdayFrom(start.AddDate(0, 0, 7*s.CycleWeeks)); inSpan(day) {
			single(KeyCooldown, fmt.Sprintf("[관리] 정리 주 시작 — 사이클 %d 끝", k), desc(descCooldown), day)
		}
	}

	// 5. 3-month check and 6-month review.
	if day := s.workdayFrom(mondayOf(s.Week0.AddDate(0, 3, 0))); inSpan(day) {
		single(KeyMonth3, "[관리] 3개월 점검", desc(descMonth3), day)
	}
	if day := s.workdayFrom(mondayOf(s.Week0.AddDate(0, 6, 0))); inSpan(day) {
		single(KeyMonth6, "[관리] 6개월 재검토", desc(descMonth6), day)
	}

	// Keep the Friday rule first, then the single events by date.
	sort.SliceStable(evs[1:], func(a, b int) bool { return evs[1+a].Start.Before(evs[1+b].Start) })
	return evs
}

func desc(text string) string { return strings.ReplaceAll(text, "{NB}", NotebookURL) }

// Build renders the whole VCALENDAR. now is used only for DTSTAMP.
func Build(s Settings, now time.Time) []byte {
	var w writer
	stamp := now.UTC().Format("20060102T150405Z")
	w.line("BEGIN:VCALENDAR")
	w.line("VERSION:2.0")
	w.line("PRODID:-//manager-notebook//manager-notebook 2.0//KO")
	w.line("CALSCALE:GREGORIAN")
	w.line("METHOD:PUBLISH")
	w.line("X-WR-CALNAME:" + Escape("관리 수첩 2"))
	w.line("X-WR-TIMEZONE:Asia/Seoul")
	w.line("BEGIN:VTIMEZONE")
	w.line("TZID:Asia/Seoul")
	w.line("BEGIN:STANDARD")
	w.line("DTSTART:19700101T000000")
	w.line("TZOFFSETFROM:+0900")
	w.line("TZOFFSETTO:+0900")
	w.line("TZNAME:KST")
	w.line("END:STANDARD")
	w.line("END:VTIMEZONE")
	for _, e := range s.Events() {
		w.line("BEGIN:VEVENT")
		w.line("UID:" + e.UID())
		w.line("DTSTAMP:" + stamp)
		w.line("DTSTART;TZID=Asia/Seoul:" + local(e.Start))
		w.line("DTEND;TZID=Asia/Seoul:" + local(e.Start.Add(e.Duration)))
		if e.RRule != "" {
			w.line("RRULE:" + e.RRule)
		}
		if len(e.ExDates) > 0 {
			w.line("EXDATE;TZID=Asia/Seoul:" + joinLocal(e.ExDates))
		}
		w.line("SUMMARY:" + Escape(e.Summary))
		w.line("DESCRIPTION:" + Escape(e.Description))
		w.line("CLASS:PRIVATE")
		w.line("TRANSP:TRANSPARENT")
		w.line("CATEGORIES:" + Escape("관리"))
		w.line("BEGIN:VALARM")
		w.line("ACTION:DISPLAY")
		w.line("DESCRIPTION:" + Escape(e.Summary))
		w.line("TRIGGER:PT0M")
		w.line("END:VALARM")
		w.line("END:VEVENT")
	}
	w.line("END:VCALENDAR")
	return []byte(w.b.String())
}

func local(t time.Time) string { return t.In(KST).Format("20060102T150405") }

func joinLocal(ts []time.Time) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = local(t)
	}
	return strings.Join(parts, ",")
}

// Escape escapes a TEXT value (RFC 5545 §3.3.11).
func Escape(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case ';':
			b.WriteString(`\;`)
		case ',':
			b.WriteString(`\,`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

type writer struct{ b strings.Builder }

// line writes one content line folded at 75 octets (never inside a UTF-8
// sequence) and ends it with CRLF.
func (w *writer) line(s string) { w.b.WriteString(Fold(s)) }

// Fold folds a content line to at most 75 octets per physical line
// (continuation lines start with one space, which counts) and adds CRLF.
func Fold(s string) string {
	var b strings.Builder
	limit := 75
	n := 0
	for _, r := range s {
		size := len(string(r))
		if n+size > limit {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += size
	}
	b.WriteString("\r\n")
	return b.String()
}

// String helps debugging.
func (e Event) String() string {
	return fmt.Sprintf("%s %s %s", e.UID(), local(e.Start), e.RRule)
}
