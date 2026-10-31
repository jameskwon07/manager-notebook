package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jameskwon07/manager-notebook/internal/store"
)

type askEnv struct {
	t   *testing.T
	st  *store.Store
	ask *Ask
	ts  *httptest.Server
	log *bytes.Buffer
	now time.Time
}

func newAskEnv(t *testing.T) *askEnv {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &askEnv{t: t, st: st, log: &bytes.Buffer{}, now: time.Date(2026, 11, 2, 10, 0, 0, 0, time.FixedZone("KST", 9*3600))}
	e.ask = NewAsk(AskConfig{
		Version: "2.2.0",
		Store:   st,
		Web: fstest.MapFS{
			"ask.html":    {Data: []byte("<!doctype html><title>요청 보내기</title>")},
			"ask.js":      {Data: []byte("console.log(3)")},
			"ask.css":     {Data: []byte(".ask{}")},
			"app.css":     {Data: []byte("body{}")},
			"core.js":     {Data: []byte("window.NBCore={}")},
			"favicon.svg": {Data: []byte("<svg/>")},
			"index.html":  {Data: []byte("<!doctype html><title>관리 수첩</title>")},
			"intake.html": {Data: []byte("<!doctype html><title>요청 접수함</title>")},
			"app.js":      {Data: []byte("console.log(1)")},
		},
		Logger: log.New(e.log, "", 0),
		Now:    func() time.Time { return e.now },
	})
	e.ts = httptest.NewServer(e.ask)
	t.Cleanup(e.ts.Close)
	ctx := context.Background()
	for _, p := range []store.Doc{
		{"order": 2, "name": "로그 수집기", "active": true},
		{"order": 1, "name": "주문 처리 서비스", "active": true},
		{"order": 3, "name": "쉬는 프로그램", "active": false},
	} {
		if _, err := st.Create(ctx, "programs", p); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func (e *askEnv) do(method, path, body string, hdr ...string) (*http.Response, []byte) {
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
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, b
}

func (e *askEnv) programs() []askProgram {
	e.t.Helper()
	_, b := e.do("GET", "/api/ask/form", "")
	var f struct {
		Programs []askProgram `json:"programs"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		e.t.Fatalf("form: %s", b)
	}
	return f.Programs
}

func (e *askEnv) send(body string) map[string]any {
	e.t.Helper()
	resp, b := e.do("POST", "/api/ask", body)
	if resp.StatusCode != 201 {
		e.t.Fatalf("send %s: %d %s", body, resp.StatusCode, b)
	}
	return jsonMap(e.t, b)
}

// TestAskDoorServesOnlyItsOwnPaths: the door never reaches the notebook screen or its API.
func TestAskDoorServesOnlyItsOwnPaths(t *testing.T) {
	e := newAskEnv(t)
	for p, want := range map[string]string{"/": "요청 보내기", "/r/abcdefghjk": "요청 보내기", "/ask.js": "console.log(3)", "/ask.css": ".ask{}", "/core.js": "NBCore", "/app.css": "body{}"} {
		resp, b := e.do("GET", p, "")
		if resp.StatusCode != 200 || !strings.Contains(string(b), want) {
			t.Errorf("%s: %d %q", p, resp.StatusCode, b)
		}
		if resp.Header.Get("Content-Security-Policy") != CSP || resp.Header.Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: security headers missing", p)
		}
	}
	for _, p := range []string{"/index.html", "/intake", "/intake.html", "/app.js", "/content.json", "/login", "/api/state", "/api/export", "/api/guide", "/api/calendar.ics", "/api/docs/requests", "/api/settings"} {
		resp, _ := e.do("GET", p, "")
		if resp.StatusCode != 404 {
			t.Errorf("door %s: %d, want 404", p, resp.StatusCode)
		}
	}
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		resp, _ := e.do(m, "/api/docs/requests", `{"what":"x"}`)
		if resp.StatusCode != 404 {
			t.Errorf("door %s /api/docs/requests: %d", m, resp.StatusCode)
		}
	}
	resp, _ := e.do("POST", "/api/import", `{}`)
	if resp.StatusCode != 404 {
		t.Errorf("door import: %d", resp.StatusCode)
	}
}

func TestAskFormListsActivePrograms(t *testing.T) {
	e := newAskEnv(t)
	ps := e.programs()
	if len(ps) != 2 || ps[0].Name != "주문 처리 서비스" || ps[1].Name != "로그 수집기" {
		t.Fatalf("programs %+v", ps)
	}
}

func TestAskCreateViewAndFill(t *testing.T) {
	e := newAskEnv(t)
	prog := e.programs()[1].ID
	v := e.send(fmt.Sprintf(`{"what":"  로그 내보내기 버튼  ","from":"다른 파트 개발자","why":"","when":"11/20, 고객 시연","raw":"줄1\r\n줄2\u0007","program":%q,"note":"무시"}`, prog))
	code, _ := v["code"].(string)
	if !askCodeRe.MatchString(code) || v["what"] != "로그 내보내기 버튼" || v["raw"] != "줄1\n줄2" || v["programName"] != "로그 수집기" ||
		v["status"] != "접수" || v["editable"] != true || v["receivedAt"] != "2026-11-02T01:00:00.000Z" {
		t.Fatalf("created view: %v", v)
	}
	if miss := fmt.Sprint(v["missing"]); miss != "[why check]" {
		t.Fatalf("missing %s", miss)
	}

	// 수첩 쪽에는 창구로 온 요청으로 보입니다.
	docs, _ := e.st.List(context.Background(), "requests")
	if len(docs) != 1 || docs[0]["via"] != "창구" || docs[0]["askCode"] != code || docs[0]["note"] != "" || docs[0]["status"] != "접수" || docs[0]["from"] != "다른 파트 개발자" {
		t.Fatalf("stored: %v", docs)
	}
	id := docs[0]["id"].(string)
	if _, err := e.st.Patch(context.Background(), "requests", id, store.Doc{"note": "관리자 메모", "est": map[string]any{"mid": 3}}); err != nil {
		t.Fatal(err)
	}

	resp, b := e.do("GET", "/api/ask/"+code, "")
	if resp.StatusCode != 200 || strings.Contains(string(b), "관리자 메모") || strings.Contains(string(b), "est") {
		t.Fatalf("view leaks manager fields: %d %s", resp.StatusCode, b)
	}

	// 빈칸 채우기: 빈 값은 무시하고, 덧붙임은 원문 뒤에 붙습니다.
	e.now = e.now.Add(time.Hour)
	resp, b = e.do("PATCH", "/api/ask/"+code, `{"why":"팀장 지시","check":"","when":"","more":"화면 캡처는 메일로 보냈습니다"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("fill: %d %s", resp.StatusCode, b)
	}
	v = jsonMap(t, b)
	if v["why"] != "팀장 지시" || v["when"] != "11/20, 고객 시연" || !strings.HasSuffix(v["raw"].(string), "[덧붙임 11/2 11:00] 화면 캡처는 메일로 보냈습니다") || v["addedAt"] != "2026-11-02T02:00:00.000Z" {
		t.Fatalf("filled: %v", v)
	}
	if fmt.Sprint(v["missing"]) != "[check]" {
		t.Fatalf("missing after fill: %v", v["missing"])
	}
	resp, b = e.do("PATCH", "/api/ask/"+code, `{"check":"  "}`)
	wantErr(t, resp, b, 400, "empty")
	resp, b = e.do("PATCH", "/api/ask/"+code, `{"program":"없는-id"}`)
	wantErr(t, resp, b, 400, "bad_program")

	// 리드에게 넘어가면 더는 고칠 수 없습니다.
	if _, err := e.st.Patch(context.Background(), "requests", id, store.Doc{"status": "리드 전달", "forwardedAt": "2026-11-02T03:00:00.000Z"}); err != nil {
		t.Fatal(err)
	}
	resp, b = e.do("PATCH", "/api/ask/"+code, `{"check":"화면에서 버튼 확인"}`)
	wantErr(t, resp, b, 409, "locked")
	_, b = e.do("GET", "/api/ask/"+code, "")
	if v = jsonMap(t, b); v["editable"] != false || v["status"] != "리드 전달" {
		t.Fatalf("after forward: %v", v)
	}

	// 기록(log)에는 링크 코드가 남지 않습니다.
	if strings.Contains(e.log.String(), code) || strings.Contains(e.log.String(), "로그 내보내기") {
		t.Fatalf("log has code or content:\n%s", e.log.String())
	}
	if !strings.Contains(e.log.String(), "창구 GET /api/ask/{code} 200") {
		t.Fatalf("log lacks the route shape:\n%s", e.log.String())
	}
}

func TestAskCreateValidation(t *testing.T) {
	e := newAskEnv(t)
	cases := []struct {
		body, code string
		status     int
	}{
		{`{"from":"나"}`, "need_what", 400},
		{`{"what":"  ","from":"나"}`, "need_what", 400},
		{`{"what":"무엇"}`, "need_from", 400},
		{`{"what":"무엇","from":"나","program":"nope"}`, "bad_program", 400},
		{`{"what":1,"from":"나"}`, "bad_field", 400},
		{`{"what":"` + strings.Repeat("가", 121) + `","from":"나"}`, "too_long", 400},
		{`[1]`, "bad_json", 400},
		{`{"what":"무엇","from":"나"} {}`, "bad_json", 400},
	}
	for _, c := range cases {
		resp, b := e.do("POST", "/api/ask", c.body)
		wantErr(t, resp, b, c.status, c.code)
	}
	resp, b := e.do("POST", "/api/ask", `{"what":"`+strings.Repeat("가", AskMaxBody)+`","from":"나"}`)
	wantErr(t, resp, b, 413, "too_large")
	// 다른 사이트에서 보낸 글은 받지 않습니다.
	resp, b = e.do("POST", "/api/ask", `{"what":"무엇","from":"나"}`, "Origin", "http://evil.example")
	wantErr(t, resp, b, 403, "origin")
	if docs, _ := e.st.List(context.Background(), "requests"); len(docs) != 0 {
		t.Fatalf("nothing should be stored: %v", docs)
	}
}

func TestAskLimits(t *testing.T) {
	e := newAskEnv(t)
	for i := 0; i < askLimits["miss"]; i++ {
		if resp, _ := e.do("GET", "/api/ask/zzzzzzzzzz", ""); resp.StatusCode != 404 {
			t.Fatalf("miss %d: %d", i, resp.StatusCode)
		}
	}
	resp, b := e.do("GET", "/api/ask/zzzzzzzzzz", "")
	wantErr(t, resp, b, 429, "too_many")
	resp, b = e.do("GET", "/api/ask/BAD", "")
	wantErr(t, resp, b, 429, "too_many")
	// 10분이 지나면 다시 됩니다.
	e.now = e.now.Add(askWindow)
	if resp, _ := e.do("GET", "/api/ask/zzzzzzzzzz", ""); resp.StatusCode != 404 {
		t.Fatalf("after window: %d", resp.StatusCode)
	}

	for i := 0; i < askLimits["post"]; i++ {
		e.send(`{"what":"요청","from":"나"}`)
	}
	resp, b = e.do("POST", "/api/ask", `{"what":"요청","from":"나"}`)
	wantErr(t, resp, b, 429, "too_many")

	// 하루 상한
	e.now = e.now.Add(askWindow)
	e.ask.dayMu.Lock()
	e.ask.dayCount = AskDailyCap
	e.ask.dayMu.Unlock()
	resp, b = e.do("POST", "/api/ask", `{"what":"요청","from":"나"}`)
	wantErr(t, resp, b, 429, "daily_cap")
	e.now = e.now.Add(24 * time.Hour)
	e.send(`{"what":"다음 날","from":"나"}`)
}

func TestNewAskCode(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c := newAskCode()
		if !askCodeRe.MatchString(c) || seen[c] {
			t.Fatalf("code %q", c)
		}
		seen[c] = true
	}
}
