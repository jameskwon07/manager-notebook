package demo

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/jameskwon07/manager-notebook/internal/store"
)

func TestSeedFillsEveryCollection(t *testing.T) {
	st, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 11, 4, 15, 0, 0, 0, time.Local) // 수요일
	if err := Seed(context.Background(), st, now); err != nil {
		t.Fatal(err)
	}
	state, err := st.LoadState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Settings == nil || state.Settings["startDate"] != "2026-10-12" {
		t.Fatalf("startDate: %v", state.Settings["startDate"])
	}
	want := map[string]int{"programs": 7, "requests": 8, "decisions": 3, "mapitems": 4, "timelog": 5, "blockers": 2, "notes": 2, "weeks": 1, "cycles": 1, "fridays": 1}
	for coll, n := range want {
		if got := len(state.Colls[coll]); got != n {
			t.Errorf("%s: %d docs, want %d", coll, got, n)
		}
	}
	// risks are left empty on purpose: the page seeds them from content.json.
	if got := len(state.Colls["risks"]); got != 0 {
		t.Errorf("risks: %d, want 0", got)
	}
	// Every request points at a program that exists (or none).
	ids := map[string]bool{"": true}
	for _, p := range state.Colls["programs"] {
		ids[p["id"].(string)] = true
	}
	for _, r := range state.Colls["requests"] {
		if !ids[r["program"].(string)] {
			t.Errorf("request %v points at unknown program %v", r["id"], r["program"])
		}
		// 창구 예시의 링크 코드는 실제 창구가 받는 모양(헷갈리는 글자 없는 10자)이어야 합니다.
		if c, ok := r["askCode"].(string); ok && !regexp.MustCompile(`^[23456789abcdefghjkmnpqrstuvwxyz]{10}$`).MatchString(c) {
			t.Errorf("request %v askCode %q", r["id"], c)
		}
	}
	// The cycle doc is keyed by the first cycle's Monday (start + 2 weeks).
	if state.Colls["cycles"][0]["id"] != "2026-10-26" {
		t.Errorf("cycle id %v", state.Colls["cycles"][0]["id"])
	}
	if state.Colls["weeks"][0]["id"] != "2026-11-02" {
		t.Errorf("week id %v", state.Colls["weeks"][0]["id"])
	}
}
