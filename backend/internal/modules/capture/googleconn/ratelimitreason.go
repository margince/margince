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
)

// The rate-limit reasons a throttled Google call is labelled with. The set is
// closed so a metric label and a log field stay bounded: Google's own code when
// it is one of these, RateLimitConcurrent for the per-user concurrency cap,
// RateLimitUnspecified when the body names no reason, RateLimitOther for any
// code outside the set.
const (
	RateLimitConcurrent      = "concurrent"
	RateLimitUser            = "userRateLimitExceeded"
	RateLimitDaily           = "dailyLimitExceeded"
	RateLimitQuota           = "quotaExceeded"
	RateLimitRate            = "rateLimitExceeded"
	RateLimitGeneric         = "limitExceeded"
	RateLimitUnspecified     = "unspecified"
	RateLimitOther           = "other"
	concurrentRequestsPhrase = "concurrent requests"
)

// rateLimitPrecedence orders the reasons from most to least specific, so a body
// naming several is labelled with the one that says most about the remedy.
var rateLimitPrecedence = []string{
	RateLimitConcurrent, RateLimitUser, RateLimitDaily, RateLimitQuota, RateLimitRate, RateLimitGeneric,
}

// rateLimitCodes folds each reason code Google spells, classic and ErrorInfo
// alike, onto the vocabulary above.
var rateLimitCodes = map[string]string{
	RateLimitUser:           RateLimitUser,
	RateLimitDaily:          RateLimitDaily,
	RateLimitQuota:          RateLimitQuota,
	reasonQuotaExceededEnum: RateLimitQuota,
	RateLimitRate:           RateLimitRate,
	reasonRateLimitEnum:     RateLimitRate,
	RateLimitGeneric:        RateLimitGeneric,
}

// RateLimitReason labels a throttled response body with the most specific
// reason it names. The concurrency cap carries the generic rateLimitExceeded
// code and is told apart only by its fixed message, which is matched here and
// never carried further.
func RateLimitReason(body []byte) string {
	codes, _ := reasonCodes(body)
	found := map[string]bool{}
	for _, raw := range codes {
		if reason, ok := rateLimitCodes[raw]; ok {
			found[reason] = true
		} else if raw != "" {
			found[RateLimitOther] = true
		}
	}
	if namesConcurrency(body) {
		found[RateLimitConcurrent] = true
	}
	for _, reason := range rateLimitPrecedence {
		if found[reason] {
			return reason
		}
	}
	if found[RateLimitOther] {
		return RateLimitOther
	}
	return RateLimitUnspecified
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
	return strings.Contains(strings.ToLower(parsed.Error.Message), concurrentRequestsPhrase)
}
