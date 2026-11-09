package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/ma99us/tau-shell/internal/session"
)

// LaunchMarker is the environment variable that tags every process of one
// WSL launch, so leftovers can be found and stopped after the host process is
// killed (a setsid or nohup child survives terminating wsl.exe).
const LaunchMarker = "TAUSHELL_LAUNCH"

var (
	launchTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
	toolNamePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	versionPattern     = regexp.MustCompile(`\d+(?:\.\d+)+`)
	mntPath            = regexp.MustCompile("^/mnt/([a-z])(?:/(.*))?$")
	driveRoot          = regexp.MustCompile(`^([A-Za-z]):(?:[\\/]|$)`)
	uncPrefix          = regexp.MustCompile(`^\\\\(?:wsl\.localhost|wsl\$)\\([^\\]+)(?:\\(.*))?$`)
	// Standard directories for a harness started without a login shell. The
	// harness's own folder is prepended so its Node shebang picks the matching node.
	defaultPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

// findToolScript prints the first executable named $1 in the usual user-level
// install folders or on PATH, skipping Windows interop entries under /mnt.
const findToolScript = `for d in "$HOME/.local/node/bin" "$HOME/.local/bin" "$HOME/.npm-global/bin"; do
  if [ -x "$d/$1" ] && [ ! -d "$d/$1" ]; then printf '%s\n' "$d/$1"; exit 0; fi
done
IFS=:
for d in $PATH; do
  case "$d" in /mnt/*) continue;; esac
  if [ -x "$d/$1" ] && [ ! -d "$d/$1" ]; then printf '%s\n' "$d/$1"; exit 0; fi
done
exit 1`

// cleanupScript stops every process whose environment carries the launch
// marker, politely first, then by force. It prints the number found.
const cleanupScript = `marker="TAUSHELL_LAUNCH=$1"
found=""
for p in /proc/[0-9]*; do
  pid=${p#/proc/}
  [ "$pid" = "$$" ] && continue
  if tr '\0' '\n' < "$p/environ" 2>/dev/null | grep -qxF "$marker"; then found="$found $pid"; fi
done
count=0
for pid in $found; do kill -TERM "$pid" 2>/dev/null && count=$((count+1)); done
if [ "$count" -gt 0 ]; then
  sleep 1
  for pid in $found; do kill -KILL "$pid" 2>/dev/null; done
fi
printf '%s\n' "$count"`

// Runner runs wsl.exe with arguments and returns its stdout.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

func execRunner(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "wsl.exe", args...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("wsl.exe %s: %w", strings.Join(args, " "), err)
	}
	return stdout.Bytes(), nil
}

// WSL runs a session's harness inside a WSL2 distro through wsl.exe.
type WSL struct {
	distro   string
	lookPath func(string) (string, error)
	run      Runner
}

// NewWSL returns the runtime for one distro.
func NewWSL(distro string) (*WSL, error) {
	if err := session.ValidateWSLDistro(distro); err != nil {
		return nil, err
	}
	return &WSL{distro: distro, lookPath: exec.LookPath, run: execRunner}, nil
}

func (runtime *WSL) Kind() Kind { return KindWSL }

// Distro is the distro name this runtime targets.
func (runtime *WSL) Distro() string { return runtime.distro }

