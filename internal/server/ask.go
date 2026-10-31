package server

// 요청 창구(ask door): 요청자가 직접 요청을 적어 보내는 작은 화면과 API.
//
// 관리 수첩과 다른 포트로 따로 열고, 같은 망의 누구나 열 수 있습니다.
// 이 핸들러에는 '요청 보내기'와 '내 요청 보기·빈칸 채우기' 말고는 길이
// 없습니다. 수첩 화면·/api/state·내보내기 같은 것은 이 포트로 닿지 않습니다.
// 비밀번호를 켜 두어도 창구는 로그인 없이 열립니다(창구가 하는 일이 그것).

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jameskwon07/manager-notebook/internal/store"
)

// Door limits.
const (
	AskMaxBody   = 32 << 10
	AskDailyCap  = 300 // 창구로 하루에 받는 요청 수(전체)
	askCodeLen   = 10
	askCodeChars = "23456789abcdefghjkmnpqrstuvwxyz" // 헷갈리는 0·1·i·l·o 뺌
	askWindow    = 10 * time.Minute
)

// Per-address limits in askWindow. Behind Docker's port mapping every
// requester may share one address, so these are generous.
var askLimits = map[string]int{"post": 30, "patch": 60, "get": 300, "miss": 60}

// Field length limits in characters.
var askFieldMax = map[string]int{"what": 120, "from": 60, "why": 500, "when": 500, "check": 500, "raw": 4000, "more": 1000}

var askCodeRe = regexp.MustCompile(`^[` + askCodeChars + `]{` + fmt.Sprint(askCodeLen) + `}$`)

// AskStatus tells the notebook screen whether the door is open and where.
type AskStatus struct {
	On   bool     `json:"on"`
	Port int      `json:"port,omitempty"`
	URLs []string `json:"urls"` // links to give requesters, best first
}

// AskConfig wires the door.
type AskConfig struct {
	Version string
	Store   *store.Store // required
	Web     fs.FS        // rooted at the web/ folder
	Logger  *log.Logger  // nil = log.Default()
	Now     func() time.Time
	Demo    bool
}

// Ask is the door's http.Handler.
type Ask struct {
	cfg AskConfig
	mux *http.ServeMux
	log *log.Logger

	limMu sync.Mutex
	lim   map[string]*askWin

	dayMu    sync.Mutex
	day      string
	dayCount int
}

type askWin struct {
	start time.Time
	n     int
}

var askStatic = map[string]string{
	"/":            "ask.html",
	"/ask.js":      "ask.js",
	"/ask.css":     "ask.css",
	"/app.css":     "app.css",
	"/core.js":     "core.js",
	"/favicon.svg": "favicon.svg",
}

// NewAsk builds the door handler.
func NewAsk(cfg AskConfig) *Ask {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	a := &Ask{cfg: cfg, log: cfg.Logger, lim: map[string]*askWin{}}
	m := http.NewServeMux()
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": a.cfg.Version, "door": true})
	})
	m.HandleFunc("GET /api/ask/form", a.form)
	m.HandleFunc("POST /api/ask", a.create)
	m.HandleFunc("GET /api/ask/{code}", a.view)
	m.HandleFunc("PATCH /api/ask/{code}", a.add)
	m.HandleFunc("GET /r/{code}", a.page)
	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "not_found", "없는 주소입니다.")
	})
	m.HandleFunc("/", a.static)
	a.mux = m
	return a
}

// ServeHTTP applies, in order: recover, log, security headers, origin check, routing.
func (a *Ask) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := a.cfg.Now()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	defer func() {
		if p := recover(); p != nil {
			a.log.Printf("창구: panic on %s", r.Method)
			if !rec.wrote {
				writeErr(rec, http.StatusInternalServerError, "internal", "서버 안에서 문제가 생겼습니다.")
			}
		}
		if r.URL.Path == "/api/health" && rec.status == http.StatusOK {
			return
		}
		// 접수 링크(코드)는 남기지 않습니다. 길의 모양만 적습니다.
		a.log.Printf("창구 %s %s %d %s", r.Method, askLogPath(r.URL.Path), rec.status, a.cfg.Now().Sub(start).Round(time.Millisecond))
	}()

	h := rec.Header()
	h.Set("Content-Security-Policy", CSP)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		h.Set("Cache-Control", "no-store")
	}
	if !sameOrigin(r) {
		writeErr(rec, http.StatusForbidden, "origin", "다른 사이트에서 온 요청은 받지 않습니다.")
		return
	}
	a.mux.ServeHTTP(rec, r)
}

