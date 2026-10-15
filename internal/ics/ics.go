// Package ics builds the notebook's personal reminder calendar (RFC 5545).
//
// Dates are recomputed from the saved settings (start date, cycle, holidays).
// With the default start date 2026-10-19 the occurrence dates match the
// reviewed plan in ref/calendar_plan.json (SPEC §7). The rules themselves are
// written so Outlook and groupware calendars can import them: no multi-ordinal
// monthly BYDAY and no RDATE. No library is used.
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
	// DefaultHolidays mirrors the front end's default holiday list.
	DefaultHolidays = "2026-12-25 성탄절\n2027-01-01 신정\n2027-02-08 설날 연휴\n2027-02-09 설날 대체공휴일\n2027-03-01 삼일절\n2027-05-03 노동절 대체공휴일\n2027-05-05 어린이날\n2027-05-13 부처님오신날\n2027-07-19 제헌절 대체공휴일\n2027-08-16 광복절 대체공휴일\n2027-09-14 추석 연휴\n2027-09-15 추석\n2027-09-16 추석 연휴\n2027-10-04 개천절 대체공휴일\n2027-10-11 한글날 대체공휴일\n2027-12-27 성탄절 대체공휴일"
	// MovedOneOnOnePrefix starts the key of a single event that replaces a
	// first-Monday 1:1 notice falling on a holiday (moved to the next workday).
	MovedOneOnOnePrefix = "monthly-mon-blocker-1on1-moved-"
	// NotebookURL is where the notebook runs on the user's PC.
	NotebookURL = "http://localhost:8080/"
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
		hs, _ := h.(string)
		s.Holidays = ParseHolidays(hs)
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

func mondayOf(t time.Time) time.Time {
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, KST)
	off := (int(t.Weekday()) + 6) % 7 // Monday=0
	return t.AddDate(0, 0, -off)
}

// Event is one VEVENT before serialisation.
type Event struct {
	Key         string
	Summary     string
	Description string
	Start       time.Time // KST wall time
	Duration    time.Duration
	RRule       string // without "RRULE:" prefix; empty for single events
	ExDates     []time.Time
	Alarm       bool
}

type dates struct {
	w0, w1Mon, w1Tue, w1Wed, w1Fri  time.Time
	cycleMon, cycleFri, retroMon    time.Time
	week4Fri, cycle1EndFri, week13  time.Time
	sixMonth, twelveMonth, endFri   time.Time
	cdFirstMon, cdLastWed, cdLastFr time.Time
	cycle2Mon                       time.Time
	until                           time.Time // UTC
}

func (s Settings) dates() dates {
	w := func(week, day int) time.Time { return s.Week0.AddDate(0, 0, week*7+day) }
	var d dates
	d.w0 = s.Week0
	d.w1Mon, d.w1Tue, d.w1Wed, d.w1Fri = w(1, 0), w(1, 1), w(1, 2), w(1, 4)
	d.cycleMon, d.cycleFri = w(s.CycleStartWeek, 0), w(s.CycleStartWeek, 4)
	d.retroMon = w(5, 0)
	d.week4Fri = w(4, 4)
	lastCycleWeek := s.CycleStartWeek + s.CycleWeeks - 1
	d.cycle1EndFri = w(lastCycleWeek, 4)
	d.cdFirstMon = w(lastCycleWeek+1, 0)
	d.cdLastWed = w(lastCycleWeek+s.CooldownWeeks, 2)
	d.cdLastFr = w(lastCycleWeek+s.CooldownWeeks, 4)
	d.cycle2Mon = w(lastCycleWeek+s.CooldownWeeks+1, 0)
	d.week13 = w(13, 0)
	d.sixMonth = mondayOf(s.Week0.AddDate(0, 6, 0))
	d.twelveMonth = mondayOf(s.Week0.AddDate(0, 12, 0))
	d.endFri = d.twelveMonth.AddDate(0, 0, 4)
	d.until = time.Date(d.endFri.Year(), d.endFri.Month(), d.endFri.Day(), 23, 59, 59, 0, KST).UTC()
	return d
}

func at(day time.Time, hh, mm int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, KST)
}

func md(t time.Time) string { return t.Format("01-02") }

func mdRange(a, b time.Time) string {
	if a.Month() == b.Month() && a.Year() == b.Year() {
		return md(a) + "~" + b.Format("02")
	}
	return md(a) + "~" + md(b)
}

func (s Settings) isHoliday(t time.Time) bool { return s.Holidays[t.Format("2006-01-02")] }

func (s Settings) nextWorkday(t time.Time) time.Time {
	for i := 0; i < 30; i++ {
		t = t.AddDate(0, 0, 1)
		if wd := t.Weekday(); wd != time.Saturday && wd != time.Sunday && !s.isHoliday(t) {
			return t
		}
	}
	return t
}

