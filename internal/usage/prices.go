// Package usage reads token usage from harness session logs, prices it, and
// keeps a durable per-session ledger so totals survive restarts and resumes.
package usage

import (
	"regexp"
	"strings"
)

// BuiltinPricesDate is when the built-in Claude price table was last checked
// against Anthropic's pricing page.
const BuiltinPricesDate = "2026-10-27"

const largePromptSuffix = "#large"

// Price is a Claude price in USD per million tokens.
type Price struct {
	Input        float64 `json:"input"`
	CacheWrite5m float64 `json:"cache_write_5m"`
	CacheWrite1h float64 `json:"cache_write_1h"`
	CacheRead    float64 `json:"cache_read"`
	Output       float64 `json:"output"`
}

// PriceTable maps normalized model keys such as "opus-5.5" to prices. The key
// "haiku-5.5>100k" holds the tier for prompts over 100,000 tokens.
type PriceTable map[string]Price

var modelPattern = regexp.MustCompile(`(opus|sonnet|haiku|fable|mythos)-(\d+)(?:-(\d{1,2}))?(?:-|\[|$)`)

// NormalizeModel maps a transcript model ID such as "claude-opus-5-5",
// "claude-sonnet-4-5-20250929" or "claude-opus-5-5[1m]" to a table key. The
// bool is false when the ID is not a recognizable Claude model.
func NormalizeModel(model string) (string, bool) {
	large := strings.HasSuffix(model, largePromptSuffix)
	model = strings.ToLower(strings.TrimSuffix(model, largePromptSuffix))
	match := modelPattern.FindStringSubmatch(model)
	if match == nil {
		return "", false
	}
	key := match[1] + "-" + match[2]
	if match[3] != "" {
		key += "." + match[3]
	}
	if large {
		key += ">100k"
	}
	return key, true
}

// Lookup finds the price for a transcript model ID, falling back from a
// missing prompt-size tier to the base tier.
func (table PriceTable) Lookup(model string) (Price, bool) {
	key, ok := NormalizeModel(model)
	if !ok {
		return Price{}, false
	}
	if price, found := table[key]; found {
		return price, true
	}
	price, found := table[strings.TrimSuffix(key, ">100k")]
	return price, found
}

// Cost returns the USD cost of the given token counts.
func (price Price) Cost(input, output, cacheRead, write5m, write1h int64) float64 {
	return (float64(input)*price.Input +
		float64(output)*price.Output +
		float64(cacheRead)*price.CacheRead +
		float64(write5m)*price.CacheWrite5m +
		float64(write1h)*price.CacheWrite1h) / 1e6
}

func standardPrice(input, output float64) Price {
	return Price{Input: input, CacheWrite5m: input * 1.25, CacheWrite1h: input * 2, CacheRead: input * 0.1, Output: output}
}

// BuiltinPrices is the offline default Claude price table.
func BuiltinPrices() PriceTable {
	opus := standardPrice(5, 25)
	sonnet := standardPrice(3, 15)
	opusLegacy := Price{Input: 15, CacheWrite5m: 18.75, CacheWrite1h: 30, CacheRead: 1.5, Output: 75}
	fable := Price{Input: 10, CacheWrite5m: 12.5, CacheWrite1h: 20, CacheRead: 0.25, Output: 50}
	fable5 := fable
	fable5.CacheRead = 1
	return PriceTable{
		"opus-5.5":       {Input: 4, CacheWrite5m: 5, CacheWrite1h: 8, CacheRead: 0.2, Output: 20},
		"opus-5":         opus,
		"opus-4.8":       opus,
		"opus-4.7":       opus,
		"opus-4.6":       opus,
		"opus-4.5":       opus,
		"opus-4.1":       opusLegacy,
		"opus-4":         opusLegacy,
		"sonnet-5.5":     {Input: 2, CacheWrite5m: 2.5, CacheWrite1h: 4, CacheRead: 0.1, Output: 10},
		"sonnet-5":       {Input: 2, CacheWrite5m: 2.5, CacheWrite1h: 4, CacheRead: 0.2, Output: 10},
		"sonnet-4.6":     sonnet,
		"sonnet-4.5":     sonnet,
		"sonnet-4":       sonnet,
		"haiku-5.5":      {Input: 0.1, CacheWrite5m: 0.125, CacheWrite1h: 0.2, CacheRead: 0.01, Output: 0.5},
		"haiku-5.5>100k": {Input: 0.5, CacheWrite5m: 0.625, CacheWrite1h: 1, CacheRead: 0.05, Output: 2.5},
		"haiku-4.5":      standardPrice(1, 5),
		"haiku-3.5":      {Input: 0.8, CacheWrite5m: 1, CacheWrite1h: 1.6, CacheRead: 0.08, Output: 4},
		"fable-5.1":      fable,
		"mythos-5.1":     fable,
		"fable-5":        fable5,
		"mythos-5":       fable5,
	}
}

// Merged returns base overlaid with each of overlays in order.
func Merged(base PriceTable, overlays ...PriceTable) PriceTable {
	result := make(PriceTable, len(base))
	for key, price := range base {
		result[key] = price
	}
	for _, overlay := range overlays {
		for key, price := range overlay {
			result[key] = price
		}
	}
	return result
}
