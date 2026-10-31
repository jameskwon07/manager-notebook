package main

// Running as a desktop program (Windows .exe, or the binary on a Mac/Linux PC)
// instead of inside the container image.
//
// The container image sets NOTEBOOK_ADDR and NOTEBOOK_DATA_DIR, so when both
// are empty we are on a PC: listen on 127.0.0.1 only (no firewall prompt, no
// one else on the network can open it), keep data in the user's own app-data
// folder, open the browser, and reuse an instance that is already running.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const usageText = `사용법: manager-notebook [옵션]
       manager-notebook healthcheck | version | help

옵션:
  --demo         체험 모드: 예시 데이터로 엽니다. 기록은 메모리에만 있고 끄면 사라집니다.
  --data 폴더    데이터를 둘 폴더 (기본: Windows는 %LOCALAPPDATA%\manager-notebook, 컨테이너는 /data)
  --port 번호    포트 (기본 8080, 체험 모드 8090). PC에서 실행하면 이 PC 안(127.0.0.1)에서만 열립니다.
  --no-browser   시작할 때 브라우저를 열지 않습니다.
  --only intake  요청 접수함 하나만 엽니다(전체 수첩과 같은 기록을 씁니다).
  --share        요청 창구를 엽니다: 같은 망의 동료가 링크로 요청을 직접 적어 보냅니다.
                 창구는 따로 연 포트(기본 8180, 체험 모드 8190)로 요청 보내기 화면만 엽니다.
                 수첩 화면은 그대로 이 PC 안에서만 열립니다.
  --share-port 번호  요청 창구 포트 (--share 를 함께 켭니다)
