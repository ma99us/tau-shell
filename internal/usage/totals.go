package usage

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ma99us/tau-shell/internal/session"
)

// Totals is token usage and its estimated USD cost.
type Totals struct {
	Input      int64   `json:"input"`
	Output     int64   `json:"output"`
	CacheRead  int64   `json:"cacheRead"`
	CacheWrite int64   `json:"cacheWrite"`
	Tokens     int64   `json:"tokens"`
	USD        float64 `json:"usd"`
	// Unpriced is set when some usage had no known price, so USD is a floor.
	Unpriced bool `json:"unpriced"`
}

func (totals *Totals) add(other Totals) {
	totals.Input += other.Input
	totals.Output += other.Output
	totals.CacheRead += other.CacheRead
	totals.CacheWrite += other.CacheWrite
	totals.Tokens += other.Tokens
	totals.USD += other.USD
	totals.Unpriced = totals.Unpriced || other.Unpriced
}

type ModelTotals struct {
	Model string `json:"model"`
	Totals
}

type ConversationTotals struct {
	HarnessSessionID string `json:"harnessSessionId"`
	FirstDay         string `json:"firstDay"`
	LastDay          string `json:"lastDay"`
	Totals
}

// SessionUsage is one TauShell session's running total across every
// conversation it has had.
type SessionUsage struct {
	Total         Totals               `json:"total"`
	Models        []ModelTotals        `json:"models"`
	Conversations []ConversationTotals `json:"conversations"`
	Prices        PriceInfo            `json:"prices"`
}

// Summary is the Home view: a total per live session plus monthly totals for
// all sessions (including deleted ones) and for each harness.
type Summary struct {
	Sessions    map[string]Totals `json:"sessions"`
	Month       Totals            `json:"month"`
	MonthByTool map[string]Totals `json:"monthByTool"`
	PrevMonth   Totals            `json:"prevMonth"`
	PrevByTool  map[string]Totals `json:"prevMonthByTool"`
	MonthStart  string            `json:"monthStart"`
	PrevStart   string            `json:"prevMonthStart"`
	Prices      PriceInfo         `json:"prices"`
}

const dayLayout = "2006-01-02"

// MonthStart returns the start (local midnight) of the usage month that
// contains now, for a month that begins on startDay (1-28).
func MonthStart(now time.Time, startDay int) time.Time {
	if startDay < 1 || startDay > 28 {
		startDay = 1
	}
	year, month, day := now.Date()
	if day < startDay {
		month--
	}
	return time.Date(year, month, startDay, 0, 0, 0, 0, now.Location())
}

type ledgerRow struct {
	sessionID, harnessID, tool, model, day string
	input, output, cacheRead, write5m      int64
	write1h                                int64
	usd                                    float64
	records, unpriced                      int64
}

func (row ledgerRow) totals(prices PriceTable) Totals {
	totals := Totals{
		Input:      row.input,
		Output:     row.output,
		CacheRead:  row.cacheRead,
		CacheWrite: row.write5m + row.write1h,
	}
	totals.Tokens = totals.Input + totals.Output + totals.CacheRead + totals.CacheWrite
	if RecordsOwnCost(session.Harness(row.tool)) {
		totals.USD = row.usd
		totals.Unpriced = row.unpriced > 0
		return totals
	}
	if price, ok := prices.Lookup(row.model); ok {
		totals.USD = price.Cost(row.input, row.output, row.cacheRead, row.write5m, row.write1h)
	} else if totals.Tokens > 0 {
		totals.Unpriced = true
	}
	return totals
}

