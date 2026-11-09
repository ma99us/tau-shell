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
	LaunchToken      string
	Status           RuntimeStatus
	ProcessID        int
	LastError        string
	UpdatedAt        time.Time
}

type Store struct {
	db *sql.DB
	mu sync.Mutex
}

// SchemaVersion is stored in the database's user_version. Bump it whenever a
// change would stop an older TauShell from reading the database correctly.
const SchemaVersion = 1

// ErrNewerSchema means the database was written by a newer TauShell.
var ErrNewerSchema = errors.New("the session state database was written by a newer TauShell")

var (
	ErrHarnessSessionIDClaimed = errors.New("harness session ID is already recorded by another TauShell session")
	ErrHarnessLaunchChanged    = errors.New("session launch changed before the harness session ID could be recorded")
)

type runtimeQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open session state database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := checkSchemaVersion(db); err != nil {
		_ = db.Close()
		return nil, err
	}
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
			updated_at TEXT NOT NULL,
			launch_token TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS session_order (
			session_id TEXT PRIMARY KEY,
			position INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS session_deletions (
			session_id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS prompt_queue (
			session_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			text TEXT NOT NULL,
			PRIMARY KEY (session_id, position)
		)
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize session state database: %w", err)
	}
	if err := ensureLaunchTokenColumn(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate session state database: %w", err)
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("record session state schema version: %w", err)
	}
	return &Store{db: db}, nil
}

// checkSchemaVersion refuses a database written by a newer schema, before any
// change is made to it. Version 0 is a new or pre-versioning database.
func checkSchemaVersion(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read session state schema version: %w", err)
	}
	if version > SchemaVersion {
		return fmt.Errorf("%w (schema %d, this build understands %d); update TauShell or restore a state backup from ~/.tau_shell/backups", ErrNewerSchema, version, SchemaVersion)
	}
	return nil
}

func ensureLaunchTokenColumn(db *sql.DB) error {
	found, err := launchTokenColumnExists(db)
	if err != nil || found {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE session_runtime ADD COLUMN launch_token TEXT NOT NULL DEFAULT ''`); err != nil {
		found, checkErr := launchTokenColumnExists(db)
		if checkErr == nil && found {
			return nil
		}
		return errors.Join(err, checkErr)
	}
	return nil
}

func launchTokenColumnExists(db *sql.DB) (bool, error) {
	rows, err := db.Query("PRAGMA table_info(session_runtime)")
	if err != nil {
		return false, err
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return false, err
		}
		if name == "launch_token" {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	return found, nil
}

func (store *Store) Close() error {
	if err := store.db.Close(); err != nil {
		return fmt.Errorf("close session state database: %w", err)
	}
	return nil
}

func (store *Store) Delete(ctx context.Context, sessionID string) error {
	return store.delete(ctx, sessionID, false, nil)
}

// DeleteWith removes runtime/order state and records a recoverable deletion
// marker in the same transaction. beforeCommit may update related state in
// this transaction, such as detaching usage history.
func (store *Store) DeleteWith(ctx context.Context, sessionID string, beforeCommit func(*sql.Tx) error) error {
	return store.delete(ctx, sessionID, true, beforeCommit)
}

func (store *Store) delete(ctx context.Context, sessionID string, journal bool, beforeCommit func(*sql.Tx) error) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete session %q: begin transaction: %w", sessionID, err)
	}
	defer func() { _ = tx.Rollback() }()
	runtime, err := readRuntime(ctx, tx, sessionID)
	if err != nil {
		return fmt.Errorf("check runtime before deleting %q: %w", sessionID, err)
	}
	if runtime.Status == StatusRunning || runtime.Status == StatusStarting {
		return fmt.Errorf("session %q is active in status %s; stop it before deleting", sessionID, runtime.Status)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM session_runtime WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("delete runtime state for %q: %w", sessionID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM session_order WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("delete display order for %q: %w", sessionID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM prompt_queue WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("delete queued prompts for %q: %w", sessionID, err)
	}
	if journal {
		if _, err := tx.ExecContext(ctx, `INSERT INTO session_deletions (session_id, created_at) VALUES (?, ?)`, sessionID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record deletion for %q: %w", sessionID, err)
		}
	}
	if beforeCommit != nil {
		if err := beforeCommit(tx); err != nil {
			return fmt.Errorf("delete related state for %q: %w", sessionID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete session %q: commit transaction: %w", sessionID, err)
	}
	return nil
}

func (store *Store) PendingDeletions(ctx context.Context) ([]string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	rows, err := store.db.QueryContext(ctx, `SELECT session_id FROM session_deletions ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("read pending session deletions: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read pending session deletions: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read pending session deletions: %w", err)
	}
	return ids, nil
}

