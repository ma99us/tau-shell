//go:build windows

package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ma99us/tau-shell/internal/appconfig"
	"github.com/ma99us/tau-shell/internal/session"
)

func TestAttentionDecisionHonorsForegroundAndFlags(t *testing.T) {
	off := false
	on := true
	if flash, toast := attentionDecision(appconfig.Notifications{}, false); !flash || !toast {
		t.Fatal("both alerts are on by default in the background")
	}
	if flash, toast := attentionDecision(appconfig.Notifications{}, true); flash || toast {
		t.Fatal("no alerts while TauShell is the foreground window")
	}
	if flash, toast := attentionDecision(appconfig.Notifications{Toast: &off, TaskbarFlash: &on}, false); !flash || toast {
		t.Fatalf("flags ignored: flash=%v toast=%v", flash, toast)
	}
}

func TestAttentionBodyHasNoPathsOrContent(t *testing.T) {
	if got := attentionBody("Api", attentionInput); got != "Api is waiting for your answer" {
		t.Fatal(got)
	}
	if got := attentionBody("Api", attentionDone); got != "Api finished" {
		t.Fatal(got)
	}
}

func TestToastActivationArg(t *testing.T) {
	if !isToastActivationArg([]string{"-Embedding"}) || isToastActivationArg([]string{"list"}) || isToastActivationArg(nil) {
		t.Fatal("activation argument detection is wrong")
	}
}

func TestNotifyAttentionUsesHooksAndSettings(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "sessions")
	application := &App{manager: &closeTestManager{}, configDirectory: configDir}
	snapshot, err := application.CreateSession("Api", string(session.ClaudeCode), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	foreground := false
	var flashes int
	var bodies []string
	application.attentionHooks = &attentionHooks{
		foreground: func() bool { return foreground },
		flash:      func() error { flashes++; return nil },
		toast: func(_, body, _ string) error {
			bodies = append(bodies, body)
			return nil
		},
	}
	application.attention.toastReady = true
	if err := application.NotifyAttention(snapshot.ID, attentionInput); err != nil {
		t.Fatal(err)
	}
	if flashes != 1 || len(bodies) != 1 || strings.Contains(bodies[0], configDir) {
		t.Fatalf("flashes=%d bodies=%v", flashes, bodies)
	}
	foreground = true
	if err := application.NotifyAttention(snapshot.ID, attentionDone); err != nil || flashes != 1 || len(bodies) != 1 {
		t.Fatalf("foreground alerts raised: %d %v %v", flashes, bodies, err)
	}
	if err := application.NotifyAttention(snapshot.ID, "bogus"); err == nil {
		t.Fatal("unknown kind accepted")
	}
	foreground = false
	application.attentionHooks.toast = func(_, _, _ string) error { return errors.New("boom") }
	if err := application.NotifyAttention(snapshot.ID, attentionDone); err == nil {
		t.Fatal("toast failure was swallowed")
	}
}
