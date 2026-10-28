package usage

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ma99us/tau-shell/internal/session"

	_ "modernc.org/sqlite"
)

const deletedPrefix = "deleted:"

// Ledger is the durable usage record in state.db. Every harness log file is
// read once, incrementally: the byte offset reached is saved with the totals so
// a restart or a resume continues counting without double counting, and the
// totals outlive the harness's own logs.
type Ledger struct {
	db *sql.DB
	mu sync.Mutex
}

func OpenLedger(path string) (*Ledger, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open usage ledger: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA busy_timeout = 5000;
		CREATE TABLE IF NOT EXISTS usage_ledger (
			session_id TEXT NOT NULL,
			harness_session_id TEXT NOT NULL,
			tool TEXT NOT NULL,
			model TEXT NOT NULL,
			day TEXT NOT NULL,
			source_file TEXT NOT NULL,
			input INTEGER NOT NULL DEFAULT 0,
			output INTEGER NOT NULL DEFAULT 0,
			cache_read INTEGER NOT NULL DEFAULT 0,
			cache_write_5m INTEGER NOT NULL DEFAULT 0,
			cache_write_1h INTEGER NOT NULL DEFAULT 0,
			copilot_usd REAL NOT NULL DEFAULT 0,
			records INTEGER NOT NULL DEFAULT 0,
			unpriced INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (session_id, harness_session_id, model, day, source_file)
		);
		CREATE TABLE IF NOT EXISTS usage_files (
			path TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			harness_session_id TEXT NOT NULL,
			offset INTEGER NOT NULL,
			carry TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize usage ledger: %w", err)
	}
	return &Ledger{db: db}, nil
}

func (ledger *Ledger) Close() error {
	return ledger.db.Close()
}

type delta struct {
	input, output, cacheRead, write5m, write1h int64
	usd                                        float64
	records, unpriced                          int64
}

// carried is the last Claude message counted from a file. Its chunks can
// continue in the next read, replacing what was counted.
type carried struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Day     string `json:"day"`
	Input   int64  `json:"input"`
	Output  int64  `json:"output"`
	Read    int64  `json:"read"`
	Write5m int64  `json:"write5m"`
	Write1h int64  `json:"write1h"`
}

func (item carried) record() Record {
	return Record{MessageID: item.ID, Model: item.Model, Input: item.Input, Output: item.Output,
		CacheRead: item.Read, CacheWrite5m: item.Write5m, CacheWrite1h: item.Write1h, Priced: true}
}

// IngestFile counts the usage appended to one log file since it was last read.
func (ledger *Ledger) IngestFile(sessionID string, tool session.Harness, harnessSessionID, path string) error {
	parse := LineParser(tool)
	if parse == nil && tool != session.Codex {
		return nil
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	var offset int64
	var carryJSON string
	known := true
	err = ledger.db.QueryRow(`SELECT offset, carry FROM usage_files WHERE path = ?`, path).Scan(&offset, &carryJSON)
	if errors.Is(err, sql.ErrNoRows) {
		known = false
	} else if err != nil {
		return fmt.Errorf("read usage file state: %w", err)
	}
	reset := false
	if known && info.Size() < offset {
		reset, offset, carryJSON = true, 0, ""
	}
	if known && !reset && info.Size() == offset {
		return nil
	}

	if tool == session.Codex {
		parse = newCodexParser(codexModelBefore(path, offset)).parse
	}
	var previous *carried
	if carryJSON != "" {
		var item carried
		if json.Unmarshal([]byte(carryJSON), &item) == nil && item.ID != "" {
			previous = &item
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open usage log: %w", err)
	}
	defer file.Close()
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("read usage log: %w", err)
	}

	deltas := map[string]*delta{}
	days := map[string]string{}
	models := map[string]string{}
	apply := func(record Record, day string, sign int64) {
		key := record.Model + "\x00" + day
		current := deltas[key]
		if current == nil {
			current = &delta{}
			deltas[key] = current
			days[key], models[key] = day, record.Model
		}
		current.input += sign * record.Input
		current.output += sign * record.Output
		current.cacheRead += sign * record.CacheRead
		current.write5m += sign * record.CacheWrite5m
		current.write1h += sign * record.CacheWrite1h
		current.records += sign
		if RecordsOwnCost(tool) {
			current.usd += float64(sign) * record.USD
			if !record.Priced {
				current.unpriced += sign
			}
		}
	}

	seen := map[string]carried{}
	if previous != nil {
		seen[previous.ID] = *previous
	}
	last := previous
	reader := bufio.NewReaderSize(file, 256<<10)
	consumed := offset
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return fmt.Errorf("read usage log: %w", readErr)
			}
			break
		}
		consumed += int64(len(line))
		record, ok := parse(bytes.TrimSpace(line))
		if !ok {
			continue
		}
		day := record.Time.Local().Format(dayLayout)
		if record.MessageID != "" {
			if earlier, dup := seen[record.MessageID]; dup {
				apply(earlier.record(), earlier.Day, -1)
			}
			item := carried{ID: record.MessageID, Model: record.Model, Day: day, Input: record.Input, Output: record.Output,
				Read: record.CacheRead, Write5m: record.CacheWrite5m, Write1h: record.CacheWrite1h}
			seen[record.MessageID] = item
			last = &item
		}
		apply(record, day, 1)
	}

	carryOut := ""
	if last != nil {
		data, err := json.Marshal(last)
		if err != nil {
			return fmt.Errorf("save usage carry: %w", err)
		}
		carryOut = string(data)
	}
	return ledger.commit(path, sessionID, tool, harnessSessionID, consumed, carryOut, reset, deltas, days, models)
}

