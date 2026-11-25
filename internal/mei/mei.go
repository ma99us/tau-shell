// Package mei reads the Models Engineering Index (https://git-aarvin.github.io/mei/),
// a third-party ranking of language models on engineering tasks. MEI is an
// external site that can disappear or change at any time, so every consumer
// must treat the data as optional: Store always returns something usable
// (fresh, stale or built-in snapshot data, or nothing) and never blocks.
package mei

import (
	"errors"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Entry is one row of the MEI ranking table.
type Entry struct {
	Rank   int    `json:"rank"`
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	// Slug is the site path segment, for example "opus-5.5".
	Slug string `json:"slug"`
	// ID is MEI's canonical model id, for example "claude-opus-5-5".
	ID     string  `json:"id"`
	Score  float64 `json:"score"`
	Cost   float64 `json:"cost"`
	Tokens float64 `json:"tokens"`
	// Confidence is "confident", "rough" or empty when MEI shows no badge.
	Confidence string `json:"confidence,omitempty"`
}

const (
	minRows  = 10
	maxScore = 200.0
)

var (
	errNoTable = errors.New("mei: ranking table not found")
	rowSplit   = regexp.MustCompile(`(?i)<tr[\s>]`)
	modelLink  = regexp.MustCompile(`(?is)<a[^>]*class="model-link"[^>]*>`)
	hrefAttr   = regexp.MustCompile(`href="\./([^"/]+)/([^"/]+)"`)
	idAttr     = regexp.MustCompile(`data-model-link="([^"]+)"`)
	nameAttr   = regexp.MustCompile(`(?s)<td class="model" data-value="([^"]*)"`)
	rankAttr   = regexp.MustCompile(`<td class="rank" data-value="([0-9]+)"`)
	numCell    = regexp.MustCompile(`<td class="num" data-value="([^"]*)"`)
	badgeAttr  = regexp.MustCompile(`class="badge"[^>]*aria-label="([^"]*)"`)
)

// Parse reads the ranking table from the MEI home page. It fails rather than
// returning a partial table: callers keep their last good data on error.
func Parse(page string) ([]Entry, error) {
	start := strings.Index(page, `aria-label="Model rankings"`)
	if start < 0 {
		return nil, errNoTable
	}
	table := page[start:]
	if end := strings.Index(table, "</table>"); end >= 0 {
		table = table[:end]
	}
	var entries []Entry
	for _, row := range rowSplit.Split(table, -1)[1:] {
		if !strings.Contains(row, `class="model-link"`) {
			continue
		}
		entry, err := parseRow(row)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, Validate(entries)
}

func parseRow(row string) (Entry, error) {
	var entry Entry
	rank := rankAttr.FindStringSubmatch(row)
	name := nameAttr.FindStringSubmatch(row)
	link := modelLink.FindString(row)
	href := hrefAttr.FindStringSubmatch(link)
	id := idAttr.FindStringSubmatch(link)
	numbers := numCell.FindAllStringSubmatch(row, -1)
	if rank == nil || name == nil || href == nil || id == nil || len(numbers) != 3 {
		return entry, errors.New("mei: ranking row has an unexpected layout")
	}
	var err error
	if entry.Rank, err = strconv.Atoi(rank[1]); err != nil {
		return entry, fmt.Errorf("mei: bad rank %q", rank[1])
	}
	values := [3]float64{}
	for index, match := range numbers {
		if values[index], err = strconv.ParseFloat(match[1], 64); err != nil {
			return entry, fmt.Errorf("mei: bad number %q", match[1])
		}
	}
	entry.Name = html.UnescapeString(name[1])
	entry.Vendor, entry.Slug, entry.ID = href[1], href[2], id[1]
	entry.Score, entry.Cost, entry.Tokens = values[0], values[1], values[2]
	if badge := badgeAttr.FindStringSubmatch(row); badge != nil {
		switch strings.ToLower(badge[1]) {
		case "confident score":
			entry.Confidence = "confident"
		case "rough estimate":
			entry.Confidence = "rough"
		}
	}
	return entry, nil
}

// Validate rejects tables that are too small or contain implausible values, so
// a changed page layout or an error page never replaces good data.
func Validate(entries []Entry) error {
	if len(entries) < minRows {
		return fmt.Errorf("mei: only %d ranking rows (need at least %d)", len(entries), minRows)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.ID == "" || entry.Name == "" || entry.Vendor == "" {
			return errors.New("mei: ranking row is missing a name, vendor or id")
		}
		if entry.Score < 0 || entry.Score > maxScore || entry.Cost < 0 || entry.Tokens < 0 {
			return fmt.Errorf("mei: implausible numbers for %q", entry.Name)
		}
		if seen[entry.ID] {
			return fmt.Errorf("mei: duplicate model id %q", entry.ID)
		}
		seen[entry.ID] = true
	}
	return nil
}