func askLogPath(p string) string {
	switch {
	case strings.HasPrefix(p, "/api/ask/") && p != "/api/ask/form":
		return "/api/ask/{code}"
	case strings.HasPrefix(p, "/r/"):
		return "/r/{code}"
	}
	if _, ok := askStatic[p]; ok || p == "/api/ask" || p == "/api/ask/form" {
		return p
	}
	return "(기타)"
}

// ---- limits ----

func clientAddr(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

// allow counts one hit of kind for the caller's address and reports whether
// it is still under the limit.
func (a *Ask) allow(r *http.Request, kind string) bool {
	now := a.cfg.Now()
	key := kind + " " + clientAddr(r)
	a.limMu.Lock()
	defer a.limMu.Unlock()
	if len(a.lim) > 10000 {
		for k, v := range a.lim {
			if now.Sub(v.start) >= askWindow {
				delete(a.lim, k)
			}
		}
	}
	win := a.lim[key]
	if win == nil || now.Sub(win.start) >= askWindow {
		win = &askWin{start: now}
		a.lim[key] = win
	}
	win.n++
	return win.n <= askLimits[kind]
}

func tooMany(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "600")
	writeErr(w, http.StatusTooManyRequests, "too_many", "잠깐 사이에 너무 많이 보냈습니다. 10분 뒤에 다시 해 주세요.")
}

// takeDaily counts one new request for today and reports whether today's cap still allows it.
func (a *Ask) takeDaily() bool {
	day := a.cfg.Now().Format("2006-01-02")
	a.dayMu.Lock()
	defer a.dayMu.Unlock()
	if day != a.day {
		a.day, a.dayCount = day, 0
	}
	if a.dayCount >= AskDailyCap {
		return false
	}
	a.dayCount++
	return true
}

// ---- text ----

// cleanField trims, normalizes line ends and drops control characters.
// Only "raw" and "more" keep line breaks.
func cleanField(key, s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	multi := key == "raw" || key == "more"
	var b strings.Builder
	for _, c := range s {
		switch {
		case c == '\n' && multi:
			b.WriteRune(c)
		case c == '\n' || c == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(c) || c == utf8.RuneError:
			// drop
		default:
			b.WriteRune(c)
		}
	}
	return strings.TrimSpace(b.String())
}

func tooLong(key, s string) bool { return utf8.RuneCountInString(s) > askFieldMax[key] }

var askFieldNames = map[string]string{"what": "무엇", "from": "보내는 분", "why": "왜·누가 정했는지", "when": "언제까지와 이유", "check": "확인 방법", "raw": "자세히", "more": "덧붙임"}

// newAskCode returns a random code for a requester's link.
func newAskCode() string {
	var b [askCodeLen]byte
	_, _ = rand.Read(b[:])
	out := make([]byte, askCodeLen)
	for i, x := range b {
		out[i] = askCodeChars[int(x)%len(askCodeChars)]
	}
	return string(out)
}

func (a *Ask) nowISO() string { return a.cfg.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00") }

// ---- data ----

type askProgram struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// programs returns active programs (id and name only), in screen order.
func (a *Ask) programs(r *http.Request) ([]askProgram, error) {
	docs, err := a.cfg.Store.List(r.Context(), "programs")
	if err != nil {
		return nil, err
	}
	type ranked struct {
		p     askProgram
		order float64
	}
	var list []ranked
	for _, d := range docs {
		if act, ok := d["active"].(bool); ok && !act {
			continue
		}
		id, _ := d["id"].(string)
		name, _ := d["name"].(string)
		if name = strings.TrimSpace(name); id == "" || name == "" {
			continue
		}
		o, ok := d["order"].(float64)
		if !ok {
			o = 1e9
		}
		list = append(list, ranked{askProgram{id, name}, o})
	}
	// 화면(core.js sortedPrograms)과 같은 순서: order 순, 같으면 원래 순서.
	sort.SliceStable(list, func(i, j int) bool { return list[i].order < list[j].order })
	out := make([]askProgram, 0, len(list))
	for _, x := range list {
		out = append(out, x.p)
	}
	return out, nil
}

// findByCode returns the request whose askCode is code.
func (a *Ask) findByCode(r *http.Request, code string) (store.Doc, error) {
	docs, err := a.cfg.Store.List(r.Context(), "requests")
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		if c, _ := d["askCode"].(string); c == code {
			return d, nil
		}
	}
	return nil, store.ErrNotFound
}

func str(d store.Doc, k string) string {
	if s, ok := d[k].(string); ok {
		return s
	}
	return ""
}

// editable: the requester may still fill blanks before the request goes to a lead.
func editable(d store.Doc) bool {
	if u, _ := d["urgent"].(bool); u {
		return false
	}
	st := str(d, "status")
	return (st == "" || st == "접수" || st == "정리 중") && str(d, "forwardedAt") == ""
}

