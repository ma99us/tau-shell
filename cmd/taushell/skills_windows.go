//go:build windows

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ma99us/tau-shell/internal/appconfig"
	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/harness"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/sessionhost"
	"github.com/ma99us/tau-shell/internal/skills"
)

const skillCheckInterval = 24 * time.Hour

// personalSkillDirs lists the user's own skill folders per harness; skills
// found there win over TauShell's copies.
func personalSkillDirs() map[string][]string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	claude := filepath.Join(home, ".claude")
	if override := os.Getenv("CLAUDE_CONFIG_DIR"); override != "" {
		claude = override
	}
	return map[string][]string{
		string(session.ClaudeCode): {filepath.Join(claude, "skills")},
		string(session.CopilotCLI): {filepath.Join(home, ".copilot", "skills"), filepath.Join(home, ".agents", "skills")},
	}
}

// initSkills opens the skill library. A broken config.yaml leaves skills off
// for this run and is logged; sessions still launch.
func (application *App) initSkills(manager *sessionhost.Manager) {
	settings, err := appconfig.Load(application.settingsPath())
	if err != nil {
		diagnostics.Error("load settings for skills failed; skills are off", err)
		return
	}
	library, err := skills.NewLibrary(filepath.Dir(application.configDirectory), settings.Skills.Resolve(), personalSkillDirs())
	if err != nil {
		diagnostics.Error("open skill library failed; skills are off", err)
		return
	}
	application.skills = library
	library.GC()
	manager.SetSkills(library)
}

// startSkillChecks looks for new cookbook commits in the background at
// startup and then daily.
func (application *App) startSkillChecks(ctx context.Context) {
	if application.skills == nil {
		return
	}
	go func() {
		application.checkSkills(ctx)
		ticker := time.NewTicker(skillCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				application.checkSkills(ctx)
			}
		}
	}()
}

func (application *App) checkSkills(ctx context.Context) {
	if err := application.skills.Check(ctx); err != nil {
		diagnostics.Warn("skill update check failed", err)
		return
	}
	diagnostics.Info("skill update check finished")
}

// SkillRow is one skill in the Home Skills panel.
type SkillRow struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Source      string   `json:"source"`
	Enabled     bool     `json:"enabled"`
	Personal    []string `json:"personal,omitempty"`
}

// SkillsView is the Skills panel content.
type SkillsView struct {
	Status skills.Status `json:"status"`
	Skills []SkillRow    `json:"skills"`
	// Unsupported lists installed tools whose version has no --plugin-dir.
	Unsupported []string `json:"unsupported,omitempty"`
}

func (application *App) skillsView(ctx context.Context) SkillsView {
	if application.skills == nil {
		return SkillsView{Skills: []SkillRow{}}
	}
	view := SkillsView{Status: application.skills.Status(), Skills: []SkillRow{}}
	index := map[string]int{}
	for _, tool := range []session.Harness{session.ClaudeCode, session.CopilotCLI} {
		for _, entry := range application.skills.List(string(tool)) {
			position, seen := index[entry.Name]
			if !seen {
				position = len(view.Skills)
				index[entry.Name] = position
				view.Skills = append(view.Skills, SkillRow{Name: entry.Name, Description: entry.Description, Source: entry.Source, Enabled: entry.Enabled})
			}
			if entry.Personal {
				view.Skills[position].Personal = append(view.Skills[position].Personal, string(tool))
			}
		}
		adapter, err := harness.For(tool)
		if err != nil {
			continue
		}
		if executable, err := adapter.FindExecutable(); err == nil && !harness.SupportsPluginDir(ctx, executable) {
			view.Unsupported = append(view.Unsupported, string(tool))
		}
	}
	return view
}

// GetSkills returns the skill library state for the Skills panel.
func (application *App) GetSkills() SkillsView {
	return application.skillsView(context.Background())
}

// CheckSkills looks for a newer cookbook commit now (the Refresh button).
func (application *App) CheckSkills() SkillsView {
	if application.skills != nil {
		application.checkSkills(context.Background())
	}
	return application.skillsView(context.Background())
}

// UpdateSkills applies the newest cookbook commit found by a check.
func (application *App) UpdateSkills() (view SkillsView, err error) {
	if application.skills == nil {
		return SkillsView{Skills: []SkillRow{}}, errors.New("skills are not available")
	}
	defer func() {
		if err != nil {
			diagnostics.Error("update skills failed", err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	err = application.skills.Apply(ctx)
	return application.skillsView(ctx), err
}

// SetSkillEnabled turns one skill on or off for sessions launched afterwards.
func (application *App) SetSkillEnabled(name string, enabled bool) (err error) {
	if application.skills == nil {
		return errors.New("skills are not available")
	}
	defer func() {
		if err != nil {
			diagnostics.Error("set skill enabled failed", err, "skill", name)
		}
	}()
	return application.skills.SetEnabled(name, enabled)
}
