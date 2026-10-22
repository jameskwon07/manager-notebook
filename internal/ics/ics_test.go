package ics

import (
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// vevent is one parsed VEVENT: its unfolded property lines by name
// (parameters stripped from the name).
type vevent map[string]string

// parseICS unfolds the calendar and returns its VEVENTs. It checks nothing;
// the tests below do.
func parseICS(t *testing.T, out string) []vevent {
	t.Helper()
	unfolded := strings.ReplaceAll(out, "\r\n ", "")
	var evs []vevent
	var cur vevent
	inAlarm := false
	for _, l := range strings.Split(strings.TrimSuffix(unfolded, "\r\n"), "\r\n") {
		switch l {
		case "BEGIN:VEVENT":
			cur = vevent{}
			continue
		case "END:VEVENT":
			evs = append(evs, cur)
			cur = nil
			continue
		case "BEGIN:VALARM":
			inAlarm = true
			cur["VALARM"] = "yes"
			continue
		case "END:VALARM":
			inAlarm = false
			continue
		}
		if cur == nil || inAlarm {
			continue
		}
		name, val, ok := strings.Cut(l, ":")
		if !ok {
			t.Fatalf("bad line %q", l)
		}
		if i := strings.IndexByte(name, ';'); i >= 0 {
			name = name[:i]
		}
		cur[name] = val
	}
	return evs
}

// keyOf is the UID key: the part before "-YYYY-MM-DD-v2@manager-notebook".
func keyOf(t *testing.T, uid string) (key, date string) {
	t.Helper()
	rest, ok := strings.CutSuffix(uid, "-v2@manager-notebook")
	if !ok || len(rest) < 12 || rest[len(rest)-11] != '-' {
		t.Fatalf("uid shape %q", uid)
	}
	return rest[:len(rest)-11], rest[len(rest)-10:]
}

func kst(s string) time.Time {
	d, err := time.ParseInLocation("20060102T150405", s, KST)
	if err != nil {
		panic(err)
	}
	return d
}

// TestDefaultSchedule checks the calendar built from the default settings
// (start 2026-10-19, cycle 2/6/1, the 1.0 default holidays). Every expected
// date below was worked out by hand from a paper calendar and the holiday
// list; none of it is computed with this package's code.
func TestDefaultSchedule(t *testing.T) {
	now := time.Date(2026, 10, 16, 1, 2, 3, 0, time.UTC)
	out := string(Build(FromDoc(nil), now))

	// Envelope, CRLF everywhere, every physical line at most 75 octets.
	if !strings.HasPrefix(out, "BEGIN:VCALENDAR\r\n") || !strings.HasSuffix(out, "END:VCALENDAR\r\n") {
		t.Fatal("bad envelope")
	}
	if strings.Count(out, "\n") != strings.Count(out, "\r\n") || strings.Count(out, "\r") != strings.Count(out, "\r\n") {
		t.Fatal("line ending is not CRLF everywhere")
	}
	for _, l := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
		if len(l) > 75 {
			t.Fatalf("line longer than 75 octets: %d %q", len(l), l)
		}
		if !utf8.ValidString(l) {
			t.Fatalf("fold split a UTF-8 sequence: %q", l)
		}
	}
	for _, want := range []string{
		"BEGIN:VTIMEZONE\r\nTZID:Asia/Seoul\r\nBEGIN:STANDARD\r\n",
		"TZOFFSETTO:+0900\r\n",
		"X-WR-TIMEZONE:Asia/Seoul\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}

	evs := parseICS(t, out)
	// 1 weekly Friday rule + 12 monthly risk + 8 cycle starts + 7 cool-down
	// starts + 3-month + 6-month.
	if len(evs) != 30 {
		t.Fatalf("VEVENT count %d, want 30", len(evs))
	}

	uids := map[string]bool{}
	got := map[string][]string{} // key -> start dates (single events)
	for _, e := range evs {
		uid := e["UID"]
		if uids[uid] {
			t.Fatalf("duplicate UID %s", uid)
		}
		uids[uid] = true
		key, uidDate := keyOf(t, uid)
		if !strings.HasPrefix(e["SUMMARY"], "[관리] ") {
			t.Errorf("summary prefix: %q", e["SUMMARY"])
		}
		if e["VALARM"] != "yes" {
			t.Errorf("%s has no alarm", uid)
		}
		start, end := kst(e["DTSTART"]), kst(e["DTEND"])
		if end.Sub(start) != 30*time.Minute {
			t.Errorf("%s length %s", uid, end.Sub(start))
		}
		if e["DTSTAMP"] != "20261016T010203Z" {
			t.Errorf("dtstamp %s", e["DTSTAMP"])
		}
		if !strings.Contains(e["DESCRIPTION"], `관리 수첩: http://localhost:8080/#`) {
			t.Errorf("%s description has no notebook link", uid)
		}
		if strings.Contains(strings.ReplaceAll(e["DESCRIPTION"], `\,`, ""), ",") {
			t.Errorf("unescaped comma in %s", uid)
		}
		if key == KeyFriday {
			continue
		}
		if e["RRULE"] != "" || e["EXDATE"] != "" {
			t.Errorf("single event %s has a rule", uid)
		}
		if start.Format("1504") != "0920" || start.Weekday() == time.Saturday || start.Weekday() == time.Sunday {
			t.Errorf("%s starts %s", uid, start)
		}
		if uidDate != start.Format("2006-01-02") {
			t.Errorf("%s uid date differs from start %s", uid, start)
		}
		got[key] = append(got[key], start.Format("2006-01-02"))
	}

	want := map[string][]string{
		// First Monday of each month from 2026-11; 03-01, 05-03 and 10-04
		// are holidays, so the next workday (Tuesday) is used.
		KeyRisk: {"2026-11-02", "2026-12-07", "2027-01-04", "2027-02-01", "2027-03-02", "2027-04-05",
			"2027-05-04", "2027-06-07", "2027-07-05", "2027-08-02", "2027-09-06", "2027-10-05"},
		// Week 2, 9, 16, ... 51. Week 16 is 2027-02-08 (holiday, and 02-09
		// too) → 02-10; week 51 is 2027-10-11 (holiday) → 10-12.
		KeyCycle: {"2026-11-02", "2026-12-21", "2027-02-10", "2027-03-29", "2027-05-17", "2027-07-05",
			"2027-08-23", "2027-10-12"},
		// Week 8, 15, ... 50. 2027-08-16 and 2027-10-04 are holidays.
		KeyCooldown: {"2026-12-14", "2027-02-01", "2027-03-22", "2027-05-10", "2027-06-28",
			"2027-08-17", "2027-10-05"},
		// 2026-10-19 + 3 months = 2027-01-19 (Tue) → that week's Monday.
		KeyMonth3: {"2027-01-18"},
		// + 6 months = 2027-04-19, a Monday.
		KeyMonth6: {"2027-04-19"},
	}
	for k, w := range want {
		g := got[k]
		sort.Strings(g)
		if strings.Join(g, " ") != strings.Join(w, " ") {
			t.Errorf("%s dates\n got %v\nwant %v", k, g, w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("unexpected keys: %v", got)
	}
	// The cycle titles count up.
	for _, e := range evs {
		if e["UID"] == "cycle-start-2027-02-10-v2@manager-notebook" && e["SUMMARY"] != "[관리] 사이클 3 시작 — 자리 계산·지침 네 칸" {
			t.Errorf("cycle 3 summary %q", e["SUMMARY"])
		}
	}

	// The weekly Friday rule.
	var fri vevent
	for _, e := range evs {
		if strings.HasPrefix(e["UID"], KeyFriday+"-") {
			fri = e
		}
	}
	if fri == nil {
		t.Fatal("no Friday event")
	}
	if fri["UID"] != "friday-read-2026-10-19-v2@manager-notebook" {
		t.Errorf("friday uid %s", fri["UID"])
	}
	if fri["DTSTART"] != "20261023T135500" || fri["DTEND"] != "20261023T142500" {
		t.Errorf("friday start %s end %s", fri["DTSTART"], fri["DTEND"])
	}
	// Week 0 + 12 months = 2027-10-19 (Tue), its Friday is
	// 2027-10-22; 13:55 KST = 04:55 UTC.
	if fri["RRULE"] != "FREQ=WEEKLY;BYDAY=FR;UNTIL=20271022T045500Z" {
		t.Errorf("friday rule %s", fri["RRULE"])
	}
	// 2026-12-25 and 2027-01-01 are the only holiday Fridays in the list.
	if fri["EXDATE"] != "20261225T135500,20270101T135500" {
		t.Errorf("friday exdate %s", fri["EXDATE"])
	}
	if !strings.Contains(out, "EXDATE;TZID=Asia/Seoul:20261225T135500,20270101T135500\r\n") {
		t.Error("EXDATE without TZID")
	}
	// Expand the rule here (weekly from DTSTART up to UNTIL, minus EXDATE)
	// and compare with a plain count of the Fridays in the span.
	until, _ := time.Parse("20060102T150405Z", "20271022T045500Z")
	ex := map[string]bool{}
	for _, x := range strings.Split(fri["EXDATE"], ",") {
		ex[x] = true
	}
	n := 0
	for d := kst(fri["DTSTART"]); !d.After(until); d = d.AddDate(0, 0, 7) {
		if !ex[d.Format("20060102T150405")] {
			n++
		}
	}
	// 2026-10-23 .. 2027-10-22 holds 53 Fridays; 2 are holidays.
	if n != 51 {
		t.Errorf("friday occurrences %d, want 51", n)
	}

	// The moved dates really come from the default holiday list.
	for _, h := range []string{"2026-12-25", "2027-01-01", "2027-02-08", "2027-02-09", "2027-03-01",
		"2027-05-03", "2027-08-16", "2027-10-04", "2027-10-11"} {
		if !strings.Contains(DefaultHolidays, h+" ") {
			t.Errorf("default holidays lack %s", h)
		}
	}

	if out != string(Build(FromDoc(nil), now)) {
		t.Error("not deterministic")
	}
	if !strings.Contains(out, "PRODID:-//manager-notebook//manager-notebook 2.0//KO\r\n") {
		t.Error("prodid")
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

// TestOtherSettings: a Wednesday start date moves to its Monday, no holidays
// means no EXDATE and no moved dates, and the cycle settings are used.
func TestOtherSettings(t *testing.T) {
	s := FromDoc(map[string]any{"startDate": "2027-03-03", "holidays": "", "cycleStartWeek": 1.0, "cycleWeeks": "4", "cooldownWeeks": 2.0})
	if got := s.Week0.Format("2006-01-02"); got != "2027-03-01" {
		t.Fatalf("week0 %s", got)
	}
	evs := parseICS(t, string(Build(s, time.Now())))
	got := map[string][]string{}
	for _, e := range evs {
		key, _ := keyOf(t, e["UID"])
		got[key] = append(got[key], kst(e["DTSTART"]).Format("2006-01-02"))
		if e["EXDATE"] != "" {
			t.Errorf("EXDATE without holidays: %s", e["UID"])
		}
		if key == KeyFriday {
			// 2027-03-01 + 12 months = 2028-03-01 (Wed) → Friday 2028-03-03.
			if e["UID"] != "friday-read-2027-03-01-v2@manager-notebook" || e["DTSTART"] != "20270305T135500" ||
				e["RRULE"] != "FREQ=WEEKLY;BYDAY=FR;UNTIL=20280303T045500Z" {
				t.Errorf("friday %v", e)
			}
		}
	}
	// Cycle k starts at week 1 + 6(k-1): 03-08, 04-19, 05-31, ...; the
	// cool-down starts 4 weeks later.
	if g := got[KeyCycle]; len(g) < 3 || g[0] != "2027-03-08" || g[1] != "2027-04-19" || g[2] != "2027-05-31" {
		t.Errorf("cycles %v", g)
	}
	if g := got[KeyCooldown]; len(g) < 2 || g[0] != "2027-04-05" || g[1] != "2027-05-17" {
		t.Errorf("cool-downs %v", g)
	}
	// 9 cycles start on or before 2028-03-03 (week 49 = 2028-02-07);
	// the 9th cool-down (2028-03-06) is past the end.
	if len(got[KeyCycle]) != 9 || len(got[KeyCooldown]) != 8 {
		t.Errorf("cycle count %d cool-down count %d", len(got[KeyCycle]), len(got[KeyCooldown]))
	}
	// First Mondays 2027-03-01 .. 2028-03-06: 03-01 is week 0 itself; the span
	// ends 2028-03-03, so 2028-03-06 is out → 12 months.
	if g := got[KeyRisk]; len(g) != 12 || g[0] != "2027-03-01" || g[11] != "2028-02-07" {
		t.Errorf("risk %v", g)
	}
	// 3 months: 2027-06-01 (Tue) → 05-31; 6 months: 2027-09-01 (Wed) → 08-30.
	if got[KeyMonth3][0] != "2027-05-31" || got[KeyMonth6][0] != "2027-08-30" {
		t.Errorf("3/6 months %v %v", got[KeyMonth3], got[KeyMonth6])
	}
}

// TestHolidayListForms: holidays may be saved as one text or as a list.
func TestHolidayListForms(t *testing.T) {
	a := FromDoc(map[string]any{"holidays": []any{"2026-11-02 쉬는 날", 5.0}})
	if !a.Holidays["2026-11-02"] || len(a.Holidays) != 1 {
		t.Fatalf("list form %v", a.Holidays)
	}
	b := FromDoc(map[string]any{"holidays": "2026-11-02 쉬는 날, 2026-11-03"})
	if len(b.Holidays) != 2 {
		t.Fatalf("text form %v", b.Holidays)
	}
	// null falls back to the default list, "" means none.
	if c := FromDoc(map[string]any{"holidays": nil}); len(c.Holidays) != 16 {
		t.Fatalf("null → default: %d", len(c.Holidays))
	}
	if d := FromDoc(map[string]any{"holidays": ""}); len(d.Holidays) != 0 {
		t.Fatalf("empty → none: %v", d.Holidays)
	}
	// 2026-11-02 is both a first Monday and the cycle-1 start: both move.
	var moved int
	for _, e := range a.Events() {
		if (e.Key == KeyRisk || e.Key == KeyCycle) && ymd(e.Start) == "2026-11-03" {
			moved++
		}
	}
	if moved != 2 {
		t.Fatalf("moved events %d", moved)
	}
}
