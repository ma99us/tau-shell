package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/ma99us/tau-shell/internal/mei"
	"github.com/ma99us/tau-shell/internal/models"
	"github.com/ma99us/tau-shell/internal/tools"
)

// ModelRanking says where the MEI ranking behind a model menu came from. The
// ranking is optional: when MEI is unreachable the menu still lists models.
type ModelRanking struct {
	// Source is "cache", "snapshot" or "none".
	Source string `json:"source"`
	// AsOf is the date of the ranking data (YYYY-MM-DD), empty when there is none.
	AsOf  string `json:"asOf,omitempty"`
	Stale bool   `json:"stale"`
	// Error is the latest failed MEI refresh, empty when the last refresh worked.
	Error string `json:"error,omitempty"`
}

// ModelMenu is the model list for one harness, ranked by MEI where possible.
type ModelMenu struct {
	Options []models.Option `json:"options"`
	Ranking ModelRanking    `json:"ranking"`
	// Probing is true while the installed harness is being asked for its models.
	Probing bool `json:"probing"`
	// ProbeError is the latest failed probe; the manifest list is used then.
	ProbeError string `json:"probeError,omitempty"`
}

func (application *App) initModels() {
	home, _ := os.UserHomeDir()
	application.meiStore = mei.NewStore(filepath.Join(filepath.Dir(application.configDirectory), "mei.json"), mei.Fetcher{})
	environment := tools.DefaultEnvironment()
	application.modelProbes = models.NewService(models.Env{
		Run:  environment.Run,
		Home: home,
		Executable: func(ctx context.Context, harnessID string) string {
			spec, ok := tools.Lookup(tools.ID(harnessID))
			if !ok {
				return ""
			}
			return tools.Detect(ctx, environment, spec).ExecutablePath
		},
	}, func(harnessID string) { application.emit("models-updated", harnessID) })
}

// startModelRanking refreshes the MEI ranking in the background. It never runs
// on menu open, and failures only keep the previous data.
func (application *App) startModelRanking(ctx context.Context) {
	if application.meiStore == nil {
		return
	}
	go func() {
		refresh := func() {
			if !application.meiStore.NeedsRefresh() {
				return
			}
			fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if application.meiStore.Refresh(fetchCtx) == nil {
				application.emit("models-updated", "")
			}
		}
		refresh()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()
}

// ListModels returns the model menu for a harness. It never blocks on the
// network: ranking and probe results come from caches that refresh in the
// background, and the manifest list is the floor.
func (application *App) ListModels(harnessID string) ModelMenu {
	return application.modelMenu(harnessID, false)
}

// RefreshModels re-reads MEI and re-probes the harness, then returns the menu.
func (application *App) RefreshModels(harnessID string) ModelMenu {
	if application.meiStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = application.meiStore.Refresh(ctx)
		cancel()
	}
	return application.modelMenu(harnessID, true)
}

func (application *App) modelMenu(harnessID string, force bool) ModelMenu {
	var manifest []string
	if spec, ok := tools.Lookup(tools.ID(harnessID)); ok && spec.Widgets != nil {
		manifest = spec.Widgets.Models
	}
	var probe models.Status
	if application.modelProbes != nil {
		probe = application.modelProbes.Get(context.Background(), harnessID, force)
	}
	var data mei.Data
	if application.meiStore != nil {
		data = application.meiStore.Data()
	}
	menu := ModelMenu{
		Options:    models.Rank(models.Combine(manifest, probe.Result), data.Entries),
		Probing:    probe.Probing,
		ProbeError: probe.Error,
		Ranking:    ModelRanking{Source: data.Source, Stale: data.Stale, Error: data.LastError},
	}
	if menu.Ranking.Source == "" {
		menu.Ranking.Source = mei.SourceNone
	}
	if !data.FetchedAt.IsZero() {
		menu.Ranking.AsOf = data.FetchedAt.Local().Format("2006-01-02")
	}
	return menu
}
