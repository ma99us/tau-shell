package main

import (
	"testing"

	"github.com/ma99us/tau-shell/internal/mei"
)

func TestListModelsWithoutRankingKeepsManifestList(t *testing.T) {
	application := &App{}
	menu := application.ListModels("claude-code")
	if len(menu.Options) == 0 {
		t.Fatal("manifest models must still be listed when MEI and probes are unavailable")
	}
	for _, option := range menu.Options {
		if option.Ranked {
			t.Fatalf("%s ranked without MEI data", option.ID)
		}
	}
	if menu.Ranking.Source != mei.SourceNone {
		t.Fatalf("source = %q, want none", menu.Ranking.Source)
	}
}
