//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/harness/manifest"
)

// loadHarnessManifests installs the built-in and user harness manifests for
// this run. User manifests load once at startup, so edits need a restart. A bad
// file is logged and skipped; it never prevents TauShell from starting.
func loadHarnessManifests() {
	home, err := os.UserHomeDir()
	if err != nil {
		diagnostics.Warn("user harness manifests unavailable", err)
		return
	}
	root := filepath.Join(home, ".tau_shell")
	registry := manifest.Load(filepath.Join(root, "harnesses"))
	manifest.SetDefault(registry)
	for _, failure := range registry.Errors() {
		diagnostics.Warn("harness manifest skipped", fmt.Errorf("%s", failure.Error), "path", failure.Path)
	}
	trust, err := manifest.OpenTrust(filepath.Join(root, "harness-trust.json"))
	if err != nil {
		diagnostics.Warn("harness trust file unreadable; user harnesses stay unconfirmed", err)
		trust = manifest.NewTrust(filepath.Join(root, "harness-trust.json"))
	}
	manifest.SetDefaultTrust(trust)
}

// HarnessInfo is one harness for the UI: the manifest view plus what only the
// host knows.
type HarnessInfo struct {
	*manifest.Manifest
	Trusted bool `json:"trusted"`
	// ResolvedPath is where the executable was found on PATH, or "".
	ResolvedPath string `json:"resolvedPath,omitempty"`
	// ResumeVerified is false when no reader can confirm a conversation exists.
	ResumeVerified bool `json:"resumeVerified"`
}

// HarnessCatalog is what Home and the cue engine read at startup.
type HarnessCatalog struct {
	Harnesses []HarnessInfo        `json:"harnesses"`
	Errors    []manifest.LoadError `json:"errors"`
}

// GetHarnesses lists every harness with its cues, brand and trust state.
func (application *App) GetHarnesses() HarnessCatalog {
	catalog := HarnessCatalog{Harnesses: []HarnessInfo{}, Errors: manifest.Default().Errors()}
	for _, item := range manifest.All() {
		info := HarnessInfo{Manifest: item, Trusted: manifest.DefaultTrust().Trusted(item), ResumeVerified: item.ResumeCheck != ""}
		if path, err := exec.LookPath(item.Executable); err == nil {
			info.ResolvedPath = path
		}
		catalog.Harnesses = append(catalog.Harnesses, info)
	}
	return catalog
}

// ConfirmHarness records the user's confirmation of one user manifest as it is
// now. Editing the file later asks again.
func (application *App) ConfirmHarness(id string) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("confirm harness failed", err, "harness", id)
		}
	}()
	item, ok := manifest.Get(id)
	if !ok {
		return fmt.Errorf("unknown harness %q", id)
	}
	trust := manifest.DefaultTrust()
	if trust == nil {
		return fmt.Errorf("the harness trust store is unavailable")
	}
	if err := trust.Approve(item); err != nil {
		return err
	}
	diagnostics.Info("user harness manifest confirmed", "harness", id, "hash", item.Hash)
	return nil
}
