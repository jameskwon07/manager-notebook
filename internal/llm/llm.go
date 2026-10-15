// Package llm is a tiny client for an OpenAI-compatible
// POST {base}/chat/completions endpoint. It is off unless a base URL is set.
//
// It never logs request or response bodies.
package llm

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Config comes from LLM_* environment variables (SPEC §3).
type Config struct {
	BaseURL            string
	APIKey             string
	Model              string
	Timeout            time.Duration
	ExtraHeaders       map[string]string
	InsecureSkipVerify bool
	// TrimmedPath is true when a trailing "/chat/completions" was removed
	// from LLM_BASE_URL (a common copy-paste mistake). main logs it.
	TrimmedPath bool
}

// ConfigFromEnv reads LLM_* variables through getenv.
func ConfigFromEnv(getenv func(string) string) Config {
	c := Config{
		BaseURL:      strings.TrimRight(strings.TrimSpace(getenv("LLM_BASE_URL")), "/"),
		APIKey:       strings.TrimSpace(getenv("LLM_API_KEY")),
		Model:        strings.TrimSpace(getenv("LLM_MODEL")),
		Timeout:      90 * time.Second,
		ExtraHeaders: ParseHeaders(getenv("LLM_EXTRA_HEADERS")),
	}
	if strings.HasSuffix(strings.ToLower(c.BaseURL), "/chat/completions") {
		c.BaseURL = strings.TrimRight(c.BaseURL[:len(c.BaseURL)-len("/chat/completions")], "/")
		c.TrimmedPath = true
	}
	if v := strings.TrimSpace(getenv("LLM_TIMEOUT_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Timeout = time.Duration(n) * time.Second
		}
	}
	switch strings.ToLower(strings.TrimSpace(getenv("LLM_INSECURE_SKIP_VERIFY"))) {
	case "1", "true", "yes", "on":
		c.InsecureSkipVerify = true
	}
	return c
}

// ParseHeaders parses "Key1=Val1;Key2=Val2". Empty or malformed parts are skipped.
func ParseHeaders(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(part, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" || strings.ContainsAny(k, " \t\r\n:") || strings.ContainsAny(v, "\r\n") {
			continue
		}
		out[http.CanonicalHeaderKey(k)] = strings.TrimSpace(v)
	}
	return out
}

// Message is one chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Errors returned by Complete.
var (
	ErrDisabled = errors.New("llm disabled")
	ErrTimeout  = errors.New("llm timeout")
	ErrBadInput = errors.New("bad messages")
)

// UpstreamError carries only the upstream HTTP status, never the body.
// When no response came back (Status 0), Cause says why in one word:
// "cert", "dns", "proxy", "refused" or "connect". Host is the upstream host
// name (no path, no query, no credentials) for the log line.
type UpstreamError struct {
	Status int
	Cause  string
	Host   string
}

func (e *UpstreamError) Error() string {
	if e.Status == 0 && e.Cause != "" {
		return fmt.Sprintf("upstream connect error: %s", e.Cause)
	}
	return fmt.Sprintf("upstream status %d", e.Status)
}

// ConnectCause sorts a transport error into a short code without keeping
// any request data.
func ConnectCause(err error) string {
	var ua x509.UnknownAuthorityError
	var ci x509.CertificateInvalidError
	var hn x509.HostnameError
	var tv *tls.CertificateVerificationError
	var dns *net.DNSError
	var op *net.OpError
	switch {
	case errors.As(err, &op) && op.Op == "proxyconnect", strings.Contains(err.Error(), "proxyconnect"):
		return "proxy"
	case errors.As(err, &ua), errors.As(err, &ci), errors.As(err, &hn), errors.As(err, &tv):
		return "cert"
	case errors.As(err, &dns):
		return "dns"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "refused"
	}
	return "connect"
}

// Client sends chat completions.
type Client struct {
	cfg  Config
	host string
	http *http.Client
}

// New builds a client. It returns nil when BaseURL is empty (feature off).
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		return nil
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.InsecureSkipVerify {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in last resort, warned at startup
	}
	return &Client{cfg: cfg, host: hostOf(cfg.BaseURL), http: &http.Client{
		Transport: tr,
		// Never follow redirects: requests go to LLM_BASE_URL only (SPEC §1-2).
		// A 3xx becomes UpstreamError{Status: 3xx} → 502.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func hostOf(base string) string {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return "?"
	}
	return u.Host
}

// Host returns the upstream host (nil-safe), for logs.
func (c *Client) Host() string {
	if c == nil {
		return ""
	}
	return c.host
}

// PlainHTTPWithKey reports whether an API key would travel over plain http.
func (c Config) PlainHTTPWithKey() bool {
	return c.APIKey != "" && strings.HasPrefix(strings.ToLower(c.BaseURL), "http://")
}

// Enabled reports whether the client is usable (nil-safe).
func (c *Client) Enabled() bool { return c != nil }

// Model returns the configured model name (nil-safe).
func (c *Client) Model() string {
	if c == nil {
		return ""
	}
	return c.cfg.Model
}

// Timeout returns the request time limit (nil-safe).
func (c *Client) Timeout() time.Duration {
	if c == nil {
		return 0
	}
	return c.cfg.Timeout
}

// ValidRole reports whether role is allowed.
func ValidRole(r string) bool { return r == "system" || r == "user" || r == "assistant" }

const maxResponseBytes = 8 << 20

// Complete sends messages and returns the first choice's text.
func (c *Client) Complete(ctx context.Context, msgs []Message) (string, error) {
	if c == nil {
		return "", ErrDisabled
	}
	if len(msgs) == 0 || len(msgs) > 50 {
		return "", ErrBadInput
	}
	for _, m := range msgs {
		if !ValidRole(m.Role) {
			return "", ErrBadInput
		}
	}
	body := map[string]any{"messages": msgs, "stream": false}
	if c.cfg.Model != "" {
		body["model"] = c.cfg.Model
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range c.cfg.ExtraHeaders {
		req.Header.Set(k, v)
	}
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || isTimeout(err) {
			return "", ErrTimeout
		}
		return "", &UpstreamError{Status: 0, Cause: ConnectCause(err), Host: c.host}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return "", &UpstreamError{Status: resp.StatusCode, Host: c.host}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || isTimeout(err) {
			return "", ErrTimeout
		}
		return "", &UpstreamError{Status: resp.StatusCode, Host: c.host}
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return "", &UpstreamError{Status: resp.StatusCode, Host: c.host}
	}
	ch := out.Choices[0]
	if s := contentText(ch.Message.Content); s != "" {
		return s, nil
	}
	return ch.Text, nil
}

// contentText accepts a plain string or an array of {type:"text",text:"..."} parts.
func contentText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		var b strings.Builder
		for _, p := range x {
			if m, ok := p.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					b.WriteString(t)
				}
			}
		}
		return b.String()
	}
	return ""
}

func isTimeout(err error) bool {
	var te interface{ Timeout() bool }
	return errors.As(err, &te) && te.Timeout()
}
