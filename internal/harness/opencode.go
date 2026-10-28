package harness

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// OpenCode 1.19 keeps its conversations in a SQLite file. TauShell only ever
// reads it, read-only, and checks the schema first; any mismatch disables the
// OpenCode-specific features instead of guessing.

func openCodeDatabasePath() string {
	root := os.Getenv("XDG_DATA_HOME")
	if root == "" {
		root = filepath.Join(userHome(), ".local", "share")
	}
	return filepath.Join(root, "opencode", "opencode.db")
}

var errNoOpenCodeDatabase = errors.New("OpenCode database not found")

func openOpenCodeDatabase(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, errNoOpenCodeDatabase
	}
	location := url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(path), RawQuery: "mode=ro&_pragma=busy_timeout(3000)"}
	db, err := sql.Open("sqlite", location.String())
	if err != nil {
		return nil, fmt.Errorf("open OpenCode database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := checkOpenCodeSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func checkOpenCodeSchema(db *sql.DB) error {
	want := map[string][]string{
		"session": {"id", "directory", "parent_id", "time_created"},
		"message": {"id", "session_id", "time_created", "time_updated", "data"},
	}
	for table, columns := range want {
		rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			return fmt.Errorf("probe OpenCode schema: %w", err)
		}
		have := map[string]bool{}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				_ = rows.Close()
				return fmt.Errorf("probe OpenCode schema: %w", err)
			}
			have[name] = true
		}
		_ = rows.Close()
		for _, column := range columns {
			if !have[column] {
				return fmt.Errorf("OpenCode database schema changed (%s.%s missing); OpenCode features are disabled", table, column)
			}
		}
	}
	return nil
}

func discoverOpenCodeSession(workingDir string, since time.Time, taken map[string]bool) (string, error) {
	db, err := openOpenCodeDatabase(openCodeDatabasePath())
	if errors.Is(err, errNoOpenCodeDatabase) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer db.Close()
	return discoverOpenCodeIn(db, workingDir, since, taken)
}

func discoverOpenCodeIn(db *sql.DB, workingDir string, since time.Time, taken map[string]bool) (string, error) {
	rows, err := db.Query(`SELECT id, directory FROM session WHERE parent_id IS NULL AND time_created >= ? ORDER BY time_created`,
		since.Add(-2*time.Second).UnixMilli())
	if err != nil {
		return "", fmt.Errorf("find OpenCode session: %w", err)
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var id, directory string
		if err := rows.Scan(&id, &directory); err != nil {
			return "", fmt.Errorf("find OpenCode session: %w", err)
		}
		if !taken[id] && samePath(directory, workingDir) {
			found = append(found, id)
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("find OpenCode session: %w", err)
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		return found[0], nil
	default:
		return "", ErrAmbiguousSession
	}
}

func checkOpenCodeResume(harnessSessionID string) error {
	db, err := openOpenCodeDatabase(openCodeDatabasePath())
	if err != nil {
		// Unknown storage never blocks a resume attempt.
		return nil
	}
	defer db.Close()
	return checkOpenCodeResumeIn(db, harnessSessionID)
}

func checkOpenCodeResumeIn(db *sql.DB, harnessSessionID string) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM message WHERE session_id = ?`, harnessSessionID).Scan(&count); err != nil {
		return nil
	}
	if count == 0 {
		return errors.New("OpenCode has no recorded conversation for this session yet; use Start fresh")
	}
	return nil
}

// OpenCodeMessage is one assistant message with the usage OpenCode recorded.
type OpenCodeMessage struct {
	ID         string
	Created    time.Time
	Model      string
	Input      int64
	Output     int64
	Reasoning  int64
	CacheRead  int64
	CacheWrite int64
	USD        float64
}

type openCodeData struct {
	Role    string  `json:"role"`
	ModelID string  `json:"modelID"`
	Cost    float64 `json:"cost"`
	Tokens  struct {
		Input     int64 `json:"input"`
		Output    int64 `json:"output"`
		Reasoning int64 `json:"reasoning"`
		Cache     struct {
			Read  int64 `json:"read"`
			Write int64 `json:"write"`
		} `json:"cache"`
	} `json:"tokens"`
	Model struct {
		ModelID string `json:"modelID"`
	} `json:"model"`
	Time struct {
		Completed int64 `json:"completed"`
	} `json:"time"`
}

// OpenCodeMessages returns the finished assistant messages of one conversation
// and a version number that changes whenever any message does.
func OpenCodeMessages(harnessSessionID string) ([]OpenCodeMessage, int64, error) {
	db, err := openOpenCodeDatabase(openCodeDatabasePath())
	if err != nil {
		if errors.Is(err, errNoOpenCodeDatabase) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	defer db.Close()
	return openCodeMessagesIn(db, harnessSessionID)
}

func openCodeMessagesIn(db *sql.DB, harnessSessionID string) ([]OpenCodeMessage, int64, error) {
	rows, err := db.Query(`SELECT id, time_created, time_updated, data FROM message WHERE session_id = ? ORDER BY time_created`, harnessSessionID)
	if err != nil {
		return nil, 0, fmt.Errorf("read OpenCode messages: %w", err)
	}
	defer rows.Close()
	var messages []OpenCodeMessage
	var version int64
	for rows.Next() {
		var id, data string
		var created, updated int64
		if err := rows.Scan(&id, &created, &updated, &data); err != nil {
			return nil, 0, fmt.Errorf("read OpenCode messages: %w", err)
		}
		if updated > version {
			version = updated
		}
		var parsed openCodeData
		if json.Unmarshal([]byte(data), &parsed) != nil || parsed.Role != "assistant" || parsed.Time.Completed == 0 {
			continue
		}
		messages = append(messages, OpenCodeMessage{
			ID: id, Created: time.UnixMilli(created), Model: parsed.ModelID,
			Input: parsed.Tokens.Input, Output: parsed.Tokens.Output, Reasoning: parsed.Tokens.Reasoning,
			CacheRead: parsed.Tokens.Cache.Read, CacheWrite: parsed.Tokens.Cache.Write, USD: parsed.Cost,
		})
	}
	return messages, version, rows.Err()
}

func openCodeModel(harnessSessionID string) string {
	db, err := openOpenCodeDatabase(openCodeDatabasePath())
	if err != nil {
		return ""
	}
	defer db.Close()
	return openCodeModelIn(db, harnessSessionID)
}

func openCodeModelIn(db *sql.DB, harnessSessionID string) string {
	rows, err := db.Query(`SELECT data FROM message WHERE session_id = ? ORDER BY time_created DESC LIMIT 20`, harnessSessionID)
	if err != nil {
		return ""
	}
	defer rows.Close()
	for rows.Next() {
		var data string
		if rows.Scan(&data) != nil {
			return ""
		}
		var parsed openCodeData
		if json.Unmarshal([]byte(data), &parsed) != nil {
			continue
		}
		if strings.TrimSpace(parsed.ModelID) != "" {
			return parsed.ModelID
		}
		if strings.TrimSpace(parsed.Model.ModelID) != "" {
			return parsed.Model.ModelID
		}
	}
	return ""
}
