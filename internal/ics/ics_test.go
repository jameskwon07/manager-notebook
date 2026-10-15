package ics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

type expectEvent struct {
	Key         string   `json:"key"`
	Summary     string   `json:"summary"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Recurrence  []string `json:"recurrence"`
	Alarm       bool     `json:"alarm"`
	Description string   `json:"description"`
}

func loadExpect(t *testing.T) []expectEvent {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "plan_expect.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Events []expectEvent `json:"events"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f.Events
}

func iso(t time.Time) string { return t.In(KST).Format("2006-01-02T15:04:05") }

// expand lists the occurrence start times (KST, "2006-01-02T15:04") of one
// event from DTSTART and its recurrence lines. It understands the rule forms
// this package and the plan use: FREQ=WEEKLY;BYDAY=list and
// FREQ=MONTHLY;BYDAY=nXX list, each with UNTIL, plus EXDATE and RDATE.
func expand(t *testing.T, start time.Time, rec []string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	key := func(x time.Time) string { return x.In(KST).Format("2006-01-02T15:04") }
	var rrule string
	var ex, rd []string
	for _, l := range rec {
		switch {
		case strings.HasPrefix(l, "RRULE:"):
			rrule = strings.TrimPrefix(l, "RRULE:")
		case strings.HasPrefix(l, "EXDATE;TZID=Asia/Seoul:"):
			ex = strings.Split(strings.TrimPrefix(l, "EXDATE;TZID=Asia/Seoul:"), ",")
		case strings.HasPrefix(l, "RDATE;TZID=Asia/Seoul:"):
			rd = strings.Split(strings.TrimPrefix(l, "RDATE;TZID=Asia/Seoul:"), ",")
		default:
			t.Fatalf("unknown recurrence line %q", l)
		}
	}
	if rrule == "" {
		out[key(start)] = true
	} else {
		parts := map[string]string{}
		for _, kv := range strings.Split(rrule, ";") {
			k, v, _ := strings.Cut(kv, "=")
			parts[k] = v
		}
		until, err := time.Parse("20060102T150405Z", parts["UNTIL"])
		if err != nil {
			t.Fatalf("until %q", parts["UNTIL"])
		}
		days := map[string]time.Weekday{"SU": 0, "MO": 1, "TU": 2, "WE": 3, "TH": 4, "FR": 5, "SA": 6}
		type byday struct {
			n  int
			wd time.Weekday
		}
		var bd []byday
		for _, b := range strings.Split(parts["BYDAY"], ",") {
			n := 0
			if len(b) > 2 {
				n, _ = strconv.Atoi(b[:len(b)-2])
			}
			bd = append(bd, byday{n, days[b[len(b)-2:]]})
		}
		for d := start; !d.After(until); d = d.AddDate(0, 0, 1) {
			for _, b := range bd {
				if d.Weekday() != b.wd {
					continue
				}
				switch parts["FREQ"] {
				case "WEEKLY":
					out[key(d)] = true
				case "MONTHLY":
					if (d.Day()-1)/7+1 == b.n {
						out[key(d)] = true
					}
				default:
					t.Fatalf("freq %q", parts["FREQ"])
				}
			}
		}
	}
	for _, x := range ex {
		tm, err := time.ParseInLocation("20060102T150405", x, KST)
		if err != nil {
			t.Fatal(err)
		}
		delete(out, key(tm))
	}
	for _, x := range rd {
		tm, err := time.ParseInLocation("20060102T150405", x, KST)
		if err != nil {
			t.Fatal(err)
		}
		out[key(tm)] = true
	}
	return out
}

