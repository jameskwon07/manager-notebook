package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jameskwon07/manager-notebook/internal/guide"
	"github.com/jameskwon07/manager-notebook/internal/llm"
	"github.com/jameskwon07/manager-notebook/internal/store"
)

type env struct {
	t   *testing.T
	srv *Server
	ts  *httptest.Server
	c   *http.Client
	log *bytes.Buffer
}

func newEnv(t *testing.T, mod func(*Config)) *env {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	g, _, _ := guide.Load(fstest.MapFS{
		"guide/index.json": {Data: []byte(`[{"slug":"start","title":"시작 준비","order":1}]`)},
		"guide/start.md":   {Data: []byte("# 시작\n\n| a | b |\n|---|---|\n| 1 | 2 |\n")},
	}, "guide")
	var logBuf bytes.Buffer
	cfg := Config{
		Version: "2.0.0",
		Store:   st,
		Guide:   g,
		Web: fstest.MapFS{
			"index.html":   {Data: []byte("<!doctype html><title>관리 수첩</title>")},
			"core.js":      {Data: []byte("window.NBCore={}")},
			"app.js":       {Data: []byte("console.log(1)")},
			"app.css":      {Data: []byte("body{}")},
			"content.json": {Data: []byte(`{"app":{}}`)},
			"favicon.svg":  {Data: []byte("<svg/>")},
			"intake.html":  {Data: []byte("<!doctype html><title>요청 접수함</title>")},
			"intake.js":    {Data: []byte("console.log(2)")},
			"intake.css":   {Data: []byte(".ib{}")},
		},
		Logger:    log.New(&logBuf, "", 0),
		FailDelay: 10 * time.Millisecond,
		Now:       func() time.Time { return time.Date(2026, 10, 16, 9, 0, 0, 0, time.FixedZone("KST", 9*3600)) },
	}
	if mod != nil {
		mod(&cfg)
	}
	s := New(cfg)
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	c := ts.Client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &env{t: t, srv: s, ts: ts, c: c, log: &logBuf}
}

func (e *env) do(method, path, body string, hdr ...string) (*http.Response, []byte) {
	e.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, e.ts.URL+path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := e.c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, b
}

func jsonMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not json: %s", b)
	}
	return m
}

func wantErr(t *testing.T, resp *http.Response, b []byte, status int, code string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("status %d want %d: %s", resp.StatusCode, status, b)
	}
	m := jsonMap(t, b)
	if m["error"] != code || m["message"] == "" {
		t.Fatalf("error body %s, want code %s", b, code)
	}
}

func TestHealthAndHeaders(t *testing.T) {
	e := newEnv(t, nil)
	resp, b := e.do("GET", "/api/health", "")
	if resp.StatusCode != 200 || !strings.Contains(string(b), `"version":"2.0.0"`) || !strings.Contains(string(b), `"ok":true`) {
		t.Fatalf("health %d %s", resp.StatusCode, b)
	}
	for _, p := range []string{"/api/health", "/", "/nope", "/api/nope"} {
		resp, _ := e.do("GET", p, "")
		h := resp.Header
		if h.Get("Content-Security-Policy") != CSP || h.Get("X-Content-Type-Options") != "nosniff" ||
			h.Get("Referrer-Policy") != "no-referrer" || h.Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s missing security headers: %v", p, h)
		}
		if strings.HasPrefix(p, "/api/") && h.Get("Cache-Control") != "no-store" {
			t.Errorf("%s missing no-store", p)
		}
	}
}

