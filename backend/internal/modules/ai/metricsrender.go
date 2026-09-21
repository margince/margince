// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The render half of the AI exposition: how the counters collected next door
// in metrics.go are turned into Prometheus text.
//
// Split from the collector for the file ceiling, and because the two answer
// different questions — metrics.go decides WHAT is counted and at what
// cardinality, this file only decides how it is spelled.
//
// EVERY FAMILY NAME IS A LITERAL AT ITS HEADER CALL, and that is a constraint
// rather than a style. backend/gates/metricsuffix_test.go is a census over the
// names this tree emits and it reads them out of SOURCE: a family whose name
// reaches its header through a variable is invisible to it, so `_total` would
// quietly stop meaning counter for exactly the families nobody could see. The
// header helpers answer the name they wrote, so each is spelled once here and
// still reaches the series lines below.

import (
	"io"
	"sort"

	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// callLatencyBounds are the AI call-duration histogram's upper bounds in
// seconds, ascending.
//
// Chosen for THIS surface rather than copied from the HTTP one, which stops at
// 10s: a cheap completion lands near 1s, a frontier one with reasoning runs to
// tens of seconds, and the provider timeouts this tree sets are past a minute.
// A shared bucket set would put every model call in the HTTP histogram's +Inf
// bucket, which is the bucket that is supposed to mean "unbounded".
var callLatencyBounds = []float64{
	0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 30, 60, 120,
}

// WritePrometheus renders every AI family.
//
// The snapshot is taken under the lock and rendered outside it: w is typically
// the /metrics HTTP response, and a slow scrape client must never hold the
// mutex every completion's observe takes — that would let one stalled scrape
// block AI calls process-wide.
func (m *callMetrics) WritePrometheus(w io.Writer) {
	snap := m.snapshot()

	writeRouteFamily(w, counterHeader(w, "margince_ai_calls_total",
		"AI logical calls that reached a terminal outcome since process start."), snap.calls)
	writeRouteFamily(w, counterHeader(w, "margince_ai_call_attempts_total",
		"AI attempts made since process start -- above calls_total by exactly the ladder walking and the retries."), snap.attempts)
	writeRouteFamily(w, counterHeader(w, "margince_ai_call_degraded_total",
		"AI attempts served on a demoted ladder because the budget guardrail forced one, since process start."), snap.degraded)
	writeRouteFamily(w, counterHeader(w, "margince_ai_call_cache_hits_total",
		"AI attempts answered from the result cache without reaching a provider, since process start."), snap.cacheHit)

	writeErrorFamily(w, counterHeader(w, "margince_ai_call_errors_total",
		"AI attempt failures since process start, by the sentinel the router classified them as."), snap.errors)
	writeFinishFamily(w, counterHeader(w, "margince_ai_call_finish_reasons_total",
		"Provider-reported stop reasons since process start, counted only for attempts that reached a provider."), snap.finish)
	writeTokenFamily(w, counterHeader(w, "margince_ai_tokens_total",
		"AI tokens billed since process start, split into disjoint classes."), snap.tokens)
	writeLatencyFamily(w, histogramHeader(w, "margince_ai_call_duration_seconds",
		"Wall time one AI attempt spent at the provider, in seconds. Cache hits are excluded: they measure a map lookup."), snap.latency)

	writeTaskCounterFamily(w, counterHeader(w, "margince_ai_company_context_bytes_total",
		"Company-context bytes supplied to AI attempts."), snap.contextBytes)
	writeTaskCounterFamily(w, counterHeader(w, "margince_ai_company_context_tokens_estimate_total",
		"Estimated company-context tokens supplied to AI attempts."), snap.contextTokens)
}

// counterHeader writes one family's HELP and TYPE lines and answers its name.
//
// Returning it is what lets the caller put the family name at the call site as
// a literal — which the metric-suffix census reads — and still reach the series
// lines below without a second copy of the string to keep in step.
func counterHeader(w io.Writer, name, help string) string {
	httpserver.WriteLine(w, "# HELP %s %s\n# TYPE %s counter\n", name, help, name)
	return name
}

func histogramHeader(w io.Writer, name, help string) string {
	httpserver.WriteLine(w, "# HELP %s %s\n# TYPE %s histogram\n", name, help, name)
	return name
}

// metricsSnapshot is the detached copy WritePrometheus renders from.
type metricsSnapshot struct {
	calls, attempts, degraded, cacheHit map[routeKey]uint64
	errors                              map[errorKey]uint64
	finish                              map[finishKey]uint64
	tokens                              map[tokenKey]int64
	latency                             map[routeKey]httpserver.Histogram
	contextBytes, contextTokens         map[string]int64
}

func (m *callMetrics) snapshot() metricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap := metricsSnapshot{
		calls:         copyRouteCounters(m.calls),
		attempts:      copyRouteCounters(m.attempts),
		degraded:      copyRouteCounters(m.degraded),
		cacheHit:      copyRouteCounters(m.cacheHit),
		errors:        make(map[errorKey]uint64, len(m.errors)),
		finish:        make(map[finishKey]uint64, len(m.finish)),
		tokens:        make(map[tokenKey]int64, len(m.tokens)),
		latency:       make(map[routeKey]httpserver.Histogram, len(m.latency)),
		contextBytes:  copyTaskCounters(m.contextBytes),
		contextTokens: copyTaskCounters(m.contextTokens),
	}
	for k, v := range m.errors {
		snap.errors[k] = v
	}
	for k, v := range m.finish {
		snap.finish[k] = v
	}
	for k, v := range m.tokens {
		snap.tokens[k] = v
	}
	for k, v := range m.latency {
		snap.latency[k] = v.Snapshot()
	}
	return snap
}

