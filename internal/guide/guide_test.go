package guide

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadAndRender(t *testing.T) {
	fsys := fstest.MapFS{
		"guide/index.json": {Data: []byte(`[{"slug":"b","title":"둘째","order":2},{"slug":"a","title":"첫째","order":1},{"slug":"../x","title":"bad","order":3},{"slug":"missing","title":"m","order":4},{"slug":"c","title":"셋째","order":5,"file":"03-c.md"},{"slug":"d","title":"bad file","order":6,"file":"../x.md"}]`)},
		"guide/03-c.md":    {Data: []byte("셋째 본문")},
		"guide/a.md":       {Data: []byte("# 제목\n\n| 칸 | 값 |\n|---|---|\n| 1 | 2 |\n\n- [x] 끝\n- [ ] 남음\n\n<script>alert(1)</script>\n")},
		"guide/b.md":       {Data: []byte("본문")},
	}
	g, warns, err := Load(fsys, "guide")
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 3 {
		t.Fatalf("warns %v", warns)
	}
	l := g.List()
	if len(l) != 3 || l[0].Slug != "a" || l[1].Slug != "b" || l[2].Slug != "c" {
		t.Fatalf("list %v", l)
	}
	p, err := g.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<table>", "<h1", `type="checkbox"`} {
		if !strings.Contains(p.HTML, want) {
			t.Errorf("missing %s in %s", want, p.HTML)
		}
	}
	if strings.Contains(p.HTML, "<script>") {
		t.Error("raw HTML rendered")
	}
	if c, err := g.Get("c"); err != nil || !strings.Contains(c.HTML, "셋째 본문") {
		t.Error("file field not used")
	}
	if _, err := g.Get("../x"); err != ErrNotFound {
		t.Error("bad slug served")
	}
	if _, err := g.Get("missing"); err != ErrNotFound {
		t.Error("missing served")
	}
}

func TestMissingOrBadIndex(t *testing.T) {
	g, _, err := Load(fstest.MapFS{}, "guide")
	if err != nil || len(g.List()) != 0 {
		t.Fatal("missing index should give empty guide")
	}
	g, w, err := Load(fstest.MapFS{"guide/index.json": {Data: []byte("PLACEHOLDER")}}, "guide")
	if err != nil || len(g.List()) != 0 || len(w) != 1 {
		t.Fatal("bad index should give empty guide with warning")
	}
}