func TestStaticFiles(t *testing.T) {
	e := newEnv(t, nil)
	cases := map[string]string{
		"/":             "text/html; charset=utf-8",
		"/core.js":      "text/javascript; charset=utf-8",
		"/app.js":       "text/javascript; charset=utf-8",
		"/app.css":      "text/css; charset=utf-8",
		"/content.json": "application/json; charset=utf-8",
		"/favicon.svg":  "image/svg+xml",
	}
	for p, ct := range cases {
		resp, _ := e.do("GET", p, "")
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != ct {
			t.Errorf("%s: %d %s", p, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
	resp, b := e.do("GET", "/core.js", "")
	if string(b) != "window.NBCore={}" {
		t.Errorf("/core.js body %q", b)
	}
	resp, _ = e.do("GET", "/../go.mod", "")
	if resp.StatusCode == 200 {
		t.Error("path escape served")
	}
	resp, _ = e.do("GET", "/login", "")
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("/login without password should redirect, got %d", resp.StatusCode)
	}
}

func TestCRUDAndCollWhitelist(t *testing.T) {
	e := newEnv(t, nil)
	resp, b := e.do("POST", "/api/docs/notes", `{"kind":"q","text":"메모","done":false,"n":12345678901234567890}`)
	if resp.StatusCode != 201 {
		t.Fatalf("create %d %s", resp.StatusCode, b)
	}
	if !strings.Contains(string(b), "12345678901234567890") {
		t.Errorf("big number not preserved: %s", b)
	}
	id := jsonMap(t, b)["id"].(string)

	resp, b = e.do("PATCH", "/api/docs/notes/"+id, `{"done":true,"kind":null}`)
	if resp.StatusCode != 200 {
		t.Fatalf("patch %d %s", resp.StatusCode, b)
	}
	m := jsonMap(t, b)
	if _, ok := m["kind"]; ok || m["done"] != true || m["id"] != id {
		t.Fatalf("patch result %s", b)
	}
	resp, b = e.do("PATCH", "/api/docs/notes/missing", `{"a":1}`)
	wantErr(t, resp, b, 404, "not_found")

	resp, b = e.do("PUT", "/api/docs/fridays/2026-10-26", `{"week":"2026-10-26"}`)
	if resp.StatusCode != 200 || jsonMap(t, b)["id"] != "2026-10-26" {
		t.Fatalf("put %d %s", resp.StatusCode, b)
	}
	resp, b = e.do("PUT", "/api/docs/fridays/bad%20id", `{}`)
	wantErr(t, resp, b, 400, "bad_id")

	resp, b = e.do("GET", "/api/state", "")
	st := jsonMap(t, b)
	if len(st["notes"].([]any)) != 1 || len(st["fridays"].([]any)) != 1 || st["settings"] != nil {
		t.Fatalf("state %s", b)
	}
	if l := st["llm"].(map[string]any); l["enabled"] != false {
		t.Fatalf("llm should be off: %s", b)
	}
	if st["auth"] != false {
		t.Fatalf("auth should be false without password: %s", b)
	}

	resp, _ = e.do("DELETE", "/api/docs/notes/"+id, "")
	if resp.StatusCode != 204 {
		t.Fatalf("delete %d", resp.StatusCode)
	}

	for _, p := range []string{"/api/docs/settings", "/api/docs/meta", "/api/docs/docs"} {
		resp, b = e.do("POST", p, `{}`)
		wantErr(t, resp, b, 404, "not_found")
	}
	resp, b = e.do("PUT", "/api/docs/settings/main", `{}`)
	wantErr(t, resp, b, 404, "not_found")

	resp, b = e.do("POST", "/api/docs/notes", `[1,2]`)
	wantErr(t, resp, b, 400, "bad_json")
	resp, b = e.do("POST", "/api/docs/notes", `{"a":1} {"b":2}`)
	wantErr(t, resp, b, 400, "bad_json")
	resp, b = e.do("POST", "/api/docs/notes", `null`)
	wantErr(t, resp, b, 400, "bad_json")
	resp, b = e.do("GET", "/api/docs/notes", "")
	wantErr(t, resp, b, 405, "method")
}

func TestSizeLimits(t *testing.T) {
	e := newEnv(t, nil)
	big := `{"text":"` + strings.Repeat("a", MaxBody) + `"}`
	resp, b := e.do("POST", "/api/docs/notes", big)
	wantErr(t, resp, b, 413, "too_large")

	doc := `{"text":"` + strings.Repeat("a", 300<<10) + `"}`
	resp, b = e.do("POST", "/api/docs/notes", doc)
	wantErr(t, resp, b, 413, "too_large")

	imp := `{"format":"manager-notebook-export","notes":[{"t":"` + strings.Repeat("a", MaxImportBody) + `"}]}`
	resp, b = e.do("POST", "/api/import", imp)
	wantErr(t, resp, b, 413, "too_large")

	// Import accepts more than 1MB in total.
	var notes []string
	for i := 0; i < 8; i++ {
		notes = append(notes, `{"text":"`+strings.Repeat("b", 200<<10)+`"}`)
	}
	resp, b = e.do("POST", "/api/import", `{"format":"manager-notebook-export","notes":[`+strings.Join(notes, ",")+`]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("1.6MB import %d %s", resp.StatusCode, b)
	}
}

func TestSettingsExportImport(t *testing.T) {
	e := newEnv(t, nil)
	resp, b := e.do("PUT", "/api/settings", `{"startDate":"2026-10-19","cycleWeeks":6}`)
	if resp.StatusCode != 200 || jsonMap(t, b)["startDate"] != "2026-10-19" {
		t.Fatalf("settings %d %s", resp.StatusCode, b)
	}
	e.do("POST", "/api/docs/notes", `{"text":"one"}`)
	e.do("POST", "/api/docs/blockers", `{"text":"two"}`)
	e.do("POST", "/api/docs/requests", `{"what":"req","size":"S","status":"접수"}`)
	e.do("PUT", "/api/docs/weeks/2026-10-26", `{"signals":{"reviewEmpty":true},"note":""}`)

	resp, exp := e.do("GET", "/api/export", "")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Disposition") != `attachment; filename="manager-notebook-20261016.json"` {
		t.Fatalf("export %d %v", resp.StatusCode, resp.Header)
	}
	m := jsonMap(t, exp)
	if m["format"] != "manager-notebook-export" || m["version"] != "2.0.0" || m["exportedAt"] != "2026-10-16T09:00:00+09:00" {
		t.Fatalf("export body %s", exp)
	}
	if len(m) != 4+len(store.Collections) {
		t.Fatalf("export keys %d: %s", len(m), exp)
	}
	for _, c := range store.Collections {
		if _, ok := m[c].([]any); !ok {
			t.Fatalf("export %s missing: %s", c, exp)
		}
	}
	if !strings.HasPrefix(string(exp), `{"format":"manager-notebook-export","version":"2.0.0","exportedAt":`) {
		t.Fatalf("export key order: %.80s", exp)
	}

	e.do("POST", "/api/docs/notes", `{"text":"after export"}`)
	e.do("POST", "/api/docs/risks", `{"code":"R1","title":"after export"}`)
	resp, b = e.do("POST", "/api/import", string(exp))
	want := `{"replaced":["notes","blockers","fridays","requests","programs","decisions","mapitems","timelog","weeks","cycles","risks"],` +
		`"counts":{"blockers":1,"cycles":0,"decisions":0,"fridays":0,"mapitems":0,"notes":1,"programs":0,"requests":1,"risks":0,"timelog":0,"weeks":1},"settings":true}`
	if resp.StatusCode != 200 || string(bytes.TrimSpace(b)) != want {
		t.Fatalf("import %d %s", resp.StatusCode, b)
	}
	_, b = e.do("GET", "/api/state", "")
	if strings.Contains(string(b), "after export") || !strings.Contains(string(b), `"cycleWeeks":6`) {
		t.Fatalf("import did not replace: %s", b)
	}

	// A 1.0 export replaces only notes, blockers, fridays and settings.
	e.do("POST", "/api/docs/programs", `{"order":1,"name":"프로그램 1","active":true}`)
	v1 := `{"format":"manager-notebook-export","version":"1.0.0","exportedAt":"2026-10-16T09:00:00+09:00",` +
		`"settings":{"startDate":"2026-11-02"},"notes":[{"id":"n1","kind":"q","text":"1.0 메모"}],"blockers":[],"fridays":[]}`
	resp, b = e.do("POST", "/api/import", v1)
	if resp.StatusCode != 200 || string(bytes.TrimSpace(b)) !=
		`{"replaced":["notes","blockers","fridays"],"counts":{"blockers":0,"fridays":0,"notes":1},"settings":true}` {
		t.Fatalf("1.0 import %d %s", resp.StatusCode, b)
	}
	_, b = e.do("GET", "/api/state", "")
	st := jsonMap(t, b)
	if len(st["programs"].([]any)) != 1 || len(st["requests"].([]any)) != 1 || len(st["weeks"].([]any)) != 1 ||
		len(st["blockers"].([]any)) != 0 || st["settings"].(map[string]any)["startDate"] != "2026-11-02" {
		t.Fatalf("state after 1.0 import: %s", b)
	}

	// settings null and missing keys keep everything else.
	resp, b = e.do("POST", "/api/import", `{"format":"manager-notebook-export","settings":null,"decisions":[]}`)
	if resp.StatusCode != 200 || string(bytes.TrimSpace(b)) != `{"replaced":["decisions"],"counts":{"decisions":0},"settings":false}` {
		t.Fatalf("partial import %d %s", resp.StatusCode, b)
	}
	_, b = e.do("GET", "/api/state", "")
	if !strings.Contains(string(b), `"startDate":"2026-11-02"`) || !strings.Contains(string(b), "1.0 메모") {
		t.Fatalf("partial import changed other data: %s", b)
	}

	for _, bad := range []string{`{"format":"other"}`, `{}`, `null`, `{"format":"manager-notebook-export","notes":{}}`} {
		resp, b = e.do("POST", "/api/import", bad)
		wantErr(t, resp, b, 400, "bad_format")
	}
	resp, b = e.do("POST", "/api/import", `[1,2]`)
	wantErr(t, resp, b, 400, "bad_json")
	resp, b = e.do("POST", "/api/import", `{"format":`)
	wantErr(t, resp, b, 400, "bad_json")
}

// TestStateHasAllCollections: /api/state always carries the 11 collection
// keys, empty lists included.
func TestStateHasAllCollections(t *testing.T) {
	e := newEnv(t, nil)
	_, b := e.do("GET", "/api/state", "")
	st := jsonMap(t, b)
	want := []string{"notes", "blockers", "fridays", "requests", "programs", "decisions", "mapitems", "timelog", "weeks", "cycles", "risks"}
	for _, c := range want {
		l, ok := st[c].([]any)
		if !ok || len(l) != 0 {
			t.Fatalf("state %s: %s", c, b)
		}
	}
	for _, k := range []string{"version", "settings", "llm", "auth", "demo", "only", "ask"} {
		if _, ok := st[k]; !ok {
			t.Fatalf("state lacks %s: %s", k, b)
		}
	}
	if len(st) != len(want)+7 || st["version"] != "2.0.0" || st["settings"] != nil || st["demo"] != false || st["only"] != "" {
		t.Fatalf("state keys: %s", b)
	}
	if ask, _ := st["ask"].(map[string]any); ask["on"] != false || len(ask["urls"].([]any)) != 0 {
		t.Fatalf("state ask: %s", b)
	}
}

// TestNewCollectionsCRUD: POST, PUT, PATCH and DELETE work on every 2.0
// collection; other names are 404.
func TestNewCollectionsCRUD(t *testing.T) {
	e := newEnv(t, nil)
	for _, c := range []string{"requests", "programs", "decisions", "mapitems", "timelog", "weeks", "cycles", "risks"} {
		resp, b := e.do("POST", "/api/docs/"+c, `{"text":"새 기록","n":1}`)
		if resp.StatusCode != 201 {
			t.Fatalf("%s create %d %s", c, resp.StatusCode, b)
		}
		id := jsonMap(t, b)["id"].(string)
		resp, b = e.do("PATCH", "/api/docs/"+c+"/"+id, `{"n":null,"done":true}`)
		m := jsonMap(t, b)
		if _, ok := m["n"]; resp.StatusCode != 200 || ok || m["done"] != true || m["text"] != "새 기록" {
			t.Fatalf("%s patch %d %s", c, resp.StatusCode, b)
		}
		resp, b = e.do("PUT", "/api/docs/"+c+"/2026-10-26", `{"checks":{"0":true}}`)
		if resp.StatusCode != 200 || jsonMap(t, b)["id"] != "2026-10-26" {
			t.Fatalf("%s put %d %s", c, resp.StatusCode, b)
		}
		_, b = e.do("GET", "/api/state", "")
		if l := jsonMap(t, b)[c].([]any); len(l) != 2 {
			t.Fatalf("%s state %s", c, b)
		}
		resp, _ = e.do("DELETE", "/api/docs/"+c+"/"+id, "")
		if resp.StatusCode != 204 {
			t.Fatalf("%s delete %d", c, resp.StatusCode)
		}
		_, b = e.do("GET", "/api/state", "")
		if l := jsonMap(t, b)[c].([]any); len(l) != 1 {
			t.Fatalf("%s after delete %s", c, b)
		}
	}
	for _, p := range []string{"/api/docs/tasks", "/api/docs/request", "/api/docs/Programs", "/api/docs/people"} {
		resp, b := e.do("POST", p, `{}`)
		wantErr(t, resp, b, 404, "not_found")
		resp, b = e.do("PATCH", p+"/x", `{}`)
		wantErr(t, resp, b, 404, "not_found")
		resp, b = e.do("DELETE", p+"/x", "")
		wantErr(t, resp, b, 404, "not_found")
	}
}

func TestCalendarAndGuide(t *testing.T) {
	e := newEnv(t, nil)
	resp, b := e.do("GET", "/api/calendar.ics", "")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/calendar; charset=utf-8" ||
		resp.Header.Get("Content-Disposition") != `attachment; filename="manager-notebook.ics"` {
		t.Fatalf("ics %d %v", resp.StatusCode, resp.Header)
	}
	if strings.Count(string(b), "BEGIN:VEVENT") != 30 || !strings.Contains(string(b), "EXDATE;TZID=Asia/Seoul:20261225T135500,20270101T135500") {
		t.Fatalf("ics body: %d events", strings.Count(string(b), "BEGIN:VEVENT"))
	}
	e.do("PUT", "/api/settings", `{"startDate":"2026-11-02","holidays":""}`)
	_, b = e.do("GET", "/api/calendar.ics", "")
	if !strings.Contains(string(b), "UID:friday-read-2026-11-02-v2@manager-notebook") || strings.Contains(string(b), "EXDATE") {
		t.Fatal("ics ignores settings")
	}

	resp, b = e.do("GET", "/api/guide", "")
	if resp.StatusCode != 200 || string(bytes.TrimSpace(b)) != `[{"slug":"start","title":"시작 준비","order":1}]` {
		t.Fatalf("guide list %s", b)
	}
	resp, b = e.do("GET", "/api/guide/start", "")
	if resp.StatusCode != 200 || !strings.Contains(jsonMap(t, b)["html"].(string), "<table>") {
		t.Fatalf("guide page %s", b)
	}
	resp, b = e.do("GET", "/api/guide/nope", "")
	wantErr(t, resp, b, 404, "not_found")
}

func TestLLMDisabledAndEnabled(t *testing.T) {
	e := newEnv(t, nil)
	resp, b := e.do("POST", "/api/llm", `{"messages":[{"role":"user","content":"hi"}]}`)
	wantErr(t, resp, b, 404, "llm_disabled")

	var status = 200
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(400)
			return
		}
		if status != 200 {
			w.WriteHeader(status)
			w.Write([]byte(`{"error":{"message":"SECRET-UPSTREAM-DETAIL"}}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"team\":[]}"}}]}`))
	}))
	defer fake.Close()
	e = newEnv(t, func(c *Config) {
		c.LLM = llm.New(llm.Config{BaseURL: fake.URL + "/v1", APIKey: "k", Model: "m1", Timeout: 2 * time.Second})
	})
	_, b = e.do("GET", "/api/state", "")
	if !strings.Contains(string(b), `"llm":{"enabled":true,"model":"m1"}`) {
		t.Fatalf("state llm %s", b)
	}
	resp, b = e.do("POST", "/api/llm", `{"messages":[{"role":"system","content":"s"},{"role":"user","content":"u"}]}`)
	if resp.StatusCode != 200 || jsonMap(t, b)["text"] != `{"team":[]}` {
		t.Fatalf("llm %d %s", resp.StatusCode, b)
	}
	resp, b = e.do("POST", "/api/llm", `{"messages":[{"role":"tool","content":"u"}]}`)
	wantErr(t, resp, b, 400, "bad_request")
	resp, b = e.do("POST", "/api/llm", `{"messages":[{"role":"user","content":5}]}`)
	wantErr(t, resp, b, 400, "bad_request")

	status = 500
	resp, b = e.do("POST", "/api/llm", `{"messages":[{"role":"user","content":"SECRET-PROMPT"}]}`)
	wantErr(t, resp, b, 502, "upstream")
	if strings.Contains(string(b), "SECRET-UPSTREAM-DETAIL") || !strings.Contains(string(b), "500") {
		t.Fatalf("upstream body leaked or status missing: %s", b)
	}
	if strings.Contains(e.log.String(), "SECRET") {
		t.Fatalf("log contains content: %s", e.log.String())
	}
}

func TestLLMTimeout(t *testing.T) {
	done := make(chan struct{})
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-done:
		}
	}))
	defer fake.Close()
	defer close(done)
	e := newEnv(t, func(c *Config) {
		c.LLM = llm.New(llm.Config{BaseURL: fake.URL, Timeout: 100 * time.Millisecond})
	})
	resp, b := e.do("POST", "/api/llm", `{"messages":[{"role":"user","content":"u"}]}`)
	wantErr(t, resp, b, 504, "timeout")
}

func TestAuth(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.Password = "pw-123" })

	// Open paths.
	for _, p := range []string{"/api/health", "/", "/core.js", "/app.js", "/app.css", "/content.json", "/favicon.svg", "/login", "/login.css"} {
		resp, _ := e.do("GET", p, "")
		if resp.StatusCode != 200 {
			t.Errorf("%s should be open, got %d", p, resp.StatusCode)
		}
	}
	// Closed paths.
	for _, p := range []string{"/api/state", "/api/export", "/api/calendar.ics", "/api/guide", "/api/guide/start", "/api/nope"} {
		resp, b := e.do("GET", p, "")
		wantErr(t, resp, b, 401, "auth")
	}
	resp, b := e.do("POST", "/api/docs/notes", `{"a":1}`)
	wantErr(t, resp, b, 401, "auth")

	start := time.Now()
	resp, b = e.do("POST", "/api/login", `{"password":"wrong"}`)
	wantErr(t, resp, b, 401, "auth")
	if time.Since(start) < 10*time.Millisecond {
		t.Error("no delay after wrong password")
	}

	resp, _ = e.do("POST", "/api/login", `{"password":"pw-123"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("login %d", resp.StatusCode)
	}
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == CookieName {
			cookie = c
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != 12*3600 {
		t.Fatalf("cookie %+v", cookie)
	}
	ck := CookieName + "=" + cookie.Value
	resp, sb := e.do("GET", "/api/state", "", "Cookie", ck)
	if resp.StatusCode != 200 {
		t.Fatalf("state with cookie %d", resp.StatusCode)
	}
	if jsonMap(t, sb)["auth"] != true {
		t.Fatalf("auth should be true with password: %s", sb)
	}
	resp, _ = e.do("GET", "/api/state", "", "Cookie", CookieName+"=forged")
	if resp.StatusCode != 401 {
		t.Fatal("forged cookie accepted")
	}

	resp, _ = e.do("POST", "/api/logout", "", "Cookie", ck)
	if resp.StatusCode != 200 {
		t.Fatalf("logout %d", resp.StatusCode)
	}
	resp, _ = e.do("GET", "/api/state", "", "Cookie", ck)
	if resp.StatusCode != 401 {
		t.Fatal("session survived logout")
	}

	// Form login (no JavaScript).
	req, _ := http.NewRequest("POST", e.ts.URL+"/api/login", strings.NewReader(url.Values{"password": {"pw-123"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ = e.c.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" || len(resp.Cookies()) == 0 {
		t.Fatalf("form login %d %v", resp.StatusCode, resp.Header)
	}
	req, _ = http.NewRequest("POST", e.ts.URL+"/api/login", strings.NewReader("password=no"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ = e.c.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login?fail=1" {
		t.Fatalf("form login fail %d %v", resp.StatusCode, resp.Header)
	}
	// /login serves the front end's index.html, which draws its own form.
	_, b = e.do("GET", "/login", "")
	if !strings.Contains(string(b), "<title>관리 수첩</title>") {
		t.Fatalf("login page should be index.html: %s", b)
	}
}

func TestBuiltinLoginPage(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.Password = "p"; c.Web = fstest.MapFS{} })
	resp, b := e.do("GET", "/login?fail=1", "")
	if resp.StatusCode != 200 || !strings.Contains(string(b), "비밀번호가 맞지 않습니다") ||
		strings.Contains(string(b), "<script") || strings.Contains(string(b), "style=") {
		t.Fatalf("login page %s", b)
	}
	resp, _ = e.do("GET", "/login.css", "")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/css; charset=utf-8" {
		t.Fatalf("login.css %d", resp.StatusCode)
	}
	resp, b = e.do("GET", "/", "")
	wantErr(t, resp, b, 404, "not_found")
}

func TestSessionExpiry(t *testing.T) {
	now := time.Now()
	e := newEnv(t, func(c *Config) {
		c.Password = "p"
		c.Now = func() time.Time { return now }
	})
	resp, _ := e.do("POST", "/api/login", `{"password":"p"}`)
	ck := CookieName + "=" + resp.Cookies()[0].Value
	now = now.Add(SessionTTL + time.Second)
	resp, _ = e.do("GET", "/api/state", "", "Cookie", ck)
	if resp.StatusCode != 401 {
		t.Fatal("expired session accepted")
	}
}

func TestLoginWithoutPassword(t *testing.T) {
	e := newEnv(t, nil)
	resp, b := e.do("POST", "/api/login", `{"password":"x"}`)
	wantErr(t, resp, b, 404, "not_found")
}

func TestOriginAndHostChecks(t *testing.T) {
	e := newEnv(t, nil)
	resp, b := e.do("POST", "/api/docs/notes", `{"a":1}`, "Origin", "http://evil.example")
	wantErr(t, resp, b, 403, "origin")
	resp, b = e.do("POST", "/api/import", `{"format":"manager-notebook-export"}`, "Origin", "http://localhost:3000")
	wantErr(t, resp, b, 403, "origin")
	resp, b = e.do("DELETE", "/api/docs/notes/x", "", "Sec-Fetch-Site", "cross-site")
	wantErr(t, resp, b, 403, "origin")
	resp, _ = e.do("POST", "/api/docs/notes", `{"a":1}`, "Origin", e.ts.URL)
	if resp.StatusCode != 201 {
		t.Fatalf("same origin rejected: %d", resp.StatusCode)
	}

	// DNS rebinding: a foreign Host is refused when there is no password.
	req, _ := http.NewRequest("GET", e.ts.URL+"/api/state", nil)
	req.Host = "attacker.example:8080"
	resp, _ = e.c.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("foreign host allowed: %d", resp.StatusCode)
	}
	for _, h := range []string{"localhost:8080", "127.0.0.1:18080", "[::1]:8080", "app.localhost"} {
		if !loopbackHost(h) {
			t.Errorf("%s should be allowed", h)
		}
	}

	// With a password any host may be used (login protects it).
	e2 := newEnv(t, func(c *Config) { c.Password = "p" })
	req, _ = http.NewRequest("GET", e2.ts.URL+"/api/health", nil)
	req.Host = "notebook.team.example"
	resp, _ = e2.c.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("host with password: %d", resp.StatusCode)
	}
}

func TestLogHasNoContent(t *testing.T) {
	e := newEnv(t, nil)
	e.do("POST", "/api/docs/notes?q=SECRET-QUERY", `{"text":"SECRET-NOTE"}`)
	l := e.log.String()
	if strings.Contains(l, "SECRET") || !strings.Contains(l, "POST /api/docs/notes 201") {
		t.Fatalf("log: %s", l)
	}
}

// TestHealthNotLogged: the Docker HEALTHCHECK hits /api/health every 30s;
// a 200 there must not fill the log. Other requests are still logged.
func TestHealthNotLogged(t *testing.T) {
	e := newEnv(t, nil)
	e.do("GET", "/api/health", "")
	if strings.Contains(e.log.String(), "/api/health") {
		t.Fatalf("health 200 logged: %s", e.log.String())
	}
	e.do("GET", "/api/state", "")
	if !strings.Contains(e.log.String(), "GET /api/state 200") {
		t.Fatalf("state not logged: %s", e.log.String())
	}
}

// TestLLMConnectError: a connect failure gives a cause-specific message and a
// log line with the cause code and host only.
func TestLLMConnectError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens now → connection refused
	e := newEnv(t, func(c *Config) {
		c.LLM = llm.New(llm.Config{BaseURL: "http://" + addr + "/v1", APIKey: "SECRETKEY", Timeout: 2 * time.Second})
	})
	resp, b := e.do("POST", "/api/llm", `{"messages":[{"role":"user","content":"SECRET-PROMPT"}]}`)
	wantErr(t, resp, b, 502, "upstream")
	if !strings.Contains(string(b), "연결 거부") {
		t.Fatalf("message should name the cause: %s", b)
	}
	lg := e.log.String()
	if !strings.Contains(lg, "llm connect error: refused ("+addr+")") || strings.Contains(lg, "SECRET") {
		t.Fatalf("log: %s", lg)
	}
}

