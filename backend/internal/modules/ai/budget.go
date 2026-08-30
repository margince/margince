// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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

// quotaWrapped classifies a provider's 429, which every vendor here uses for
// both an exhausted account and an ordinary burst limit.
//
// Retry-After is what separates them: a throttle names the moment the caller
// may return, because the provider expects it to. An account over its cap has
// no such moment to name — nothing changes until a human raises the limit — so
// a 429 that names no moment is read as the refusal it almost always is.
// Reading it the wrong way is survivable in one direction only: a throttle
// mistaken for a refusal costs one abandoned call, while a refusal mistaken
// for a throttle burns every remaining tier against an account that cannot pay
// for any of them.
//
// The header is read through the kernel's one reader, which knows both RFC
// 9110 forms and answers zero for a header that is absent, unparseable, or
// already past — all three of which name no moment to come back at.
func quotaWrapped(resp *http.Response, err error) error {
	if resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		return err
	}
	if retryafter.Of(resp) > 0 {
		return fmt.Errorf("%w: %w", ErrProviderThrottled, err)
	}
	return fmt.Errorf("%w: %w", ErrProviderQuota, err)
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
