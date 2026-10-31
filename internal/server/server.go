// Package server is the notebook's HTTP layer: JSON API, static files,
// optional password login and security headers (SPEC §4).
package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/jameskwon07/manager-notebook/internal/guide"
	"github.com/jameskwon07/manager-notebook/internal/ics"
	"github.com/jameskwon07/manager-notebook/internal/llm"
	"github.com/jameskwon07/manager-notebook/internal/store"
)

// Limits from SPEC §4.
const (
	MaxBody       = 1 << 20
	MaxImportBody = 10 << 20
	SessionTTL    = 12 * time.Hour
	CookieName    = "mn_session"
)

// CSP is the Content-Security-Policy sent on every response.
const CSP = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

// Config wires the server.
type Config struct {
	Version   string
	Password  string       // empty = no login
	Store     *store.Store // required
	LLM       *llm.Client  // nil = AI off
	Guide     *guide.Guide // may be nil
	Web       fs.FS        // rooted at the web/ folder
	Logger    *log.Logger  // nil = log.Default()
	Now       func() time.Time
	FailDelay time.Duration // delay after a wrong password (default 1s)
	Demo      bool          // demo mode: example data in memory (/api/state says so)
}

// Server is an http.Handler.
type Server struct {
	cfg     Config
	mux     *http.ServeMux
	log     *log.Logger
	pwHash  [32]byte
	loginMu sync.Mutex

	sessMu   sync.Mutex
	sessions map[string]time.Time
}

// New builds the handler.
func New(cfg Config) *Server {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	if cfg.FailDelay == 0 {
		cfg.FailDelay = time.Second
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	s := &Server{cfg: cfg, log: cfg.Logger, sessions: map[string]time.Time{}}
	if cfg.Password != "" {
		s.pwHash = sha256.Sum256([]byte(cfg.Password))
	}
	m := http.NewServeMux()
	m.HandleFunc("/api/health", s.method("GET", s.health))
	m.HandleFunc("/api/state", s.method("GET", s.state))
	m.HandleFunc("/api/docs/{coll}", s.method("POST", s.createDoc))
	m.HandleFunc("/api/docs/{coll}/{id}", s.docByID)
	m.HandleFunc("/api/settings", s.method("PUT", s.putSettings))
	m.HandleFunc("/api/export", s.method("GET", s.export))
	m.HandleFunc("/api/import", s.method("POST", s.importData))
	m.HandleFunc("/api/llm", s.method("POST", s.llmProxy))
	m.HandleFunc("/api/calendar.ics", s.method("GET", s.calendar))
	m.HandleFunc("/api/guide", s.method("GET", s.guideList))
	m.HandleFunc("/api/guide/{slug}", s.method("GET", s.guidePage))
	m.HandleFunc("/api/login", s.method("POST", s.login))
	m.HandleFunc("/api/logout", s.method("POST", s.logout))
	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "not_found", "없는 주소입니다.")
	})
	m.HandleFunc("/login", s.method("GET", s.loginPage))
	m.HandleFunc("/login.css", s.method("GET", s.loginCSS))
	m.HandleFunc("/", s.static)
	s.mux = m
	return s
}

// AuthEnabled reports whether a password is set.
func (s *Server) AuthEnabled() bool { return s.cfg.Password != "" }

