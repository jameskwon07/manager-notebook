// Command manager-notebook runs the offline manager's notebook.
//
//	manager-notebook [--demo] [--data DIR] [--port N] [--no-browser]
//	                              start the web server (see usageText)
//	manager-notebook healthcheck  exit 0 when /api/health answers ok (for HEALTHCHECK)
//	manager-notebook version      print the version
package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // TZ works in a distroless image without zoneinfo
	"unicode/utf8"

	assets "github.com/jameskwon07/manager-notebook"
	"github.com/jameskwon07/manager-notebook/internal/demo"
	"github.com/jameskwon07/manager-notebook/internal/guide"
	"github.com/jameskwon07/manager-notebook/internal/llm"
	"github.com/jameskwon07/manager-notebook/internal/server"
	"github.com/jameskwon07/manager-notebook/internal/store"
)

// version is set with -ldflags "-X main.version=2.1.0".
var version = "2.1.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Println(version)
			return
		case "healthcheck":
			os.Exit(healthcheck())
		case "help", "--help", "-h":
			fmt.Print(usageText)
			return
		}
	}
	o, err := parseOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n\n%s", err, usageText)
		os.Exit(2)
	}
	if err := run(o); err != nil {
		log.Printf("시작하지 못했습니다: %v", err)
		waitBeforeClose(runtime.GOOS)
		os.Exit(1)
	}
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func setupTZ() {
	if os.Getenv("TZ") != "" {
		return // Go already loaded it (embedded tzdata as fallback)
	}
	if loc, err := time.LoadLocation("Asia/Seoul"); err == nil {
		time.Local = loc
	}
}

func run(o options) error {
	setupTZ()
	log.SetFlags(log.LstdFlags)
	plan, err := makePlan(runtime.GOOS, os.Getenv, o)
	if err != nil {
		return err
	}
	password := os.Getenv("NOTEBOOK_PASSWORD") // not trimmed: spaces are allowed

	// Listen first: if a notebook already runs on this PC, open it instead of
	// starting a second copy on the same data.
	ln, port, err := listenPlan(plan)
	var running errAlreadyRunning
	if errors.As(err, &running) {
		fmt.Printf("%s을 브라우저로 엽니다: %s\n", running.Error(), localURL(running.port))
		if plan.openBrowser {
			if berr := openBrowser(runtime.GOOS, localURL(running.port)); berr != nil {
				fmt.Printf("브라우저를 열지 못했습니다. 위 주소를 직접 여세요.\n")
			}
		}
		return nil
	}
	if err != nil {
		return err
	}

	var st *store.Store
	if plan.demo {
		st, err = store.OpenMemory()
		if err == nil {
			err = demo.Seed(context.Background(), st, time.Now())
		}
		if err != nil {
			ln.Close()
			return fmt.Errorf("체험 모드 예시 데이터를 만들지 못했습니다: %w", err)
		}
	} else {
		st, err = store.Open(plan.dataDir)
		if err != nil {
			ln.Close()
			if plan.desktop {
				return fmt.Errorf("데이터 폴더(%s)를 열 수 없습니다. 쓸 수 있는 폴더인지 확인하거나 --data 로 다른 폴더를 주세요: %w", plan.dataDir, err)
			}
			return fmt.Errorf("데이터 폴더(%s)를 열 수 없습니다. 볼륨이 붙어 있고 쓸 수 있는지 확인하세요: %w", plan.dataDir, err)
		}
	}
	defer st.Close()

	lcfg := llm.ConfigFromEnv(os.Getenv)
	client := llm.New(lcfg)
	if client.Enabled() {
		log.Printf("사내 AI 켜짐 (주소: %s, 모델: %s, 제한 시간: %s)", client.Host(), orDash(lcfg.Model), lcfg.Timeout)
		if lcfg.TrimmedPath {
			log.Printf("안내: LLM_BASE_URL 끝의 /chat/completions 를 잘라 냈습니다. /v1 까지만 적으면 됩니다.")
		}
		if lcfg.PlainHTTPWithKey() {
			log.Printf("경고: LLM_BASE_URL 이 http:// 입니다. LLM_API_KEY 가 암호화 없이 갑니다. https:// 주소를 쓰세요.")
		}
		checkCertFile(os.Getenv("SSL_CERT_FILE"))
		if lcfg.InsecureSkipVerify {
			log.Printf("경고: LLM_INSECURE_SKIP_VERIFY=true 입니다. 사내 AI 인증서를 확인하지 않습니다. SSL_CERT_FILE 사용을 먼저 권합니다.")
		}
	} else {
		log.Printf("사내 AI 꺼짐 (LLM_BASE_URL 없음)")
	}

	web, err := fs.Sub(assets.FS, "web")
	if err != nil {
		return err
	}
	g, warns, err := guide.Load(assets.FS, "guide")
	if err != nil {
		return err
	}
	for _, w := range warns {
		log.Printf("가이드 경고: %s", w)
	}

	srv := server.New(server.Config{
		Version:  version,
		Password: password,
		Store:    st,
		LLM:      client,
		Guide:    g,
		Web:      web,
		Demo:     plan.demo,
	})
	if srv.AuthEnabled() {
		log.Printf("로그인 켜짐 (NOTEBOOK_PASSWORD 있음)")
		if utf8.RuneCountInString(password) < 8 {
			log.Printf("경고: 비밀번호가 짧습니다(8자 미만). 더 길게 바꾸기를 권합니다.")
		}
	} else {
		log.Printf("로그인 꺼짐: localhost 로만 열 수 있습니다")
	}

	hs := &http.Server{
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      lcfg.Timeout + 30*time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          log.New(os.Stderr, "http: ", log.LstdFlags),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		where := plan.dataDir
		if plan.demo {
			where = "메모리(체험 모드)"
		}
		log.Printf("manager-notebook %s 시작: %s (데이터: %s)", version, ln.Addr(), where)
		errCh <- hs.Serve(ln)
	}()
	if plan.desktop {
		printDesktopBanner(os.Stdout, plan, port, version)
		if plan.openBrowser {
			if err := openBrowser(runtime.GOOS, localURL(port)); err != nil {
				log.Printf("브라우저를 열지 못했습니다(%v). %s 을 직접 여세요.", err, localURL(port))
			}
		}
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Printf("멈추는 중…")
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := hs.Shutdown(sctx); err != nil {
			log.Printf("멈추는 중 오류: %v", err)
		}
	}
	return nil
}

