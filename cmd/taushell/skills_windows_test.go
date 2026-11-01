//go:build windows

package main

import (
	"context"
	"strings"
	"testing"
)

func TestSkillsViewSurfacesInitializationFailure(t *testing.T) {
	application := &App{skillsInitError: "Skills are unavailable: invalid saved preferences"}
	view := application.skillsView(context.Background())
	if view.Status.Enabled || !strings.Contains(view.Status.Message, "invalid saved preferences") || len(view.Skills) != 0 {
		t.Fatalf("skills view = %+v", view)
	}
}
