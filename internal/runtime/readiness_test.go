package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func fixtureRunner(version, verbose, running string) Runner {
	return func(_ context.Context, args ...string) ([]byte, error) {
		switch {
		case args[0] == "--version":
			if version == "" {
				return utf16le("Usage: wsl.exe [Argument] [Options...]\r\n", true), errors.New("exit 1")
			}
			return utf16le(version, true), nil
		case len(args) == 2:
			return utf16le(verbose, true), nil
		default:
			return utf16le(running, true), nil
		}
	}
}

func found(string) (string, error)    { return `C:\Windows\System32\wsl.exe`, nil }
func notFound(string) (string, error) { return "", errors.New("not found") }

func TestDetectReady(t *testing.T) {
	result := Detect(context.Background(), found, fixtureRunner(
		"WSL version: 2.6.2.0\r\nKernel version: 6.6.87.2-1\r\nWSLg version: 1.0.71\r\n",
		"  NAME STATE VERSION\r\n* Ubuntu Running 2\r\n  docker-desktop Stopped 2\r\n", "Ubuntu\r\n"))
	if !result.Installed || result.Version != "2.6.2.0" || result.DefaultDistro != "Ubuntu" || len(result.Distros) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Warnings) != 0 || !result.Checks[1].OK {
		t.Fatalf("warnings=%v checks=%+v", result.Warnings, result.Checks)
	}
}

func TestDetectNotInstalledAndStub(t *testing.T) {
	if result := Detect(context.Background(), notFound, nil); result.Installed || result.Error == "" {
		t.Fatalf("missing wsl.exe: %+v", result)
	}
	stub := Detect(context.Background(), found, fixtureRunner("", "", ""))
	if stub.Installed || !strings.Contains(stub.Error, "in-box stub") {
		t.Fatalf("stub: %+v", stub)
	}
}

func TestDetectNoDistroAndWSL1(t *testing.T) {
	version := "WSL version: 2.6.2.0\r\nKernel version: 6.6.87.2\r\nWSLg version: 1.0.71\r\n"
	none := Detect(context.Background(), found, fixtureRunner(version, "  NAME STATE VERSION\r\n", ""))
	if !none.Installed || none.Checks[1].OK || len(none.Warnings) == 0 {
		t.Fatalf("no distro: %+v", none)
	}
	legacy := Detect(context.Background(), found, fixtureRunner(version, "  NAME STATE VERSION\r\n* Old Stopped 1\r\n", ""))
	if legacy.Checks[1].OK || !strings.Contains(strings.Join(legacy.Warnings, " "), "only WSL 2") {
		t.Fatalf("wsl1: %+v", legacy)
	}
}
