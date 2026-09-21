// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

// The Prometheus histogram this tree writes by hand.
//
// It was private to the HTTP section until a second surface needed it. The AI
// router's call-duration family is the second, and the alternative was a second
// implementation of bucket arithmetic.
//
// Bounds are the caller's, deliberately. A route's latency and a model call's
// latency do not share a scale, and a shared bucket set would put every model
// call in the top bucket of one and every route in the bottom of the other.

import (
	"io"
	"sort"
	"strconv"
)

// Histogram is one hand-rolled Prometheus histogram: cumulative bucket
// counts against ascending upper bounds, plus the sum and count every
// quantile read needs.
//
// Not safe for concurrent use. Every caller here holds the collector's mutex
// while observing, which is also what makes Snapshot's copy meaningful.
type Histogram struct {
	bounds []float64
	counts []uint64
	sum    float64
	count  uint64
}

// NewHistogram builds a histogram over bounds, which must be ascending upper
// bounds in the sample's own unit.
//
// The slice is retained rather than defensively duplicated, so a caller must
// treat it as immutable once handed over: the counts already taken are indexed
// against these bounds, and moving one under a live histogram re-interprets
// every sample behind it instead of failing. Every caller passes a
// package-level literal, which is what makes that safe.
func NewHistogram(bounds []float64) *Histogram {
	return &Histogram{bounds: bounds, counts: make([]uint64, len(bounds)+1)}
}

// Observe records one sample.
func (h *Histogram) Observe(value float64) {
	h.sum += value
	h.count++
	// SearchFloat64s finds the first bound the sample does NOT exceed; every
	// bucket from there up is cumulative and gains the sample too.
	for i := sort.SearchFloat64s(h.bounds, value); i < len(h.counts); i++ {
		h.counts[i]++
	}
}

// Snapshot answers a detached copy, so a slow scrape renders outside the lock
// that the observing path takes on every call.
func (h *Histogram) Snapshot() Histogram {
	out := Histogram{bounds: h.bounds, sum: h.sum, count: h.count}
	out.counts = make([]uint64, len(h.counts))
	copy(out.counts, h.counts)
	return out
}

// WriteSeries renders this histogram's three series groups under name, with
// labels already rendered as a non-empty `k=v,k=v` fragment. Every histogram in
// this tree is keyed by something; a family with no labels of its own would
// need the brace handling this deliberately does not carry.
//
// The +Inf bucket is written unconditionally and equals count by definition: a
// histogram without its terminator is not a histogram, because every quantile
// read walks to it.
//
// No error is returned, and that is the contract rather than an omission: w is
// always the exposition writer, which remembers the FIRST refused write and
// turns every write after it into a no-op, and the handler asks it once at the
// end of the scrape. Returning an error here would add a branch per bucket to
// report what the writer already holds, and both callers would discard it.
func (h Histogram) WriteSeries(w io.Writer, name, labels string) {
	for i, bound := range h.bounds {
		WriteLine(w, "%s_bucket{%s,le=%s} %d\n",
			name, labels, Label(strconv.FormatFloat(bound, 'g', -1, 64)), h.counts[i])
	}
	WriteLine(w, "%s_bucket{%s,le=\"+Inf\"} %d\n", name, labels, h.count)
	WriteLine(w, "%s_sum{%s} %g\n", name, labels, h.sum)
	WriteLine(w, "%s_count{%s} %d\n", name, labels, h.count)
}

// prefix answers labels with the separator a further label needs after it, so
// a family with no labels of its own still renders `{le="..."}` correctly
// rather than `{,le="..."}`.
