//go:build windows

package main

import (
	"fmt"
	"sync/atomic"

	"github.com/ma99us/tau-shell/internal/appconfig"
	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// resolvedTheme is the theme the window shows ("light" or "dark"); the UI reports it
// because only the UI knows what "system" resolves to.
var resolvedTheme atomic.Value

func (application *App) colorScheme() string {
	value, _ := resolvedTheme.Load().(string)
	return value
}

// GetAppearance returns the saved theme and text sizes, with defaults applied.
// An unreadable settings file falls back to the defaults and reports the error
// through the settings-error event like other settings readers.
func (application *App) GetAppearance() appconfig.ResolvedAppearance {
	settings, err := appconfig.Load(application.settingsPath())
	if err != nil {
		diagnostics.Error("read appearance settings failed", err)
		return appconfig.Appearance{}.Resolve()
	}
	return settings.Appearance.Resolve()
}

// SetAppearance validates and saves the theme and text sizes (config.yaml is
// edited in place) and switches the window title bar to match.
func (application *App) SetAppearance(value appconfig.ResolvedAppearance) error {
	if err := appconfig.SetAppearance(application.settingsPath(), value); err != nil {
		diagnostics.Error("save appearance settings failed", err)
		return fmt.Errorf("Could not save the appearance settings: %w", err)
	}
	application.applyWindowTheme(value.Theme)
	return nil
}

// ReportTheme records the theme the UI resolved ("light" or "dark") so new
// sessions can advertise it to their harness through COLORFGBG.
func (application *App) ReportTheme(resolved string) {
	if resolved == appconfig.ThemeLight || resolved == appconfig.ThemeDark {
		resolvedTheme.Store(resolved)
	}
}

func (application *App) applyWindowTheme(theme string) {
	if application.ctx == nil {
		return
	}
	switch theme {
	case appconfig.ThemeLight:
		runtime.WindowSetLightTheme(application.ctx)
	case appconfig.ThemeSystem:
		runtime.WindowSetSystemDefaultTheme(application.ctx)
	default:
		runtime.WindowSetDarkTheme(application.ctx)
	}
}

// startupWindowTheme is the title bar theme before the UI is up.
func startupWindowTheme(theme string) windows.Theme {
	switch theme {
	case appconfig.ThemeLight:
		return windows.Light
	case appconfig.ThemeSystem:
		return windows.SystemDefault
	}
	return windows.Dark
}
