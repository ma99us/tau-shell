//go:build windows

package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ShutdownSession is one row of the shutdown progress popup.
type ShutdownSession struct {
	Name string `json:"name"`
	// State is "stopping", "stopped" or "force-stopped".
	State string `json:"state"`
}

// ShutdownProgress is emitted as "shutdown-progress" while TauShell stops
// running sessions after the user confirmed closing.
type ShutdownProgress struct {
	// Phase is "stopping" while sessions are being stopped and "done" after.
	Phase    string            `json:"phase"`
	Sessions []ShutdownSession `json:"sessions"`
	Elapsed  int               `json:"elapsedSeconds"`
}

func (application *App) emitShutdownProgress(progress ShutdownProgress) {
	if application.emitEvent != nil {
		application.emitEvent("shutdown-progress", progress)
		return
	}
	if application.ctx != nil {
		runtime.EventsEmit(application.ctx, "shutdown-progress", progress)
	}
}

// trackShutdownProgress publishes per-session progress until the returned
// function is called. It does nothing when there is no UI to receive events.
func (application *App) trackShutdownProgress(active []string) func() {
	if application.emitEvent == nil && application.ctx == nil {
		return func() {}
	}
	started := time.Now()
	wanted := make(map[string]bool, len(active))
	for _, name := range active {
		wanted[name] = true
	}
	snapshot := func(phase string) ShutdownProgress {
		progress := ShutdownProgress{Phase: phase, Elapsed: int(time.Since(started).Seconds())}
		listed := map[string]string{}
		if sessions, err := application.manager.List(context.Background()); err == nil {
			for _, current := range sessions {
				if !wanted[current.Name] {
					continue
				}
				switch {
				case current.RunningHere:
					listed[current.Name] = "stopping"
				case strings.Contains(current.LastError, "force-terminated"):
					listed[current.Name] = "force-stopped"
				default:
					listed[current.Name] = "stopped"
				}
			}
		}
		for _, name := range active {
			state := listed[name]
			if state == "" {
				state = "stopping"
			}
			progress.Sessions = append(progress.Sessions, ShutdownSession{Name: name, State: state})
		}
		return progress
	}

	application.emitShutdownProgress(snapshot("stopping"))
	stop := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		ticker := time.NewTicker(400 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				application.emitShutdownProgress(snapshot("stopping"))
			}
		}
	}()
	return func() {
		close(stop)
		wait.Wait()
		application.emitShutdownProgress(snapshot("done"))
	}
}
