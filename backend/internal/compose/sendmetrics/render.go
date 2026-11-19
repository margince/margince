// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package sendmetrics

// Every family name is a literal at its header call: the metric-suffix census
// in backend/gates reads the names out of source.

import (
	"io"
	"maps"
	"sort"

	"github.com/margince/margince/backend/internal/platform/httpserver"
)

type snapshot struct {
	outcomes, reviews, anyway map[pair]uint64
	decisions                 map[string]httpserver.Histogram
}

// snapshot copies the collector under its lock, so a slow scrape never holds
// the mutex every Send takes.
func (c *collector) snapshot() snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := snapshot{
		outcomes: maps.Clone(c.outcomes), reviews: maps.Clone(c.reviews), anyway: maps.Clone(c.anyway),
		decisions: make(map[string]httpserver.Histogram, len(c.decisions)),
	}
	for verdict, h := range c.decisions {
		out.decisions[verdict] = h.Snapshot()
	}
	return out
}

func (c *collector) writePrometheus(w io.Writer) {
	snap := c.snapshot()
	writeCounters(w, counterHeader(w, "margince_email_send_outcomes_total",
		"Distinct first Sends of a human in the email composer since process start, by composer flow and outcome. Repeat presses and Send anyway confirmations are not first Sends."),
		"outcome", snap.outcomes)
	writeCounters(w, counterHeader(w, "margince_email_send_reviews_total",
		"First Sends that the engine refused and that opened a review, by composer flow and the context class of the resolved category."),
		"context", snap.reviews)
	writeCounters(w, counterHeader(w, "margince_email_send_anyway_total",
		"Send anyway confirmations since process start, by the composer that confirmed and what the confirmation came to."),
		"result", snap.anyway)
	name := histogramHeader(w, "margince_email_send_decision_seconds",
		"Wall time of the engine's decision on a human's fresh Send at staging, in seconds. Provider delivery is not part of it.")
	for _, verdict := range verdicts {
		snap.decisions[verdict].WriteSeries(w, name, "verdict="+httpserver.Label(verdict))
	}
}

func counterHeader(w io.Writer, name, help string) string {
	httpserver.WriteLine(w, "# HELP %s %s\n# TYPE %s counter\n", name, help, name)
	return name
}

func histogramHeader(w io.Writer, name, help string) string {
	httpserver.WriteLine(w, "# HELP %s %s\n# TYPE %s histogram\n", name, help, name)
	return name
}

// writeCounters renders a family sorted by its labels, so a scrape read by
// hand does not reshuffle between requests.
func writeCounters(w io.Writer, name, label string, family map[pair]uint64) {
	labels := make(map[pair]string, len(family))
	keys := make([]pair, 0, len(family))
	for k := range family {
		keys = append(keys, k)
		labels[k] = "flow=" + httpserver.Label(k.flow) + "," + label + "=" + httpserver.Label(k.value)
	}
	sort.Slice(keys, func(i, j int) bool { return labels[keys[i]] < labels[keys[j]] })
	for _, k := range keys {
		httpserver.WriteLine(w, "%s{%s} %d\n", name, labels[k], family[k])
	}
}