func recurrence(e Event) []string {
	var out []string
	if e.RRule != "" {
		out = append(out, "RRULE:"+e.RRule)
	}
	if len(e.ExDates) > 0 {
		out = append(out, "EXDATE;TZID=Asia/Seoul:"+joinLocal(e.ExDates))
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestDefaultMatchesPlan: with the default settings every planned event
// fires on exactly the same dates and times as the reviewed plan, with the
// same summary, length, alarm and text. Single events whose key starts with
// MovedOneOnOnePrefix count as occurrences of the first-Monday event (they
// replace the plan's RDATE).
func TestDefaultMatchesPlan(t *testing.T) {
	want := loadExpect(t)
	got := FromDoc(nil).Events()
	byKey := map[string]Event{}
	moved := map[string]bool{}
	for _, g := range got {
		if strings.HasPrefix(g.Key, MovedOneOnOnePrefix) {
			moved[g.Start.In(KST).Format("2006-01-02T15:04")] = true
			if g.RRule != "" || len(g.ExDates) > 0 || !g.Alarm || g.Summary != "[관리] 막힘 확인 + 이달 1:1 안내" {
				t.Errorf("moved event %s is not a plain single reminder", g.Key)
			}
			continue
		}
		byKey[g.Key] = g
	}
	if len(byKey) != len(want) {
		t.Fatalf("got %d planned events, want %d", len(byKey), len(want))
	}
	if len(moved) != 3 {
		t.Fatalf("moved 1:1 events %v, want 3", sortedKeys(moved))
	}
	for _, w := range want {
		g, ok := byKey[w.Key]
		if !ok {
			t.Fatalf("event %s missing", w.Key)
		}
		if g.Summary != w.Summary {
			t.Errorf("%s summary %q want %q", w.Key, g.Summary, w.Summary)
		}
		ws, _ := time.ParseInLocation("2006-01-02T15:04:05", w.Start, KST)
		we, _ := time.ParseInLocation("2006-01-02T15:04:05", w.End, KST)
		if g.Duration != we.Sub(ws) {
			t.Errorf("%s duration %s want %s", w.Key, g.Duration, we.Sub(ws))
		}
		gotOcc := expand(t, g.Start, recurrence(g))
		if w.Key == "monthly-mon-blocker-1on1" {
			for k := range moved {
				gotOcc[k] = true
			}
		}
		wantOcc := expand(t, ws, w.Recurrence)
		if strings.Join(sortedKeys(gotOcc), ",") != strings.Join(sortedKeys(wantOcc), ",") {
			t.Errorf("%s occurrences differ\n got %v\nwant %v", w.Key, sortedKeys(gotOcc), sortedKeys(wantOcc))
		}
		if g.Alarm != w.Alarm {
			t.Errorf("%s alarm %v want %v", w.Key, g.Alarm, w.Alarm)
		}
		if g.Description != w.Description {
			t.Errorf("%s description\n got %q\nwant %q", w.Key, g.Description, w.Description)
		}
	}
}

// TestOutlookFriendlyRules: no rule needs features Outlook drops on import.
func TestOutlookFriendlyRules(t *testing.T) {
	out := string(Build(FromDoc(nil), time.Now()))
	if strings.Contains(out, "RDATE") {
		t.Error("RDATE present")
	}
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r\n ", ""), "\r\n") {
		if strings.HasPrefix(l, "RRULE:FREQ=MONTHLY") && strings.Contains(l, ",") {
			t.Errorf("monthly rule with several BYDAY values: %s", l)
		}
	}
}

var (
	reBr   = regexp.MustCompile(`<br>`)
	reB    = regexp.MustCompile(`</?b>`)
	reA    = regexp.MustCompile(`<a href="([^"]+)">([^<]*)</a>`)
	reNote = regexp.MustCompile(`^https://[^#]+(#\w+)?$`) // a link whose text is the URL itself = notebook link
)

// localEdits are the deliberate wording changes from the cloud plan: in the
// in-house notebook the kickoff and settings texts are documents in the
// guide tab, so the week-0 marker links straight to them.
var localEdits = strings.NewReplacer(
	"모두 실행 가이드(http://localhost:8080/#guide)의 '시작 준비'·'끄기·설정' 탭에 있습니다.", "모두 가이드 탭의 '시작 준비'(http://localhost:8080/#guide/kickoff)·'끄기·설정'(http://localhost:8080/#guide/settings) 문서에 있습니다.",
	"상사에게 할 말(시작 준비 탭 1절 3문장)", "상사에게 할 말(시작 준비 문서 1절 3문장)",
	"반대가 나오면 답은 실행 가이드(http://localhost:8080/#guide) '시작 준비' 탭 1절을 봅니다.", "반대가 나오면 답은 '시작 준비' 문서 1절(http://localhost:8080/#guide/kickoff)을 봅니다.",
)

// TestGoldenMatchesRef re-derives the golden file from ../../../ref/calendar_plan.json
// when that reference exists (it is not part of the image build context).
func TestGoldenMatchesRef(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "ref", "calendar_plan.json"))
	if err != nil {
		t.Skip("ref/calendar_plan.json not present")
	}
	var plan struct {
		Events []struct {
			Key    string `json:"key"`
			Create struct {
				Summary     string   `json:"summary"`
				Start       string   `json:"startTime"`
				End         string   `json:"endTime"`
				Recurrence  []string `json:"recurrenceData"`
				Description string   `json:"description"`
				Reminders   []any    `json:"overrideReminders"`
			} `json:"create"`
		} `json:"events"`
	}
	if err := json.Unmarshal(b, &plan); err != nil {
		t.Fatal(err)
	}
	want := loadExpect(t)
	if len(plan.Events) != len(want) {
		t.Fatalf("plan has %d events, golden %d", len(plan.Events), len(want))
	}
	for i, p := range plan.Events {
		w := want[i]
		desc := reBr.ReplaceAllString(p.Create.Description, "\n")
		desc = reB.ReplaceAllString(desc, "")
		desc = reA.ReplaceAllStringFunc(desc, func(m string) string {
			sub := reA.FindStringSubmatch(m)
			if n := reNote.FindStringSubmatch(sub[1]); n != nil && sub[1] == sub[2] {
				return NotebookURL + n[1]
			}
			return sub[2] + "(" + NotebookURL + "#guide)"
		})
		desc = localEdits.Replace(desc)
		if p.Key != w.Key || p.Create.Summary != w.Summary || p.Create.Start != w.Start || p.Create.End != w.End ||
			strings.Join(p.Create.Recurrence, "|") != strings.Join(w.Recurrence, "|") ||
			(len(p.Create.Reminders) > 0) != w.Alarm || desc != w.Description {
			t.Errorf("golden differs from ref for %s", p.Key)
		}
	}
}