// Available proves readiness by starting the distro and running a trivial
// command; the optional-feature query needs elevation and the firmware flag
// reads False while WSL2 runs.
func (runtime *WSL) Available() error {
	if _, err := runtime.lookPath("wsl.exe"); err != nil {
		return fmt.Errorf("WSL (wsl.exe) is not installed: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := runtime.run(ctx, "-d", runtime.distro, "--exec", "true"); err != nil {
		return fmt.Errorf("WSL distro %q could not be started: %w", runtime.distro, err)
	}
	return nil
}

// Command builds `wsl.exe -d D --cd DIR --exec env PATH=... HARNESS ARGS...`.
// --exec uses argv directly, so no shell parses managed or user arguments.
func (runtime *WSL) Command(spec LaunchSpec) (Command, error) {
	if !strings.HasPrefix(spec.Executable, "/") {
		return Command{}, fmt.Errorf("WSL harness path %q must be absolute", spec.Executable)
	}
	if spec.WorkingDir == "" {
		return Command{}, errors.New("WSL launch needs a working directory")
	}
	directory, err := runtime.ToNativePath(spec.WorkingDir)
	if err != nil {
		return Command{}, err
	}
	folder := spec.Executable[:strings.LastIndex(spec.Executable, "/")]
	if folder == "" {
		folder = "/"
	}
	args := []string{"-d", runtime.distro, "--cd", directory, "--exec", "/usr/bin/env", "PATH=" + folder + ":" + defaultPath, spec.Executable}
	return Command{Executable: "wsl.exe", Args: append(args, spec.Args...)}, nil
}

// Environment returns the Windows-side environment for wsl.exe: the session's
// variables plus the launch marker, with WSLENV naming every variable so it
// is forwarded into the distro. Values never appear on a command line.
func (runtime *WSL) Environment(environment map[string]string, launchToken string) (map[string]string, error) {
	if !launchTokenPattern.MatchString(launchToken) {
		return nil, errors.New("invalid launch token")
	}
	result := make(map[string]string, len(environment)+2)
	names := make([]string, 0, len(environment)+1)
	for name, value := range environment {
		if name == "WSLENV" || name == LaunchMarker {
			continue
		}
		result[name] = value
		names = append(names, name)
	}
	sort.Strings(names)
	result[LaunchMarker] = launchToken
	names = append(names, LaunchMarker)
	result["WSLENV"] = strings.Join(names, ":")
	return result, nil
}

// HomeDir is the distro user's home as a UNC path readable from Windows.
func (runtime *WSL) HomeDir() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output, err := runtime.run(ctx, "-d", runtime.distro, "--exec", "sh", "-c", `printf '%s' "$HOME"`)
	if err != nil {
		return "", err
	}
	home := strings.TrimSpace(string(output))
	if !strings.HasPrefix(home, "/") {
		return "", fmt.Errorf("WSL home %q is not absolute", home)
	}
	return runtime.ToWindowsPath(home)
}

// FindTool returns the absolute Linux path of a command inside the distro.
func (runtime *WSL) FindTool(ctx context.Context, name string) (string, error) {
	if !toolNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid tool name %q", name)
	}
	output, err := runtime.run(ctx, "-d", runtime.distro, "--exec", "sh", "-c", findToolScript, "sh", name)
	path := strings.TrimSpace(string(output))
	if err != nil || !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("%s was not found inside WSL distro %q", name, runtime.distro)
	}
	return path, nil
}

// Cleanup stops processes left behind by a launch and returns how many it found.
func (runtime *WSL) Cleanup(ctx context.Context, launchToken string) (int, error) {
	if !launchTokenPattern.MatchString(launchToken) {
		return 0, errors.New("invalid launch token")
	}
	output, err := runtime.run(ctx, "-d", runtime.distro, "--exec", "sh", "-c", cleanupScript, "sh", launchToken)
	if err != nil {
		return 0, err
	}
	count, parseErr := strconv.Atoi(strings.TrimSpace(string(output)))
	if parseErr != nil {
		return 0, fmt.Errorf("unexpected WSL cleanup output %q", strings.TrimSpace(string(output)))
	}
	return count, nil
}