// imageCABundle is the public CA bundle path the distroless base image puts
// in SSL_CERT_FILE by default. Only a user-set path is reported.
const imageCABundle = "/etc/ssl/certs/ca-certificates.crt"

// checkCertFile logs how many PEM certificates SSL_CERT_FILE holds, or a
// warning when none could be read. Go silently trusts nothing in that case,
// and every AI request would then fail with a certificate error.
func checkCertFile(p string) {
	p = strings.TrimSpace(p)
	if p == "" || p == imageCABundle {
		return // not set by the user (distroless sets the public bundle by default)
	}
	n, err := countPEMCerts(p)
	if err != nil || n == 0 {
		log.Printf("경고: SSL_CERT_FILE(%s)에서 인증서를 못 읽었습니다. PEM(-----BEGIN CERTIFICATE-----) 파일인지, -v 경로가 맞는지 확인하세요.", p)
		return
	}
	log.Printf("사내 인증서 %d개 읽음 (%s)", n, p)
}

// countPEMCerts counts parseable CERTIFICATE blocks in a PEM file.
func countPEMCerts(p string) (int, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	if fi.IsDir() {
		return 0, fmt.Errorf("%s is a directory", p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return 0, err
	}
	n := 0
	for {
		var b *pem.Block
		b, data = pem.Decode(data)
		if b == nil {
			break
		}
		if b.Type != "CERTIFICATE" {
			continue
		}
		if _, err := x509.ParseCertificate(b.Bytes); err == nil {
			n++
		}
	}
	return n, nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// healthcheck asks the local server and returns a process exit code.
func healthcheck() int {
	port := "8080"
	if _, p, err := net.SplitHostPort(getenv("NOTEBOOK_ADDR", ":8080")); err == nil && p != "" {
		port = p
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + port + "/api/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	defer resp.Body.Close()
	var body struct {
		OK bool `json:"ok"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&body) != nil || !body.OK {
		fmt.Fprintln(os.Stderr, "unhealthy: status", resp.StatusCode)
		return 1
	}
	return 0
}