func (store *Store) CompleteDeletion(ctx context.Context, sessionID string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, err := store.db.ExecContext(ctx, `DELETE FROM session_deletions WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("complete session deletion for %q: %w", sessionID, err)
	}
	return nil
}

// Setting returns an app-level UI preference, or "" when it was never saved.
func (store *Store) Setting(ctx context.Context, key string) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	var value string
	err := store.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read setting %q: %w", key, err)
	}
	return value, nil
}

// SetSetting saves an app-level UI preference.
func (store *Store) SetSetting(ctx context.Context, key, value string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
		return fmt.Errorf("save setting %q: %w", key, err)
	}
	return nil
}

// PromptQueues returns the saved queued prompts of every session, in send order.
func (store *Store) PromptQueues(ctx context.Context) (map[string][]string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	rows, err := store.db.QueryContext(ctx, `SELECT session_id, text FROM prompt_queue ORDER BY session_id, position`)
	if err != nil {
		return nil, fmt.Errorf("read queued prompts: %w", err)
	}
	defer rows.Close()
	queues := map[string][]string{}
	for rows.Next() {
		var id, text string
		if err := rows.Scan(&id, &text); err != nil {
			return nil, fmt.Errorf("read queued prompts: %w", err)
		}
		queues[id] = append(queues[id], text)
	}
	return queues, rows.Err()
}

// SetPromptQueue replaces one session's saved queued prompts. An empty list removes them.
func (store *Store) SetPromptQueue(ctx context.Context, sessionID string, prompts []string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save queued prompts: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM prompt_queue WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("save queued prompts: %w", err)
	}
	for position, text := range prompts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO prompt_queue (session_id, position, text) VALUES (?, ?, ?)`, sessionID, position, text); err != nil {
			return fmt.Errorf("save queued prompts: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save queued prompts: %w", err)
	}
	return nil
}

// SessionOrder returns the saved display position of each session. Sessions
// without a saved position are absent.
func (store *Store) SessionOrder(ctx context.Context) (map[string]int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	rows, err := store.db.QueryContext(ctx, `SELECT session_id, position FROM session_order`)
	if err != nil {
		return nil, fmt.Errorf("read session order: %w", err)
	}
	defer rows.Close()
	order := map[string]int{}
	for rows.Next() {
		var id string
		var position int
		if err := rows.Scan(&id, &position); err != nil {
			return nil, fmt.Errorf("read session order: %w", err)
		}
		order[id] = position
	}
	return order, rows.Err()
}

// SetSessionOrder replaces the saved display order with ids, in order.
func (store *Store) SetSessionOrder(ctx context.Context, ids []string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save session order: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM session_order`); err != nil {
		return fmt.Errorf("save session order: %w", err)
	}
	for position, id := range ids {
		if _, err := tx.ExecContext(ctx, `INSERT INTO session_order (session_id, position) VALUES (?, ?)`, id, position); err != nil {
			return fmt.Errorf("save session order: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save session order: %w", err)
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
		SELECT session_id, harness, harness_session_id, status, process_id, last_error, updated_at, launch_token
		FROM session_runtime WHERE session_id = ?
	`, sessionID).Scan(
		&runtime.SessionID,
		&runtime.Harness,
		&runtime.HarnessSessionID,
		&runtime.Status,
		&runtime.ProcessID,
		&runtime.LastError,
		&updatedAt,
		&runtime.LaunchToken,
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
	return store.begin(ctx, sessionID, harness, restoring, false, true)
}

func (store *Store) BeginFresh(ctx context.Context, sessionID string, harness Harness) (Runtime, error) {
	return store.begin(ctx, sessionID, harness, false, true, true)
}

// BeginDiscovered is Begin for harnesses that pick their own conversation ID:
// a new start records no ID until ClaimHarnessSessionID stores the discovered one.
func (store *Store) BeginDiscovered(ctx context.Context, sessionID string, harness Harness, restoring, fresh bool) (Runtime, error) {
	return store.begin(ctx, sessionID, harness, restoring && !fresh, fresh, false)
}

