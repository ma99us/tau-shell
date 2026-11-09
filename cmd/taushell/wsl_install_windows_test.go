//go:build windows

package main

import (
	"testing"

	hostruntime "github.com/ma99us/tau-shell/internal/runtime"
)

func TestFinishWSLInstall(t *testing.T) {
	ready := hostruntime.Readiness{Installed: true, Distros: []hostruntime.Distro{{Name: "Ubuntu"}}}
	cases := []struct {
		action, distro, outcome string
		status                  hostruntime.Readiness
	}{
		{hostruntime.ActionInstallWSL, "", "installed", ready},
		{hostruntime.ActionInstallWSL, "", "restart-required", hostruntime.Readiness{}},
		{hostruntime.ActionInstallDistro, "ubuntu", "installed", ready},
		{hostruntime.ActionInstallDistro, "Debian", "failed", ready},
		{hostruntime.ActionUpdateWSL, "", "installed", ready},
		{hostruntime.ActionUpdateWSL, "", "failed", hostruntime.Readiness{}},
	}
	for _, test := range cases {
		if got := finishWSLInstall(test.action, test.distro, test.status).Outcome; got != test.outcome {
			t.Errorf("%s %q = %s, want %s", test.action, test.distro, got, test.outcome)
		}
	}
}
