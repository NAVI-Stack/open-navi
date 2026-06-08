package cron

import "regexp"

var transientPatterns = map[string]*regexp.Regexp{
	"rate_limit": regexp.MustCompile(`(?i)(rate[_ ]limit|too many requests|429|resource has been exhausted|cloudflare|tokens per day)`),
	"overloaded": regexp.MustCompile(`(?i)(\b529\b|\boverloaded(?:_error)?\b|high demand|temporar(?:ily|y) overloaded|capacity exceeded)`),
	"network":    regexp.MustCompile(`(?i)(network|econnreset|econnrefused|fetch failed|socket)`),
	"timeout":    regexp.MustCompile(`(?i)(timeout|etimedout)`),
	"server_err": regexp.MustCompile(`(?i)\b5\d{2}\b`),
}

func defaultBackoffMs() []int64 {
	return []int64{
		30_000,
		60_000,
		5 * 60_000,
		15 * 60_000,
		60 * 60_000,
	}
}

// IsTransientError classifies textual errors suitable for backoff / limited retries (one-shot at jobs).
func IsTransientError(errText string, retryOn []string) bool {
	if errText == "" {
		return false
	}
	keys := retryOn
	if len(keys) == 0 {
		keys = make([]string, 0, len(transientPatterns))
		for k := range transientPatterns {
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		if rx := transientPatterns[k]; rx != nil && rx.MatchString(errText) {
			return true
		}
	}
	return false
}

// ErrorBackoffMs returns incremental backoff matching OpenClaw's table (indexed by consecutive error count − 1).
func ErrorBackoffMs(consecutiveErrors int, scheduleMs []int64) int64 {
	if consecutiveErrors <= 0 {
		consecutiveErrors = 1
	}
	ms := scheduleMs
	if len(ms) == 0 {
		ms = defaultBackoffMs()
	}
	idx := consecutiveErrors - 1
	if idx >= len(ms) {
		idx = len(ms) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return ms[idx]
}
