//go:build windows

package main

import (
	"context"
	"testing"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestInAppDialogRoundTrip(t *testing.T) {
	requests := make(chan DialogRequest, 1)
	application := &App{emitEvent: func(name string, data ...any) {
		if name == "dialog-request" {
			requests <- data[0].(DialogRequest)
		}
	}}
	options := runtime.MessageDialogOptions{
		Type: runtime.QuestionDialog, Title: "Sessions are still running", Message: "Stop them?",
		Buttons: []string{"Yes", "No"}, DefaultButton: "No", CancelButton: "No",
	}

	if _, shown, _ := application.inAppDialog(context.Background(), options); shown {
		t.Fatal("dialog was shown in-app before the frontend registered as a host")
	}
	if pending := application.DialogHostReady(); len(pending) != 0 {
		t.Fatalf("pending dialogs at registration = %v", pending)
	}

	type result struct {
		choice string
		shown  bool
		err    error
	}
	done := make(chan result, 1)
	go func() {
		choice, shown, err := application.inAppDialog(context.Background(), options)
		done <- result{choice, shown, err}
	}()
	var request DialogRequest
	select {
	case request = <-requests:
	case <-time.After(5 * time.Second):
		t.Fatal("dialog request was not emitted")
	}
	if request.Title != options.Title || request.Kind != "question" || request.DefaultButton != "No" {
		t.Fatalf("request = %+v", request)
	}
	if reloaded := application.DialogHostReady(); len(reloaded) != 1 || reloaded[0].ID != request.ID {
		t.Fatalf("a reloaded frontend must receive the pending dialog, got %v", reloaded)
	}
	if err := application.AnswerDialog(request.ID, "Maybe"); err == nil {
		t.Fatal("a choice outside the dialog's buttons was accepted")
	}
	if err := application.AnswerDialog("dialog-999", "Yes"); err == nil {
		t.Fatal("an unknown dialog ID was accepted")
	}
	if err := application.AnswerDialog(request.ID, "Yes"); err != nil {
		t.Fatal(err)
	}
	got := <-done
	if !got.shown || got.err != nil || got.choice != "Yes" {
		t.Fatalf("result = %+v, want shown with choice Yes", got)
	}
	if err := application.AnswerDialog(request.ID, "Yes"); err == nil {
		t.Fatal("a dialog could be answered twice")
	}
}

func TestInAppDialogCancelledContextRemovesPendingDialog(t *testing.T) {
	application := &App{emitEvent: func(string, ...any) {}}
	application.DialogHostReady()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := application.inAppDialog(ctx, runtime.MessageDialogOptions{Type: runtime.InfoDialog, Buttons: []string{"OK"}})
		done <- err
	}()
	for len(application.DialogHostReady()) == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled dialog returned no error")
	}
	if pending := application.DialogHostReady(); len(pending) != 0 {
		t.Fatalf("cancelled dialog still pending: %v", pending)
	}
}
