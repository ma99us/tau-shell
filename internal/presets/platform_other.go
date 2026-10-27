//go:build !windows

package presets

import "os/exec"

func hideWindow(*exec.Cmd) {}