// ServeHTTP applies, in order: recover, log, security headers, host check,
// origin check, auth, routing.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	defer func() {
		if p := recover(); p != nil {
			s.log.Printf("panic on %s %s", r.Method, r.URL.Path)
			if !rec.wrote {
				writeErr(rec, http.StatusInternalServerError, "internal", "서버 안에서 문제가 생겼습니다.")
			}
		}
		// Only method, path, status and duration. Never bodies or queries.
		// A healthy /api/health (Docker HEALTHCHECK every 30s) is not logged,
		// so errors stay easy to find in `docker logs --tail 50`.
		if r.URL.Path == "/api/health" && rec.status == http.StatusOK {
			return
		}
		s.log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	}()

	h := rec.Header()
	h.Set("Content-Security-Policy", CSP)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		h.Set("Cache-Control", "no-store")
	}

	if !s.AuthEnabled() && !loopbackHost(r.Host) {
		writeErr(rec, http.StatusForbidden, "host",
			fmt.Sprintf("이 주소로는 열 수 없습니다. http://localhost:%s 로 여세요.", hostPort(r.Host)))
		return
	}
	if !sameOrigin(r) {
		writeErr(rec, http.StatusForbidden, "origin", "다른 사이트에서 온 요청은 받지 않습니다.")
		return
	}
	if s.AuthEnabled() && !authExempt(r.URL.Path) && !s.validSession(r) {
		writeErr(rec, http.StatusUnauthorized, "auth", "로그인이 필요합니다.")
		return
	}
	s.mux.ServeHTTP(rec, r)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status, r.wrote = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.ResponseWriter.Write(b)
}

// ---- request guards ----

var staticFiles = map[string]string{
	"/":             "index.html",
	"/index.html":   "index.html",
	"/core.js":      "core.js",
	"/app.js":       "app.js",
	"/app.css":      "app.css",
	"/content.json": "content.json",
	"/favicon.svg":  "favicon.svg",
}

func authExempt(p string) bool {
	switch p {
	case "/api/health", "/api/login", "/api/logout", "/login", "/login.css":
		return true
	}
	_, ok := staticFiles[p]
	return ok
}

// loopbackHost accepts localhost names and loopback IPs (DNS-rebinding guard
// when there is no password).
func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.Trim(strings.ToLower(host), "[]"), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// sameOrigin blocks cross-site writes (CSRF), including from other local ports.
func sameOrigin(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	if o := r.Header.Get("Origin"); o != "" {
		if o == "null" {
			return false
		}
		u, err := url.Parse(o)
		if err != nil || !strings.EqualFold(u.Host, r.Host) {
			return false
		}
		return true
	}
	if sfs := r.Header.Get("Sec-Fetch-Site"); sfs == "cross-site" || sfs == "same-site" {
		return false
	}
	return true
}

func (s *Server) method(m string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != m && !(m == "GET" && r.Method == http.MethodHead) {
			w.Header().Set("Allow", m)
			writeErr(w, http.StatusMethodNotAllowed, "method", "이 주소에서 쓸 수 없는 방식입니다.")
			return
		}
		h(w, r)
	}
}

// ---- JSON helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": code, "message": msg})
}

var errBadJSON = errors.New("bad json")

// decodeBody decodes exactly one JSON value into v with a size limit.
func decodeBody(w http.ResponseWriter, r *http.Request, limit int64, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return err
		}
		return errBadJSON
	}
	if _, err := dec.Token(); err != io.EOF {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return err
		}
		return errBadJSON
	}
	return nil
}

func (s *Server) readObject(w http.ResponseWriter, r *http.Request, limit int64) (store.Doc, bool) {
	var d store.Doc
	if err := decodeBody(w, r, limit, &d); err != nil {
		s.bodyErr(w, err)
		return nil, false
	}
	if d == nil {
		writeErr(w, http.StatusBadRequest, "bad_json", "JSON 객체를 보내야 합니다.")
		return nil, false
	}
	return d, true
}

func (s *Server) bodyErr(w http.ResponseWriter, err error) {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		writeErr(w, http.StatusRequestEntityTooLarge, "too_large", "보낸 내용이 너무 큽니다.")
		return
	}
	writeErr(w, http.StatusBadRequest, "bad_json", "JSON 형식이 맞지 않습니다.")
}

