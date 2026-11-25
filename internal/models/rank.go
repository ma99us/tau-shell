// Package models builds the per-harness model menu: it lists the models a
// harness can use (from its manifest plus what the installed harness reports)
// and ranks them with the MEI coding index when MEI data is available.
// Nothing here may fail the menu: probes and ranking data are optional.
package models

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ma99us/tau-shell/internal/mei"
)

// Candidate is a model a harness can use.
type Candidate struct {
	// ID is what TauShell sends to the harness (a model id or picker search text).
	ID    string
	Label string
	// MatchKey is what is compared against MEI; it defaults to ID.
	MatchKey string
}

// Option is a candidate with optional MEI ranking data.
type Option struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	Ranked     bool    `json:"ranked"`
	Rank       int     `json:"rank,omitempty"`
	MEIName    string  `json:"meiName,omitempty"`
	Score      float64 `json:"score,omitempty"`
	Cost       float64 `json:"cost,omitempty"`
	Tokens     float64 `json:"tokens,omitempty"`
	Confidence string  `json:"confidence,omitempty"`
}

var (
	provider   = regexp.MustCompile(`^.*/`)
	dateSuffix = regexp.MustCompile(`-\d{8}$`)
	digits     = regexp.MustCompile(`^\d+$`)
	separators = strings.NewReplacer(".", "-", "_", "-", " ", "-", ":", "-")
	// Words that name a provider or a release channel rather than a model.
	noise = map[string]bool{
		"github": true, "copilot": true, "anthropic": true, "openai": true,
		"google": true, "free": true, "preview": true, "latest": true,
	}
	// Anthropic family aliases accepted by Claude Code, resolved to the newest
	// MEI entry of that family unless the harness settings pin a model.
	aliases = map[string]bool{"opus": true, "sonnet": true, "haiku": true, "fable": true}
)

type modelKey struct {
	words   []string
	version []int
}

func (key modelKey) wordString() string { return strings.Join(key.words, "-") }

func parseKey(text string) modelKey {
	text = strings.ToLower(strings.TrimSpace(provider.ReplaceAllString(text, "")))
	text = dateSuffix.ReplaceAllString(separators.Replace(text), "")
	var key modelKey
	for _, token := range strings.Split(text, "-") {
		switch {
		case token == "":
		case digits.MatchString(token):
			number, _ := strconv.Atoi(token)
			key.version = append(key.version, number)
		case noise[token]:
		default:
			key.words = append(key.words, token)
		}
	}
	sort.Strings(key.words)
	return key
}

func sameVersion(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func newerVersion(left, right []int) bool {
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index] != right[index] {
			return left[index] > right[index]
		}
	}
	return len(left) > len(right)
}

func subset(small, large []string) bool {
	set := map[string]bool{}
	for _, word := range large {
		set[word] = true
	}
	for _, word := range small {
		if !set[word] {
			return false
		}
	}
	return len(small) > 0
}

type indexed struct {
	entry mei.Entry
	key   modelKey
	// name is the key of MEI's short display name, such as "V4.1 Flash".
	name modelKey
	// vendor holds the words of the vendor slug, which harness ids often add.
	vendor map[string]bool
}

func index(entries []mei.Entry) []indexed {
	out := make([]indexed, len(entries))
	for position, entry := range entries {
		out[position] = indexed{entry: entry, key: parseKey(entry.ID), name: parseKey(entry.Name), vendor: map[string]bool{}}
		for _, word := range strings.Split(strings.ToLower(entry.Vendor), "-") {
			out[position].vendor[word] = true
		}
		if entry.Vendor == "anthropic" {
			out[position].vendor["claude"] = true
		}
	}
	return out
}

// onlyVendorWords reports whether every word in extra is a vendor word.
func onlyVendorWords(extra []string, vendor map[string]bool) bool {
	for _, word := range extra {
		if !vendor[word] {
			return false
		}
	}
	return true
}

func without(words, remove []string) []string {
	drop := map[string]bool{}
	for _, word := range remove {
		drop[word] = true
	}
	var rest []string
	for _, word := range words {
		if !drop[word] {
			rest = append(rest, word)
		}
	}
	return rest
}

// match finds the MEI entry for a model id: an exact match on MEI's id, then
// a family alias (opus, sonnet), then MEI's display name with only vendor
// words added. A model with extra words (for example "-flash") never matches a
// different model's entry, and an ambiguous match stays unranked.
func match(entries []indexed, text string) (mei.Entry, bool) {
	key := parseKey(text)
	if len(key.words) == 0 {
		return mei.Entry{}, false
	}
	for _, item := range entries {
		if item.key.wordString() == key.wordString() && sameVersion(item.key.version, key.version) {
			return item.entry, true
		}
	}
	if len(key.version) == 0 {
		if len(key.words) == 1 && aliases[key.words[0]] {
			var best *indexed
			for position := range entries {
				item := &entries[position]
				if item.entry.Vendor == "anthropic" && subset(key.words, item.key.words) &&
					(best == nil || newerVersion(item.key.version, best.key.version)) {
					best = item
				}
			}
			if best != nil {
				return best.entry, true
			}
		}
		return mei.Entry{}, false
	}
	var found []mei.Entry
	for _, item := range entries {
		if sameVersion(item.name.version, key.version) && subset(item.name.words, key.words) &&
			onlyVendorWords(without(key.words, item.name.words), item.vendor) {
			found = append(found, item.entry)
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return mei.Entry{}, false
}

// Rank attaches MEI data to candidates. Ranked options come first, best score
// first; unranked options follow in their original order. With no MEI data,
// the candidates come back unranked and in order.
func Rank(candidates []Candidate, entries []mei.Entry) []Option {
	table := index(entries)
	var ranked, unranked []Option
	seen := map[string]bool{}
	for _, candidate := range candidates {
		id := strings.TrimSpace(candidate.ID)
		if id == "" || seen[strings.ToLower(id)] {
			continue
		}
		seen[strings.ToLower(id)] = true
		option := Option{ID: id, Label: candidate.Label}
		if option.Label == "" {
			option.Label = id
		}
		matchText := candidate.MatchKey
		if matchText == "" {
			matchText = id
		}
		if entry, ok := match(table, matchText); ok {
			option.Ranked, option.Rank, option.MEIName = true, entry.Rank, entry.Name
			option.Score, option.Cost, option.Tokens, option.Confidence = entry.Score, entry.Cost, entry.Tokens, entry.Confidence
			ranked = append(ranked, option)
			continue
		}
		unranked = append(unranked, option)
	}
	sort.SliceStable(ranked, func(left, right int) bool { return ranked[left].Rank < ranked[right].Rank })
	return append(ranked, unranked...)
}