func copyRouteCounters(source map[routeKey]uint64) map[routeKey]uint64 {
	out := make(map[routeKey]uint64, len(source))
	for k, v := range source {
		out[k] = v
	}
	return out
}

// routeLabels renders the five labels every AI family carries. Every value
// goes through httpserver.Label, not %q: the model identity is provider- or
// operator-supplied, and Go quoting would emit escapes that cost the process
// its whole scrape.
func routeLabels(k routeKey) string {
	return "provider=" + httpserver.Label(k.provider) +
		",model=" + httpserver.Label(k.model) +
		",served_identity_source=" + httpserver.Label(k.servedIdentitySource) +
		",task=" + httpserver.Label(k.task) +
		",tier=" + httpserver.Label(k.tier)
}

// series is one rendered label set beside the key it came from, so a family's
// sort renders each label set ONCE instead of rebuilding it inside the
// comparator on every comparison.
type series[K comparable] struct {
	key    K
	labels string
}

// sortedSeries orders a family by its rendered labels. Prometheus does not
// care, but a human reading a scrape by hand does, and ranging a map would
// reshuffle the block on every request.
func sortedSeries[K comparable, V any](family map[K]V, labelsOf func(K) string) []series[K] {
	out := make([]series[K], 0, len(family))
	for k := range family {
		out = append(out, series[K]{key: k, labels: labelsOf(k)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].labels < out[j].labels })
	return out
}

func writeRouteFamily(w io.Writer, name string, family map[routeKey]uint64) {
	for _, s := range sortedSeries(family, routeLabels) {
		httpserver.WriteLine(w, "%s{%s} %d\n", name, s.labels, family[s.key])
	}
}

func writeErrorFamily(w io.Writer, name string, family map[errorKey]uint64) {
	labelsOf := func(k errorKey) string {
		return routeLabels(k.routeKey) + ",sentinel=" + httpserver.Label(k.sentinel)
	}
	for _, s := range sortedSeries(family, labelsOf) {
		httpserver.WriteLine(w, "%s{%s} %d\n", name, s.labels, family[s.key])
	}
}

func writeFinishFamily(w io.Writer, name string, family map[finishKey]uint64) {
	labelsOf := func(k finishKey) string {
		return routeLabels(k.routeKey) + ",reason=" + httpserver.Label(k.reason)
	}
	for _, s := range sortedSeries(family, labelsOf) {
		httpserver.WriteLine(w, "%s{%s} %d\n", name, s.labels, family[s.key])
	}
}

// writeTokenFamily renders the token counters. The direction label rides
// beside class so sum by (direction) still answers "prompt versus completion"
// after the split -- and because the classes are disjoint (metrics.go's
// uncachedTokensIn), that sum is the honest total rather than one that counts
// cached input twice.
func writeTokenFamily(w io.Writer, name string, family map[tokenKey]int64) {
	labelsOf := func(k tokenKey) string {
		return routeLabels(k.routeKey) +
			",class=" + httpserver.Label(k.class) +
			",direction=" + httpserver.Label(directionOf(k.class))
	}
	for _, s := range sortedSeries(family, labelsOf) {
		httpserver.WriteLine(w, "%s{%s} %d\n", name, s.labels, family[s.key])
	}
}

func writeLatencyFamily(w io.Writer, name string, family map[routeKey]httpserver.Histogram) {
	for _, s := range sortedSeries(family, routeLabels) {
		h := family[s.key]
		h.WriteSeries(w, name, s.labels)
	}
}

func writeTaskCounterFamily(w io.Writer, name string, family map[string]int64) {
	tasks := make([]string, 0, len(family))
	for task := range family {
		tasks = append(tasks, task)
	}
	sort.Strings(tasks)
	for _, task := range tasks {
		httpserver.WriteLine(w, "%s{task=%s} %d\n", name, httpserver.Label(task), family[task])
	}
}