// occurrences lists days from first (inclusive) to last (inclusive) whose
// weekday is in wds and which keep(day) accepts.
func occurrences(first, last time.Time, keep func(time.Time) bool) []time.Time {
	var out []time.Time
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		if keep(d) {
			out = append(out, d)
		}
	}
	return out
}

func firstMatch(from, last time.Time, keep func(time.Time) bool) time.Time {
	for d := from; !d.After(last); d = d.AddDate(0, 0, 1) {
		if keep(d) {
			return d
		}
	}
	return from
}

func isMon(t time.Time) bool      { return t.Weekday() == time.Monday }
func isFirstMon(t time.Time) bool { return isMon(t) && t.Day() <= 7 }
func isLaterMon(t time.Time) bool { return isMon(t) && t.Day() > 7 }
func weekday(w time.Weekday) func(time.Time) bool {
	return func(t time.Time) bool { return t.Weekday() == w }
}

// Events computes every event for s.
func (s Settings) Events() []Event {
	d := s.dates()
	untilStr := d.until.Format("20060102T150405Z")
	rep := strings.NewReplacer(
		"{W1_MON}", md(d.w1Mon),
		"{DEMO_FROM}", md(d.cycleFri),
		"{CD_RANGE}", mdRange(d.cdFirstMon, d.cdLastFr),
		"{CD_FRI}", md(d.cdLastFr),
		"{CD_WED}", md(d.cdLastWed),
		"{C2_MON}", md(d.cycle2Mon),
		"{CW}", strconv.Itoa(s.CycleWeeks),
		"{CD}", strconv.Itoa(s.CooldownWeeks),
		"{NB}", NotebookURL,
	)
	desc := func(key string) string { return rep.Replace(descriptions[key]) }

	// exclude returns the holiday occurrences of a series (plus extra days).
	exclude := func(occ []time.Time, extra ...time.Time) []time.Time {
		var out []time.Time
		for _, o := range occ {
			skip := s.isHoliday(o)
			for _, e := range extra {
				if o.Equal(e) {
					skip = true
				}
			}
			if skip {
				out = append(out, o)
			}
		}
		return out
	}
	withTime := func(days []time.Time, hh, mm int) []time.Time {
		out := make([]time.Time, len(days))
		for i, x := range days {
			out[i] = at(x, hh, mm)
		}
		return out
	}

	var evs []Event

	// Monday 09:20 blocker check on 2nd–5th Mondays, from the first cycle week.
	// Written as a plain weekly rule with every first Monday in EXDATE (the
	// first-Monday event below covers those), because Outlook and many
	// groupware calendars cannot import a monthly rule with several ordinals
	// (BYDAY=2MO,3MO,4MO,5MO). The occurrence dates are the same.
	monStart := firstMatch(d.cycleMon, d.endFri, isLaterMon)
	allMon := occurrences(monStart, d.endFri, isMon)
	var monEx []time.Time
	for _, o := range allMon {
		if isFirstMon(o) || s.isHoliday(o) || o.Equal(d.retroMon) {
			monEx = append(monEx, o)
		}
	}
	evs = append(evs, Event{
		Key: "weekly-mon-blocker", Summary: "[관리] 막힘 확인: 없으면 9:30까지 취소",
		Start: at(monStart, 9, 20), Duration: 10 * time.Minute,
		RRule:   "FREQ=WEEKLY;BYDAY=MO;UNTIL=" + untilStr,
		ExDates: withTime(monEx, 9, 20),
		Alarm:   true, Description: desc("weekly-mon-blocker"),
	})

	// First Monday 09:20: blocker check + monthly 1:1 notice.
	// When the first Monday is a holiday it is excluded and a separate single
	// event is made on the next workday (Outlook ignores RDATE).
	firstStart := firstMatch(d.cycleMon, d.endFri, isFirstMon)
	firstOcc := occurrences(firstStart, d.endFri, isFirstMon)
	firstEx := exclude(firstOcc)
	evs = append(evs, Event{
		Key: "monthly-mon-blocker-1on1", Summary: "[관리] 막힘 확인 + 이달 1:1 안내",
		Start: at(firstStart, 9, 20), Duration: 10 * time.Minute,
		RRule:   "FREQ=MONTHLY;BYDAY=1MO;UNTIL=" + untilStr,
		ExDates: withTime(firstEx, 9, 20),
		Alarm:   true, Description: desc("monthly-mon-blocker-1on1"),
	})
	for _, x := range firstEx {
		moved := s.nextWorkday(x)
		if moved.After(d.endFri) {
			continue
		}
		evs = append(evs, Event{
			Key:     MovedOneOnOnePrefix + moved.Format("20060102"),
			Summary: "[관리] 막힘 확인 + 이달 1:1 안내",
			Start:   at(moved, 9, 20), Duration: 10 * time.Minute,
			Alarm: true, Description: desc("monthly-mon-blocker-1on1"),
		})
	}

	// Tuesday 10:00–12:00 quiet time, from week 1.
	tueOcc := occurrences(d.w1Tue, d.endFri, weekday(time.Tuesday))
	evs = append(evs, Event{
		Key: "weekly-tue-quiet", Summary: "[관리] 조용한 시간 10~12: DM 보내지 않기",
		Start: at(d.w1Tue, 10, 0), Duration: 2 * time.Hour,
		RRule:   "FREQ=WEEKLY;BYDAY=TU;UNTIL=" + untilStr,
		ExDates: withTime(exclude(tueOcc), 10, 0),
		Alarm:   true, Description: desc("weekly-tue-quiet"),
	})

	// Wednesday/Thursday quiet time: shown only, no alarm, no exclusions.
	evs = append(evs, Event{
		Key: "weekly-wedthu-quiet-silent", Summary: "[관리] 조용한 시간(알림 없음)",
		Start: at(d.w1Wed, 10, 0), Duration: 2 * time.Hour,
		RRule: "FREQ=WEEKLY;BYDAY=WE,TH;UNTIL=" + untilStr,
		Alarm: false, Description: desc("weekly-wedthu-quiet-silent"),
	})

	// Friday 13:55 open the notebook, from week 1.
	friOcc := occurrences(d.w1Fri, d.endFri, weekday(time.Friday))
	evs = append(evs, Event{
		Key: "weekly-fri-notebook", Summary: "[관리] 금요일 수첩 열기",
		Start: at(d.w1Fri, 13, 55), Duration: 5 * time.Minute,
		RRule:   "FREQ=WEEKLY;BYDAY=FR;UNTIL=" + untilStr,
		ExDates: withTime(exclude(friOcc), 13, 55),
		Alarm:   true, Description: desc("weekly-fri-notebook"),
	})

	marker := func(key, summary string, start time.Time, dur time.Duration) {
		evs = append(evs, Event{Key: key, Summary: summary, Start: start, Duration: dur, Alarm: true, Description: desc(key)})
	}
	marker("marker-week0-checklist", "[관리] 표지: 0주 준비(상사 3문장은 주간보고 자리에서)", at(d.w0, 9, 20), 20*time.Minute)
	marker("marker-week4-fidelity", "[관리] 표지: 4주 실행 점검(결과는 판단 안 함)", at(d.week4Fri, 13, 55), 15*time.Minute)
	marker("marker-week4-retro", "[관리] 오늘 10시는 4주 회고(취소 안 함)", at(d.retroMon, 9, 20), 10*time.Minute)
	marker("marker-cycle1-cooldown", "[관리] 표지: 다음 주는 정리 주(사이클 1 끝)", at(d.cycle1EndFri, 13, 55), 15*time.Minute)
	marker("marker-week13-dial", "[관리] 표지: 13주, 사람별 조정 시작 가능", at(d.week13, 9, 20), 15*time.Minute)
	marker("marker-6month-result", "[관리] 표지: 6개월 결과 1차 보기(팀 단위)", at(d.sixMonth, 9, 20), 20*time.Minute)
	marker("marker-12month-result", "[관리] 표지: 12개월 결과 판단 + 알림 계속할지", at(d.twelveMonth, 9, 20), 20*time.Minute)

	for i := range evs {
		sort.Slice(evs[i].ExDates, func(a, b int) bool { return evs[i].ExDates[a].Before(evs[i].ExDates[b]) })
	}
	return evs
}

// Build renders the whole VCALENDAR. now is used only for DTSTAMP.
func Build(s Settings, now time.Time) []byte {
	var w writer
	stamp := now.UTC().Format("20060102T150405Z")
	start := s.Week0.Format("2006-01-02")
	w.line("BEGIN:VCALENDAR")
	w.line("VERSION:2.0")
	w.line("PRODID:-//manager-notebook//manager-notebook 1.0//KO")
	w.line("CALSCALE:GREGORIAN")
	w.line("METHOD:PUBLISH")
	w.line("X-WR-CALNAME:" + Escape("관리 수첩"))
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
		w.line("UID:" + e.Key + "-" + start + "@manager-notebook")
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
		if e.Alarm {
			w.line("BEGIN:VALARM")
			w.line("ACTION:DISPLAY")
			w.line("DESCRIPTION:" + Escape(e.Summary))
			w.line("TRIGGER:PT0M")
			w.line("END:VALARM")
		}
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

// Filename is the download name.
const Filename = "manager-notebook.ics"

// String helps debugging.
func (e Event) String() string {
	return fmt.Sprintf("%s %s %s", e.Key, local(e.Start), e.RRule)
}
