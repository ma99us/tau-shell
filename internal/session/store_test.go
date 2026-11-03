package session

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
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

func TestSetHarnessSessionIDChoosesConversationSafely(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, err := store.Begin(ctx, "agent", ClaudeCode, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetHarnessSessionID(ctx, "agent", ClaudeCode, "older"); err == nil {
		t.Fatal("expected a starting session to refuse a conversation change")
	}
	if err := store.Finish(ctx, "agent", StatusStopped, ""); err != nil {
		t.Fatal(err)
	}
	previous, err := store.SetHarnessSessionID(ctx, "agent", ClaudeCode, "older")
	if err != nil || previous != first.HarnessSessionID {
		t.Fatalf("previous = %q, err = %v", previous, err)
	}
	runtime, _ := store.Runtime(ctx, "agent")
	if runtime.HarnessSessionID != "older" {
		t.Fatalf("saved ID = %q", runtime.HarnessSessionID)
	}
	if _, err := store.SetHarnessSessionID(ctx, "agent", CopilotCLI, "x"); err == nil {
		t.Fatal("expected a different harness to be refused")
	}
	other, err := store.Begin(ctx, "other", ClaudeCode, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Finish(ctx, "other", StatusStopped, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetHarnessSessionID(ctx, "agent", ClaudeCode, other.HarnessSessionID); !errors.Is(err, ErrHarnessSessionIDClaimed) {
		t.Fatalf("expected a claimed conversation to be refused, got %v", err)
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

func TestStoreDeleteRollsBackRuntimeAndOrderTogether(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.Begin(ctx, "delete-me", ClaudeCode, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Finish(ctx, "delete-me", StatusStopped, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSessionOrder(ctx, []string{"delete-me"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_order_delete BEFORE DELETE ON session_order BEGIN SELECT RAISE(FAIL, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "delete-me"); err == nil {
		t.Fatal("expected injected order deletion failure")
	}
	runtime, err := store.Runtime(ctx, "delete-me")
	if err != nil || runtime.Status != StatusStopped {
		t.Fatalf("runtime after rollback = %+v, %v", runtime, err)
	}
	order, err := store.SessionOrder(ctx)
	if err != nil || order["delete-me"] != 0 {
		t.Fatalf("order after rollback = %v, %v", order, err)
	}
}

func TestStoreDeleteRefusesActiveRuntime(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.Begin(ctx, "active", ClaudeCode, false); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteWith(ctx, "active", nil); err == nil {
		t.Fatal("active session deletion succeeded")
	}
	runtime, err := store.Runtime(ctx, "active")
	if err != nil || runtime.Status != StatusStarting {
		t.Fatalf("active runtime after rejected delete = %+v, %v", runtime, err)
	}
}

func TestDeleteWithRollsBackWhenRelatedStateFails(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.Begin(ctx, "delete-me", ClaudeCode, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Finish(ctx, "delete-me", StatusStopped, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSessionOrder(ctx, []string{"delete-me"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_delete_journal BEFORE INSERT ON session_deletions BEGIN SELECT RAISE(FAIL, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteWith(ctx, "delete-me", nil); err == nil {
		t.Fatal("expected injected deletion journal failure")
	}
	runtime, err := store.Runtime(ctx, "delete-me")
	if err != nil || runtime.Status != StatusStopped {
		t.Fatalf("runtime after rollback = %+v, %v", runtime, err)
	}
	order, err := store.SessionOrder(ctx)
	if err != nil || order["delete-me"] != 0 {
		t.Fatalf("order after rollback = %v, %v", order, err)
	}
	if pending, err := store.PendingDeletions(ctx); err != nil || len(pending) != 0 {
		t.Fatalf("pending deletions after rollback = %v, %v", pending, err)
	}
}

func TestConfigDeletionRecoveryRestoresOrFinalizesByJournal(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	directory := t.TempDir()
	config := Config{ID: "delete-me", Name: "Delete me", Tool: ClaudeCode, WorkingDir: t.TempDir()}
	path, err := SaveConfig(directory, config)
	if err != nil {
		t.Fatal(err)
	}

	staged, err := StageConfigDeletion(directory, config.ID)
	if err != nil {
		t.Fatal(err)
	}
	staged.release()
	if err := RecoverPendingConfigDeletions(directory, store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("uncommitted staged definition was not restored: %v", err)
	}

	staged, err = StageConfigDeletion(directory, config.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteWith(context.Background(), config.ID, nil); err != nil {
		t.Fatal(err)
	}
	staged.release()
	if err := RecoverPendingConfigDeletions(directory, store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed deleted definition still exists: %v", err)
	}
	if pending, err := store.PendingDeletions(context.Background()); err != nil || len(pending) != 0 {
		t.Fatalf("pending deletions after recovery = %v, %v", pending, err)
	}
}

func TestConfigDeletionRecoveryPreservesRecreatedDefinition(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	directory := t.TempDir()
	config := Config{ID: "delete-recreated", Name: "Old", Tool: ClaudeCode, WorkingDir: t.TempDir()}
	path, err := SaveConfig(directory, config)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := StageConfigDeletion(directory, config.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteWith(context.Background(), config.ID, nil); err != nil {
		t.Fatal(err)
	}
	staged.release()
	newDefinition := []byte("id: delete-recreated\nname: New\ntool: claude-code\nworking_dir: " + filepath.ToSlash(config.WorkingDir) + "\n")
	if err := os.WriteFile(path, newDefinition, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RecoverPendingConfigDeletions(directory, store); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, newDefinition) {
		t.Fatalf("new definition was changed during recovery: %q, %v", got, err)
	}
	if _, err := os.Stat(path + ".deleting"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old tombstone remains after recovery: %v", err)
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
	if claimed, err := store.ClaimHarnessSessionID(ctx, "a", Codex, runtime.LaunchToken, "cx-1"); err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}
	if claimed, err := store.ClaimHarnessSessionID(ctx, "a", Codex, runtime.LaunchToken, "cx-2"); claimed || !errors.Is(err, ErrHarnessSessionIDClaimed) {
		t.Fatalf("conflicting claim = %v, %v", claimed, err)
	}
	if claimed, err := store.ClaimHarnessSessionID(ctx, "a", Codex, runtime.LaunchToken, "cx-1"); err != nil || !claimed {
		t.Fatalf("idempotent claim = %v, %v", claimed, err)
	}
	taken, err := store.HarnessSessionIDs(ctx, Codex)
	if err != nil || !taken["cx-1"] {
		t.Fatal("recorded ID missing from the taken set")
	}
}

func TestConcurrentHarnessSessionIDClaimsAcrossStores(t *testing.T) {
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

	one, err := first.BeginDiscovered(context.Background(), "one", Codex, false, false)
	if err != nil {
		t.Fatal(err)
	}
	two, err := second.BeginDiscovered(context.Background(), "two", Codex, false, false)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	type result struct {
		claimed bool
		err     error
	}
	results := make(chan result, 2)
	var group sync.WaitGroup
	for _, candidate := range []struct {
		store   *Store
		session string
		token   string
	}{{first, "one", one.LaunchToken}, {second, "two", two.LaunchToken}} {
		group.Add(1)
		go func(candidate struct {
			store   *Store
			session string
			token   string
		}) {
			defer group.Done()
			<-start
			claimed, err := candidate.store.ClaimHarnessSessionID(context.Background(), candidate.session, Codex, candidate.token, "shared-id")
			results <- result{claimed: claimed, err: err}
		}(candidate)
	}
	close(start)
	group.Wait()
	close(results)

	succeeded, conflicts := 0, 0
	for result := range results {
		if result.claimed && result.err == nil {
			succeeded++
		} else if !result.claimed && errors.Is(result.err, ErrHarnessSessionIDClaimed) {
			conflicts++
		} else {
			t.Errorf("unexpected claim result: %+v", result)
		}
	}
	if succeeded != 1 || conflicts != 1 {
		t.Fatalf("claims: succeeded=%d conflicts=%d; want one of each", succeeded, conflicts)
	}
}

func TestHarnessSessionIDClaimRejectsStaleLaunch(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	first, err := store.BeginDiscovered(ctx, "session", Codex, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Finish(ctx, "session", StatusStopped, ""); err != nil {
		t.Fatal(err)
	}
	second, err := store.BeginDiscovered(ctx, "session", Codex, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.LaunchToken == first.LaunchToken {
		t.Fatal("new launch reused the previous launch token")
	}
	if claimed, err := store.ClaimHarnessSessionID(ctx, "session", Codex, first.LaunchToken, "stale-id"); claimed || !errors.Is(err, ErrHarnessLaunchChanged) {
		t.Fatalf("stale claim = %v, %v", claimed, err)
	}
	runtime, err := store.Runtime(ctx, "session")
	if err != nil || runtime.HarnessSessionID != "" || runtime.LaunchToken != second.LaunchToken {
		t.Fatalf("stale claim changed current runtime: %+v, %v", runtime, err)
	}
}

func TestHarnessSessionIDClaimsAreScopedByHarness(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	codex, err := store.BeginDiscovered(ctx, "codex-session", Codex, false, false)
	if err != nil {
		t.Fatal(err)
	}
	opencode, err := store.BeginDiscovered(ctx, "opencode-session", OpenCode, false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []struct {
		session string
		harness Harness
		token   string
	}{{"codex-session", Codex, codex.LaunchToken}, {"opencode-session", OpenCode, opencode.LaunchToken}} {
		if claimed, err := store.ClaimHarnessSessionID(ctx, candidate.session, candidate.harness, candidate.token, "shared-vendor-id"); err != nil || !claimed {
			t.Fatalf("claim for %s = %v, %v", candidate.harness, claimed, err)
		}
	}
}

func TestOpenStoreAddsLaunchTokenWithoutChangingLegacyDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE session_runtime (
			session_id TEXT PRIMARY KEY,
			harness TEXT NOT NULL,
			harness_session_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'never-started',
			process_id INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		);
		INSERT INTO session_runtime (session_id, harness, harness_session_id, status, updated_at)
		VALUES ('one', 'codex', 'legacy-duplicate', 'stopped', '2026-01-01T00:00:00Z'),
		       ('two', 'codex', 'legacy-duplicate', 'stopped', '2026-01-01T00:00:00Z');
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, id := range []string{"one", "two"} {
		runtime, err := store.Runtime(context.Background(), id)
		if err != nil || runtime.HarnessSessionID != "legacy-duplicate" {
			t.Fatalf("legacy runtime %q = %+v, %v", id, runtime, err)
		}
	}
}