func TestBuildFormat(t *testing.T) {
	now := time.Date(2026, 10, 16, 1, 2, 3, 0, time.UTC)
	out := string(Build(FromDoc(nil), now))

	if !strings.HasPrefix(out, "BEGIN:VCALENDAR\r\n") || !strings.HasSuffix(out, "END:VCALENDAR\r\n") {
		t.Fatal("bad envelope")
	}
	// Every line ends with CRLF; no bare LF.
	if strings.Count(out, "\n") != strings.Count(out, "\r\n") {
		t.Fatal("bare LF found")
	}
	lines := strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n")
	for _, l := range lines {
		if len(l) > 75 {
			t.Fatalf("line longer than 75 octets: %d %q", len(l), l)
		}
		if !utf8.ValidString(l) {
			t.Fatalf("fold split a UTF-8 sequence: %q", l)
		}
	}
	// 12 planned events + 3 single events for first-Monday 1:1 notices moved
	// off holidays (2027-03-02, 05-04, 10-05).
	if n := strings.Count(out, "BEGIN:VEVENT"); n != 15 {
		t.Fatalf("VEVENT count %d", n)
	}
	if n := strings.Count(out, "BEGIN:VALARM"); n != 14 {
		t.Fatalf("VALARM count %d (wed/thu has none)", n)
	}
	unfolded := strings.ReplaceAll(out, "\r\n ", "")
	for _, want := range []string{
		"BEGIN:VTIMEZONE\r\nTZID:Asia/Seoul\r\n",
		"TZOFFSETTO:+0900",
		"UID:weekly-mon-blocker-2026-10-19@manager-notebook\r\n",
		"DTSTAMP:20261016T010203Z\r\n",
		"DTSTART;TZID=Asia/Seoul:20261109T092000\r\n",
		"RRULE:FREQ=WEEKLY;BYDAY=MO;UNTIL=20271022T145959Z\r\n",
		"EXDATE;TZID=Asia/Seoul:20270301T092000,20270503T092000,20271004T092000\r\n",
		"UID:monthly-mon-blocker-1on1-moved-20270302-2026-10-19@manager-notebook\r\n",
		"DTSTART;TZID=Asia/Seoul:20270302T092000\r\n",
		"RRULE:FREQ=WEEKLY;BYDAY=WE,TH;UNTIL=20271022T145959Z\r\n",
		"TRIGGER:PT0M",
	} {
		if !strings.Contains(unfolded, want) {
			t.Errorf("missing %q", want)
		}
	}
	// Every summary starts with "[관리] " and text is escaped.
	for _, l := range strings.Split(unfolded, "\r\n") {
		if strings.HasPrefix(l, "SUMMARY:") && !strings.HasPrefix(l, "SUMMARY:[관리] ") {
			t.Errorf("summary prefix: %q", l)
		}
		if strings.HasPrefix(l, "DESCRIPTION:") && strings.Contains(strings.ReplaceAll(l, `\,`, ""), ",") {
			t.Errorf("unescaped comma: %q", l)
		}
	}
	if !strings.Contains(unfolded, `DESCRIPTION:할 일\n1. Synology Chat '#막힘-헷갈림' 채널에 남은 막힘만 봅니다(Jira 플래그가 있으면 함께).\n`) {
		t.Error("description newline escaping wrong")
	}
	if strings.Contains(out, "claude.ai") {
		t.Error("cloud link leaked into ics")
	}
	// Deterministic apart from DTSTAMP.
	if out != string(Build(FromDoc(nil), now)) {
		t.Error("not deterministic")
	}
}

