package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCRUD(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	d, err := s.Create(ctx, "notes", Doc{"kind": "q", "text": "hello", "done": false})
	if err != nil {
		t.Fatal(err)
	}
	id, _ := d["id"].(string)
	if !ValidID(id) {
		t.Fatalf("bad id %q", id)
	}
	got, err := s.Get(ctx, "notes", id)
	if err != nil || got["text"] != "hello" || got["id"] != id {
		t.Fatalf("get: %v %v", got, err)
	}

	if _, err := s.Put(ctx, "notes", id, Doc{"kind": "l", "text": "replaced"}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Get(ctx, "notes", id)
	if got["text"] != "replaced" || got["done"] != nil {
		t.Fatalf("put did not replace: %v", got)
	}

	// PUT creates when missing.
	if _, err := s.Put(ctx, "fridays", "2026-10-26", Doc{"week": "2026-10-26"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "fridays", "2026-10-26"); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(ctx, "notes", id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "notes", id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
	// Deleting twice is fine.
	if err := s.Delete(ctx, "notes", id); err != nil {
		t.Fatal(err)
	}
}

func TestBadCollAndID(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if _, err := s.Create(ctx, "settings", Doc{}); !errors.Is(err, ErrBadColl) {
		t.Fatalf("settings via Create: %v", err)
	}
	if _, err := s.Put(ctx, "notes", "bad id!", Doc{}); !errors.Is(err, ErrBadID) {
		t.Fatalf("bad id: %v", err)
	}
	if _, err := s.Put(ctx, "notes", strings.Repeat("a", 65), Doc{}); !errors.Is(err, ErrBadID) {
		t.Fatalf("long id: %v", err)
	}
}

func TestPatchNullDeletes(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	d, _ := s.Create(ctx, "blockers", Doc{"kind": "b", "pausedAt": "2026-11-02T00:00:00Z", "pausedDays": 1.0})
	id := d["id"].(string)

	out, err := s.Patch(ctx, "blockers", id, Doc{"pausedAt": nil, "state": "done", "id": "hijack"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["pausedAt"]; ok {
		t.Fatalf("pausedAt not deleted: %v", out)
	}
	got, _ := s.Get(ctx, "blockers", id)
	if _, ok := got["pausedAt"]; ok || got["state"] != "done" || got["kind"] != "b" || got["id"] != id {
		t.Fatalf("patch result wrong: %v", got)
	}
	if _, err := s.Patch(ctx, "blockers", "missing", Doc{"a": 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("patch missing: %v", err)
	}
}

func TestUpdateIsAtomic(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	d, _ := s.Create(ctx, "requests", Doc{"what": "x", "adds": []any{}})
	id := d["id"].(string)

	// 여러 고루틴이 동시에 덧붙여도 하나도 잃지 않습니다(읽기·검사·쓰기가 한 트랜잭션).
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.Update(ctx, "requests", id, func(cur Doc) (Doc, error) {
				list, _ := cur["adds"].([]any)
				cur["adds"] = append(list, map[string]any{"n": i})
				return cur, nil
			})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	got, _ := s.Get(ctx, "requests", id)
	if n := len(got["adds"].([]any)); n != 30 {
		t.Fatalf("adds = %d, want 30", n)
	}

	stop := errors.New("stop")
	if _, err := s.Update(ctx, "requests", id, func(cur Doc) (Doc, error) {
		cur["what"] = "바뀌면 안 됨"
		return nil, stop
	}); !errors.Is(err, stop) {
		t.Fatalf("fn error not returned: %v", err)
	}
	if got, _ := s.Get(ctx, "requests", id); got["what"] != "x" {
		t.Fatalf("aborted update wrote: %v", got["what"])
	}
	if _, err := s.Update(ctx, "requests", "missing", func(cur Doc) (Doc, error) { return cur, nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := s.Update(ctx, "nope", id, func(cur Doc) (Doc, error) { return cur, nil }); !errors.Is(err, ErrBadColl) {
		t.Fatalf("bad coll: %v", err)
	}
}

func TestDocTooLarge(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	big := strings.Repeat("가", MaxDocBytes/3+10)
	if _, err := s.Create(ctx, "notes", Doc{"text": big}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want too large, got %v", err)
	}
}

func TestIDsSortable(t *testing.T) {
	s := open(t)
	var ids []string
	for i := 0; i < 500; i++ {
		ids = append(ids, s.NewID())
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatal("ids not sorted")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] || !ValidID(id) {
			t.Fatalf("dup or invalid id %q", id)
		}
		seen[id] = true
	}
}

func TestSettingsAndState(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	st, err := s.LoadState(ctx)
	if err != nil || st.Settings != nil {
		t.Fatalf("empty state wrong: %+v %v", st, err)
	}
	if len(st.Colls) != 11 {
		t.Fatalf("want 11 collections, got %d", len(st.Colls))
	}
	for _, c := range Collections {
		if docs, ok := st.Colls[c]; !ok || docs == nil || len(docs) != 0 {
			t.Fatalf("%s should be an empty non-nil list: %v", c, docs)
		}
	}
	if _, err := s.PutSettings(ctx, Doc{"startDate": "2026-10-19", "cycleWeeks": 6.0}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutSettings(ctx, Doc{"startDate": "2026-11-02"}); err != nil {
		t.Fatal(err)
	}
	st, _ = s.LoadState(ctx)
	if st.Settings["startDate"] != "2026-11-02" || st.Settings["cycleWeeks"] != nil || st.Settings["id"] != nil {
		t.Fatalf("settings not replaced: %v", st.Settings)
	}
	v, err := s.Schema(ctx)
	if err != nil || v != SchemaVersion {
		t.Fatalf("schema %q %v", v, err)
	}
}

// TestAllCollectionsCRUD runs create, put, patch, get, list and delete once
// on each of the 11 collections.
func TestAllCollectionsCRUD(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if len(Collections) != 11 {
		t.Fatalf("want 11 collections, got %v", Collections)
	}
	want := []string{"notes", "blockers", "fridays", "requests", "programs", "decisions", "mapitems", "timelog", "weeks", "cycles", "risks"}
	for i, c := range want {
		if Collections[i] != c || !ValidColl(c) {
			t.Fatalf("collection %d: %q", i, Collections[i])
		}
	}
	for _, bad := range []string{"settings", "meta", "docs", "", "Notes", "request"} {
		if ValidColl(bad) {
			t.Fatalf("%q should not be a collection", bad)
		}
	}
	for _, c := range Collections {
		d, err := s.Create(ctx, c, Doc{"text": c, "n": 1.0})
		if err != nil {
			t.Fatalf("%s create: %v", c, err)
		}
		id := d["id"].(string)
		if _, err := s.Put(ctx, c, "2026-10-26", Doc{"week": "2026-10-26"}); err != nil {
			t.Fatalf("%s put: %v", c, err)
		}
		if _, err := s.Patch(ctx, c, id, Doc{"n": nil, "done": true}); err != nil {
			t.Fatalf("%s patch: %v", c, err)
		}
		got, err := s.Get(ctx, c, id)
		if err != nil || got["text"] != c || got["done"] != true || got["n"] != nil {
			t.Fatalf("%s get: %v %v", c, got, err)
		}
		list, _ := s.List(ctx, c)
		if len(list) != 2 {
			t.Fatalf("%s list: %v", c, list)
		}
		if err := s.Delete(ctx, c, id); err != nil {
			t.Fatalf("%s delete: %v", c, err)
		}
		if _, err := s.Get(ctx, c, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s after delete: %v", c, err)
		}
	}
	st, _ := s.LoadState(ctx)
	for _, c := range Collections {
		if len(st.Colls[c]) != 1 || st.Colls[c][0]["id"] != "2026-10-26" {
			t.Fatalf("%s state: %v", c, st.Colls[c])
		}
	}
}

// parse turns a JSON text into the raw key map the server passes in.
func parse(t *testing.T, s string) map[string]json.RawMessage {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func importText(t *testing.T, s *Store, text string) (*ImportResult, error) {
	t.Helper()
	f, err := ParseImport(parse(t, text))
	if err != nil {
		return nil, err
	}
	return s.Import(context.Background(), f)
}

func fill(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	for _, c := range Collections {
		if _, err := s.Create(ctx, c, Doc{"text": "old " + c}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.PutSettings(ctx, Doc{"startDate": "2026-10-19"}); err != nil {
		t.Fatal(err)
	}
}

func TestExportShapeAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	fill(t, s)
	if _, err := s.Create(ctx, "requests", Doc{"what": "second", "est": map[string]any{"min": 1.0, "mid": 2.5, "max": 4.0}}); err != nil {
		t.Fatal(err)
	}
	ex, err := s.MakeExport(ctx, "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(ex)
	if err != nil {
		t.Fatal(err)
	}
	// Flat keys in a fixed order.
	wantOrder := []string{`"format"`, `"version"`, `"exportedAt"`, `"settings"`}
	for _, c := range Collections {
		wantOrder = append(wantOrder, `"`+c+`":[`)
	}
	pos := -1
	for _, k := range wantOrder {
		i := strings.Index(string(b), k)
		if i <= pos {
			t.Fatalf("key %s missing or out of order in %s", k, b)
		}
		pos = i
	}
	raw := parse(t, string(b))
	if len(raw) != 4+11 || string(raw["format"]) != `"manager-notebook-export"` || string(raw["version"]) != `"2.0.0"` {
		t.Fatalf("export keys: %s", b)
	}

	// Change everything, then import the export back.
	for _, c := range Collections {
		s.Create(ctx, c, Doc{"text": "after export"})
	}
	s.PutSettings(ctx, Doc{"startDate": "2027-01-04"})
	res, err := importText(t, s, string(b))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Replaced) != 11 || !res.Settings || res.Counts["requests"] != 2 || res.Counts["notes"] != 1 {
		t.Fatalf("result %+v", res)
	}
	for i, c := range Collections {
		if res.Replaced[i] != c {
			t.Fatalf("replaced order %v", res.Replaced)
		}
	}
	st, _ := s.LoadState(ctx)
	if st.Settings["startDate"] != "2026-10-19" {
		t.Fatalf("settings not restored: %v", st.Settings)
	}
	for _, c := range Collections {
		for _, d := range st.Colls[c] {
			if d["text"] == "after export" {
				t.Fatalf("%s kept a doc made after the export", c)
			}
		}
	}
	b2, _ := json.Marshal(&Export{Format: ExportFormat, Version: "2.0.0", ExportedAt: ex.ExportedAt, Settings: st.Settings, Colls: st.Colls})
	if string(b2) != string(b) {
		t.Fatalf("round trip differs:\n%s\n%s", b, b2)
	}
}

// TestImportV1KeepsV2 imports a 1.0 export (notes, blockers, fridays,
// settings only): those are replaced and the 2.0 collections are kept.
func TestImportV1KeepsV2(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	fill(t, s)
	v1 := `{"format":"manager-notebook-export","version":"1.0.0","exportedAt":"2026-10-16T09:00:00+09:00",
		"settings":{"startDate":"2026-11-02","holidays":""},
		"notes":[{"id":"n1","kind":"q","text":"메모","done":false},{"text":"no id"},{"id":"n1","text":"dup id"}],
		"blockers":[],
		"fridays":[{"id":"2026-10-23","team":"t","boss5":"b","kept_line":"k"}]}`
	res, err := importText(t, s, v1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.Replaced, ",") != "notes,blockers,fridays" || !res.Settings ||
		len(res.Counts) != 3 || res.Counts["notes"] != 3 || res.Counts["blockers"] != 0 || res.Counts["fridays"] != 1 {
		t.Fatalf("result %+v", res)
	}
	st, _ := s.LoadState(ctx)
	if len(st.Colls["notes"]) != 3 || len(st.Colls["blockers"]) != 0 || len(st.Colls["fridays"]) != 1 {
		t.Fatalf("1.0 collections: %v", st.Colls)
	}
	ids := map[string]bool{}
	for _, d := range st.Colls["notes"] {
		ids[d["id"].(string)] = true
	}
	if len(ids) != 3 || !ids["n1"] {
		t.Fatalf("ids not unique: %v", ids)
	}
	for _, c := range Collections[3:] {
		if len(st.Colls[c]) != 1 || st.Colls[c][0]["text"] != "old "+c {
			t.Fatalf("2.0 collection %s changed: %v", c, st.Colls[c])
		}
	}
	if st.Settings["startDate"] != "2026-11-02" {
		t.Fatalf("settings not replaced: %v", st.Settings)
	}
}

func TestImportKeyRules(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	fill(t, s)

	// Empty array empties; missing and null keys keep; settings null keeps.
	res, err := importText(t, s, `{"format":"manager-notebook-export","settings":null,"requests":[],"risks":null,"llm":{"enabled":true}}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.Replaced, ",") != "requests" || res.Settings || res.Counts["requests"] != 0 || len(res.Counts) != 1 {
		t.Fatalf("result %+v", res)
	}
	b, _ := json.Marshal(res)
	if string(b) != `{"replaced":["requests"],"counts":{"requests":0},"settings":false}` {
		t.Fatalf("result json %s", b)
	}
	st, _ := s.LoadState(ctx)
	if len(st.Colls["requests"]) != 0 || len(st.Colls["risks"]) != 1 || len(st.Colls["notes"]) != 1 || st.Settings["startDate"] != "2026-10-19" {
		t.Fatalf("state %+v", st)
	}

	// No settings key at all keeps the settings too.
	if _, err := importText(t, s, `{"format":"manager-notebook-export","weeks":[{"id":"2026-10-26","signals":{"reviewEmpty":true}}]}`); err != nil {
		t.Fatal(err)
	}
	st, _ = s.LoadState(ctx)
	if st.Settings["startDate"] != "2026-10-19" || len(st.Colls["weeks"]) != 1 || st.Colls["weeks"][0]["id"] != "2026-10-26" {
		t.Fatalf("state %+v", st)
	}

	// Format errors change nothing.
	for _, bad := range []string{
		`{"format":"nope","notes":[]}`,
		`{"notes":[]}`,
		`{"format":5}`,
		`{"format":"manager-notebook-export","notes":{"a":1}}`,
		`{"format":"manager-notebook-export","settings":"x"}`,
		`{"format":"manager-notebook-export","requests":[1]}`,
	} {
		if _, err := importText(t, s, bad); !errors.Is(err, ErrBadFormat) {
			t.Fatalf("%s: want bad format, got %v", bad, err)
		}
	}
	if _, err := ParseImport(nil); !errors.Is(err, ErrBadFormat) {
		t.Fatalf("nil map: %v", err)
	}
	if _, err := importText(t, s, `{"format":"manager-notebook-export","notes":[],"requests":[null]}`); !errors.Is(err, ErrNotObject) {
		t.Fatalf("null doc: %v", err)
	}
	// A too-large doc aborts the whole import, including earlier collections.
	big := `{"format":"manager-notebook-export","notes":[],"risks":[{"t":"` + strings.Repeat("x", MaxDocBytes+1) + `"}]}`
	if _, err := importText(t, s, big); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("too large import: %v", err)
	}
	st, _ = s.LoadState(ctx)
	if len(st.Colls["notes"]) != 1 || len(st.Colls["risks"]) != 1 {
		t.Fatal("failed import changed data")
	}
}

func TestCollectionLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	ctx := context.Background()
	s := open(t)
	docs := make([]Doc, MaxDocsPerCol)
	for i := range docs {
		docs[i] = Doc{"t": i}
	}
	if _, err := s.Import(ctx, &ImportFile{Colls: map[string][]Doc{"timelog": docs}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, "timelog", Doc{"t": "one more"}); !errors.Is(err, ErrLimit) {
		t.Fatalf("want limit, got %v", err)
	}
	// Replacing an existing doc is still allowed.
	st, _ := s.LoadState(ctx)
	if _, err := s.Put(ctx, "timelog", st.Colls["timelog"][0]["id"].(string), Doc{"t": "x"}); err != nil {
		t.Fatal(err)
	}
	over := make([]Doc, MaxDocsPerCol+1)
	if _, err := s.Import(ctx, &ImportFile{Colls: map[string][]Doc{"notes": over}}); !errors.Is(err, ErrLimit) {
		t.Fatalf("import over limit: %v", err)
	}
}
