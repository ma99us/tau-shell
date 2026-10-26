//go:build windows

package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"sync"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// DialogRequest is a confirmation or notice the Go side asks the UI to show
// as an in-app modal instead of a native message box.
type DialogRequest struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Title         string   `json:"title"`
	Message       string   `json:"message"`
	Buttons       []string `json:"buttons"`
	DefaultButton string   `json:"defaultButton"`
	CancelButton  string   `json:"cancelButton"`
}

type pendingDialog struct {
	request DialogRequest
	answer  chan string
}

// dialogBroker hands dialogs to the frontend and waits for the user's choice.
type dialogBroker struct {
	mu      sync.Mutex
	ready   bool
	nextID  int
	pending []*pendingDialog
}

func (broker *dialogBroker) add(options runtime.MessageDialogOptions) (*pendingDialog, bool) {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if !broker.ready {
		return nil, false
	}
	broker.nextID++
	dialog := &pendingDialog{
		request: DialogRequest{
			ID:            "dialog-" + strconv.Itoa(broker.nextID),
			Kind:          string(options.Type),
			Title:         options.Title,
			Message:       options.Message,
			Buttons:       slices.Clone(options.Buttons),
			DefaultButton: options.DefaultButton,
			CancelButton:  options.CancelButton,
		},
		answer: make(chan string, 1),
	}
	broker.pending = append(broker.pending, dialog)
	return dialog, true
}

func (broker *dialogBroker) remove(id string) *pendingDialog {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	for index, dialog := range broker.pending {
		if dialog.request.ID == id {
			broker.pending = slices.Delete(broker.pending, index, index+1)
			return dialog
		}
	}
	return nil
}

// DialogHostReady is called by the frontend once it can render dialogs. It
// returns dialogs still waiting for an answer, such as after a page reload.
func (application *App) DialogHostReady() []DialogRequest {
	application.dialogs.mu.Lock()
	defer application.dialogs.mu.Unlock()
	application.dialogs.ready = true
	requests := make([]DialogRequest, 0, len(application.dialogs.pending))
	for _, dialog := range application.dialogs.pending {
		requests = append(requests, dialog.request)
	}
	return requests
}

// AnswerDialog delivers the user's choice; only one of the dialog's own buttons is accepted.
func (application *App) AnswerDialog(id, choice string) error {
	application.dialogs.mu.Lock()
	var target *pendingDialog
	for _, dialog := range application.dialogs.pending {
		if dialog.request.ID == id {
			target = dialog
			break
		}
	}
	application.dialogs.mu.Unlock()
	if target == nil {
		return fmt.Errorf("dialog %q is not waiting for an answer", id)
	}
	if !slices.Contains(target.request.Buttons, choice) {
		return fmt.Errorf("%q is not a choice of dialog %q", choice, id)
	}
	if application.dialogs.remove(id) == nil {
		return fmt.Errorf("dialog %q is not waiting for an answer", id)
	}
	target.answer <- choice
	return nil
}

// inAppDialog shows options in the TauShell window. It reports false when the
// frontend has not registered as a dialog host, so the caller can use a native box.
func (application *App) inAppDialog(ctx context.Context, options runtime.MessageDialogOptions) (string, bool, error) {
	dialog, ok := application.dialogs.add(options)
	if !ok {
		return "", false, nil
	}
	diagnostics.Debug("in-app dialog requested", "dialog_id", dialog.request.ID, "kind", dialog.request.Kind)
	if application.emitEvent != nil {
		application.emitEvent("dialog-request", dialog.request)
	} else {
		runtime.EventsEmit(application.ctx, "dialog-request", dialog.request)
	}
	select {
	case choice := <-dialog.answer:
		return choice, true, nil
	case <-ctx.Done():
		application.dialogs.remove(dialog.request.ID)
		return "", true, ctx.Err()
	}
}