func (s *Server) storeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found", "찾는 기록이 없습니다.")
	case errors.Is(err, store.ErrBadColl):
		writeErr(w, http.StatusNotFound, "not_found", "없는 주소입니다.")
	case errors.Is(err, store.ErrBadID):
		writeErr(w, http.StatusBadRequest, "bad_id", "id 형식이 맞지 않습니다.")
	case errors.Is(err, store.ErrTooLarge):
		writeErr(w, http.StatusRequestEntityTooLarge, "too_large", "기록 하나는 256KB까지입니다.")
	case errors.Is(err, store.ErrLimit):
		writeErr(w, http.StatusConflict, "limit", "기록이 너무 많습니다. 한 종류에 20,000개까지입니다.")
	case errors.Is(err, store.ErrNotObject):
		writeErr(w, http.StatusBadRequest, "bad_json", "JSON 객체를 보내야 합니다.")
	case errors.Is(err, store.ErrBadFormat):
		writeErr(w, http.StatusBadRequest, "bad_format", "관리 수첩 내보내기 파일이 아닙니다.")
	default:
		s.log.Printf("store error: %v", err)
		writeErr(w, http.StatusInternalServerError, "internal", "저장소에서 문제가 생겼습니다.")
	}
}

// ---- handlers ----

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.cfg.Version})
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	st, err := s.cfg.Store.LoadState(r.Context())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	out := map[string]any{
		"version":  s.cfg.Version,
		"settings": st.Settings,
		"llm":      map[string]any{"enabled": s.cfg.LLM.Enabled(), "model": s.cfg.LLM.Model()},
		"auth":     s.AuthEnabled(), // 프런트엔드가 로그아웃 버튼을 보일지 정함
		"demo":     s.cfg.Demo,      // 체험 모드면 화면 위에 안내 띠를 보임
	}
	// Every collection key is present, an empty list when there is nothing.
	for _, c := range store.Collections {
		docs := st.Colls[c]
		if docs == nil {
			docs = []store.Doc{}
		}
		out[c] = docs
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createDoc(w http.ResponseWriter, r *http.Request) {
	coll := r.PathValue("coll")
	if !store.ValidColl(coll) {
		writeErr(w, http.StatusNotFound, "not_found", "없는 주소입니다.")
		return
	}
	d, ok := s.readObject(w, r, MaxBody)
	if !ok {
		return
	}
	out, err := s.cfg.Store.Create(r.Context(), coll, d)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) docByID(w http.ResponseWriter, r *http.Request) {
	coll, id := r.PathValue("coll"), r.PathValue("id")
	if !store.ValidColl(coll) {
		writeErr(w, http.StatusNotFound, "not_found", "없는 주소입니다.")
		return
	}
	if !store.ValidID(id) {
		writeErr(w, http.StatusBadRequest, "bad_id", "id 형식이 맞지 않습니다.")
		return
	}
	ctx := r.Context()
	switch r.Method {
	case http.MethodPut:
		d, ok := s.readObject(w, r, MaxBody)
		if !ok {
			return
		}
		out, err := s.cfg.Store.Put(ctx, coll, id, d)
		if err != nil {
			s.storeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPatch:
		d, ok := s.readObject(w, r, MaxBody)
		if !ok {
			return
		}
		out, err := s.cfg.Store.Patch(ctx, coll, id, d)
		if err != nil {
			s.storeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodDelete:
		if err := s.cfg.Store.Delete(ctx, coll, id); err != nil {
			s.storeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "PUT, PATCH, DELETE")
		writeErr(w, http.StatusMethodNotAllowed, "method", "이 주소에서 쓸 수 없는 방식입니다.")
	}
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	d, ok := s.readObject(w, r, MaxBody)
	if !ok {
		return
	}
	out, err := s.cfg.Store.PutSettings(r.Context(), d)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	ex, err := s.cfg.Store.MakeExport(r.Context(), s.cfg.Version)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	ex.ExportedAt = s.cfg.Now().Format(time.RFC3339)
	name := "manager-notebook-" + s.cfg.Now().Format("20060102") + ".json"
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	writeJSON(w, http.StatusOK, ex)
}

// importData replaces only the collections whose key is in the file, and the
// settings when "settings" is present and not null (SPEC2 §3). The body is
// first split into top-level keys so a missing key differs from an empty one.
func (s *Server) importData(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if err := decodeBody(w, r, MaxImportBody, &raw); err != nil {
		s.bodyErr(w, err)
		return
	}
	f, err := store.ParseImport(raw)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	res, err := s.cfg.Store.Import(r.Context(), f)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) llmProxy(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.LLM.Enabled() {
		writeErr(w, http.StatusNotFound, "llm_disabled", "사내 AI가 꺼져 있습니다. 설정하면 켜집니다.")
		return
	}
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := decodeBody(w, r, MaxBody, &req); err != nil {
		s.bodyErr(w, err)
		return
	}
	msgs := make([]llm.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		c, ok := m.Content.(string)
		if !ok || !llm.ValidRole(m.Role) {
			writeErr(w, http.StatusBadRequest, "bad_request", "messages 형식이 맞지 않습니다.")
			return
		}
		msgs = append(msgs, llm.Message{Role: m.Role, Content: c})
	}
	text, err := s.cfg.LLM.Complete(r.Context(), msgs)
	var ue *llm.UpstreamError
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]string{"text": text})
	case errors.Is(err, llm.ErrBadInput):
		writeErr(w, http.StatusBadRequest, "bad_request", "messages 형식이 맞지 않습니다.")
	case errors.Is(err, llm.ErrTimeout):
		writeErr(w, http.StatusGatewayTimeout, "timeout", "사내 AI가 제한 시간 안에 답하지 않았습니다.")
	case errors.As(err, &ue):
		// Only a short code and the host name. Never bodies or keys.
		if ue.Status == 0 {
			s.log.Printf("llm connect error: %s (%s)", ue.Cause, ue.Host)
			writeErr(w, http.StatusBadGateway, "upstream", connectMessage(ue.Cause))
		} else {
			s.log.Printf("llm upstream status %d (%s)", ue.Status, ue.Host)
			msg := fmt.Sprintf("사내 AI가 오류를 돌려주었습니다(상태 %d).", ue.Status)
			if ue.Status >= 300 && ue.Status < 400 {
				msg = fmt.Sprintf("사내 AI 주소가 다른 곳으로 넘기려 합니다(상태 %d). LLM_BASE_URL을 확인하세요.", ue.Status)
			}
			writeErr(w, http.StatusBadGateway, "upstream", msg)
		}
	default:
		if errors.Is(err, context.Canceled) {
			return
		}
		writeErr(w, http.StatusBadGateway, "upstream", "사내 AI에 연결하지 못했습니다.")
	}
}

