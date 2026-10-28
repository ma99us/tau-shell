package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	AnthropicPricingURL = "https://platform.claude.com/docs/en/about-claude/pricing.md"
	LiteLLMPricesURL    = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

	SourceAnthropic = "anthropic"
	SourceLiteLLM   = "litellm"

	anthropicMaxBytes = 2 << 20
	litellmMaxBytes   = 8 << 20
	fetchTimeout      = 15 * time.Second
	maxPricePerMTok   = 1000
)

// FetchedPrices is the on-disk cache of prices read from the web. It is
// runtime state, kept apart from the hand-edited config.yaml.
type FetchedPrices struct {
	FetchedAt time.Time  `json:"fetched_at"`
	Source    string     `json:"source"`
	SourceURL string     `json:"source_url"`
	Models    PriceTable `json:"models"`
}

// Stale reports whether the cache is missing or older than maxAge.
func (fetched FetchedPrices) Stale(now time.Time, maxAge time.Duration) bool {
	return len(fetched.Models) == 0 || fetched.FetchedAt.IsZero() || now.Sub(fetched.FetchedAt) >= maxAge
}

var (
	pricedCell    = regexp.MustCompile(`\$\s*([0-9][0-9,]*(?:\.[0-9]+)?)`)
	tableModel    = regexp.MustCompile(`^Claude\s+(Opus|Sonnet|Haiku|Fable|Mythos)\s+(\d+(?:\.\d+)?)`)
	superscript   = regexp.MustCompile(`(?i)<sup>.*?</sup>`)
	errNoPricing  = errors.New("no usable Claude prices found")
	requiredTiers = []string{"opus", "sonnet"}
)

// ParseAnthropicPricing reads the model pricing table from Anthropic's
// pricing page markdown: Model | Input | 5m write | 1h write | Cache hit |
// Output, each price written as "$x / MTok".
func ParseAnthropicPricing(markdown string) (PriceTable, error) {
	table := PriceTable{}
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) < 6 {
			continue
		}
		name := strings.TrimSpace(superscript.ReplaceAllString(cells[0], ""))
		large := strings.Contains(name, "over 100,000") || strings.Contains(name, "over 100k")
		if cut := strings.Index(name, "("); cut >= 0 {
			name = strings.TrimSpace(name[:cut])
		}
		match := tableModel.FindStringSubmatch(name)
		if match == nil {
			continue
		}
		var values [5]float64
		valid := true
		for index := range values {
			parsed := pricedCell.FindStringSubmatch(superscript.ReplaceAllString(cells[index+1], ""))
			if parsed == nil {
				valid = false
				break
			}
			value, err := strconv.ParseFloat(strings.ReplaceAll(parsed[1], ",", ""), 64)
			if err != nil {
				valid = false
				break
			}
			values[index] = value
		}
		if !valid {
			continue
		}
		key := strings.ToLower(match[1]) + "-" + match[2]
		if large {
			key += ">100k"
		}
		table[key] = Price{Input: values[0], CacheWrite5m: values[1], CacheWrite1h: values[2], CacheRead: values[3], Output: values[4]}
	}
	return validatePrices(table)
}

// ParseLiteLLMPrices reads Anthropic models from LiteLLM's community price
// list. LiteLLM prices are per token; this converts them to USD per MTok.
func ParseLiteLLMPrices(source io.Reader) (PriceTable, error) {
	var entries map[string]struct {
		Provider  string  `json:"litellm_provider"`
		Input     float64 `json:"input_cost_per_token"`
		Output    float64 `json:"output_cost_per_token"`
		Write5m   float64 `json:"cache_creation_input_token_cost"`
		Write1h   float64 `json:"cache_creation_input_token_cost_above_1hr"`
		CacheRead float64 `json:"cache_read_input_token_cost"`
	}
	if err := json.NewDecoder(source).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode LiteLLM prices: %w", err)
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	table := PriceTable{}
	for _, name := range keys {
		entry := entries[name]
		if entry.Provider != "anthropic" || !strings.HasPrefix(name, "claude-") || strings.Contains(name, "/") {
			continue
		}
		key, ok := NormalizeModel(name)
		if !ok || entry.Input <= 0 || entry.Output <= 0 {
			continue
		}
		if _, exists := table[key]; exists {
			continue
		}
		perMTok := func(perToken float64) float64 { return math.Round(perToken*1e12) / 1e6 }
		price := Price{Input: perMTok(entry.Input), Output: perMTok(entry.Output), CacheWrite5m: perMTok(entry.Write5m), CacheWrite1h: perMTok(entry.Write1h), CacheRead: perMTok(entry.CacheRead)}
		if price.CacheWrite5m == 0 {
			price.CacheWrite5m = price.Input * 1.25
		}
		if price.CacheWrite1h == 0 {
			price.CacheWrite1h = price.Input * 2
		}
		if price.CacheRead == 0 {
			price.CacheRead = price.Input * 0.1
		}
		table[key] = price
	}
	return validatePrices(table)
}

