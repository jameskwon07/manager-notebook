package main

import (
	"errors"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestParseOptions(t *testing.T) {
	o, err := parseOptions([]string{"--demo", "--port", "9000", "--no-browser", "--data", " C:\\nb "})
	if err != nil {
		t.Fatal(err)
	}
	if !o.demo || !o.noBrowser || o.port != 9000 || o.dataDir != "C:\\nb" {
		t.Fatalf("got %+v", o)
	}
	if _, err := parseOptions([]string{"serve"}); err == nil {
		t.Fatal("stray argument should fail")
	}
	if _, err := parseOptions([]string{"--port", "70000"}); err == nil {
		t.Fatal("port out of range should fail")
	}
	if _, err := parseOptions([]string{"--nope"}); err == nil {
		t.Fatal("unknown flag should fail")
	}
}

func TestMakePlanContainer(t *testing.T) {
	// The image sets both variables: listen on all interfaces, data in /data, no browser.
	p, err := makePlan("linux", env(map[string]string{"NOTEBOOK_ADDR": ":8080", "NOTEBOOK_DATA_DIR": "/data"}), options{})
	if err != nil {
		t.Fatal(err)
	}
	if p.desktop || p.openBrowser || p.dataDir != "/data" || p.addr(p.ports[0]) != ":8080" || len(p.ports) != 1 {
		t.Fatalf("container plan: %+v addr=%s", p, p.addr(p.ports[0]))
	}
	// --demo in a container keeps data in memory and leaves /data alone.
	p, _ = makePlan("linux", env(map[string]string{"NOTEBOOK_ADDR": ":8080", "NOTEBOOK_DATA_DIR": "/data"}), options{demo: true})
	if p.dataDir != "" || p.addr(p.ports[0]) != ":8080" {
		t.Fatalf("container demo: %+v", p)
	}
}

func TestMakePlanWindowsDesktop(t *testing.T) {
	e := env(map[string]string{"LOCALAPPDATA": filepath.Join("C:", "Users", "u", "AppData", "Local")})
	p, err := makePlan("windows", e, options{})
	if err != nil {
		t.Fatal(err)
	}
	if !p.desktop || !p.openBrowser {
		t.Fatalf("desktop plan: %+v", p)
	}
	if want := filepath.Join("C:", "Users", "u", "AppData", "Local", "manager-notebook"); p.dataDir != want {
		t.Fatalf("dataDir %q want %q", p.dataDir, want)
	}
	if p.addr(p.ports[0]) != "127.0.0.1:8080" || len(p.ports) != 10 || p.ports[9] != 8089 {
		t.Fatalf("ports %v addr %s", p.ports, p.addr(p.ports[0]))
	}
	// Demo uses 8090.. and memory.
	p, _ = makePlan("windows", e, options{demo: true, noBrowser: true})
	if p.dataDir != "" || p.ports[0] != 8090 || p.openBrowser {
		t.Fatalf("demo plan: %+v", p)
	}
	// --port fixes the port, --data wins over the default folder.
	p, _ = makePlan("windows", e, options{port: 9100, dataDir: "D:\\nb"})
	if len(p.ports) != 1 || p.addr(9100) != "127.0.0.1:9100" || p.dataDir != "D:\\nb" {
		t.Fatalf("fixed plan: %+v", p)
	}
	// NOTEBOOK_NO_BROWSER turns the browser off.
	p, _ = makePlan("windows", env(map[string]string{"LOCALAPPDATA": "L", "NOTEBOOK_NO_BROWSER": "1"}), options{})
	if p.openBrowser {
		t.Fatal("NOTEBOOK_NO_BROWSER should stop the browser")
	}
}

func TestUserDataDir(t *testing.T) {
	cases := []struct {
		goos string
		env  map[string]string
		want string
	}{
		{"windows", map[string]string{"USERPROFILE": "U"}, filepath.Join("U", "AppData", "Local", "manager-notebook")},
		{"darwin", map[string]string{"HOME": "H"}, filepath.Join("H", "Library", "Application Support", "manager-notebook")},
		{"linux", map[string]string{"XDG_DATA_HOME": "X", "HOME": "H"}, filepath.Join("X", "manager-notebook")},
		{"linux", map[string]string{"HOME": "H"}, filepath.Join("H", ".local", "share", "manager-notebook")},
	}
	for _, c := range cases {
		got, err := userDataDir(c.goos, env(c.env))
		if err != nil || got != c.want {
			t.Errorf("%s %v: %q %v, want %q", c.goos, c.env, got, err, c.want)
		}
	}
	if _, err := userDataDir("windows", env(nil)); err == nil {
		t.Error("no profile folder should fail")
	}
}

// freePort returns a port that is free right now.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestListenPlanSkipsBusyPort(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port
	next := freePort(t)

	old := probeNotebook
	defer func() { probeNotebook = old }()
	probeNotebook = func(int) (string, bool) { return "", false } // something else holds the port

	p := launchPlan{desktop: true, host: "127.0.0.1", ports: []int{busyPort, next}}
	ln, port, err := listenPlan(p)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if port != next {
		t.Fatalf("port %d, want %d", port, next)
	}
}

func TestListenPlanReusesRunningNotebook(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port

	old := probeNotebook
	defer func() { probeNotebook = old }()
	probeNotebook = func(port int) (string, bool) { return "2.1.0", port == busyPort }

	p := launchPlan{desktop: true, host: "127.0.0.1", ports: []int{busyPort, freePort(t)}}
	_, port, err := listenPlan(p)
	var running errAlreadyRunning
	if !errors.As(err, &running) || port != busyPort || running.version != "2.1.0" {
		t.Fatalf("want errAlreadyRunning on %d, got port %d err %v", busyPort, port, err)
	}
	if !strings.Contains(running.Error(), strconv.Itoa(busyPort)) {
		t.Fatalf("message should name the port: %s", running.Error())
	}
}

func TestListenPlanFixedPortBusy(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port
	old := probeNotebook
	defer func() { probeNotebook = old }()
	probeNotebook = func(int) (string, bool) { return "", false }

	_, _, err = listenPlan(launchPlan{desktop: true, host: "127.0.0.1", ports: []int{busyPort}})
	if err == nil || !strings.Contains(err.Error(), "--port") {
		t.Fatalf("want a hint about --port, got %v", err)
	}
}

func TestPrintDesktopBanner(t *testing.T) {
	var b strings.Builder
	printDesktopBanner(&b, launchPlan{desktop: true, dataDir: "D"}, 8081, "2.1.0")
	out := b.String()
	for _, want := range []string{"http://localhost:8081/", filepath.Join("D", "notebook.db"), "Ctrl+C"} {
		if !strings.Contains(out, want) {
			t.Errorf("banner lacks %q:\n%s", want, out)
		}
	}
	b.Reset()
	printDesktopBanner(&b, launchPlan{desktop: true, demo: true}, 8090, "2.1.0")
	if !strings.Contains(b.String(), "체험 모드") || strings.Contains(b.String(), "notebook.db") {
		t.Errorf("demo banner:\n%s", b.String())
	}
}