func (ledger *Ledger) rows(where string, args ...any) ([]ledgerRow, error) {
	query := `SELECT session_id, harness_session_id, tool, model, day,
		SUM(input), SUM(output), SUM(cache_read), SUM(cache_write_5m), SUM(cache_write_1h),
		SUM(copilot_usd), SUM(records), SUM(unpriced)
		FROM usage_ledger ` + where + ` GROUP BY session_id, harness_session_id, tool, model, day`
	rows, err := ledger.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("read usage ledger: %w", err)
	}
	defer rows.Close()
	var result []ledgerRow
	for rows.Next() {
		var row ledgerRow
		if err := rows.Scan(&row.sessionID, &row.harnessID, &row.tool, &row.model, &row.day,
			&row.input, &row.output, &row.cacheRead, &row.write5m, &row.write1h,
			&row.usd, &row.records, &row.unpriced); err != nil {
			return nil, fmt.Errorf("read usage ledger: %w", err)
		}
		if row.records <= 0 {
			continue
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func displayModel(model string) string {
	return strings.TrimSuffix(model, largePromptSuffix)
}

// SessionUsage returns the running total for one session, priced with prices.
func (ledger *Ledger) SessionUsage(sessionID string, prices PriceTable) (SessionUsage, error) {
	rows, err := ledger.rows(`WHERE session_id = ?`, sessionID)
	if err != nil {
		return SessionUsage{}, err
	}
	result := SessionUsage{Models: []ModelTotals{}, Conversations: []ConversationTotals{}}
	models := map[string]*ModelTotals{}
	conversations := map[string]*ConversationTotals{}
	for _, row := range rows {
		totals := row.totals(prices)
		result.Total.add(totals)
		name := displayModel(row.model)
		model := models[name]
		if model == nil {
			model = &ModelTotals{Model: name}
			models[name] = model
		}
		model.add(totals)
		conversation := conversations[row.harnessID]
		if conversation == nil {
			conversation = &ConversationTotals{HarnessSessionID: row.harnessID, FirstDay: row.day, LastDay: row.day}
			conversations[row.harnessID] = conversation
		}
		conversation.add(totals)
		if row.day < conversation.FirstDay {
			conversation.FirstDay = row.day
		}
		if row.day > conversation.LastDay {
			conversation.LastDay = row.day
		}
	}
	for _, model := range models {
		result.Models = append(result.Models, *model)
	}
	sort.Slice(result.Models, func(a, b int) bool { return result.Models[a].Tokens > result.Models[b].Tokens })
	for _, conversation := range conversations {
		result.Conversations = append(result.Conversations, *conversation)
	}
	sort.Slice(result.Conversations, func(a, b int) bool {
		return result.Conversations[a].FirstDay < result.Conversations[b].FirstDay
	})
	return result, nil
}

// Summary totals every session (deleted ones only count toward the monthly
// figures) and the current and previous usage months.
func (ledger *Ledger) Summary(now time.Time, startDay int, prices PriceTable) (Summary, error) {
	rows, err := ledger.rows(``)
	if err != nil {
		return Summary{}, err
	}
	monthStart := MonthStart(now, startDay)
	prevStart := MonthStart(monthStart.AddDate(0, 0, -1), startDay)
	monthDay, prevDay := monthStart.Format(dayLayout), prevStart.Format(dayLayout)
	summary := Summary{
		Sessions:    map[string]Totals{},
		MonthByTool: map[string]Totals{},
		PrevByTool:  map[string]Totals{},
		MonthStart:  monthDay,
		PrevStart:   prevDay,
	}
	for _, row := range rows {
		totals := row.totals(prices)
		if !strings.HasPrefix(row.sessionID, deletedPrefix) {
			session := summary.Sessions[row.sessionID]
			session.add(totals)
			summary.Sessions[row.sessionID] = session
		}
		switch {
		case row.day >= monthDay:
			summary.Month.add(totals)
			byTool := summary.MonthByTool[row.tool]
			byTool.add(totals)
			summary.MonthByTool[row.tool] = byTool
		case row.day >= prevDay:
			summary.PrevMonth.add(totals)
			byTool := summary.PrevByTool[row.tool]
			byTool.add(totals)
			summary.PrevByTool[row.tool] = byTool
		}
	}
	return summary, nil
}