`

// options are the command-line flags.
type options struct {
	demo      bool
	noBrowser bool
	dataDir   string
	port      int
	only      string // "" (전체 수첩) 또는 "intake"(요청 접수함만)
	share     bool   // 요청 창구를 엽니다
	sharePort int
}

// parseOptions reads the flags after the program name.
func parseOptions(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("manager-notebook", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.demo, "demo", false, "")
	fs.BoolVar(&o.noBrowser, "no-browser", false, "")
	fs.StringVar(&o.dataDir, "data", "", "")
	fs.IntVar(&o.port, "port", 0, "")
	fs.StringVar(&o.only, "only", "", "")
	fs.BoolVar(&o.share, "share", false, "")
	fs.IntVar(&o.sharePort, "share-port", 0, "")
	if err := fs.Parse(args); err != nil {
		return o, fmt.Errorf("옵션을 읽지 못했습니다: %v", err)
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("모르는 인자입니다: %s", fs.Arg(0))
	}
	if o.port < 0 || o.port > 65535 {
		return o, fmt.Errorf("포트는 1~65535 사이여야 합니다: %d", o.port)
	}
	if o.sharePort < 0 || o.sharePort > 65535 {
		return o, fmt.Errorf("창구 포트는 1~65535 사이여야 합니다: %d", o.sharePort)
	}
	if o.sharePort > 0 {
		o.share = true
	}
	o.dataDir = strings.TrimSpace(o.dataDir)
	o.only = strings.TrimSpace(o.only)
	if o.only != "" && o.only != "intake" {
		return o, fmt.Errorf("--only 에는 intake 만 쓸 수 있습니다: %s", o.only)
	}
	return o, nil
}

// launchPlan is where to listen and where to keep data.
type launchPlan struct {
	desktop     bool   // running on a PC, not in the container image
	demo        bool   // example data in memory
	dataDir     string // "" in demo mode
	host        string // "127.0.0.1" on a PC, "" (all interfaces) in a container
	ports       []int  // ports to try in order; one entry when the port was given
	envAddr     string // NOTEBOOK_ADDR when set and no --port: listen exactly there
	openBrowser bool
	only        string // "intake": 요청 접수함만

	// 요청 창구: 모든 주소에서 열고(같은 망의 동료가 들어옴), 요청 보내기 화면만 둡니다.
	askPorts   []int  // 비어 있으면 창구를 열지 않음
	askEnvAddr string // NOTEBOOK_ASK_ADDR: 정확히 이 주소로 엶
	askURL     string // NOTEBOOK_ASK_URL: 요청자에게 줄 링크(컨테이너처럼 주소를 알 수 없을 때)
}

func (p launchPlan) askAddr(port int) string {
	if p.askEnvAddr != "" {
		return p.askEnvAddr
	}
	return net.JoinHostPort("", strconv.Itoa(port))
}

func (p launchPlan) addr(port int) string {
	if p.envAddr != "" {
		return p.envAddr
	}
	return net.JoinHostPort(p.host, strconv.Itoa(port))
}

// makePlan decides the address and data folder from the flags, the
// environment and the operating system.
func makePlan(goos string, getenv func(string) string, o options) (launchPlan, error) {
	envAddr := strings.TrimSpace(getenv("NOTEBOOK_ADDR"))
	envData := strings.TrimSpace(getenv("NOTEBOOK_DATA_DIR"))
	p := launchPlan{desktop: envAddr == "" && envData == "", demo: o.demo}
	if p.desktop {
		p.host = "127.0.0.1"
	}

	switch {
	case o.port > 0:
		p.ports = []int{o.port}
	case envAddr != "":
		p.envAddr = envAddr
		port := 8080
		if _, ps, err := net.SplitHostPort(envAddr); err == nil {
			if n, err := strconv.Atoi(ps); err == nil {
				port = n
			}
		}
		p.ports = []int{port}
	default:
		base := 8080
		if o.demo {
			base = 8090
		}
		for i := 0; i < 10; i++ {
			p.ports = append(p.ports, base+i)
		}
	}

	switch {
	case o.demo:
		p.dataDir = ""
	case o.dataDir != "":
		p.dataDir = o.dataDir
	case envData != "":
		p.dataDir = envData
	case p.desktop:
		d, err := userDataDir(goos, getenv)
		if err != nil {
			return p, err
		}
		p.dataDir = d
	default:
		p.dataDir = "/data"
	}

	p.openBrowser = p.desktop && !o.noBrowser && strings.TrimSpace(getenv("NOTEBOOK_NO_BROWSER")) == ""
	p.only = o.only
	if p.only == "" && strings.TrimSpace(getenv("NOTEBOOK_ONLY")) == "intake" {
		p.only = "intake"
	}

	askEnv := strings.TrimSpace(getenv("NOTEBOOK_ASK_ADDR"))
	p.askURL = strings.TrimSpace(getenv("NOTEBOOK_ASK_URL"))
	switch {
	case o.sharePort > 0:
		p.askPorts = []int{o.sharePort}
	case askEnv != "":
		p.askEnvAddr = askEnv
		port := 8180
		if _, ps, err := net.SplitHostPort(askEnv); err == nil {
			if n, err := strconv.Atoi(ps); err == nil {
				port = n
			}
		}
		p.askPorts = []int{port}
	case o.share:
		base := 8180
		if o.demo {
			base = 8190
		}
		for i := 0; i < 10; i++ {
			p.askPorts = append(p.askPorts, base+i)
		}
	}
	return p, nil
}

// listenAsk opens the first free door port of the plan.
func listenAsk(p launchPlan) (net.Listener, int, error) {
	var lastErr error
	for _, port := range p.askPorts {
		ln, err := net.Listen("tcp", p.askAddr(port))
		if err == nil {
			return ln, port, nil
		}
		lastErr = err
	}
	if len(p.askPorts) == 1 {
		return nil, 0, fmt.Errorf("요청 창구 %d 포트를 열 수 없습니다(다른 프로그램이 쓰는 중일 수 있습니다). --share-port 로 다른 번호를 주세요: %w", p.askPorts[0], lastErr)
	}
	return nil, 0, fmt.Errorf("요청 창구 %d~%d 포트를 모두 열 수 없습니다. --share-port 로 다른 번호를 주세요: %w", p.askPorts[0], p.askPorts[len(p.askPorts)-1], lastErr)
}

// askURLs are the links to give requesters: NOTEBOOK_ASK_URL when set,
// otherwise this PC's network addresses (company LAN ranges first).
// In a container the PC's address is unknown, so only NOTEBOOK_ASK_URL counts.
func askURLs(p launchPlan, port int, addrs []net.Addr) []string {
	if p.askURL != "" {
		u := p.askURL
		if !strings.HasSuffix(u, "/") {
			u += "/"
		}
		return []string{u}
	}
	if !p.desktop {
		return []string{}
	}
	type cand struct {
		ip   net.IP
		rank int
	}
	var cs []cand
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := n.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			continue
		}
		rank := 3
		switch {
		case ip[0] == 10:
			rank = 0
		case ip[0] == 192 && ip[1] == 168:
			rank = 1
		case ip[0] == 172 && ip[1]&0xf0 == 16:
			rank = 2 // 가상 네트워크(WSL·Docker·Hyper-V)가 자주 쓰는 대역이라 뒤로
		}
		cs = append(cs, cand{ip, rank})
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].rank < cs[j].rank })
	out := []string{}
	for _, c := range cs {
		out = append(out, "http://"+net.JoinHostPort(c.ip.String(), strconv.Itoa(port))+"/")
	}
	return out
}

// userDataDir is the per-user folder a desktop program should write to.
func userDataDir(goos string, getenv func(string) string) (string, error) {
	const name = "manager-notebook"
	switch goos {
	case "windows":
		if d := getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, name), nil
		}
		if d := getenv("USERPROFILE"); d != "" {
			return filepath.Join(d, "AppData", "Local", name), nil
		}
	case "darwin":
		if d := getenv("HOME"); d != "" {
			return filepath.Join(d, "Library", "Application Support", name), nil
		}
	default:
		if d := getenv("XDG_DATA_HOME"); d != "" {
			return filepath.Join(d, name), nil
		}
		if d := getenv("HOME"); d != "" {
			return filepath.Join(d, ".local", "share", name), nil
		}
	}
	return "", errors.New("사용자 폴더를 찾지 못했습니다. --data 로 데이터 폴더를 지정하세요")
}

// probeNotebook reports whether a notebook already answers on this port.
var probeNotebook = func(port int) (string, bool) {
	c := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := c.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/api/health")
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	var body struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body) != nil || !body.OK {
		return "", false
	}
	return body.Version, true
}

// errAlreadyRunning means another notebook already listens on alreadyPort.
type errAlreadyRunning struct {
	port    int
	version string
}

func (e errAlreadyRunning) Error() string {
	return fmt.Sprintf("이미 켜져 있는 관리 수첩(%s, %d 포트)", e.version, e.port)
}

// listenPlan opens the first free port of the plan. When a port is taken by
// a notebook that is already running, it returns errAlreadyRunning so the
// caller can open that one instead of starting a second copy on the same data.
func listenPlan(p launchPlan) (net.Listener, int, error) {
	var lastErr error
	for _, port := range p.ports {
		ln, err := net.Listen("tcp", p.addr(port))
		if err == nil {
			return ln, port, nil
		}
		lastErr = err
		if p.desktop {
			if v, ok := probeNotebook(port); ok {
				return nil, port, errAlreadyRunning{port: port, version: v}
			}
		}
		if len(p.ports) == 1 {
			break
		}
	}
	if len(p.ports) == 1 {
		return nil, 0, fmt.Errorf("%d 포트를 열 수 없습니다(다른 프로그램이 쓰는 중일 수 있습니다). --port 로 다른 번호를 주세요: %w", p.ports[0], lastErr)
	}
	return nil, 0, fmt.Errorf("%d~%d 포트를 모두 열 수 없습니다. --port 로 다른 번호를 주세요: %w", p.ports[0], p.ports[len(p.ports)-1], lastErr)
}

// openBrowser asks the operating system to open url in the default browser.
var openBrowser = func(goos, url string) error {
	var cmd *exec.Cmd
	switch goos {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() //nolint:errcheck // reap the helper process
	return nil
}

func localURL(port int) string { return "http://localhost:" + strconv.Itoa(port) + "/" }

// printDesktopBanner tells a person at the console what is running and how to stop it.
// askLinks are the door links when the door is open.
func printDesktopBanner(w io.Writer, p launchPlan, port int, ver string, askLinks ...string) {
	fmt.Fprintln(w, "")
	name := "관리 수첩"
	if p.only == "intake" {
		name = "요청 접수함(관리 수첩"
		ver += ")"
	}
	if p.demo {
		fmt.Fprintf(w, "  %s %s — 체험 모드 (예시 데이터, 끄면 사라집니다)\n", name, ver)
	} else {
		fmt.Fprintf(w, "  %s %s\n", name, ver)
	}
	fmt.Fprintf(w, "  주소   %s   (브라우저가 열리지 않으면 이 주소를 여세요)\n", localURL(port))
	if p.dataDir != "" {
		fmt.Fprintf(w, "  데이터 %s\n", filepath.Join(p.dataDir, "notebook.db"))
	}
	if len(askLinks) > 0 {
		fmt.Fprintf(w, "  창구   %s   (동료에게 줄 요청 보내기 링크)\n", askLinks[0])
		for _, l := range askLinks[1:] {
			fmt.Fprintf(w, "         %s\n", l)
		}
		fmt.Fprintln(w, "         같은 망의 누구나 요청을 보낼 수 있습니다. 수첩 화면은 이 PC에서만 열립니다.")
		fmt.Fprintln(w, "         Windows 방화벽 창이 뜨면 [액세스 허용]을 눌러야 동료가 들어올 수 있습니다.")
	}
	fmt.Fprintln(w, "  끄기   이 창을 닫거나 Ctrl+C")
	fmt.Fprintln(w, "")
}

// waitBeforeClose keeps a double-clicked console window open long enough to
// read an error. It only waits when stdin is a console.
func waitBeforeClose(goos string) {
	if goos != "windows" {
		return
	}
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return
	}
	fmt.Fprintln(os.Stderr, "엔터를 누르면 창을 닫습니다.")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	done := make(chan struct{})
	go func() {
		var b [1]byte
		os.Stdin.Read(b[:]) //nolint:errcheck
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