func (ledger *Ledger) commit(path, sessionID string, tool session.Harness, harnessID string, offset int64, carry string,
	reset bool, deltas map[string]*delta, days, models map[string]string) error {
	tx, err := ledger.db.Begin()
	if err != nil {
		return fmt.Errorf("save usage: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if reset {
		if _, err := tx.Exec(`DELETE FROM usage_ledger WHERE source_file = ?`, path); err != nil {
			return fmt.Errorf("save usage: %w", err)
		}
	}
	for key, change := range deltas {
		if _, err := tx.Exec(`
			INSERT INTO usage_ledger (session_id, harness_session_id, tool, model, day, source_file,
				input, output, cache_read, cache_write_5m, cache_write_1h, copilot_usd, records, unpriced)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(session_id, harness_session_id, model, day, source_file) DO UPDATE SET
				input = input + excluded.input, output = output + excluded.output,
				cache_read = cache_read + excluded.cache_read,
				cache_write_5m = cache_write_5m + excluded.cache_write_5m,
				cache_write_1h = cache_write_1h + excluded.cache_write_1h,
				copilot_usd = copilot_usd + excluded.copilot_usd,
				records = records + excluded.records, unpriced = unpriced + excluded.unpriced
		`, sessionID, harnessID, string(tool), models[key], days[key], path,
			change.input, change.output, change.cacheRead, change.write5m, change.write1h, change.usd, change.records, change.unpriced); err != nil {
			return fmt.Errorf("save usage: %w", err)
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO usage_files (path, session_id, harness_session_id, offset, carry) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET session_id = excluded.session_id, harness_session_id = excluded.harness_session_id,
			offset = excluded.offset, carry = excluded.carry
	`, path, sessionID, harnessID, offset, carry); err != nil {
		return fmt.Errorf("save usage: %w", err)
	}
	return tx.Commit()
}

// Tombstone detaches a deleted session's usage: it keeps counting toward the
// monthly totals but no longer belongs to any session, so a new session that
// reuses the ID starts at zero.
func (ledger *Ledger) Tombstone(sessionID string, now time.Time) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	tx, err := ledger.db.Begin()
	if err != nil {
		return fmt.Errorf("detach usage of %q: %w", sessionID, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE usage_ledger SET session_id = ? WHERE session_id = ?`,
		deletedPrefix+sessionID+":"+strconv.FormatInt(now.UnixNano(), 10), sessionID); err != nil {
		return fmt.Errorf("detach usage of %q: %w", sessionID, err)
	}
	if _, err := tx.Exec(`DELETE FROM usage_files WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("detach usage of %q: %w", sessionID, err)
	}
	return tx.Commit()
}