// publicView is what a requester sees for their own request: no manager notes, no estimates.
func (a *Ask) publicView(r *http.Request, d store.Doc) map[string]any {
	progName := ""
	if id := str(d, "program"); id != "" {
		if ps, err := a.programs(r); err == nil {
			for _, p := range ps {
				if p.ID == id {
					progName = p.Name
				}
			}
		}
	}
	var miss []string
	for _, k := range []string{"why", "when", "check", "program"} {
		if strings.TrimSpace(str(d, k)) == "" {
			miss = append(miss, k)
		}
	}
	if miss == nil {
		miss = []string{}
	}
	st := str(d, "status")
	if st == "" {
		st = "접수"
	}
	return map[string]any{
		"code": str(d, "askCode"), "what": str(d, "what"), "raw": str(d, "raw"), "from": str(d, "from"),
		"why": str(d, "why"), "when": str(d, "when"), "check": str(d, "check"),
		"program": str(d, "program"), "programName": progName,
		"status": st, "receivedAt": str(d, "receivedAt"), "forwardedAt": str(d, "forwardedAt"),
		"closedAt": str(d, "closedAt"), "addedAt": str(d, "addedAt"),
		"missing": miss, "editable": editable(d),
	}
}

// ---- handlers ----

func (a *Ask) form(w http.ResponseWriter, r *http.Request) {
	ps, err := a.programs(r)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if ps == nil {
		ps = []askProgram{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": a.cfg.Version, "demo": a.cfg.Demo, "programs": ps, "limits": askFieldMax})
}

func (a *Ask) storeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found", "이 링크의 요청을 찾지 못했습니다. 링크를 끝까지 복사했는지 확인해 주세요.")
	case errors.Is(err, store.ErrLimit):
		writeErr(w, http.StatusConflict, "limit", "접수함이 가득 찼습니다. 담당자에게 직접 알려 주세요.")
	default:
		a.log.Printf("창구 저장소 오류: %v", err)
		writeErr(w, http.StatusInternalServerError, "internal", "저장소에서 문제가 생겼습니다. 잠시 뒤 다시 해 주세요.")
	}
}

// readFields decodes a flat JSON object of strings and cleans the known keys.
func (a *Ask) readFields(w http.ResponseWriter, r *http.Request, keys ...string) (map[string]string, bool) {
	var in map[string]any
	if err := decodeBody(w, r, AskMaxBody, &in); err != nil {
		bodyErr(w, err)
		return nil, false
	}
	if in == nil {
		writeErr(w, http.StatusBadRequest, "bad_json", "JSON 객체를 보내야 합니다.")
		return nil, false
	}
	out := map[string]string{}
	for _, k := range keys {
		v, ok := in[k]
		if !ok || v == nil {
			continue
		}
		s, ok := v.(string)
		if !ok {
			writeErr(w, http.StatusBadRequest, "bad_field", askFieldNames[k]+" 칸은 글자여야 합니다.")
			return nil, false
		}
		if k == "program" {
			out[k] = strings.TrimSpace(s)
			continue
		}
		s = cleanField(k, s)
		if tooLong(k, s) {
			writeErr(w, http.StatusBadRequest, "too_long", fmt.Sprintf("%s 칸은 %d자까지입니다.", askFieldNames[k], askFieldMax[k]))
			return nil, false
		}
		out[k] = s
	}
	return out, true
}

