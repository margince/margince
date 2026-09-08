// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"io"
	"sync"

	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// callMetrics is the in-process AI counter set exposed on /metrics. Counters
// only (monotonic since process start); hand-rolled text like the rest of
// the metrics surface, so no client_golang dependency. Never labels by
// content or workspace.
//
// Cardinality is the product of closed vocabularies — task, tier, provider,
// error sentinel, finish reason — with ONE exception that is not closed:
// model. A tier binding's Model is operator-supplied and ValidateTierBinding
// does not constrain it (an ollama or vLLM identity is genuinely arbitrary),
// so a rebind mints a new series that lives for the process lifetime. That is
// affordable because a rebind is an admin action and not traffic, and it is
// why every label value here goes through httpserver.Label rather than %q:
// Go quoting emits \xNN escapes the Prometheus text parser rejects, and it
// rejects the whole scrape rather than the offending line.
type callMetrics struct {
	mu      sync.Mutex
	calls   map[routeKey]uint64
	errors  map[errorKey]uint64
	finish  map[finishKey]uint64
	tokens  map[tokenKey]int64
	latency map[routeKey]*httpserver.Histogram
	// attempts counts every ladder rung walked, against calls' one-per-
	// logical-call terminals: the ratio is what a tier failing over on
	// every call looks like, and the terminal count alone cannot show it.
	attempts map[routeKey]uint64
	degraded map[routeKey]uint64
	cacheHit map[routeKey]uint64

	contextBytes  map[string]int64
	contextTokens map[string]int64
}

// routeKey is the identity every AI family is keyed by.
//
// The model dimension is the SERVED identity, not the configured one, and the
// two are not interchangeable. Call.ModelID is whatever the tier binding
// declared, which is empty for any provider bound without an explicit model id
// — every --ai-fake deployment, and any operator who leaves the field blank —
// so keying on it ships a literal model="" on every series that installation
// publishes. Call.ServedModel is already the graded answer: servedIdentity
// prefers what the provider reported, falls back to the configured binding,
// and is empty only when a call never reached a provider at all.
//
// servedIdentitySource is what makes that safe to read, which is why it is a
// label rather than a second model dimension: "response" is a vendor
// confirming what ran, "echo" is an OpenAI-compatible wire reflecting the
// request back, "configured" is nobody having said. A dashboard that treats
// an echo as a confirmation is wrong about which model answered, and this
// label is the only thing that tells it apart.
type routeKey struct{ task, tier, provider, model, servedIdentitySource string }

type errorKey struct {
	routeKey
	sentinel string
}

type finishKey struct {
	routeKey
	reason string
}

type tokenKey struct {
	routeKey
	class string
}

// Token classes. The four input classes are DISJOINT by construction, which
// they are not in the Call struct: TokensIn is cache-inclusive (model.Response's
// pinned contract), already counting both CachedTokens and CacheWriteTokens.
// Summing the classes as reported would therefore count cached input twice, and
// it would do so inside sum by (direction) — the query a dashboard writes
// without thinking. classPrompt is the leftover after both are subtracted,
// which is exactly PriceCall's uncached bucket; uncachedTokensIn is the one
// helper both call.
const (
	classPrompt     = "prompt"
	classCachedRead = "cached_read"
	classCacheWrite = "cache_write"
	classCompletion = "completion"
	classReasoning  = "reasoning"
)

const (
	directionIn  = "in"
	directionOut = "out"
)

// directionOf answers the in/out label a token class rolls up to, so
// sum by (direction) still answers "prompt versus completion" after the
// class split. Reasoning tokens are billed as output and roll up with it.
func directionOf(class string) string {
	if class == classCompletion || class == classReasoning {
		return directionOut
	}
	return directionIn
}

func newCallMetrics() *callMetrics {
	return &callMetrics{
		calls: map[routeKey]uint64{}, errors: map[errorKey]uint64{},
		finish: map[finishKey]uint64{}, tokens: map[tokenKey]int64{},
		latency:  map[routeKey]*httpserver.Histogram{},
		attempts: map[routeKey]uint64{}, degraded: map[routeKey]uint64{},
		cacheHit:      map[routeKey]uint64{},
		contextBytes:  map[string]int64{},
		contextTokens: map[string]int64{},
	}
}

