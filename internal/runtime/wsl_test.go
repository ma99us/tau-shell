package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

func newTestWSL(run Runner) *WSL {
	return &WSL{
		distro:   "Ubuntu",
		lookPath: func(name string) (string, error) { return `C:\Windows\System32\` + name, nil },
		run:      run,
	}
}

func utf16le(text string, bom bool) []byte {
	var out []byte
	if bom {
		out = append(out, 0xFF, 0xFE)
	}
	for _, unit := range utf16.Encode([]rune(text)) {
		out = append(out, byte(unit), byte(unit>>8))
	}
	return out
}

func TestWSLCommandUsesExecWithoutShell(t *testing.T) {
	command, err := newTestWSL(nil).Command(LaunchSpec{
		Executable: "/home/u/.local/node/bin/claude",
		Args:       []string{"--resume", "a b", "$HOME"},
		WorkingDir: `C:\Altura\Projects\app`,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-d", "Ubuntu", "--cd", "/mnt/c/Altura/Projects/app", "--exec", "/usr/bin/env",
		"PATH=/home/u/.local/node/bin:" + defaultPath, "/home/u/.local/node/bin/claude", "--resume", "a b", "$HOME"}
	if command.Executable != "wsl.exe" || !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("command = %q %q", command.Executable, command.Args)
	}
}

func TestWSLCommandRejectsBadInput(t *testing.T) {
	wsl := newTestWSL(nil)
	if _, err := wsl.Command(LaunchSpec{Executable: "claude", WorkingDir: "/x"}); err == nil {
		t.Fatal("relative harness path accepted")
	}
	if _, err := wsl.Command(LaunchSpec{Executable: "/bin/x", WorkingDir: `\\wsl.localhost\Debian\home\u`}); err == nil {
		t.Fatal("other distro directory accepted")
	}
	if _, err := NewWSL("bad name; rm"); err == nil {
		t.Fatal("unsafe distro accepted")
	}
}

func TestWSLPathMapping(t *testing.T) {
	wsl := newTestWSL(nil)
	toNative := map[string]string{
		`C:\a\b`:                          "/mnt/c/a/b",
		`d:/x y/z/`:                       "/mnt/d/x y/z",
		`C:\`:                             "/mnt/c",
		`\\wsl.localhost\ubuntu\home\u\p`: "/home/u/p",
		`\\wsl$\Ubuntu\home\u\p\`:         "/home/u/p",
		`\\wsl.localhost\Ubuntu`:          "/",
		"/home/u//p/":                     "/home/u/p",
		`C:\Users\Zo\u00eb`:               "/mnt/c/Users/Zo/u00eb",
	}
	for input, want := range toNative {
		got, err := wsl.ToNativePath(input)
		if err != nil || got != want {
			t.Errorf("ToNativePath(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, bad := range []string{`\\wsl.localhost\Debian\home`, `\\server\share`, "relative/dir", "", "/a\nb"} {
		if got, err := wsl.ToNativePath(bad); err == nil {
			t.Errorf("ToNativePath(%q) = %q, want error", bad, got)
		}
	}
	toWindows := map[string]string{
		"/mnt/c/a/b": `C:\a\b`,
		"/mnt/d":     `D:\`,
		"/home/u/p":  `\\wsl.localhost\Ubuntu\home\u\p`,
		"/mntx/foo":  `\\wsl.localhost\Ubuntu\mntx\foo`,
	}
	for input, want := range toWindows {
		got, err := wsl.ToWindowsPath(input)
		if err != nil || got != want {
			t.Errorf("ToWindowsPath(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestWSLEnvironmentForwardsNamesNotValues(t *testing.T) {
	environment, err := newTestWSL(nil).Environment(map[string]string{"B_KEY": "secret", "A_KEY": "1", "WSLENV": "evil"}, "token-12345678")
	if err != nil {
		t.Fatal(err)
	}
	if environment["WSLENV"] != "A_KEY:B_KEY:"+LaunchMarker || environment[LaunchMarker] != "token-12345678" || environment["B_KEY"] != "secret" {
		t.Fatalf("environment = %v", environment)
	}
	if _, err := newTestWSL(nil).Environment(nil, "bad token!"); err == nil {
		t.Fatal("invalid token accepted")
	}
}

func TestWSLFindToolAndCleanupRunScriptsThroughExec(t *testing.T) {
	var calls [][]string
	wsl := newTestWSL(func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, args)
		if args[len(args)-2] == "sh" && args[len(args)-1] == "claude" {
			return []byte("/home/u/.local/node/bin/claude\n"), nil
		}
		return []byte("3\n"), nil
	})
	path, err := wsl.FindTool(context.Background(), "claude")
	if err != nil || path != "/home/u/.local/node/bin/claude" {
		t.Fatalf("FindTool = %q, %v", path, err)
	}
	if _, err := wsl.FindTool(context.Background(), "x; rm -rf /"); err == nil {
		t.Fatal("unsafe tool name accepted")
	}
	count, err := wsl.Cleanup(context.Background(), "abcdef12-3456")
	if err != nil || count != 3 {
		t.Fatalf("Cleanup = %d, %v", count, err)
	}
	for _, call := range calls {
		if call[0] != "-d" || call[1] != "Ubuntu" || call[2] != "--exec" || call[3] != "sh" {
			t.Fatalf("call = %q", call)
		}
	}
	if _, err := wsl.Cleanup(context.Background(), "x;y"); err == nil {
		t.Fatal("unsafe token accepted")
	}
}

func TestWSLFindToolMissing(t *testing.T) {
	wsl := newTestWSL(func(context.Context, ...string) ([]byte, error) { return nil, errors.New("exit 1") })
	if _, err := wsl.FindTool(context.Background(), "claude"); err == nil || !strings.Contains(err.Error(), "not found inside WSL") {
		t.Fatalf("err = %v", err)
	}
}

func TestWSLAvailableReportsMissingDistro(t *testing.T) {
	wsl := newTestWSL(func(context.Context, ...string) ([]byte, error) { return nil, errors.New("exit -1") })
	if err := wsl.Available(); err == nil || !strings.Contains(err.Error(), `"Ubuntu"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestDecodeOutputUTF16(t *testing.T) {
	for _, bom := range []bool{true, false} {
		if got := DecodeOutput(utf16le("a\r\nb \u00e9\r\n", bom)); got != "a\nb \u00e9\n" {
			t.Fatalf("bom=%v got %q", bom, got)
		}
	}
}

func TestParseVersionIgnoresLabels(t *testing.T) {
	english := "WSL version: 2.6.2.0\nKernel version: 6.6.87.2-1\nWSLg version: 1.0.66\nMSRDC version: 1.2.6353\nWindows version: 10.0.26100.1\n"
	german := "WSL-Version: 2.6.2.0\nKernelversion: 6.6.87.2-1\nWSLg-Version: 1.0.66\n"
	cjk := "WSL \u7248\u672c\uFF1A 2.6.2.0\n\u5185\u6838\u7248\u672c\uFF1A 6.6.87.2-1\nWSLg \u7248\u672c\uFF1A 1.0.66\n"
	want := Status{WSLVersion: "2.6.2.0", KernelVersion: "6.6.87.2", WSLgVersion: "1.0.66"}
	for name, text := range map[string]string{"en": english, "de": german, "cjk": cjk} {
		if got := ParseVersion(text); got != want {
			t.Errorf("%s: got %+v", name, got)
		}
	}
	if got := ParseVersion("Usage: wsl.exe [Argument]\n"); got != (Status{}) {
		t.Errorf("stub output parsed as %+v", got)
	}
}

func TestParseDistros(t *testing.T) {
	verbose := "  NAME                   STATE           VERSION\n* Ubuntu                 Running         2\n  docker-desktop         Stopped         2\n  Legacy                 Stopped         1\n"
	distros := ParseDistros(verbose, "Ubuntu\n")
	want := []Distro{
		{Name: "Ubuntu", Default: true, Running: true, Version: 2, State: "Running"},
		{Name: "docker-desktop", Version: 2, State: "Stopped"},
		{Name: "Legacy", Version: 1, State: "Stopped"},
	}
	if !reflect.DeepEqual(distros, want) {
		t.Fatalf("distros = %+v", distros)
	}
	if got := ParseDistros("", ""); len(got) != 0 {
		t.Fatalf("empty input gave %+v", got)
	}
}

func TestInventoryDecodesBothCalls(t *testing.T) {
	run := func(_ context.Context, args ...string) ([]byte, error) {
		switch {
		case args[0] == "--version":
			return utf16le("WSL version: 2.6.2.0\r\nKernel version: 6.6.87.2-1\r\nWSLg version: 1.0.66\r\n", true), nil
		case len(args) == 2:
			return utf16le("  NAME STATE VERSION\r\n* Ubuntu Running 2\r\n", true), nil
		default:
			return utf16le("Ubuntu\r\n", true), nil
		}
	}
	status, distros, err := Inventory(context.Background(), run)
	if err != nil || status.WSLVersion != "2.6.2.0" || len(distros) != 1 || !distros[0].Running {
		t.Fatalf("Inventory = %+v %+v %v", status, distros, err)
	}
}