// validatePrices drops implausible entries so a mis-parse can never replace
// good prices, and fails when too little survives.
func validatePrices(table PriceTable) (PriceTable, error) {
	valid := PriceTable{}
	for key, price := range table {
		values := []float64{price.Input, price.CacheWrite5m, price.CacheWrite1h, price.CacheRead, price.Output}
		ok := price.Output >= price.Input*0.5
		for _, value := range values {
			if value <= 0 || value >= maxPricePerMTok {
				ok = false
			}
		}
		if ok {
			valid[key] = price
		}
	}
	if len(valid) < 3 {
		return nil, errNoPricing
	}
	for _, family := range requiredTiers {
		found := false
		for key := range valid {
			if strings.HasPrefix(key, family+"-") {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: no %s model", errNoPricing, family)
		}
	}
	return valid, nil
}

// Fetcher downloads Claude prices: Anthropic's official page first, then
// LiteLLM's list when the official page cannot be read or parsed.
type Fetcher struct {
	Client       *http.Client
	AnthropicURL string
	LiteLLMURL   string
	Now          func() time.Time
	UserAgent    string
	allowHTTP    bool
}

func (fetcher Fetcher) client() *http.Client {
	if fetcher.Client != nil {
		return fetcher.Client
	}
	return &http.Client{Timeout: fetchTimeout}
}

func (fetcher Fetcher) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	if !strings.HasPrefix(url, "https://") && !fetcher.allowHTTP {
		return nil, fmt.Errorf("refusing non-HTTPS price source %q", url)
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	agent := fetcher.UserAgent
	if agent == "" {
		agent = "TauShell"
	}
	request.Header.Set("User-Agent", agent)
	response, err := fetcher.client().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", url, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s response exceeds %d bytes", url, limit)
	}
	return body, nil
}

// Fetch returns freshly read prices or an error; it never returns an empty
// result as success.
func (fetcher Fetcher) Fetch(ctx context.Context) (FetchedPrices, error) {
	now := time.Now
	if fetcher.Now != nil {
		now = fetcher.Now
	}
	anthropicURL, litellmURL := fetcher.AnthropicURL, fetcher.LiteLLMURL
	if anthropicURL == "" {
		anthropicURL = AnthropicPricingURL
	}
	if litellmURL == "" {
		litellmURL = LiteLLMPricesURL
	}

	body, err := fetcher.get(ctx, anthropicURL, anthropicMaxBytes)
	if err == nil {
		var table PriceTable
		if table, err = ParseAnthropicPricing(string(body)); err == nil {
			return FetchedPrices{FetchedAt: now().UTC(), Source: SourceAnthropic, SourceURL: anthropicURL, Models: table}, nil
		}
	}
	primaryErr := fmt.Errorf("anthropic pricing page: %w", err)

	body, err = fetcher.get(ctx, litellmURL, litellmMaxBytes)
	if err == nil {
		var table PriceTable
		if table, err = ParseLiteLLMPrices(strings.NewReader(string(body))); err == nil {
			return FetchedPrices{FetchedAt: now().UTC(), Source: SourceLiteLLM, SourceURL: litellmURL, Models: table}, nil
		}
	}
	return FetchedPrices{}, errors.Join(primaryErr, fmt.Errorf("litellm prices: %w", err))
}

// LoadPriceCache reads the cache file. A missing or corrupt file yields an
// empty cache so callers fall back to built-in prices.
func LoadPriceCache(path string) FetchedPrices {
	data, err := os.ReadFile(path)
	if err != nil {
		return FetchedPrices{}
	}
	var cached FetchedPrices
	if json.Unmarshal(data, &cached) != nil {
		return FetchedPrices{}
	}
	if valid, err := validatePrices(cached.Models); err == nil {
		cached.Models = valid
		return cached
	}
	return FetchedPrices{}
}

// SavePriceCache writes the cache atomically.
func SavePriceCache(path string, cached FetchedPrices) error {
	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}
