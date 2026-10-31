package store

import (
	"context"
	"testing"
)

func TestOpenMemoryKeepsData(t *testing.T) {
	s, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err := s.Put(ctx, "programs", "prog-1", Doc{"name": "예시"}); err != nil {
		t.Fatal(err)
	}
	// Force the pool to drop and reopen its connection; shared cache must keep the row.
	s.db.SetMaxIdleConns(0)
	s.db.SetMaxIdleConns(1)
	d, err := s.Get(ctx, "programs", "prog-1")
	if err != nil {
		t.Fatalf("lost after reconnect: %v", err)
	}
	if d["name"] != "예시" {
		t.Fatalf("got %v", d)
	}
}
