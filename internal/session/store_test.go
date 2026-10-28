package session

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
)

func TestStorePersistsExplicitHarnessIDAndLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}

	runtime, err := store.Begin(context.Background(), "agent", ClaudeCode, false)
	if err != nil {
		t.Fatal(err)
	}
	firstID := runtime.HarnessSessionID
	if firstID == "" {
		t.Fatal("new session has no harness UUID")
	}
	if err := store.MarkRunning(context.Background(), "agent", 123); err != nil {
		t.Fatal(err)
	}
	updated, err := store.FailIfCurrent(context.Background(), runtime, "stale startup")
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("stale startup reconciliation overwrote a newer running state")
	}
	runtime, err = store.Runtime(context.Background(), "agent")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Status != StatusRunning || runtime.ProcessID != 123 {
		t.Fatalf("unexpected running state: %+v", runtime)
	}
	if err := store.Finish(context.Background(), "agent", StatusStopped, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runtime, err = store.Begin(context.Background(), "agent", ClaudeCode, true)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.HarnessSessionID != firstID {
		t.Fatalf("restore changed harness UUID: %s != %s", runtime.HarnessSessionID, firstID)
	}
	if _, err := store.Begin(context.Background(), "agent", CopilotCLI, true); err == nil {
		t.Fatal("expected mismatched harness to be rejected")
	}
	if _, err := store.Begin(context.Background(), "agent", ClaudeCode, false); err == nil {
		t.Fatal("expected existing conversation to require explicit restore")
	}
}

func TestTwoTauShellProcessesCannotStartSameSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	first, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, store := range []*Store{first, second} {
		group.Add(1)
		go func(store *Store) {
			defer group.Done()
			<-start
			_, err := store.Begin(context.Background(), "shared-session", ClaudeCode, false)
			results <- err
		}(store)
	}
	close(start)
	group.Wait()
	close(results)
	succeeded, rejected := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
		} else {
			rejected++
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("expected one start and one rejection, got %d starts and %d rejections", succeeded, rejected)
	}
}

func TestBeginFreshReplacesSavedHarnessID(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	first, err := store.Begin(context.Background(), "agent", ClaudeCode, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Finish(context.Background(), "agent", StatusStopped, ""); err != nil {
		t.Fatal(err)
	}
	fresh, err := store.BeginFresh(context.Background(), "agent", ClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.HarnessSessionID == "" || fresh.HarnessSessionID == first.HarnessSessionID {
		t.Fatalf("fresh start reused harness session ID %q", fresh.HarnessSessionID)
	}
}

func TestSessionOrderRoundTripAndDelete(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if order, err := store.SessionOrder(ctx); err != nil || len(order) != 0 {
		t.Fatalf("empty order = %v, %v", order, err)
	}
	if err := store.SetSessionOrder(ctx, []string{"c", "a", "b"}); err != nil {
		t.Fatal(err)
	}
	order, err := store.SessionOrder(ctx)
	if err != nil || order["c"] != 0 || order["a"] != 1 || order["b"] != 2 {
		t.Fatalf("order = %v, %v", order, err)
	}
	if err := store.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if order, _ = store.SessionOrder(ctx); len(order) != 2 {
		t.Fatalf("order after delete = %v", order)
	}
	if err := store.SetSessionOrder(ctx, []string{"b"}); err != nil {
		t.Fatal(err)
	}
	if order, _ = store.SessionOrder(ctx); len(order) != 1 || order["b"] != 0 {
		t.Fatalf("order after replace = %v", order)
	}
}

func TestDiscoveredIDLifecycle(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	runtime, err := store.BeginDiscovered(ctx, "a", Codex, false, false)
	if err != nil || runtime.HarnessSessionID != "" {
		t.Fatalf("BeginDiscovered = %+v, %v", runtime, err)
	}
	if ok, err := store.SetHarnessSessionID(ctx, "a", "cx-1"); err != nil || !ok {
		t.Fatalf("first set = %v, %v", ok, err)
	}
	if ok, _ := store.SetHarnessSessionID(ctx, "a", "cx-2"); ok {
		t.Fatal("an already recorded ID must not be overwritten")
	}
	taken, err := store.HarnessSessionIDs(ctx, Codex)
	if err != nil || !taken["cx-1"] {
		t.Fatal("recorded ID missing from the taken set")
	}
}
