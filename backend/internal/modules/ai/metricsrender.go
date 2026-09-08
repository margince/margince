// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The render half of the AI exposition: how the counters collected next door
// in metrics.go are turned into Prometheus text.
//
// Split from the collector for the file ceiling, and because the two answer
// different questions — metrics.go decides WHAT is counted and at what
// cardinality, this file only decides how it is spelled.

import (
	"fmt"
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

func newLatencyHistogram() *httpserver.Histogram {
	return httpserver.NewHistogram(callLatencyBounds)
}

// WritePrometheus renders every AI family.
//
// The snapshot is taken under the lock and rendered outside it: w is typically
// the /metrics HTTP response, and a slow scrape client must never hold the
// mutex every completion's observe takes — that would let one stalled scrape
// block AI calls process-wide.
func (m *callMetrics) WritePrometheus(w io.Writer) {
	snap := m.snapshot()

	writeRouteFamily(w, "margince_ai_calls_total",
		"AI logical calls that reached a terminal outcome since process start.", snap.calls)
	writeRouteFamily(w, "margince_ai_call_attempts_total",
		"AI attempts made since process start -- above calls_total by exactly the ladder walking and the retries.", snap.attempts)
	writeRouteFamily(w, "margince_ai_call_degraded_total",
		"AI attempts served on a demoted ladder because the budget guardrail forced one, since process start.", snap.degraded)
	writeRouteFamily(w, "margince_ai_call_cache_hits_total",
		"AI attempts answered from the result cache without reaching a provider, since process start.", snap.cacheHit)

	writeErrorFamily(w, snap.errors)
	writeFinishFamily(w, snap.finish)
	writeTokenFamily(w, snap.tokens)
	writeLatencyFamily(w, snap.latency)

	writeTaskCounterFamily(w, "margince_ai_company_context_bytes_total",
		"Company-context bytes supplied to AI attempts.", snap.contextBytes)
	writeTaskCounterFamily(w, "margince_ai_company_context_tokens_estimate_total",
		"Estimated company-context tokens supplied to AI attempts.", snap.contextTokens)
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
// goes through httpserver.Label, not %q: model is operator-supplied and Go
// quoting would emit escapes that cost the process its whole scrape.
func routeLabels(k routeKey) string {
	return "provider=" + httpserver.Label(k.provider) +
		",model=" + httpserver.Label(k.model) +
		",served_identity_source=" + httpserver.Label(k.servedIdentitySource) +
		",task=" + httpserver.Label(k.task) +
		",tier=" + httpserver.Label(k.tier)
}

// sortedRouteKeys orders a family's series. Prometheus does not care, but a
// human reading a scrape by hand does, and ranging a map would reshuffle the
// block on every request.
func sortedRouteKeys[V any](family map[routeKey]V) []routeKey {
	keys := make([]routeKey, 0, len(family))
	for k := range family {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return routeLabels(keys[i]) < routeLabels(keys[j]) })
	return keys
}

func writeHeader(w io.Writer, name, help, kind string) {
	writeLine(w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
}

func writeRouteFamily(w io.Writer, name, help string, family map[routeKey]uint64) {
	writeHeader(w, name, help, "counter")
	for _, k := range sortedRouteKeys(family) {
		writeLine(w, "%s{%s} %d\n", name, routeLabels(k), family[k])
	}
}

func writeErrorFamily(w io.Writer, family map[errorKey]uint64) {
	const name = "margince_ai_call_errors_total"
	writeHeader(w, name, "AI attempt failures since process start, by the sentinel the router classified them as.", "counter")
	keys := make([]errorKey, 0, len(family))
	for k := range family {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].routeKey != keys[j].routeKey {
			return routeLabels(keys[i].routeKey) < routeLabels(keys[j].routeKey)
		}
		return keys[i].sentinel < keys[j].sentinel
	})
	for _, k := range keys {
		writeLine(w, "%s{%s,sentinel=%s} %d\n", name, routeLabels(k.routeKey), httpserver.Label(k.sentinel), family[k])
	}
}

func writeFinishFamily(w io.Writer, family map[finishKey]uint64) {
	const name = "margince_ai_call_finish_reasons_total"
	writeHeader(w, name, "AI attempts by the provider's normalized stop reason -- a truncated answer and a complete one are otherwise the same call.", "counter")
	keys := make([]finishKey, 0, len(family))
	for k := range family {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].routeKey != keys[j].routeKey {
			return routeLabels(keys[i].routeKey) < routeLabels(keys[j].routeKey)
		}
		return keys[i].reason < keys[j].reason
	})
	for _, k := range keys {
		writeLine(w, "%s{%s,reason=%s} %d\n", name, routeLabels(k.routeKey), httpserver.Label(k.reason), family[k])
	}
}

// writeTokenFamily renders the token counters. The direction label rides
// beside class so sum by (direction) still answers "prompt versus completion"
// after the split -- and because the classes are disjoint (metrics.go's
// uncachedTokensIn), that sum is the honest total rather than one that counts
// cached input twice.
func writeTokenFamily(w io.Writer, family map[tokenKey]int64) {
	const name = "margince_ai_tokens_total"
	writeHeader(w, name, "AI tokens billed since process start, split into disjoint classes.", "counter")
	keys := make([]tokenKey, 0, len(family))
	for k := range family {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].routeKey != keys[j].routeKey {
			return routeLabels(keys[i].routeKey) < routeLabels(keys[j].routeKey)
		}
		return keys[i].class < keys[j].class
	})
	for _, k := range keys {
		writeLine(w, "%s{%s,class=%s,direction=%s} %d\n",
			name, routeLabels(k.routeKey), httpserver.Label(k.class), httpserver.Label(directionOf(k.class)), family[k])
	}
}

func writeLatencyFamily(w io.Writer, family map[routeKey]httpserver.Histogram) {
	const name = "margince_ai_call_duration_seconds"
	writeHeader(w, name, "Wall time one AI attempt spent at the provider, in seconds. Cache hits are excluded: they measure a map lookup.", "histogram")
	for _, k := range sortedRouteKeys(family) {
		h := family[k]
		h.WriteSeries(w, name, routeLabels(k))
	}
}

func writeTaskCounterFamily(w io.Writer, name, help string, family map[string]int64) {
	writeHeader(w, name, help, "counter")
	tasks := make([]string, 0, len(family))
	for task := range family {
		tasks = append(tasks, task)
	}
	sort.Strings(tasks)
	for _, task := range tasks {
		writeLine(w, "%s{task=%s} %d\n", name, httpserver.Label(task), family[task])
	}
}

// writeLine drops the write error deliberately, and it is the ONE place in
// this file that does so.
//
// Every caller's w is httpserver's exposition writer, which remembers the first
// refused write, turns every write after it into a no-op, and is asked once by
// the handler at the end of the scrape. A refused write here means the scraper
// hung up; there is nobody to return an error to, and threading one back
// through every renderer would add a branch per line to say what the writer
// already knows.
//
//craft:ignore swallowed-errors the exposition writer holds the first error and no-ops after it; the handler asks it once per scrape
func writeLine(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}