// ToNativePath maps a Windows path, or a path in this distro's UNC share, to
// the path the harness sees. Paths that cannot be mapped are rejected.
func (runtime *WSL) ToNativePath(path string) (string, error) {
	if strings.ContainsAny(path, "\x00\r\n") {
		return "", errors.New("path contains control characters")
	}
	if strings.HasPrefix(path, "/") {
		return cleanLinux(path), nil
	}
	if match := uncPrefix.FindStringSubmatch(path); match != nil {
		if !strings.EqualFold(match[1], runtime.distro) {
			return "", fmt.Errorf("path %q is in WSL distro %q, not %q", path, match[1], runtime.distro)
		}
		return cleanLinux("/" + strings.ReplaceAll(match[2], `\`, "/")), nil
	}
	if match := driveRoot.FindStringSubmatch(path); match != nil {
		rest := strings.ReplaceAll(path[len(match[0]):], `\`, "/")
		return cleanLinux("/mnt/" + strings.ToLower(match[1]) + "/" + rest), nil
	}
	return "", fmt.Errorf("path %q cannot be mapped into WSL", path)
}

// ToWindowsPath maps a path inside the distro to one Windows can open.
func (runtime *WSL) ToWindowsPath(path string) (string, error) {
	if strings.ContainsAny(path, "\x00\r\n") || !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("%q is not an absolute Linux path", path)
	}
	path = cleanLinux(path)
	if match := regexp.MustCompile(`^/mnt/([a-z])(?:/(.*))?$`).FindStringSubmatch(path); match != nil {
		return strings.ToUpper(match[1]) + `:\` + strings.ReplaceAll(match[2], "/", `\`), nil
	}
	return `\\wsl.localhost\` + runtime.distro + strings.ReplaceAll(path, "/", `\`), nil
}

func cleanLinux(path string) string {
	parts := strings.Split(path, "/")
	kept := parts[:0]
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return "/" + strings.Join(kept, "/")
}

// DecodeOutput converts wsl.exe's own UTF-16LE output to text. Output of
// commands run inside the distro is UTF-8 and must not go through this.
func DecodeOutput(raw []byte) string {
	if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE {
		raw = raw[2:]
	}
	if len(raw)%2 == 1 {
		raw = raw[:len(raw)-1]
	}
	units := make([]uint16, len(raw)/2)
	for index := range units {
		units[index] = uint16(raw[2*index]) | uint16(raw[2*index+1])<<8
	}
	text := string(utf16.Decode(units))
	return strings.ReplaceAll(text, "\r\n", "\n")
}

// Distro is one installed WSL distribution.
type Distro struct {
	Name    string
	Default bool
	Running bool
	Version int
	State   string // as printed by wsl.exe, possibly localized
}

// Status is what `wsl --version` reports. Labels are localized, so values are
// taken by position: WSL, kernel, WSLg.
type Status struct {
	WSLVersion    string
	KernelVersion string
	WSLgVersion   string
}

// ParseVersion reads `wsl --version` output without matching label text.
func ParseVersion(text string) Status {
	var found []string
	for _, line := range strings.Split(text, "\n") {
		separator := strings.IndexAny(line, ":\uFF1A")
		if separator < 0 {
			continue
		}
		if value := versionPattern.FindString(line[separator:]); value != "" {
			found = append(found, value)
		}
	}
	var status Status
	if len(found) > 0 {
		status.WSLVersion = found[0]
	}
	if len(found) > 1 {
		status.KernelVersion = found[1]
	}
	if len(found) > 2 {
		status.WSLgVersion = found[2]
	}
	return status
}

// ParseDistros reads `wsl --list --verbose` (header line skipped) plus the
// names from `wsl --list --running --quiet`. State text is kept as printed;
// running state comes from the structured quiet list.
func ParseDistros(verbose, running string) []Distro {
	runningNames := map[string]bool{}
	for _, line := range strings.Split(running, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			runningNames[name] = true
		}
	}
	lines := strings.Split(verbose, "\n")
	var distros []Distro
	for index, line := range lines {
		if index == 0 {
			continue
		}
		fields := strings.Fields(line)
		isDefault := false
		if len(fields) > 0 && fields[0] == "*" {
			isDefault, fields = true, fields[1:]
		}
		if len(fields) < 3 {
			continue
		}
		version, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil {
			continue
		}
		name := strings.Join(fields[:len(fields)-2], " ")
		distros = append(distros, Distro{
			Name: name, Default: isDefault, Running: runningNames[name],
			Version: version, State: fields[len(fields)-2],
		})
	}
	return distros
}

// Inventory lists distros and the WSL version through wsl.exe.
func Inventory(ctx context.Context, run Runner) (Status, []Distro, error) {
	if run == nil {
		run = execRunner
	}
	versionOutput, err := run(ctx, "--version")
	if err != nil {
		return Status{}, nil, err
	}
	status := ParseVersion(DecodeOutput(versionOutput))
	verbose, err := run(ctx, "--list", "--verbose")
	if err != nil {
		return status, nil, err
	}
	runningOutput, _ := run(ctx, "--list", "--running", "--quiet")
	return status, ParseDistros(DecodeOutput(verbose), DecodeOutput(runningOutput)), nil
}