// connectMessage turns a connect-failure code from llm.ConnectCause into a
// short Korean hint. The logs say the same code with the host.
func connectMessage(cause string) string {
	switch cause {
	case "cert":
		return "사내 AI에 연결하지 못했습니다(인증서). 사내 인증서를 넣었는지 확인하세요(README '사내 인증서')."
	case "dns":
		return "사내 AI에 연결하지 못했습니다(주소 찾기 실패). LLM_BASE_URL의 주소를 확인하세요."
	case "proxy":
		return "사내 AI에 연결하지 못했습니다(프록시). HTTPS_PROXY·NO_PROXY 설정을 확인하세요."
	case "refused":
		return "사내 AI에 연결하지 못했습니다(연결 거부). LLM_BASE_URL의 주소와 포트를 확인하세요."
	}
	return "사내 AI에 연결하지 못했습니다. docker logs --tail 50 manager-notebook 을 보세요."
}

// hostPort returns the port part of a Host header, or "8080".
func hostPort(host string) string {
	if _, p, err := net.SplitHostPort(host); err == nil && p != "" {
		return p
	}
	return "8080"
}

func (s *Server) calendar(w http.ResponseWriter, r *http.Request) {
	settings, err := s.cfg.Store.Settings(r.Context())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	body := ics.Build(ics.FromDoc(settings), s.cfg.Now())
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+ics.Filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *Server) guideList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Guide.List())
}