// sharedCallMetrics is the process-wide AI counter set. Every Router
// increments the same collector so /metrics reports one honest total
// across lanes, and it is rendered exactly once (Prometheus forbids a
// repeated metric family / duplicate series in one exposition).
var sharedCallMetrics = newCallMetrics()

// WriteProcessMetrics renders this PROCESS's AI counters into the exposition.
//
// Package-level, and named for the process, because that is what the numbers
// are: every Router in this binary increments sharedCallMetrics, so there is
// one set no matter how many routers a role builds. It used to be reachable
// only as a method on Router (and through a ModelPath wrapper above it), which
// read as "this router's counters" and cost the worker its entire AI surface —
// that role resolves a model path but wired no renderer, so every job-driven
// call was counted here and published by nobody.
func WriteProcessMetrics(w io.Writer) { sharedCallMetrics.WritePrometheus(w) }

// observeAttempt records ONE ladder rung. Every attempt carries tokens the
// vendor billed and a latency the caller waited, terminal or not, so the
// histogram and the token counters are fed here rather than from the terminal
// alone — a retry that burned 4,000 prompt tokens before failing over cost
// exactly that whether or not its row is the one the caller got back.
func (m *callMetrics) observeAttempt(c Call) {
	k := m.keyOf(c)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts[k]++
	m.observeLatencyLocked(k, c)
	m.observeTokensLocked(k, c)
	if c.CacheHit {
		m.cacheHit[k]++
	}
	if c.Degraded {
		m.degraded[k]++
	}
	if c.ErrorSentinel != "" {
		m.errors[errorKey{routeKey: k, sentinel: c.ErrorSentinel}]++
	}
	// A cache hit replays a stored response, so its FinishReason is a
	// provider's answer from some earlier call. Counting it here would make
	// finish_reasons_total climb on attempts that consulted no provider, and
	// the family's whole use is comparing stop reasons against attempts that
	// did — the same reason the histogram skips a hit.
	if c.FinishReason != "" && !c.CacheHit {
		m.finish[finishKey{routeKey: k, reason: c.FinishReason}]++
	}
}

// observe records the TERMINAL of one logical call: the outcome the caller
// actually got. It is the denominator for "how much work was asked for",
// which is a different question from how many rungs answering it took, and
// keeping the two apart is the whole point of counting both.
func (m *callMetrics) observe(c Call) {
	k := m.keyOf(c)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls[k]++
	m.contextBytes[string(c.Task)] += int64(c.ContextBytes)
	m.contextTokens[string(c.Task)] += int64(c.ContextTokensEstimate)
}

func (m *callMetrics) keyOf(c Call) routeKey {
	return routeKey{
		task: string(c.Task), tier: string(c.Tier), provider: c.Provider,
		model: c.ServedModel, servedIdentitySource: c.ServedIdentitySource,
	}
}

// observeLatencyLocked skips a cache hit rather than recording its
// microseconds: a hit measures this process's map lookup, and mixing those
// into the same histogram as a provider round trip drags every percentile
// toward zero — the percentile would then improve as the cache warmed, which
// is the opposite of what a latency panel is read for.
func (m *callMetrics) observeLatencyLocked(k routeKey, c Call) {
	if c.CacheHit {
		return
	}
	h, ok := m.latency[k]
	if !ok {
		h = newLatencyHistogram()
		m.latency[k] = h
	}
	h.Observe(float64(c.LatencyMS) / 1000)
}

// observeTokensLocked splits one attempt's usage into disjoint classes. The
// int64 widening is lossless (a usage count is a non-negative int) and is
// preferred over int→uint64, which trips gosec G115 on the sign change.
func (m *callMetrics) observeTokensLocked(k routeKey, c Call) {
	// A fixed-size array rather than a map literal: this runs on every
	// attempt, while the collector's mutex is held.
	for _, split := range [...]struct {
		class string
		n     int
	}{
		{classPrompt, uncachedTokensIn(c.TokensIn, c.CachedTokens, c.CacheWriteTokens)},
		{classCachedRead, c.CachedTokens},
		{classCacheWrite, c.CacheWriteTokens},
		{classCompletion, c.TokensOut},
		{classReasoning, c.ReasoningTokens},
	} {
		if split.n != 0 {
			m.tokens[tokenKey{routeKey: k, class: split.class}] += int64(split.n)
		}
	}
}

func copyTaskCounters(source map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