// validProgram reports whether id is "" or an active program.
func (a *Ask) validProgram(r *http.Request, id string) (bool, error) {
	if id == "" {
		return true, nil
	}
	ps, err := a.programs(r)
	if err != nil {
		return false, err
	}
	for _, p := range ps {
		if p.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func (a *Ask) create(w http.ResponseWriter, r *http.Request) {
	if !a.allow(r, "post") {
		tooMany(w)
		return
	}
	f, ok := a.readFields(w, r, "what", "from", "why", "when", "check", "raw", "program")
	if !ok {
		return
	}
	if f["what"] == "" {
		writeErr(w, http.StatusBadRequest, "need_what", "무엇이 필요한지 한 줄은 적어 주세요.")
		return
	}
	if f["from"] == "" {
		writeErr(w, http.StatusBadRequest, "need_from", "보내는 분을 적어 주세요. 되물을 때 필요합니다.")
		return
	}
	if ok, err := a.validProgram(r, f["program"]); err != nil {
		a.storeErr(w, err)
		return
	} else if !ok {
		writeErr(w, http.StatusBadRequest, "bad_program", "고른 프로그램이 목록에 없습니다. 화면을 새로 고친 뒤 다시 골라 주세요.")
		return
	}
	if !a.takeDaily() {
		writeErr(w, http.StatusTooManyRequests, "daily_cap", "오늘 창구로 받을 수 있는 요청 수를 넘었습니다. 담당자에게 직접 알려 주세요.")
		return
	}
	now := a.nowISO()
	code := newAskCode()
	doc := store.Doc{
		"raw": f["raw"], "what": f["what"], "from": f["from"], "program": f["program"], "size": "",
		"why": f["why"], "when": f["when"], "check": f["check"], "urgent": false,
		"status": "접수", "receivedAt": now, "askedAt": nil, "forwardedAt": nil, "closedAt": nil, "heldAt": nil,
		"absorbed": false, "absorbedAt": nil, "est": nil, "note": "", "createdAt": now,
		"via": "창구", "askCode": code, "addedAt": nil,
	}
	if _, err := a.cfg.Store.Create(r.Context(), "requests", doc); err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, a.publicView(r, doc))
}

func (a *Ask) lookup(w http.ResponseWriter, r *http.Request) (store.Doc, bool) {
	code := r.PathValue("code")
	if !askCodeRe.MatchString(code) {
		if !a.allow(r, "miss") {
			tooMany(w)
			return nil, false
		}
		writeErr(w, http.StatusNotFound, "not_found", "이 링크의 요청을 찾지 못했습니다. 링크를 끝까지 복사했는지 확인해 주세요.")
		return nil, false
	}
	d, err := a.findByCode(r, code)
	if errors.Is(err, store.ErrNotFound) && !a.allow(r, "miss") {
		tooMany(w)
		return nil, false
	}
	if err != nil {
		a.storeErr(w, err)
		return nil, false
	}
	return d, true
}

func (a *Ask) view(w http.ResponseWriter, r *http.Request) {
	if !a.allow(r, "get") {
		tooMany(w)
		return
	}
	d, ok := a.lookup(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, a.publicView(r, d))
}

// add lets the requester fill blanks (why, when, check, program) and append a
// note while the request has not gone to a lead yet. Empty values are ignored,
// so a requester can never erase what is already there.
func (a *Ask) add(w http.ResponseWriter, r *http.Request) {
	if !a.allow(r, "patch") {
		tooMany(w)
		return
	}
	d, ok := a.lookup(w, r)
	if !ok {
		return
	}
	f, ok := a.readFields(w, r, "why", "when", "check", "program", "more")
	if !ok {
		return
	}
	if !editable(d) {
		writeErr(w, http.StatusConflict, "locked", "이미 담당 리드에게 넘어간 요청이라 여기서는 고칠 수 없습니다. 담당자에게 직접 알려 주세요.")
		return
	}
	if ok, err := a.validProgram(r, f["program"]); err != nil {
		a.storeErr(w, err)
		return
	} else if !ok {
		writeErr(w, http.StatusBadRequest, "bad_program", "고른 프로그램이 목록에 없습니다. 화면을 새로 고친 뒤 다시 골라 주세요.")
		return
	}
	patch := store.Doc{}
	for _, k := range []string{"why", "when", "check", "program"} {
		if v := f[k]; v != "" && v != str(d, k) {
			patch[k] = v
		}
	}
	if more := f["more"]; more != "" {
		raw := str(d, "raw")
		stamp := a.cfg.Now().Format("1/2 15:04")
		next := strings.TrimSpace(raw + "\n\n[덧붙임 " + stamp + "] " + more)
		if utf8.RuneCountInString(next) > askFieldMax["raw"]+askFieldMax["more"] {
			writeErr(w, http.StatusBadRequest, "too_long", "덧붙인 글이 너무 많습니다. 담당자에게 직접 알려 주세요.")
			return
		}
		patch["raw"] = next
	}
	if len(patch) == 0 {
		writeErr(w, http.StatusBadRequest, "empty", "채운 칸이 없습니다.")
		return
	}
	patch["addedAt"] = a.nowISO()
	out, err := a.cfg.Store.Patch(r.Context(), "requests", str(d, "id"), patch)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.publicView(r, out))
}

func (a *Ask) page(w http.ResponseWriter, r *http.Request) {
	a.serveFile(w, r, "ask.html")
}

func (a *Ask) static(w http.ResponseWriter, r *http.Request) {
	name, ok := askStatic[r.URL.Path]
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "없는 주소입니다.")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET")
		writeErr(w, http.StatusMethodNotAllowed, "method", "이 주소에서 쓸 수 없는 방식입니다.")
		return
	}
	a.serveFile(w, r, name)
}

func (a *Ask) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	var web fs.FS = emptyFS{}
	if a.cfg.Web != nil {
		web = a.cfg.Web
	}
	b, err := fs.ReadFile(web, name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "화면 파일이 없습니다.")
		return
	}
	serveBytes(w, r, name, b)
}