func TestEscape(t *testing.T) {
	got := Escape("a,b;c\\d\r\ne\nf")
	if got != `a\,b\;c\\d\ne\nf` {
		t.Fatalf("%q", got)
	}
}

func TestFold(t *testing.T) {
	s := "DESCRIPTION:" + strings.Repeat("가나다", 40)
	f := Fold(s)
	if !strings.HasSuffix(f, "\r\n") {
		t.Fatal("no CRLF")
	}
	parts := strings.Split(strings.TrimSuffix(f, "\r\n"), "\r\n")
	if len(parts) < 2 {
		t.Fatal("not folded")
	}
	for i, p := range parts {
		if len(p) > 75 || !utf8.ValidString(p) {
			t.Fatalf("part %d bad: %d %q", i, len(p), p)
		}
		if i > 0 && !strings.HasPrefix(p, " ") {
			t.Fatalf("continuation without space: %q", p)
		}
	}
	if strings.ReplaceAll(strings.TrimSuffix(f, "\r\n"), "\r\n ", "") != s {
		t.Fatal("unfold mismatch")
	}
	if Fold("short") != "short\r\n" {
		t.Fatal("short line changed")
	}
	exact := strings.Repeat("a", 75)
	if Fold(exact) != exact+"\r\n" {
		t.Fatal("75-octet line should not fold")
	}
}

