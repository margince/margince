// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capturemetrics

// Every family name is a literal at its header call: the metric-suffix census
// in backend/gates reads the names out of source.

import (
	"io"
	"sort"

	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// WriteProcessMetrics renders this process's import counters. Every family's
// header is written even before its first sample, so a process that has
// imported nothing reads as idle rather than as one that renders nothing.
func WriteProcessMetrics(w io.Writer) { shared.WritePrometheus(w) }

func (c *collector) WritePrometheus(w io.Writer) {
	snap := c.snapshot()

	writeRequests(w, counterHeader(w, "margince_connector_requests_total",
		"Provider API calls since process start, by op and result."), "result", snap.requests)
	writeRequests(w, counterHeader(w, "margince_connector_rate_limited_total",
		"Provider API calls refused for a rate limit since process start, by op and the limit the provider named."), "reason", snap.rateLimited)
	writeHistograms(w, histogramHeader(w, "margince_connector_request_duration_seconds",
		"Wall time of one provider API call, in seconds."), "op", snap.requestTime)
	writeCounters(w, counterHeader(w, "margince_capture_backfill_messages_total",
		"Messages a mailbox backfill walked since process start, by the outcome each came to."), "outcome", snap.messages)
	writeHistograms(w, histogramHeader(w, "margince_capture_backfill_stage_seconds",
		"Wall time one backfilled message spent in each stage, in seconds: fetch and parse are the provider's download and our parse, sink is the capture transaction, ensure the counterparty and project work after it."),
		"stage", snap.stages)
	writeCounters(w, counterHeader(w, "margince_capture_backfill_pages_total",
		"Backfill pages since process start, by result."), "result", snap.pages)
	writeSeconds(w, counterHeader(w, "margince_capture_backfill_snooze_seconds_total",
		"Seconds a backfill chose to wait before its next page, since process start, by reason."), "reason", snap.snoozed)
	writeProviderSeconds(w, counterHeader(w, "margince_capture_backfill_retry_after_seconds_total",
		"Seconds of Retry-After the provider asked for on the faults a backfill waited out, since process start."), snap.retryAfter)
}

func counterHeader(w io.Writer, name, help string) string {
	httpserver.WriteLine(w, "# HELP %s %s\n# TYPE %s counter\n", name, help, name)
	return name
}

func histogramHeader(w io.Writer, name, help string) string {
	httpserver.WriteLine(w, "# HELP %s %s\n# TYPE %s histogram\n", name, help, name)
	return name
}

type snapshot struct {
	requests            map[requestKey]uint64
	rateLimited         map[requestKey]uint64
	requestTime, stages map[pair]httpserver.Histogram
	messages, pages     map[pair]uint64
	snoozed             map[pair]float64
	retryAfter          map[string]float64
}

// snapshot copies the collector under its lock so a slow scrape never holds
// the mutex every provider call takes.
func (c *collector) snapshot() snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return snapshot{
		requests:    copyMap(c.requests),
		rateLimited: copyMap(c.rateLimited),
		requestTime: copyHistograms(c.requestTime),
		stages:      copyHistograms(c.stages),
		messages:    copyMap(c.messages),
		pages:       copyMap(c.pages),
		snoozed:     copyMap(c.snoozed),
		retryAfter:  copyMap(c.retryAfter),
	}
}

func copyMap[K comparable, V any](source map[K]V) map[K]V {
	out := make(map[K]V, len(source))
	for k, v := range source {
		out[k] = v
	}
	return out
}

func copyHistograms(source map[pair]*httpserver.Histogram) map[pair]httpserver.Histogram {
	out := make(map[pair]httpserver.Histogram, len(source))
	for k, h := range source {
		out[k] = h.Snapshot()
	}
	return out
}

// sortedLabels renders each key's labels once and orders the family by them,
// so a scrape read by hand does not reshuffle between requests.
func sortedLabels[K comparable, V any](family map[K]V, labelsOf func(K) string) ([]K, map[K]string) {
	keys := make([]K, 0, len(family))
	labels := make(map[K]string, len(family))
	for k := range family {
		keys = append(keys, k)
		labels[k] = labelsOf(k)
	}
	sort.Slice(keys, func(i, j int) bool { return labels[keys[i]] < labels[keys[j]] })
	return keys, labels
}

func pairLabels(label string) func(pair) string {
	return func(k pair) string {
		return "provider=" + httpserver.Label(k.provider) + "," + label + "=" + httpserver.Label(k.value)
	}
}

// writeRequests renders a family keyed by provider, op and one label of its
// own, which requestKey carries as result.
func writeRequests(w io.Writer, name, label string, family map[requestKey]uint64) {
	keys, labels := sortedLabels(family, func(k requestKey) string {
		return "provider=" + httpserver.Label(k.provider) +
			",op=" + httpserver.Label(k.op) +
			"," + label + "=" + httpserver.Label(k.result)
	})
	for _, k := range keys {
		httpserver.WriteLine(w, "%s{%s} %d\n", name, labels[k], family[k])
	}
}

func writeCounters(w io.Writer, name, label string, family map[pair]uint64) {
	keys, labels := sortedLabels(family, pairLabels(label))
	for _, k := range keys {
		httpserver.WriteLine(w, "%s{%s} %d\n", name, labels[k], family[k])
	}
}

func writeSeconds(w io.Writer, name, label string, family map[pair]float64) {
	keys, labels := sortedLabels(family, pairLabels(label))
	for _, k := range keys {
		httpserver.WriteLine(w, "%s{%s} %g\n", name, labels[k], family[k])
	}
}

func writeHistograms(w io.Writer, name, label string, family map[pair]httpserver.Histogram) {
	keys, labels := sortedLabels(family, pairLabels(label))
	for _, k := range keys {
		h := family[k]
		h.WriteSeries(w, name, labels[k])
	}
}

func writeProviderSeconds(w io.Writer, name string, family map[string]float64) {
	providers := make([]string, 0, len(family))
	for provider := range family {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	for _, provider := range providers {
		httpserver.WriteLine(w, "%s{provider=%s} %g\n", name, httpserver.Label(provider), family[provider])
	}
}
