package session

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type RuntimeStatus string

const (
	StatusNeverStarted RuntimeStatus = "never-started"
	StatusStarting     RuntimeStatus = "starting"
	StatusRunning      RuntimeStatus = "running"
	StatusStopped      RuntimeStatus = "stopped"
	StatusFailed       RuntimeStatus = "failed"
)

type Runtime struct {
	SessionID        string
	Harness          Harness
	HarnessSessionID string
	Status           RuntimeStatus
	ProcessID        int
	LastError        string
	UpdatedAt        time.Time
}

type Store struct {
	db *sql.DB
	mu sync.Mutex
}

type runtimeQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open session state database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA busy_timeout = 5000;
		CREATE TABLE IF NOT EXISTS session_runtime (
			session_id TEXT PRIMARY KEY,
			harness TEXT NOT NULL,
			harness_session_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'never-started',
			process_id INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize session state database: %w", err)
	}
	return &Store{db: db}, nil
}

func (store *Store) Close() error {
	if err := store.db.Close(); err != nil {
		return fmt.Errorf("close session state database: %w", err)
	}
	return nil
}

func (store *Store) Runtime(ctx context.Context, sessionID string) (Runtime, error) {
	return readRuntime(ctx, store.db, sessionID)
}

func readRuntime(ctx context.Context, queryer runtimeQueryer, sessionID string) (Runtime, error) {
	var runtime Runtime
	var updatedAt string
	err := queryer.QueryRowContext(ctx, `
		SELECT session_id, harness, harness_session_id, status, process_id, last_error, updated_at
		FROM session_runtime WHERE session_id = ?
	`, sessionID).Scan(
		&runtime.SessionID,
		&runtime.Harness,
		&runtime.HarnessSessionID,
		&runtime.Status,
		&runtime.ProcessID,
		&runtime.LastError,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Runtime{SessionID: sessionID, Status: StatusNeverStarted}, nil
	}
	if err != nil {
		return Runtime{}, fmt.Errorf("read runtime state for %q: %w", sessionID, err)
	}
	runtime.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Runtime{}, fmt.Errorf("parse runtime update time for %q: %w", sessionID, err)
	}
	return runtime, nil
}

func (store *Store) Begin(ctx context.Context, sessionID string, harness Harness, restoring bool) (Runtime, error) {
	return store.begin(ctx, sessionID, harness, restoring, false)
}

func (store *Store) BeginFresh(ctx context.Context, sessionID string, harness Harness) (Runtime, error) {
	return store.begin(ctx, sessionID, harness, false, true)
}

func (store *Store) begin(ctx context.Context, sessionID string, harness Harness, restoring, fresh bool) (Runtime, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	conn, err := store.db.Conn(ctx)
	if err != nil {
		return Runtime{}, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return Runtime{}, fmt.Errorf("lock session runtime state: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	runtime, err := readRuntime(ctx, conn, sessionID)
	if err != nil {
		return Runtime{}, err
	}
	if runtime.Status == StatusRunning || runtime.Status == StatusStarting {
		return Runtime{}, fmt.Errorf("session %q is already marked %s (process %d); inspect it before starting another instance", sessionID, runtime.Status, runtime.ProcessID)
	}
	if runtime.Harness != "" && runtime.Harness != harness {
		return Runtime{}, fmt.Errorf("session %q belongs to harness %q, not %q; refusing to reuse its resume ID", sessionID, runtime.Harness, harness)
	}
	if fresh {
		runtime.HarnessSessionID = ""
	}
	if restoring && runtime.HarnessSessionID == "" {
		return Runtime{}, fmt.Errorf("session %q has no saved harness session ID; start it before restoring", sessionID)
	}
	if !restoring && runtime.HarnessSessionID != "" {
		return Runtime{}, fmt.Errorf("session %q already has a harness session ID; use restore to resume it", sessionID)
	}
	if runtime.HarnessSessionID == "" {
		runtime.HarnessSessionID, err = newUUID()
		if err != nil {
			return Runtime{}, fmt.Errorf("generate harness session ID: %w", err)
		}
	}
	runtime.SessionID = sessionID
	runtime.Harness = harness
	runtime.Status = StatusStarting
	runtime.ProcessID = 0
	runtime.LastError = ""
	runtime.UpdatedAt = time.Now().UTC()

	_, err = conn.ExecContext(ctx, `
		INSERT INTO session_runtime (session_id, harness, harness_session_id, status, process_id, last_error, updated_at)
		VALUES (?, ?, ?, ?, 0, '', ?)
		ON CONFLICT(session_id) DO UPDATE SET
			harness = excluded.harness,
			harness_session_id = excluded.harness_session_id,
			status = excluded.status,
			process_id = 0,
			last_error = '',
			updated_at = excluded.updated_at
	`, runtime.SessionID, runtime.Harness, runtime.HarnessSessionID, runtime.Status, runtime.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return Runtime{}, fmt.Errorf("persist starting state for %q: %w", sessionID, err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return Runtime{}, fmt.Errorf("commit starting state for %q: %w", sessionID, err)
	}
	committed = true
	return runtime, nil
}

func (store *Store) MarkRunning(ctx context.Context, sessionID string, processID int) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE session_runtime SET status = ?, process_id = ?, last_error = '', updated_at = ?
		WHERE session_id = ? AND status = ?
	`, StatusRunning, processID, time.Now().UTC().Format(time.RFC3339Nano), sessionID, StatusStarting)
	if err != nil {
		return fmt.Errorf("mark session %q running: %w", sessionID, err)
	}
	if count, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("check running-state update for %q: %w", sessionID, err)
	} else if count != 1 {
		return fmt.Errorf("session %q was not in starting state", sessionID)
	}
	return nil
}

func (store *Store) Finish(ctx context.Context, sessionID string, status RuntimeStatus, detail string) error {
	if status != StatusStopped && status != StatusFailed {
		return fmt.Errorf("invalid terminal session status %q", status)
	}
	result, err := store.db.ExecContext(ctx, `
		UPDATE session_runtime SET status = ?, process_id = 0, last_error = ?, updated_at = ?
		WHERE session_id = ?
	`, status, detail, time.Now().UTC().Format(time.RFC3339Nano), sessionID)
	if err != nil {
		return fmt.Errorf("mark session %q %s: %w", sessionID, status, err)
	}
	if count, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("check terminal-state update for %q: %w", sessionID, err)
	} else if count != 1 {
		return fmt.Errorf("session %q has no runtime state to finish", sessionID)
	}
	return nil
}

func (store *Store) FailIfCurrent(ctx context.Context, runtime Runtime, detail string) (bool, error) {
	result, err := store.db.ExecContext(ctx, `
		UPDATE session_runtime SET status = ?, process_id = 0, last_error = ?, updated_at = ?
		WHERE session_id = ? AND status = ? AND process_id = ? AND updated_at = ?
	`, StatusFailed, detail, time.Now().UTC().Format(time.RFC3339Nano),
		runtime.SessionID, runtime.Status, runtime.ProcessID, runtime.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return false, fmt.Errorf("reconcile session %q state: %w", runtime.SessionID, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check reconciled state for %q: %w", runtime.SessionID, err)
	}
	return count == 1, nil
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