func TestOtherStartDate(t *testing.T) {
	// A Wednesday start date is moved to its Monday; empty holidays mean no EXDATE.
	s := FromDoc(map[string]any{"startDate": "2027-03-03", "holidays": "", "cycleWeeks": "4", "cooldownWeeks": 2.0})
	if got := s.Week0.Format("2006-01-02"); got != "2027-03-01" {
		t.Fatalf("week0 %s", got)
	}
	evs := s.Events()
	byKey := map[string]Event{}
	for _, e := range evs {
		byKey[e.Key] = e
		if len(e.ExDates) > 0 && e.Key != "weekly-mon-blocker" {
			t.Errorf("%s has EXDATE without holidays", e.Key)
		}
	}
	// Retro Monday = week 5 = 2027-04-05 is a first Monday, so the weekly
	// Monday series excludes only first Mondays (no holidays here).
	for _, x := range byKey["weekly-mon-blocker"].ExDates {
		if !isFirstMon(x) {
			t.Errorf("mon blocker excludes a non-first Monday %s", iso(x))
		}
	}
	if iso(byKey["marker-week4-retro"].Start) != "2027-04-05T09:20:00" {
		t.Errorf("retro %s", iso(byKey["marker-week4-retro"].Start))
	}
	// Cycle: weeks 2..5 → last Friday 2027-04-09; cool-down weeks 6..7.
	if iso(byKey["marker-cycle1-cooldown"].Start) != "2027-04-09T13:55:00" {
		t.Errorf("cycle end %s", iso(byKey["marker-cycle1-cooldown"].Start))
	}
	d := byKey["marker-cycle1-cooldown"].Description
	if !strings.Contains(d, "다음 주(04-12~23)") || !strings.Contains(d, "04-23(금)") || !strings.Contains(d, "04-21(수)") || !strings.Contains(d, "사이클 2(04-26 시작)") {
		t.Errorf("cooldown text: %s", d)
	}
	if !strings.Contains(byKey["marker-week0-checklist"].Description, "4주 단위로 묶고, 끝나면 2주는") {
		t.Error("cycle words not replaced")
	}
	// First cycle Monday (week 2) = 2027-03-15 → mon blocker starts there, 1:1 on 2027-04-05.
	if iso(byKey["weekly-mon-blocker"].Start) != "2027-03-15T09:20:00" || iso(byKey["monthly-mon-blocker-1on1"].Start) != "2027-04-05T09:20:00" {
		t.Errorf("monday starts %s %s", iso(byKey["weekly-mon-blocker"].Start), iso(byKey["monthly-mon-blocker-1on1"].Start))
	}
	// 6 months from 2027-03-01 = 2027-09-01 (Wed) → Monday 2027-08-30; 12 months → 2028-02-28 (Mon).
	if iso(byKey["marker-6month-result"].Start) != "2027-08-30T09:20:00" || iso(byKey["marker-12month-result"].Start) != "2028-02-28T09:20:00" {
		t.Errorf("6/12 months %s %s", iso(byKey["marker-6month-result"].Start), iso(byKey["marker-12month-result"].Start))
	}
	if !strings.HasSuffix(byKey["weekly-fri-notebook"].RRule, "UNTIL=20280303T145959Z") {
		t.Errorf("until %s", byKey["weekly-fri-notebook"].RRule)
	}
	out := string(Build(s, time.Now()))
	if !strings.Contains(out, "UID:marker-week0-checklist-2027-03-01@manager-notebook") {
		t.Error("uid uses normalised start date")
	}
}

func TestHolidayOnFirstMondayMovesToNextWorkday(t *testing.T) {
	s := FromDoc(map[string]any{"startDate": "2026-10-19", "holidays": "2026-12-07 가상 휴일\n2026-12-08 다음 날도 휴일"})
	var found bool
	for _, e := range s.Events() {
		switch {
		case e.Key == "monthly-mon-blocker-1on1":
			if joinLocal(e.ExDates) != "20261207T092000" {
				t.Fatalf("ex %v", joinLocal(e.ExDates))
			}
		case strings.HasPrefix(e.Key, MovedOneOnOnePrefix):
			if e.Key != MovedOneOnOnePrefix+"20261209" || local(e.Start) != "20261209T092000" || e.RRule != "" {
				t.Fatalf("moved %s %s", e.Key, local(e.Start))
			}
			found = true
		}
	}
	if !found {
		t.Fatal("moved event missing")
	}
}
