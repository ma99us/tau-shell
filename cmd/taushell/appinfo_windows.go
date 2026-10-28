//go:build windows

package main

import (
	"context"
	"net/http"

	"github.com/ma99us/tau-shell/internal/appupdate"
	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// version is set at release build time with -ldflags "-X main.version=x.y.z".
var version = "dev"

// AppUpdate describes a newer published TauShell release.
type AppUpdate struct {
	Tag string `json:"tag"`
	URL string `json:"url"`
}

// GetAppVersion returns the running TauShell version ("dev" for local builds).
func (application *App) GetAppVersion() string {
	return version
}

// CheckAppUpdate reports a newer GitHub release, or nil when up to date, the
// build is a development build, or the check fails (failures are logged).
func (application *App) CheckAppUpdate() *AppUpdate {
	if !appupdate.Comparable(version) {
		return nil
	}
	release, ok, err := appupdate.Latest(context.Background(), http.DefaultClient, appupdate.ReleasesAPI)
	if err != nil {
		diagnostics.Warn("check for TauShell update failed", err)
		return nil
	}
	if !ok || !appupdate.Newer(version, release.Tag) {
		return nil
	}
	return &AppUpdate{Tag: release.Tag, URL: release.URL}
}

// OpenReleasePage opens the project's release page in the default browser.
// The URL is fixed so a WebView caller cannot open arbitrary addresses.
func (application *App) OpenReleasePage() {
	if application.openURL != nil {
		application.openURL(application.ctx, appupdate.ReleasesPrefix)
		return
	}
	runtime.BrowserOpenURL(application.ctx, appupdate.ReleasesPrefix)
}
