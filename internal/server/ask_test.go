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
	"sort"
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

	// 빈칸 채우기: 빈 값은 무시하고, 덧붙임은 원문(raw)이 아니라 adds에 따로 쌓입니다.
	e.now = e.now.Add(time.Hour)
	resp, b = e.do("PATCH", "/api/ask/"+code, `{"why":"팀장 지시","check":"","when":"","more":"화면 캡처는 메일로 보냈습니다"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("fill: %d %s", resp.StatusCode, b)
	}
	v = jsonMap(t, b)
	adds, _ := v["adds"].([]any)
	if v["why"] != "팀장 지시" || v["when"] != "11/20, 고객 시연" || v["raw"] != "줄1\n줄2" || len(adds) != 1 ||
		adds[0].(map[string]any)["text"] != "화면 캡처는 메일로 보냈습니다" || v["addedAt"] != "2026-11-02T02:00:00.000Z" || v["filledAt"] != "2026-11-02T02:00:00.000Z" {
		t.Fatalf("filled: %v", v)
	}
	if fmt.Sprint(v["missing"]) != "[check]" {
		t.Fatalf("missing after fill: %v", v["missing"])
	}
	resp, b = e.do("PATCH", "/api/ask/"+code, `{"check":"  "}`)
	wantErr(t, resp, b, 400, "empty")
	resp, b = e.do("PATCH", "/api/ask/"+code, `{"program":"없는-id"}`)
	wantErr(t, resp, b, 400, "bad_program")
	// 이미 적힌 칸은 요청자가 덮어쓰지 못합니다(담당자가 고친 것을 지키려고).
	resp, b = e.do("PATCH", "/api/ask/"+code, `{"why":"다른 이유"}`)
	wantErr(t, resp, b, 400, "empty")

	// 담당자가 오래된 사본으로 raw를 저장해도 요청자의 덧붙임은 남습니다.
	if _, err := e.st.Patch(context.Background(), "requests", id, store.Doc{"raw": "담당자가 고친 원문"}); err != nil {
		t.Fatal(err)
	}
	_, b = e.do("GET", "/api/ask/"+code, "")
	if v = jsonMap(t, b); len(v["adds"].([]any)) != 1 {
		t.Fatalf("adds lost after manager raw edit: %v", v)
	}

	// 리드에게 넘어가면 빈칸은 잠기고, 덧붙임은 계속 됩니다.
	if _, err := e.st.Patch(context.Background(), "requests", id, store.Doc{"status": "리드 전달", "forwardedAt": "2026-11-02T03:00:00.000Z"}); err != nil {
		t.Fatal(err)
	}
	resp, b = e.do("PATCH", "/api/ask/"+code, `{"check":"화면에서 버튼 확인"}`)
	wantErr(t, resp, b, 409, "locked")
	resp, b = e.do("PATCH", "/api/ask/"+code, `{"more":"리허설이 11/18로 당겨졌습니다"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("note after forward: %d %s", resp.StatusCode, b)
	}
	if v = jsonMap(t, b); v["editable"] != false || v["status"] != "리드 전달" || len(v["adds"].([]any)) != 2 || v["canConfirm"] != true {
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

// TestAskPublicViewAllowList: what a requester sees is an explicit list; the
// receiver's own fields never leak, whatever the notebook stores.
func TestAskPublicViewAllowList(t *testing.T) {
	e := newAskEnv(t)
	v := e.send(`{"what":"요청","from":"나"}`)
	code := v["code"].(string)
	docs, _ := e.st.List(context.Background(), "requests")
	id := docs[0]["id"].(string)
	long := strings.Repeat("가", askReplyMax+20)
	if _, err := e.st.Patch(context.Background(), "requests", id, store.Doc{
		"note": "SECRET-메모", "est": map[string]any{"min": 1}, "size": "L", "urgent": false, "absorbed": true, "absorbedAt": "x",
		"reply": long, "replyAt": "2026-11-02T03:00:00.000Z", "eta": "2026-11-20", "etaAt": "2026-11-02T03:00:00.000Z",
		"seenAt": "2026-11-02T03:00:00.000Z", "createdAt": "x", "extra": "SECRET-기타",
	}); err != nil {
		t.Fatal(err)
	}
	resp, b := e.do("GET", "/api/ask/"+code, "")
	if resp.StatusCode != 200 || strings.Contains(string(b), "SECRET") {
		t.Fatalf("leak: %d %s", resp.StatusCode, b)
	}
	v = jsonMap(t, b)
	var keys []string
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := "addedAt adds askedAt canAdd canConfirm canWithdraw check closedAt code doneNoAt doneOkAt editable eta etaAt filledAt forwardedAt from heldAt missing program programName raw receivedAt reopenedAt reply replyAt status statusAt what when why withdrawnAt"
	if got := strings.Join(keys, " "); got != want {
		t.Fatalf("view keys\n got %s\nwant %s", got, want)
	}
	if n := len([]rune(v["reply"].(string))); n != askReplyMax {
		t.Fatalf("reply not clipped: %d", n)
	}
	if v["eta"] != "2026-11-20" {
		t.Fatalf("eta: %v", v["eta"])
	}
	e.st.Patch(context.Background(), "requests", id, store.Doc{"eta": "다음 주쯤"})
	if _, b = e.do("GET", "/api/ask/"+code, ""); jsonMap(t, b)["eta"] != "" {
		t.Fatalf("bad eta passed through: %s", b)
	}
	// 직접 적은 요청(창구로 온 것이 아님)의 원문은 보이지 않습니다.
	e.st.Patch(context.Background(), "requests", id, store.Doc{"via": nil, "raw": "메신저에서 옮긴 남의 말"})
	if _, b = e.do("GET", "/api/ask/"+code, ""); strings.Contains(string(b), "남의 말") {
		t.Fatalf("typed-in raw shown: %s", b)
	}
}

func TestAskFormSettings(t *testing.T) {
	e := newAskEnv(t)
	_, b := e.do("GET", "/api/ask/form", "")
	f := jsonMap(t, b)
	if f["holidays"] != nil || f["owner"] != "" || f["notice"] != "" {
		t.Fatalf("defaults: %v", f)
	}
	if _, err := e.st.PutSettings(context.Background(), store.Doc{"askOwner": "  홍 파트 리드 ", "askNotice": "11/3~11/5 휴가입니다", "holidays": "2026-11-03 SECRET-건강검진\n2026-12-25 성탄절",
		"people": []any{map[string]any{"id": "p1", "label": "SECRET-사람"}}, "requesterRoles": []any{"SECRET-역할"}}); err != nil {
		t.Fatal(err)
	}
	_, b = e.do("GET", "/api/ask/form", "")
	if strings.Contains(string(b), "SECRET") {
		t.Fatalf("settings leak: %s", b)
	}
	f = jsonMap(t, b)
	if f["holidays"] != "2026-11-03\n2026-12-25" || f["owner"] != "홍 파트 리드" || f["notice"] != "11/3~11/5 휴가입니다" {
		t.Fatalf("form: %v", f)
	}
}

func TestAskWithdraw(t *testing.T) {
	e := newAskEnv(t)
	code := e.send(`{"what":"요청","from":"나"}`)["code"].(string)
	docs, _ := e.st.List(context.Background(), "requests")
	id := docs[0]["id"].(string)

	e.now = e.now.Add(time.Hour)
	resp, b := e.do("POST", "/api/ask/"+code+"/withdraw", `{"text":"다른 방법으로 해결했습니다"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("withdraw: %d %s", resp.StatusCode, b)
	}
	v := jsonMap(t, b)
	if v["withdrawnAt"] != "2026-11-02T02:00:00.000Z" || v["status"] != "접수" || v["editable"] != false || v["canWithdraw"] != false || v["canAdd"] != false {
		t.Fatalf("withdrawn view: %v", v)
	}
	// 창구는 status를 쓰지 않습니다.
	got, _ := e.st.Get(context.Background(), "requests", id)
	if got["status"] != "접수" || got["closedAt"] != nil || got["withdrawnAt"] != "2026-11-02T02:00:00.000Z" {
		t.Fatalf("stored after withdraw: %v", got)
	}
	if adds := askAdds(got); len(adds) != 1 || adds[0]["kind"] != "cancel" {
		t.Fatalf("cancel note: %v", adds)
	}
	// 두 번 눌러도 한 번입니다.
	e.now = e.now.Add(time.Hour)
	if resp, _ := e.do("POST", "/api/ask/"+code+"/withdraw", `{}`); resp.StatusCode != 200 {
		t.Fatalf("second withdraw: %d", resp.StatusCode)
	}
	if got, _ := e.st.Get(context.Background(), "requests", id); got["withdrawnAt"] != "2026-11-02T02:00:00.000Z" {
		t.Fatalf("withdrawnAt moved: %v", got["withdrawnAt"])
	}
	resp, b = e.do("PATCH", "/api/ask/"+code, `{"more":"x"}`)
	wantErr(t, resp, b, 409, "withdrawn")

	// 끝난 요청은 취소할 것이 없습니다.
	code2 := e.send(`{"what":"요청 2","from":"나"}`)["code"].(string)
	docs, _ = e.st.List(context.Background(), "requests")
	for _, d := range docs {
		if d["askCode"] == code2 {
			e.st.Patch(context.Background(), "requests", d["id"].(string), store.Doc{"status": "완료", "closedAt": "x"})
		}
	}
	resp, b = e.do("POST", "/api/ask/"+code2+"/withdraw", `{}`)
	wantErr(t, resp, b, 409, "closed")
	resp, b = e.do("POST", "/api/ask/zzzzzzzzzz/withdraw", `{}`)
	wantErr(t, resp, b, 404, "not_found")
	if !strings.Contains(e.log.String(), "창구 POST /api/ask/{code}/withdraw 200") || strings.Contains(e.log.String(), code) {
		t.Fatalf("log:\n%s", e.log.String())
	}
}

func TestAskDone(t *testing.T) {
	e := newAskEnv(t)
	code := e.send(`{"what":"요청","from":"나"}`)["code"].(string)
	docs, _ := e.st.List(context.Background(), "requests")
	id := docs[0]["id"].(string)

	// 리드에게 넘어가기 전에는 끝남을 알릴 수 없습니다.
	resp, b := e.do("POST", "/api/ask/"+code+"/done", `{"ok":true}`)
	wantErr(t, resp, b, 409, "not_yet")
	e.st.Patch(context.Background(), "requests", id, store.Doc{"status": "리드 전달", "forwardedAt": "2026-11-02T03:00:00.000Z"})

	resp, b = e.do("POST", "/api/ask/"+code+"/done", `{"ok":false}`)
	wantErr(t, resp, b, 400, "need_text")
	resp, b = e.do("POST", "/api/ask/"+code+"/done", `{"ok":"yes"}`)
	wantErr(t, resp, b, 400, "bad_field")
	e.now = e.now.Add(time.Hour)
	resp, b = e.do("POST", "/api/ask/"+code+"/done", `{"ok":false,"text":"버튼은 있는데 파일이 비어 있습니다"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("not yet: %d %s", resp.StatusCode, b)
	}
	v := jsonMap(t, b)
	if v["doneNoAt"] != "2026-11-02T02:00:00.000Z" || len(v["adds"].([]any)) != 1 || v["status"] != "리드 전달" {
		t.Fatalf("not yet view: %v", v)
	}
	e.now = e.now.Add(time.Hour)
	resp, b = e.do("POST", "/api/ask/"+code+"/done", `{"ok":true}`)
	if v = jsonMap(t, b); resp.StatusCode != 200 || v["doneOkAt"] != "2026-11-02T03:00:00.000Z" || v["addedAt"] != "2026-11-02T03:00:00.000Z" {
		t.Fatalf("done: %d %v", resp.StatusCode, v)
	}
	if got, _ := e.st.Get(context.Background(), "requests", id); got["status"] != "리드 전달" {
		t.Fatalf("door wrote status: %v", got["status"])
	}
	e.st.Patch(context.Background(), "requests", id, store.Doc{"status": "종료"})
	resp, b = e.do("POST", "/api/ask/"+code+"/done", `{"ok":true}`)
	wantErr(t, resp, b, 409, "closed")
}

func TestAskAddsCap(t *testing.T) {
	e := newAskEnv(t)
	code := e.send(`{"what":"요청","from":"나"}`)["code"].(string)
	for i := 0; i < AskMaxAdds; i++ {
		if resp, b := e.do("PATCH", "/api/ask/"+code, fmt.Sprintf(`{"more":"덧붙임 %d"}`, i)); resp.StatusCode != 200 {
			t.Fatalf("add %d: %d %s", i, resp.StatusCode, b)
		}
	}
	resp, b := e.do("PATCH", "/api/ask/"+code, `{"more":"하나 더"}`)
	wantErr(t, resp, b, 409, "too_many_adds")
	_, b = e.do("GET", "/api/ask/"+code, "")
	if v := jsonMap(t, b); v["canAdd"] != false {
		t.Fatalf("canAdd at cap: %v", v["canAdd"])
	}
}

// TestAskClosedBeforeForward: blanks on a request closed before it reached a lead get "closed", not "locked".
func TestAskClosedBeforeForward(t *testing.T) {
	e := newAskEnv(t)
	code := e.send(`{"what":"요청","from":"나"}`)["code"].(string)
	docs, _ := e.st.List(context.Background(), "requests")
	e.st.Patch(context.Background(), "requests", docs[0]["id"].(string), store.Doc{"status": "종료", "closedAt": "x"})
	resp, b := e.do("PATCH", "/api/ask/"+code, `{"why":"x"}`)
	wantErr(t, resp, b, 409, "closed")
	if resp, b := e.do("PATCH", "/api/ask/"+code, `{"more":"다시 봐 주세요"}`); resp.StatusCode != 200 {
		t.Fatalf("note on closed: %d %s", resp.StatusCode, b)
	}
}

// TestAskHeldCanBeFilled: a request on hold for missing answers can still be filled through the link.
func TestAskHeldCanBeFilled(t *testing.T) {
	e := newAskEnv(t)
	code := e.send(`{"what":"요청","from":"나"}`)["code"].(string)
	docs, _ := e.st.List(context.Background(), "requests")
	e.st.Patch(context.Background(), "requests", docs[0]["id"].(string), store.Doc{"status": "보류", "heldAt": "x"})
	resp, b := e.do("PATCH", "/api/ask/"+code, `{"why":"다음 달 시연"}`)
	if resp.StatusCode != 200 || jsonMap(t, b)["why"] != "다음 달 시연" {
		t.Fatalf("held fill: %d %s", resp.StatusCode, b)
	}
}
