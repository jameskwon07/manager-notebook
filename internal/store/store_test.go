package store

import (
	"context"
	"errors"
	"sort"
	"strings"
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
	if err != nil || st.Settings != nil || len(st.Notes) != 0 || st.Notes == nil {
		t.Fatalf("empty state wrong: %+v %v", st, err)
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

func TestImportReplacesAll(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	s.Create(ctx, "notes", Doc{"text": "old"})
	s.Create(ctx, "blockers", Doc{"text": "old"})
	s.PutSettings(ctx, Doc{"startDate": "2026-10-19"})

	ex, err := s.MakeExport(ctx, "1.0.0")
	if err != nil || ex.Format != ExportFormat || len(ex.Notes) != 1 {
		t.Fatalf("export: %+v %v", ex, err)
	}

	in := &Export{
		Format: ExportFormat,
		Notes:  []Doc{{"id": "n1", "text": "a"}, {"text": "no id"}, {"id": "n1", "text": "dup id"}},
		Fridays: []Doc{
			{"id": "2026-10-26", "week": "2026-10-26"},
		},
	}
	n, err := s.Import(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if n.Notes != 3 || n.Blockers != 0 || n.Fridays != 1 {
		t.Fatalf("counts %+v", n)
	}
	st, _ := s.LoadState(ctx)
	if len(st.Notes) != 3 || len(st.Blockers) != 0 || len(st.Fridays) != 1 || st.Settings != nil {
		t.Fatalf("state after import: %+v", st)
	}
	for _, d := range st.Notes {
		if d["text"] == "old" {
			t.Fatal("old note survived import")
		}
	}

	// Round trip: import the first export back.
	if _, err := s.Import(ctx, ex); err != nil {
		t.Fatal(err)
	}
	st, _ = s.LoadState(ctx)
	if len(st.Notes) != 1 || st.Notes[0]["text"] != "old" || st.Settings["startDate"] != "2026-10-19" {
		t.Fatalf("round trip failed: %+v", st)
	}

	// Bad format leaves data untouched.
	if _, err := s.Import(ctx, &Export{Format: "nope"}); !errors.Is(err, ErrBadFormat) {
		t.Fatalf("bad format: %v", err)
	}
	// A too-large doc aborts the whole import.
	bad := &Export{Format: ExportFormat, Notes: []Doc{{"text": strings.Repeat("x", MaxDocBytes+1)}}}
	if _, err := s.Import(ctx, bad); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("too large import: %v", err)
	}
	st, _ = s.LoadState(ctx)
	if len(st.Notes) != 1 {
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
	if _, err := s.Import(ctx, &Export{Format: ExportFormat, Notes: docs}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, "notes", Doc{"t": "one more"}); !errors.Is(err, ErrLimit) {
		t.Fatalf("want limit, got %v", err)
	}
	// Replacing an existing doc is still allowed.
	st, _ := s.LoadState(ctx)
	if _, err := s.Put(ctx, "notes", st.Notes[0]["id"].(string), Doc{"t": "x"}); err != nil {
		t.Fatal(err)
	}
	over := make([]Doc, MaxDocsPerCol+1)
	if _, err := s.Import(ctx, &Export{Format: ExportFormat, Notes: over}); !errors.Is(err, ErrLimit) {
		t.Fatalf("import over limit: %v", err)
	}
}
