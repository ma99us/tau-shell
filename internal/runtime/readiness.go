package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Check is one readiness item for the Tools panel.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Readiness is the WSL tool's detected state. It never starts a distro and
// never installs anything.
type Readiness struct {
	Installed     bool     `json:"installed"`
	Version       string   `json:"version,omitempty"`
	KernelVersion string   `json:"kernelVersion,omitempty"`
	WSLgVersion   string   `json:"wslgVersion,omitempty"`
	DefaultDistro string   `json:"defaultDistro,omitempty"`
	Distros       []Distro `json:"distros"`
	Checks        []Check  `json:"checks"`
	Warnings      []string `json:"warnings"`
	Error         string   `json:"error,omitempty"`
}

// minimumWSLVersion is the first release that reports `wsl --version`.
const minimumWSLVersion = "2.0.0"

// helperDistros are distros WSL creates for other products; they cannot run a harness.
func helperDistro(name string) bool {
	return strings.HasPrefix(name, "docker-desktop")
}

// Detect inspects WSL through wsl.exe. lookPath and run default to the real ones.
func Detect(ctx context.Context, lookPath func(string) (string, error), run Runner) Readiness {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	result := Readiness{Distros: []Distro{}, Warnings: []string{}, Checks: []Check{}}
	if _, err := lookPath("wsl.exe"); err != nil {
		result.Error = "wsl.exe was not found; WSL is not installed on this Windows"
		result.Checks = append(result.Checks, Check{Name: "WSL installed", Detail: result.Error})
		return result
	}
	status, distros, err := Inventory(ctx, run)
	if status.WSLVersion == "" {
		result.Error = "WSL did not report a version; it may be the in-box stub or an old build. Install or update WSL (wsl --install --no-distribution, or wsl --update)."
		if err != nil {
			result.Error += " (" + err.Error() + ")"
		}
		result.Checks = append(result.Checks, Check{Name: "WSL installed", Detail: result.Error})
		return result
	}
	result.Installed = true
	result.Version, result.KernelVersion, result.WSLgVersion = status.WSLVersion, status.KernelVersion, status.WSLgVersion
	result.Checks = append(result.Checks, Check{Name: "WSL installed", OK: true, Detail: "version " + status.WSLVersion})
	if version, ok := dottedVersion(status.WSLVersion); ok {
		if minimum, _ := dottedVersion(minimumWSLVersion); version[0] < minimum[0] {
			result.Warnings = append(result.Warnings, fmt.Sprintf("WSL %s is older than %s; run wsl --update.", status.WSLVersion, minimumWSLVersion))
		}
	}
	if err != nil {
		result.Error = err.Error()
		result.Checks = append(result.Checks, Check{Name: "Distributions listed", Detail: err.Error()})
		return result
	}
	usable := 0
	for _, distro := range distros {
		if helperDistro(distro.Name) {
			continue
		}
		result.Distros = append(result.Distros, distro)
		if distro.Default {
			result.DefaultDistro = distro.Name
		}
		switch {
		case distro.Version != 2:
			result.Warnings = append(result.Warnings, fmt.Sprintf("Distro %s uses WSL %d; only WSL 2 is supported (wsl --set-version %s 2).", distro.Name, distro.Version, distro.Name))
		default:
			usable++
		}
	}
	result.Checks = append(result.Checks, Check{Name: "WSL 2 distro available", OK: usable > 0, Detail: fmt.Sprintf("%d usable", usable)})
	if usable == 0 {
		result.Warnings = append(result.Warnings, "No WSL 2 distro is installed; install one (wsl --install -d Ubuntu).")
	}
	return result
}

func dottedVersion(text string) ([]int, bool) {
	var parts []int
	for _, field := range strings.Split(text, ".") {
		number := 0
		if field == "" {
			return nil, false
		}
		for _, char := range field {
			if char < '0' || char > '9' {
				return nil, false
			}
			number = number*10 + int(char-'0')
		}
		parts = append(parts, number)
	}
	return parts, len(parts) > 0
}
