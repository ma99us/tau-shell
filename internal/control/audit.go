package control

import (
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// AuditEntry records one request to the control channel. It deliberately has
// no field for arguments, prompt text or terminal content.
type AuditEntry struct {
	Time    time.Time `json:"time"`
	PID     uint32    `json:"pid"`
	Image   string    `json:"image"`
	Op      string    `json:"op"`
	Session string    `json:"session,omitempty"`
	Outcome string    `json:"outcome"`
}

// AuditLog keeps the most recent entries in memory for the UI. Nothing is
// persisted: the trail is for the current run only.
type AuditLog struct {
	mu      sync.Mutex
	limit   int
	entries []AuditEntry
	notify  func(AuditEntry)
}

// NewAuditLog keeps at most limit entries; notify, if set, is called after each add.
func NewAuditLog(limit int, notify func(AuditEntry)) *AuditLog {
	return &AuditLog{limit: limit, notify: notify}
}

// Add appends an entry, dropping the oldest when full.
func (log *AuditLog) Add(entry AuditEntry) {
	log.mu.Lock()
	log.entries = append(log.entries, entry)
	if len(log.entries) > log.limit {
		log.entries = append([]AuditEntry(nil), log.entries[len(log.entries)-log.limit:]...)
	}
	notify := log.notify
	log.mu.Unlock()
	if notify != nil {
		notify(entry)
	}
}

// Entries returns a copy, newest last.
func (log *AuditLog) Entries() []AuditEntry {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]AuditEntry(nil), log.entries...)
}

// decode unmarshals a successful response's data or returns its error.
func decode(response Response, target any) error {
	if !response.OK {
		if response.Error != nil {
			return response.Error
		}
		return errors.New("request failed")
	}
	return json.Unmarshal(response.Data, target)
}
