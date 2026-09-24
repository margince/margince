// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/retryafter"
)

// ErrBudgetDeferred identifies a background model call that its durable
// carrier must resume in the next budget window. The router owns the timing
// decision but never invents a generic job: the caller already owns the work.
var ErrBudgetDeferred = errors.New("ai: background task deferred until the next budget window")

// ErrProviderQuota identifies a provider that REFUSED the call because the
// account behind the key is out of quota or over its spending cap — not a
// model that answered badly. The two need separate words to the operator:
// this one is fixed in the provider's billing console and retrying changes
// nothing until somebody does, while a bad answer is worth another attempt.
//
// Every provider client wraps its 429 in this, so a caller classifies by
// errors.Is rather than by matching a message that differs per vendor.
var ErrProviderQuota = errors.New("ai: the configured AI provider refused the call: its account is out of budget or over its quota")

// ErrProviderThrottled is the OTHER thing a 429 means: too many calls too
// quickly, which the same call succeeds at a moment later. Separated from
// ErrProviderQuota because the two want opposite handling — a throttle is
// worth escalating past and retrying, an exhausted account is not, and telling
// an operator to raise a spending limit over a burst limit sends them to a
// console where nothing is wrong.
var ErrProviderThrottled = errors.New("ai: the configured AI provider is rate limiting this installation")

// errProviderRefused marks a 429 whose cause the provider did not make out —
// no limit_source, no recognizable words, no Retry-After. It carries no claim
// about WHY, only the fact that the model was never reached, which is the part
// a caller must not get wrong: a refusal reported as a bad model answer sends
// the operator to audit their own data.
var errProviderRefused = errors.New("ai: the configured AI provider turned the call away")

// providerRefusal classifies a provider's 429, which every vendor here uses for
// both an exhausted account and an ordinary burst limit. The two want opposite
// handling — an account is a human's to fix and every rung above bills to it,
// a burst clears by itself — and telling an operator the wrong one sends them
// to a console where nothing is wrong.
//
// What decides it is what the provider SAID, in this order:
//
//  1. limit_source, when a broker names one. "upstream_provider_shared_pool"
//     is a queue in front of a model, not a balance.
//  2. The words in the error itself. Every vendor here says "quota", "credit",
//     "billing" or "spending" for an account and "rate limit" for a burst.
//  3. Retry-After. A throttle names a moment to come back because the provider
//     expects the caller to return; an empty account has no such moment.
//
// Unclassifiable is a real answer and stays one: the refusal is marked without
// a cause, so the caller says the provider turned the call away rather than
// naming a reason invented here. Guessing "out of budget" from a bare 429 is
// what told an operator with unspent credit to go raise a spending limit.
//
// EVERY branch carries errProviderRefused, so "did we reach the model?" is one
// question with one answer whatever the cause turned out to be. A caller that
// needs the cause asks for the specific sentinel on top.
func providerRefusal(resp *http.Response, limitSource string, err error) error {
	if resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		return err
	}
	refused := fmt.Errorf("%w: %w", errProviderRefused, err)
	switch refusalKind(limitSource, err.Error(), retryafter.Of(resp)) {
	case refusalQuota:
		return fmt.Errorf("%w: %w", ErrProviderQuota, refused)
	case refusalThrottle:
		return fmt.Errorf("%w: %w", ErrProviderThrottled, refused)
	default:
		return refused
	}
}

// What a 429 turned out to be.
type refusal int

const (
	refusalUnknown refusal = iota
	refusalQuota
	refusalThrottle
)

// refusalKind reads a 429 for what it is. Split from providerRefusal so the
// decision can be stated against text and a header alone, which is how the
// vendors' real answers are pinned in the tests.
func refusalKind(limitSource, text string, retryAfter time.Duration) refusal {
	// A broker names the limit it hit, and a shared pool in front of a model
	// is a queue rather than a balance.
	switch {
	case strings.Contains(limitSource, "shared_pool"), strings.Contains(limitSource, "rate"), strings.Contains(limitSource, "retry"):
		return refusalThrottle
	case strings.Contains(limitSource, "credit"), strings.Contains(limitSource, "quota"), strings.Contains(limitSource, "balance"):
		return refusalQuota
	}
	lowered := strings.ToLower(text)
	// An invitation to come back is asked FIRST, and beats any word about an
	// account. Gemini spends "quota" on a per-minute limit as freely as on an
	// exhausted cap, so a message that says both "quota exceeded" and "retry
	// in 45s" is the retryable one — a vendor does not offer a moment to
	// return to an account that has nothing left to spend.
	for _, phrase := range []string{"retry in", "try again in", "retry after", "try again later", "try again shortly", "retry shortly", "temporarily"} {
		if strings.Contains(lowered, phrase) {
			return refusalThrottle
		}
	}
	// Then an account, which is the operator's to fix.
	for _, phrase := range []string{"insufficient_quota", "credit", "billing", "spending", "payment", "spend cap", "exceeded your current quota"} {
		if strings.Contains(lowered, phrase) {
			return refusalQuota
		}
	}
	// Then a plain burst limit.
	for _, phrase := range []string{"rate-limit", "rate limit", "ratelimit", "too many requests", "overloaded"} {
		if strings.Contains(lowered, phrase) {
			return refusalThrottle
		}
	}
	// "quota" alone is deliberately NOT here. It is the one word both causes
	// use, and reading it as an empty account is what told an operator with
	// unspent credit to go raise a limit. Unclassified is the honest answer.
	// A moment to come back at is the last signal, and only ever evidence OF a
	// throttle: an account with nothing left names none, but plenty of throttles
	// do not name one either, so its absence proves nothing.
	if retryAfter > 0 {
		return refusalThrottle
	}
	return refusalUnknown
}

// BudgetDeferralError carries the exact retry boundary to a background task's
// durable carrier. It is returned before a provider attempt or ai_call trace is
// created, so deferral is scheduling state rather than a failed model call.
type BudgetDeferralError struct {
	Task          Task
	NextAttemptAt time.Time
}

func (e *BudgetDeferralError) Error() string {
	return fmt.Sprintf("ai: task %s deferred until %s", e.Task, e.NextAttemptAt.Format(time.RFC3339))
}

func (e *BudgetDeferralError) Unwrap() error { return ErrBudgetDeferred }

// BudgetPolicy answers "how many tokens may this workspace burn per
// month". Injected so the composition layer can derive it from seat
// counts (09 §2.4: seats × 6M base × 2 safety) without this module
// reaching into identity's tables.
type BudgetPolicy interface {
	MonthlyTokenBudget(ctx context.Context, workspaceID ids.WorkspaceID) (int64, error)
}

// StaticBudget is the fixed fallback policy: the single-seat default
// until compose wires a live seat count.
type StaticBudget int64

// DefaultMonthlyTokens = 1 seat × 6M base × 2 safety factor.
const DefaultMonthlyTokens = StaticBudget(12_000_000)

func (b StaticBudget) MonthlyTokenBudget(context.Context, ids.WorkspaceID) (int64, error) {
	return int64(b), nil
}

// Utilization thresholds (§1.3, operational fill-in of the 09 §2.4
// ratified guardrail): soft-degrade band start and the hard cap.
const (
	degradeUtilization = 0.80
	queueUtilization   = 1.00
)

// premiumShareAlarmThreshold: a workspace whose costly-cloud token share
// (premium and every rung above it — see costlyCloudTiers) exceeds this over
// the trailing window gets flagged for a routing fix (§1.3) — the L2 analogue
// of "manual entry is a smell".
const premiumShareAlarmThreshold = 0.20