// TestHostErrorMessage: the 403 host message points to localhost with the
// same port and no longer suggests opening the notebook to other PCs.
func TestHostErrorMessage(t *testing.T) {
	e := newEnv(t, nil)
	req, _ := http.NewRequest("GET", e.ts.URL+"/api/state", nil)
	req.Host = "mypc:18080"
	resp, err := e.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	wantErr(t, resp, b, 403, "host")
	if !strings.Contains(string(b), "http://localhost:18080") || strings.Contains(string(b), "NOTEBOOK_PASSWORD") {
		t.Fatalf("host message: %s", b)
	}
}

// TestIntakePage: the one-feature page is served at /intake, and at / when Only is "intake".
func TestIntakePage(t *testing.T) {
	e := newEnv(t, nil)
	for p, want := range map[string]string{"/intake": "요청 접수함", "/intake.js": "console.log(2)", "/intake.css": ".ib{}", "/": "관리 수첩"} {
		resp, b := e.do("GET", p, "")
		if resp.StatusCode != 200 || !strings.Contains(string(b), want) {
			t.Errorf("%s: %d %q", p, resp.StatusCode, b)
		}
	}
	resp, _ := e.do("GET", "/intake.js", "")
	if ct := resp.Header.Get("Content-Type"); ct != "text/javascript; charset=utf-8" {
		t.Errorf("intake.js content-type %q", ct)
	}

	only := newEnv(t, func(c *Config) { c.Only = "intake" })
	resp, b := only.do("GET", "/", "")
	if resp.StatusCode != 200 || !strings.Contains(string(b), "요청 접수함") {
		t.Fatalf("only=intake /: %d %q", resp.StatusCode, b)
	}
	_, b = only.do("GET", "/api/state", "")
	if st := jsonMap(t, b); st["only"] != "intake" {
		t.Fatalf("state only: %s", b)
	}
}
