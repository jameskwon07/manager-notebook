package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{
		"LLM_BASE_URL":             " https://llm.example.internal/v1/ ",
		"LLM_API_KEY":              "k",
		"LLM_MODEL":                "m",
		"LLM_TIMEOUT_SEC":          "5",
		"LLM_EXTRA_HEADERS":        "X-Team=a; x-gw-key = b ;bad;=x;Evil=a\r\nb",
		"LLM_INSECURE_SKIP_VERIFY": "true",
	}
	c := ConfigFromEnv(func(k string) string { return env[k] })
	if c.BaseURL != "https://llm.example.internal/v1" || c.Timeout != 5*time.Second || !c.InsecureSkipVerify {
		t.Fatalf("config %+v", c)
	}
	if c.ExtraHeaders["X-Team"] != "a" || c.ExtraHeaders["X-Gw-Key"] != "b" || len(c.ExtraHeaders) != 2 {
		t.Fatalf("headers %v", c.ExtraHeaders)
	}
	d := ConfigFromEnv(func(string) string { return "" })
	if d.Timeout != 90*time.Second || New(d) != nil || New(d).Enabled() {
		t.Fatal("default should be disabled with 90s timeout")
	}
}

func TestComplete(t *testing.T) {
	var gotPath, gotAuth, gotExtra, gotCT string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotExtra, gotCT = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("X-Gw"), r.Header.Get("Content-Type")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"안녕하세요"}}]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL + "/v1", APIKey: "secret", Model: "team-model", Timeout: 5 * time.Second,
		ExtraHeaders: map[string]string{"X-Gw": "g1"}})
	text, err := c.Complete(context.Background(), []Message{{"system", "s"}, {"user", "u"}})
	if err != nil || text != "안녕하세요" {
		t.Fatalf("complete: %q %v", text, err)
	}
	if gotPath != "/v1/chat/completions" || gotAuth != "Bearer secret" || gotExtra != "g1" || gotCT != "application/json" {
		t.Fatalf("request: %s %s %s %s", gotPath, gotAuth, gotExtra, gotCT)
	}
	if gotBody["model"] != "team-model" || gotBody["stream"] != false {
		t.Fatalf("body %v", gotBody)
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages %v", gotBody["messages"])
	}
}

func TestCompleteArrayContentAndNoKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("auth header sent without key")
		}
		w.Write([]byte(`{"choices":[{"message":{"content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}}]}`))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: time.Second})
	text, err := c.Complete(context.Background(), []Message{{"user", "u"}})
	if err != nil || text != "ab" {
		t.Fatalf("%q %v", text, err)
	}
}

func TestUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"secret details"}`))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: time.Second})
	_, err := c.Complete(context.Background(), []Message{{"user", "u"}})
	var ue *UpstreamError
	if !errors.As(err, &ue) || ue.Status != 429 {
		t.Fatalf("want upstream 429, got %v", err)
	}
	if got := ue.Error(); got != "upstream status 429" {
		t.Fatalf("error text leaks: %q", got)
	}
}

func TestBadJSONIsUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: time.Second})
	_, err := c.Complete(context.Background(), []Message{{"user", "u"}})
	var ue *UpstreamError
	if !errors.As(err, &ue) {
		t.Fatalf("want upstream, got %v", err)
	}
}

func TestTimeout(t *testing.T) {
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-done:
		}
	}))
	defer srv.Close()
	defer close(done)
	c := New(Config{BaseURL: srv.URL, Timeout: 150 * time.Millisecond})
	start := time.Now()
	_, err := c.Complete(context.Background(), []Message{{"user", "u"}})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("want timeout, got %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("timeout not enforced")
	}
}

func TestBadInputAndDisabled(t *testing.T) {
	c := New(Config{BaseURL: "http://127.0.0.1:1", Timeout: time.Second})
	if _, err := c.Complete(context.Background(), nil); !errors.Is(err, ErrBadInput) {
		t.Fatal(err)
	}
	if _, err := c.Complete(context.Background(), []Message{{"tool", "x"}}); !errors.Is(err, ErrBadInput) {
		t.Fatal(err)
	}
	var off *Client
	if _, err := off.Complete(context.Background(), []Message{{"user", "x"}}); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
}

// TestNoRedirect: a 3xx from the upstream is not followed. The body and
// extra headers must never reach the redirect target (SPEC §1-2).
func TestNoRedirect(t *testing.T) {
	hit := make(chan struct{}, 1)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit <- struct{}{}
		w.Write([]byte(`{"choices":[{"message":{"content":"leaked"}}]}`))
	}))
	defer other.Close()
	for _, code := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.URL+"/v1/chat/completions", code)
		}))
		c := New(Config{BaseURL: gw.URL + "/v1", Timeout: time.Second, ExtraHeaders: map[string]string{"X-Api-Key": "s"}})
		_, err := c.Complete(context.Background(), []Message{{"user", "private note body"}})
		gw.Close()
		var ue *UpstreamError
		if !errors.As(err, &ue) || ue.Status != code {
			t.Fatalf("%d: want upstream %d, got %v", code, code, err)
		}
		select {
		case <-hit:
			t.Fatalf("%d: redirect was followed", code)
		default:
		}
	}
}

func TestConfigTrimsChatPath(t *testing.T) {
	env := map[string]string{"LLM_BASE_URL": "https://llm.example.internal/v1/chat/completions/"}
	c := ConfigFromEnv(func(k string) string { return env[k] })
	if c.BaseURL != "https://llm.example.internal/v1" || !c.TrimmedPath {
		t.Fatalf("%+v", c)
	}
	env["LLM_BASE_URL"] = "https://llm.example.internal/v1"
	if c := ConfigFromEnv(func(k string) string { return env[k] }); c.TrimmedPath {
		t.Fatal("trimmed a good URL")
	}
	if !(Config{BaseURL: "http://x/v1", APIKey: "k"}).PlainHTTPWithKey() || (Config{BaseURL: "https://x/v1", APIKey: "k"}).PlainHTTPWithKey() || (Config{BaseURL: "http://x/v1"}).PlainHTTPWithKey() {
		t.Fatal("PlainHTTPWithKey")
	}
}

func TestConnectCause(t *testing.T) {
	// Refused: a closed local port.
	ln := httptest.NewServer(http.NotFoundHandler())
	addr := ln.URL
	ln.Close()
	c := New(Config{BaseURL: addr, Timeout: 2 * time.Second})
	_, err := c.Complete(context.Background(), []Message{{"user", "u"}})
	var ue *UpstreamError
	if !errors.As(err, &ue) || ue.Status != 0 || ue.Cause != "refused" || ue.Host == "" {
		t.Fatalf("refused: %#v", err)
	}
	// Cert: a TLS server with a self-signed certificate.
	ts := httptest.NewTLSServer(http.NotFoundHandler())
	defer ts.Close()
	c = New(Config{BaseURL: ts.URL, Timeout: 2 * time.Second})
	_, err = c.Complete(context.Background(), []Message{{"user", "u"}})
	if !errors.As(err, &ue) || ue.Cause != "cert" {
		t.Fatalf("cert: %#v", err)
	}
	if got := ue.Error(); got != "upstream connect error: cert" {
		t.Fatalf("error text %q", got)
	}
	// DNS: the reserved .invalid TLD never resolves.
	c = New(Config{BaseURL: "http://llm.example.invalid/v1", Timeout: 5 * time.Second})
	_, err = c.Complete(context.Background(), []Message{{"user", "u"}})
	if !errors.As(err, &ue) || (ue.Cause != "dns" && ue.Cause != "connect") {
		t.Fatalf("dns: %#v", err)
	}
}
