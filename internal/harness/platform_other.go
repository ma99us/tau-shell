//go:build !windows

package harness

import "os/exec"

func hideWindow(*exec.Cmd) {}
