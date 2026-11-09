package sessionhost

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/ma99us/tau-shell/internal/harness"
	"github.com/ma99us/tau-shell/internal/session"
)

const maxListedConversations = 100

// Conversation is one harness conversation a session has had. Titles and
// totals come from the harness's own logs and the usage ledger; no
// conversation content is kept.
type Conversation struct {
	HarnessSessionID string  `json:"harnessSessionId"`
	Title            string  `json:"title"`
	Model            string  `json:"model"`
	FirstDay         string  `json:"firstDay"`
	LastDay          string  `json:"lastDay"`
	Tokens           int64   `json:"tokens"`
	USD              float64 `json:"usd"`
	Unpriced         bool    `json:"unpriced"`
	// Current marks the conversation Resume would continue.
	Current bool `json:"current"`
	// Unavailable explains why the harness cannot resume it; "" when it can.
	Unavailable string `json:"unavailable"`
}

// Conversations lists a session's known harness conversations, newest first.
func (manager *Manager) Conversations(ctx context.Context, id string) ([]Conversation, error) {
	config, err := session.FindConfig(manager.configDirectory, id)
	if err != nil {
		return nil, err
	}
	adapter, err := harness.For(config.Tool)
	if err != nil {
		return nil, err
	}
	runtime, err := manager.store.Runtime(ctx, id)
	if err != nil {
		return nil, err
	}
	totals, err := manager.SessionUsage(ctx, id)
	if err != nil {
		return nil, err
	}
	byID := map[string]*Conversation{}
	var list []*Conversation
	for _, item := range totals.Conversations {
		if item.HarnessSessionID == "" {
			continue
		}
		entry := &Conversation{
			HarnessSessionID: item.HarnessSessionID,
			FirstDay:         item.FirstDay,
			LastDay:          item.LastDay,
			Tokens:           item.Tokens,
			USD:              item.USD,
			Unpriced:         item.Unpriced,
		}
		byID[entry.HarnessSessionID] = entry
		list = append(list, entry)
	}
	if runtime.HarnessSessionID != "" && byID[runtime.HarnessSessionID] == nil {
		entry := &Conversation{HarnessSessionID: runtime.HarnessSessionID}
		byID[entry.HarnessSessionID] = entry
		list = append(list, entry)
	}
	if current := byID[runtime.HarnessSessionID]; current != nil {
		current.Current = true
	}
	sort.SliceStable(list, func(a, b int) bool {
		if list[a].Current != list[b].Current {
			return list[a].Current
		}
		return list[a].LastDay > list[b].LastDay
	})
	if len(list) > maxListedConversations {
		list = list[:maxListedConversations]
	}
	home, readable := manager.readerHome(ctx, config)
	result := make([]Conversation, 0, len(list))
	for _, entry := range list {
		if readable {
			entry.Title = harness.ConversationTitleIn(home, config.Tool, entry.HarnessSessionID)
			entry.Model = harness.CurrentModelIn(home, config.Tool, entry.HarnessSessionID)
			if err := adapter.CheckResumeIn(home, entry.HarnessSessionID); err != nil {
				entry.Unavailable = err.Error()
			}
		}
		result = append(result, *entry)
	}
	return result, nil
}

// ResumeConversation makes harnessSessionID the session's saved conversation
// and restores it. It only accepts a conversation this session has had, and
// puts the previous ID back if the launch fails.
func (manager *Manager) ResumeConversation(ctx context.Context, id, harnessSessionID string, columns, rows int) (Snapshot, error) {
	if current := manager.get(id); current != nil && current.isRunning() {
		return Snapshot{}, fmt.Errorf("session %q is running; stop it before choosing another conversation", id)
	}
	config, err := session.FindConfig(manager.configDirectory, id)
	if err != nil {
		return Snapshot{}, err
	}
	adapter, err := harness.For(config.Tool)
	if err != nil {
		return Snapshot{}, err
	}
	known, err := manager.Conversations(ctx, id)
	if err != nil {
		return Snapshot{}, err
	}
	var chosen *Conversation
	for index := range known {
		if known[index].HarnessSessionID == harnessSessionID {
			chosen = &known[index]
		}
	}
	if chosen == nil {
		return Snapshot{}, errors.New("that conversation does not belong to this session")
	}
	if chosen.Unavailable != "" {
		return Snapshot{}, errors.New(chosen.Unavailable)
	}
	if home, readable := manager.readerHome(ctx, config); readable {
		if err := adapter.CheckResumeIn(home, harnessSessionID); err != nil {
			return Snapshot{}, err
		}
	}
	if chosen.Current {
		return manager.start(ctx, id, true, false, columns, rows)
	}
	// Capture the outgoing conversation's unread usage before the ID changes.
	manager.ingestSession(ctx, id, true)
	previous, err := manager.store.SetHarnessSessionID(ctx, id, config.Tool, harnessSessionID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot, err := manager.start(ctx, id, true, false, columns, rows)
	if err != nil {
		if _, revertErr := manager.store.SetHarnessSessionID(ctx, id, config.Tool, previous); revertErr != nil && previous != "" {
			return Snapshot{}, errors.Join(err, fmt.Errorf("could not restore the previous conversation ID: %w", revertErr))
		}
		return Snapshot{}, err
	}
	return snapshot, nil
}
