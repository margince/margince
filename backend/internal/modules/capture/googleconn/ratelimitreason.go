// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package googleconn

// Which of Google's limits a throttled call met. Gmail answers a per-user
// limit, a project quota and a concurrency cap with the same 403 or 429, and
// they need different remedies: slow one mailbox down, raise a quota, or fetch
// fewer messages at once.

import (
	"encoding/json"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// concurrentCapMessage is how Google words the per-user concurrency cap, the
// one limit its reason code does not name.
const concurrentCapMessage = "too many concurrent requests for user"

// rateLimitPrecedence orders the reasons from most to least specific, so a body
// naming several is labelled with the one that says most about the remedy.
var rateLimitPrecedence = []string{
	connector.RateLimitConcurrent, connector.RateLimitUser, connector.RateLimitDaily,
	connector.RateLimitQuota, connector.RateLimitRate, connector.RateLimitGeneric,
}

// rateLimitCodes folds each reason code Google spells, classic and ErrorInfo
// alike, onto the vocabulary above.
var rateLimitCodes = map[string]string{
	connector.RateLimitUser:    connector.RateLimitUser,
	connector.RateLimitDaily:   connector.RateLimitDaily,
	connector.RateLimitQuota:   connector.RateLimitQuota,
	reasonQuotaExceededEnum:    connector.RateLimitQuota,
	connector.RateLimitRate:    connector.RateLimitRate,
	reasonRateLimitEnum:        connector.RateLimitRate,
	connector.RateLimitGeneric: connector.RateLimitGeneric,
}

// RateLimitReason labels a throttled response body with the most specific
// connector.RateLimit* reason it names. The concurrency cap carries the generic
// rateLimitExceeded code and is told apart only by its fixed message, which is
// matched here and never carried further.
func RateLimitReason(body []byte) string {
	codes, _ := reasonCodes(body)
	found := map[string]bool{}
	for _, raw := range codes {
		if reason, ok := rateLimitCodes[raw]; ok {
			found[reason] = true
		} else if raw != "" {
			found[connector.RateLimitOther] = true
		}
	}
	if namesConcurrency(body) {
		found[connector.RateLimitConcurrent] = true
	}
	for _, reason := range rateLimitPrecedence {
		if found[reason] {
			return reason
		}
	}
	if found[connector.RateLimitOther] {
		return connector.RateLimitOther
	}
	return connector.RateLimitUnspecified
}

// namesConcurrency reports whether Google's message is the per-user
// concurrency cap ("Too many concurrent requests for user").
func namesConcurrency(body []byte) bool {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(parsed.Error.Message), concurrentCapMessage)
}