func (s *Server) guidePage(w http.ResponseWriter, r *http.Request) {
	p, err := s.cfg.Guide.Get(r.PathValue("slug"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "없는 가이드입니다.")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ---- login ----

func newToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (s *Server) validSession(r *http.Request) bool {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return false
	}
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	exp, ok := s.sessions[c.Value]
	if !ok {
		return false
	}
	if s.cfg.Now().After(exp) {
		delete(s.sessions, c.Value)
		return false
	}
	return true
}

func (s *Server) checkPassword(pw string) bool {
	h := sha256.Sum256([]byte(pw))
	return subtle.ConstantTimeCompare(h[:], s.pwHash[:]) == 1
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.AuthEnabled() {
		writeErr(w, http.StatusNotFound, "not_found", "비밀번호가 설정되지 않아 로그인이 필요 없습니다.")
		return
	}
	isForm := !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json")
	var pw string
	if isForm {
		r.Body = http.MaxBytesReader(w, r.Body, MaxBody)
		if err := r.ParseForm(); err != nil {
			s.bodyErr(w, err)
			return
		}
		pw = r.PostForm.Get("password")
	} else {
		var body struct {
			Password string `json:"password"`
		}
		if err := decodeBody(w, r, MaxBody, &body); err != nil {
			s.bodyErr(w, err)
			return
		}
		pw = body.Password
	}

	// One attempt at a time; a wrong password costs FailDelay.
	s.loginMu.Lock()
	ok := s.checkPassword(pw)
	if !ok {
		select {
		case <-time.After(s.cfg.FailDelay):
		case <-r.Context().Done():
		}
	}
	s.loginMu.Unlock()

	if !ok {
		if isForm {
			http.Redirect(w, r, "/login?fail=1", http.StatusSeeOther)
			return
		}
		writeErr(w, http.StatusUnauthorized, "auth", "비밀번호가 맞지 않습니다.")
		return
	}
	tok := newToken()
	now := s.cfg.Now()
	s.sessMu.Lock()
	for k, exp := range s.sessions {
		if now.After(exp) {
			delete(s.sessions, k)
		}
	}
	s.sessions[tok] = now.Add(SessionTTL)
	s.sessMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: tok, Path: "/",
		MaxAge: int(SessionTTL / time.Second), HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil,
	})
	if isForm {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		s.sessMu.Lock()
		delete(s.sessions, c.Value)
		s.sessMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// loginPage serves web/index.html, which shows its login form on /login.
// Without that file (or with web/login.html absent) a built-in, script-free
// page is used. With no password set, /login sends the browser to "/".
func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if !s.AuthEnabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	for _, name := range []string{"login.html", "index.html"} {
		if b, err := fs.ReadFile(s.webFS(), name); err == nil {
			serveBytes(w, r, name, b)
			return
		}
	}
	msg := ""
	if r.URL.Query().Get("fail") != "" {
		msg = `<p class="err" role="alert">비밀번호가 맞지 않습니다. 다시 적어 주세요.</p>`
	}
	page := strings.Replace(loginHTML, "<!--MSG-->", msg, 1)
	serveBytes(w, r, "login.html", []byte(page))
}

func (s *Server) loginCSS(w http.ResponseWriter, r *http.Request) {
	if b, err := fs.ReadFile(s.webFS(), "login.css"); err == nil {
		serveBytes(w, r, "login.css", b)
		return
	}
	serveBytes(w, r, "login.css", []byte(loginCSS))
}

// ---- static ----

func (s *Server) webFS() fs.FS {
	if s.cfg.Web == nil {
		return emptyFS{}
	}
	return s.cfg.Web
}

type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	name, ok := staticFiles[r.URL.Path]
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "없는 주소입니다.")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET")
		writeErr(w, http.StatusMethodNotAllowed, "method", "이 주소에서 쓸 수 없는 방식입니다.")
		return
	}
	b, err := fs.ReadFile(s.webFS(), name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "화면 파일이 없습니다.")
		return
	}
	serveBytes(w, r, name, b)
}

func serveBytes(w http.ResponseWriter, r *http.Request, name string, b []byte) {
	ct := mime.TypeByExtension(path.Ext(name))
	switch path.Ext(name) {
	case ".html":
		ct = "text/html; charset=utf-8"
	case ".js":
		ct = "text/javascript; charset=utf-8"
	case ".css":
		ct = "text/css; charset=utf-8"
	case ".json":
		ct = "application/json; charset=utf-8"
	case ".svg":
		ct = "image/svg+xml"
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}