// ClaimHarnessSessionID records a discovered ID only if this launch still owns
// the runtime row and no other session for the harness has claimed the ID.
func (store *Store) ClaimHarnessSessionID(ctx context.Context, sessionID string, harness Harness, launchToken, harnessSessionID string) (bool, error) {
	if harnessSessionID == "" {
		return false, errors.New("harness session ID is empty")
	}
	if launchToken == "" {
		return false, ErrHarnessLaunchChanged
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	conn, err := store.db.Conn(ctx)
	if err != nil {
		return false, fmt.Errorf("claim harness session ID for %q: %w", sessionID, err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return false, fmt.Errorf("lock harness session ID claim for %q: %w", sessionID, err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	runtime, err := readRuntime(ctx, conn, sessionID)
	if err != nil {
		return false, err
	}
	if runtime.Harness != harness || runtime.LaunchToken != launchToken {
		return false, ErrHarnessLaunchChanged
	}
	if runtime.HarnessSessionID != "" {
		if runtime.HarnessSessionID == harnessSessionID {
			return true, nil
		}
		return false, ErrHarnessSessionIDClaimed
	}
	var claimed bool
	if err := conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM session_runtime
			WHERE harness = ? AND harness_session_id = ? AND session_id <> ?
		)
	`, harness, harnessSessionID, sessionID).Scan(&claimed); err != nil {
		return false, fmt.Errorf("check harness session ID claim for %q: %w", sessionID, err)
	}
	if claimed {
		return false, ErrHarnessSessionIDClaimed
	}
	result, err := conn.ExecContext(ctx, `
		UPDATE session_runtime SET harness_session_id = ?, updated_at = ?
		WHERE session_id = ? AND harness = ? AND harness_session_id = '' AND launch_token = ?
	`, harnessSessionID, time.Now().UTC().Format(time.RFC3339Nano), sessionID, harness, launchToken)
	if err != nil {
		return false, fmt.Errorf("record harness session ID for %q: %w", sessionID, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check harness session ID claim for %q: %w", sessionID, err)
	}
	if count != 1 {
		return false, ErrHarnessLaunchChanged
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return false, fmt.Errorf("commit harness session ID claim for %q: %w", sessionID, err)
	}
	committed = true
	return true, nil
}

// SetHarnessSessionID replaces the saved conversation ID of a session that is
// not running and returns the ID it replaced. It refuses an ID that another
// session of the same harness has claimed.
func (store *Store) SetHarnessSessionID(ctx context.Context, sessionID string, harness Harness, harnessSessionID string) (string, error) {
	if harnessSessionID == "" {
		return "", errors.New("harness session ID is empty")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	conn, err := store.db.Conn(ctx)
	if err != nil {
		return "", fmt.Errorf("select conversation for %q: %w", sessionID, err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return "", fmt.Errorf("lock conversation selection for %q: %w", sessionID, err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	runtime, err := readRuntime(ctx, conn, sessionID)
	if err != nil {
		return "", err
	}
	if runtime.Harness != harness {
		return "", fmt.Errorf("session %q has no conversations recorded for harness %q", sessionID, harness)
	}
	if runtime.Status == StatusRunning || runtime.Status == StatusStarting {
		return "", fmt.Errorf("session %q is running; stop it before choosing another conversation", sessionID)
	}
	var claimed bool
	if err := conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM session_runtime
			WHERE harness = ? AND harness_session_id = ? AND session_id <> ?
		)
	`, harness, harnessSessionID, sessionID).Scan(&claimed); err != nil {
		return "", fmt.Errorf("check conversation claim for %q: %w", sessionID, err)
	}
	if claimed {
		return "", ErrHarnessSessionIDClaimed
	}
	if _, err := conn.ExecContext(ctx, `
		UPDATE session_runtime SET harness_session_id = ?, updated_at = ? WHERE session_id = ?
	`, harnessSessionID, time.Now().UTC().Format(time.RFC3339Nano), sessionID); err != nil {
		return "", fmt.Errorf("save chosen conversation for %q: %w", sessionID, err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return "", fmt.Errorf("commit chosen conversation for %q: %w", sessionID, err)
	}
	committed = true
	return runtime.HarnessSessionID, nil
}

// HarnessSessionIDs returns every recorded harness conversation ID for tool.
func (store *Store) HarnessSessionIDs(ctx context.Context, harness Harness) (map[string]bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	rows, err := store.db.QueryContext(ctx, `SELECT harness_session_id FROM session_runtime WHERE harness = ? AND harness_session_id <> ''`, harness)
	if err != nil {
		return nil, fmt.Errorf("read harness session IDs: %w", err)
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read harness session IDs: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

func (store *Store) begin(ctx context.Context, sessionID string, harness Harness, restoring, fresh, generateID bool) (Runtime, error) {
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
	if runtime.HarnessSessionID == "" && generateID {
		runtime.HarnessSessionID, err = newUUID()
		if err != nil {
			return Runtime{}, fmt.Errorf("generate harness session ID: %w", err)
		}
	}
	runtime.SessionID = sessionID
	runtime.Harness = harness
	runtime.LaunchToken, err = newUUID()
	if err != nil {
		return Runtime{}, fmt.Errorf("generate launch token: %w", err)
	}
	runtime.Status = StatusStarting
	runtime.ProcessID = 0
	runtime.LastError = ""
	runtime.UpdatedAt = time.Now().UTC()

	_, err = conn.ExecContext(ctx, `
		INSERT INTO session_runtime (session_id, harness, harness_session_id, status, process_id, last_error, updated_at, launch_token)
		VALUES (?, ?, ?, ?, 0, '', ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			harness = excluded.harness,
			harness_session_id = excluded.harness_session_id,
			status = excluded.status,
			process_id = 0,
			last_error = '',
			updated_at = excluded.updated_at,
			launch_token = excluded.launch_token
	`, runtime.SessionID, runtime.Harness, runtime.HarnessSessionID, runtime.Status, runtime.UpdatedAt.Format(time.RFC3339Nano), runtime.LaunchToken)
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
