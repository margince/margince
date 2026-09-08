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
// error sentinel — with ONE dimension that is not closed, and it is worse than
// operator-supplied: MODEL IS PROVIDER WIRE INPUT. It carries Call.ServedModel,
// which every adapter reads verbatim off the response body (openaicompat's
// out.Model, ollama's, anthropic's), so a broker, a self-hosted gateway or a
// compromised upstream answering with a fresh identity per response would mint
// a new key on every call — eight maps and a histogram each, retained for the
// process lifetime, and roughly twenty exposition lines apiece carrying the
// string back on every scrape.
//
// Two bounds, because one is not enough. boundedSeriesLabel caps the LENGTH of any
// value that came off a wire, so a megabyte identity cannot amplify the scrape
// body. maxRouteSeries caps the COUNT, and past it every attempt folds into one
// fixed key that cannot itself grow — the lesson httpserver's route histogram
// learned twice, where an overflow key that still carried a caller-controlled
// dimension was defeated within minutes of being measured.
//
// Every label value also goes through httpserver.Label rather than %q: Go
// quoting emits \xNN escapes the Prometheus text parser rejects, and it rejects
// the whole scrape rather than the offending line.
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

// Token classes, DISJOINT by construction — which they are not in the Call
// struct, on either side of the ledger. model.Response pins two inclusive
// invariants: TokensIn already counts CachedTokens and CacheWriteTokens, and
// TokensOut already counts ReasoningTokens (gemini.go adds thinking tokens into
// OutputTokens for exactly that reason, so the budget meter charges true spend
// on every provider).
//
// Reporting either total whole beside its itemized parts would double-count
// inside sum by (direction) — the query a dashboard writes without thinking,
// and one that would be wrong with both halves looking plausible. classPrompt
// and classCompletion are the leftovers.
//
// Only the input subtraction is shared with PriceCall: the pricer bills
// reasoning at the ordinary output rate, so it wants TokensOut whole, while a
// token panel wants the split. Two different questions about one field, which
// is why plainCompletionTokens lives here and uncachedTokensIn does not.
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

const (
	// maxRouteSeries caps how many distinct label sets the AI families hold.
	// Sized well above the live product of the closed dimensions — 31 tasks by
	// 5 tiers by the one model a binding holds at a time is ~300 — so an
	// ordinary fleet never reaches it and a provider minting identities does.
	maxRouteSeries = 500
	// maxLabelLen bounds one wire-supplied label value in runes. A model
	// identity a human would recognise is far shorter; this is the ceiling on
	// what an upstream can make every scrape carry.
	maxLabelLen = 96
	// overflowLabel stands in every dimension of the one key past the cap. It
	// is not a value any provider reported, and it is the same in each position
	// so that the overflow key is ONE series rather than one per task.
	overflowLabel = "__over_cardinality_limit__"
)

// boundedSeriesLabel truncates a wire-supplied value, marking the cut so a
// reader does not mistake the prefix for a whole model identity.
//
// Not railsubject's boundedLabel, which bounds a company name for a wire
// payload at a different limit and cuts silently: a truncated name is still a
// name a person recognises, while a truncated series label that did not say so
// would read as a model that does not exist.
func boundedSeriesLabel(value string) string {
	runes := []rune(value)
	if len(runes) <= maxLabelLen {
		return value
	}
	return string(runes[:maxLabelLen]) + "..."
}

// plainCompletionTokens answers the completion bucket net of reasoning.
//
// Floored at 0 for the same reason uncachedTokensIn is: a provider reporting
// more reasoning than output is a defensive case, not a contract violation to
// trust blindly, and a negative counter reads to Prometheus as a reset.
func plainCompletionTokens(tokensOut, reasoning int) int {
	plain := tokensOut - reasoning
	if plain < 0 {
		return 0
	}
	return plain
}

// finishReasons is the closed set the reason label may take. A provider's raw
// stop reason is a string it chooses, so folding it here is what keeps the
// family's cardinality a property of this tree rather than of an upstream; the
// unfolded value is still recorded on the ai_call row, where a series count is
// not at stake.
var finishReasons = map[string]bool{
	"stop": true, "length": true, "content_filter": true,
	"tool_calls": true, "function_call": true, "error": true,
}

// finishReasonLabel folds anything the closed set does not name into "other".
func finishReasonLabel(reason string) string {
	if finishReasons[reason] {
		return reason
	}
	return "other"
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
// Package-level, and named for the PROCESS, because that is what the numbers
// are: every Router in this binary increments sharedCallMetrics, so there is
// one set no matter how many routers a role builds. A method on Router would
// read as "this router's counters" and invite a role to wire one router's
// renderer and believe it had covered the process — which is how a role that
// makes AI calls ends up publishing none of them.
func WriteProcessMetrics(w io.Writer) { sharedCallMetrics.WritePrometheus(w) }

// observeAttempt records ONE ladder rung. Every attempt carries tokens the
// vendor billed and a latency the caller waited, terminal or not, so the
// histogram and the token counters are fed here rather than from the terminal
// alone — a retry that burned 4,000 prompt tokens before failing over cost
// exactly that whether or not its row is the one the caller got back.
func (m *callMetrics) observeAttempt(c Call) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.keyOfLocked(c)
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
		m.finish[finishKey{routeKey: k, reason: finishReasonLabel(c.FinishReason)}]++
	}
}

// observe records the TERMINAL of one logical call: the outcome the caller
// actually got. It is the denominator for "how much work was asked for",
// which is a different question from how many rungs answering it took, and
// keeping the two apart is the whole point of counting both.
func (m *callMetrics) observe(c Call) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.keyOfLocked(c)
	m.calls[k]++
	m.contextBytes[string(c.Task)] += int64(c.ContextBytes)
	m.contextTokens[string(c.Task)] += int64(c.ContextTokensEstimate)
}

// keyOf answers the label set this attempt is counted under, folding it into
// the single overflow key once the collector is full.
//
// The check is on the CALL family alone rather than on each map: they are keyed
// alike and grow together, so one is the honest measure of how many label sets
// the collector holds, and asking eight times would only make the guard slower
// to read.
func (m *callMetrics) keyOfLocked(c Call) routeKey {
	k := routeKey{
		task: string(c.Task), tier: string(c.Tier), provider: c.Provider,
		model: boundedSeriesLabel(c.ServedModel), servedIdentitySource: c.ServedIdentitySource,
	}
	if _, held := m.attempts[k]; held || len(m.attempts) < maxRouteSeries {
		return k
	}
	return routeKey{
		task: overflowLabel, tier: overflowLabel, provider: overflowLabel,
		model: overflowLabel, servedIdentitySource: overflowLabel,
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
		h = httpserver.NewHistogram(callLatencyBounds)
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
		{classCompletion, plainCompletionTokens(c.TokensOut, c.ReasoningTokens)},
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
