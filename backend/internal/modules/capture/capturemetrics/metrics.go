// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package capturemetrics counts where a mailbox import spends its time: every
// provider API call, every message a backfill walks and the stage it waited
// in, every page, and every wait the backfill chose between pages.
//
// The counters are process-wide and in memory, like the AI router's, because
// the work is done by one process and only that process can say how long it
// took. Every label is a closed vocabulary: a provider name, an op, a stage, a
// result. No address, connection or workspace id is ever a label.
package capturemetrics

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The provider API calls a connector names, so a dashboard can tell the
// listing of ids from the per-message downloads it drives: OpGetMetadata reads
// a message's headers, OpGetRaw downloads it in full. OpToken is a round trip
// to the provider's OAuth token endpoint.
const (
	OpList        = "list"
	OpGetMetadata = "get_metadata"
	OpGetRaw      = "get_raw"
	OpHistory     = "history"
	OpToken       = "token"
	OpOther       = "other"
)

// The stages one backfilled message passes through, in order. A message its
// headers settle stops after StageFetchHeaders; StageFetch is the full download.
const (
	StageFetchHeaders = "fetch_headers"
	StageFetch        = "fetch"
	StageParse        = "parse"
	StageSink         = "sink"
	StageEnsure       = "ensure"
)

// The outcomes a message ends on besides the ones the capture trace records.
// OutcomeCaptured is the trace's own `captured`, used for a message the sink
// accepted without tracing a decision — a replay of one already stored.
// OutcomeRefused is a message the capture refused and the page walked past;
// OutcomeFailed is one whose failure ended the page.
const (
	OutcomeCaptured = "captured"
	OutcomeSkipped  = "skipped"
	OutcomeRefused  = "refused"
	OutcomeFailed   = "failed"
)

const (
	resultOK          = "ok"
	resultRateLimited = "rate_limited"
	resultAuth        = "auth"
	resultUnreachable = "unreachable"
	resultNotFound    = "not_found"
	resultError       = "error"
	resultTokenGone   = "token_rejected"
	resultFailed      = "failed"

	reasonPacing   = "pacing"
	reasonResumed  = "resumed"
	reasonInPage   = "rate_limited_in_page"
	reasonInternal = "internal"
)

// requestBounds are the provider-call histogram's upper bounds in seconds: a
// metadata call lands near 100ms, a RAW download of a large message runs to the
// client's 30s timeout.
var requestBounds = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30}

// stageBounds reach further down than requestBounds because two of the four
// stages are in-process work that finishes in a millisecond.
var stageBounds = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

// pair keys every family that carries the provider and one label of its own.
type pair struct{ provider, value string }

type requestKey struct{ provider, op, result string }

type collector struct {
	mu          sync.Mutex
	requests    map[requestKey]uint64
	rateLimited map[requestKey]uint64
	requestTime map[pair]*httpserver.Histogram
	messages    map[pair]uint64
	stages      map[pair]*httpserver.Histogram
	pages       map[pair]uint64
	snoozed     map[pair]float64
	retryAfter  map[string]float64
}

func newCollector() *collector {
	return &collector{
		requests: map[requestKey]uint64{}, rateLimited: map[requestKey]uint64{}, requestTime: map[pair]*httpserver.Histogram{},
		messages: map[pair]uint64{}, stages: map[pair]*httpserver.Histogram{},
		pages: map[pair]uint64{}, snoozed: map[pair]float64{}, retryAfter: map[string]float64{},
	}
}

// shared is the one collector every connector and backfill in this process
// increments, so the exposition renders it exactly once.
var shared = newCollector()

// ObserveRequest records one provider API call: its result, classified from
// the status and the error the client returned, and its wall time.
func ObserveRequest(provider, op string, status int, err error, elapsed time.Duration) {
	shared.observeRequest(provider, op, requestResult(status, err), elapsed)
	if limited, ok := errors.AsType[*connector.RateLimitedError](err); ok {
		shared.observeRateLimit(provider, op, connector.RateLimitReasonLabel(limited.Reason))
	}
}

// requestResult folds a call's outcome into the result vocabulary. A 404 is
// its own result because the client reports it as unreachable while it means
// the message or history was gone, which no retry changes.
func requestResult(status int, err error) string {
	switch {
	case err == nil:
		return resultOK
	case errors.Is(err, connector.ErrRateLimited):
		return resultRateLimited
	case errors.Is(err, connector.ErrAuthRejected):
		return resultAuth
	case status == http.StatusNotFound:
		return resultNotFound
	case errors.Is(err, connector.ErrUnreachable):
		return resultUnreachable
	default:
		return resultError
	}
}

// pageResult classifies a failed page the way the pager does: a rate limit or
// an unreachable provider is waited out, anything else ends the run.
func pageResult(err error) string {
	switch {
	case err == nil:
		return resultOK
	case errors.Is(err, connector.ErrRateLimited):
		return resultRateLimited
	case errors.Is(err, connector.ErrUnreachable) && !errors.Is(err, connector.ErrAuthRejected):
		return resultUnreachable
	case errors.Is(err, connector.ErrCursorGone):
		return resultTokenGone
	default:
		return resultFailed
	}
}

func (c *collector) observeRequest(provider, op, result string, elapsed time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests[requestKey{provider: provider, op: op, result: result}]++
	observeLocked(c.requestTime, pair{provider: provider, value: op}, requestBounds, elapsed)
}

func (c *collector) observeRateLimit(provider, op, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rateLimited[requestKey{provider: provider, op: op, result: reason}]++
}

func (c *collector) observeMessage(provider, outcome string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages[pair{provider: provider, value: outcome}]++
}

func (c *collector) observeStage(provider, stage string, elapsed time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	observeLocked(c.stages, pair{provider: provider, value: stage}, stageBounds, elapsed)
}

func (c *collector) observePage(provider, result string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pages[pair{provider: provider, value: result}]++
}

// observeSnooze adds one chosen wait.
func (c *collector) observeSnooze(provider, reason string, wait time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snoozed[pair{provider: provider, value: reason}] += wait.Seconds()
}

// observeFaultWait adds a wait taken after a provider fault, and beside it what
// the provider asked for, so the gap between the two is a subtraction on the
// dashboard.
func (c *collector) observeFaultWait(provider, reason string, wait, asked time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snoozed[pair{provider: provider, value: reason}] += wait.Seconds()
	c.retryAfter[provider] += asked.Seconds()
}

func observeLocked(family map[pair]*httpserver.Histogram, k pair, bounds []float64, elapsed time.Duration) {
	h, ok := family[k]
	if !ok {
		h = httpserver.NewHistogram(bounds)
		family[k] = h
	}
	h.Observe(elapsed.Seconds())
}

// deferralOf answers why a backfill waited after a failed page, and how long
// the provider itself asked for (zero when it named no Retry-After).
func deferralOf(cause error) (reason string, asked time.Duration) {
	var limited *connector.RateLimitedError
	switch {
	case errors.As(cause, &limited):
		return resultRateLimited, limited.RetryAfter
	case errors.Is(cause, connector.ErrRateLimited):
		return resultRateLimited, 0
	case errors.Is(cause, connector.ErrUnreachable):
		return resultUnreachable, 0
	case errors.Is(cause, connector.ErrCursorGone):
		return resultTokenGone, 0
	default:
		return reasonInternal, 0
	}
}
