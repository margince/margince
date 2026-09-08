// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

// The Prometheus histogram this tree writes by hand, and the label escaper
// every hand-rolled family must use.
//
// Both were private to the HTTP section until a second surface needed them.
// The AI router's call-duration family is the second, and the alternative was
// a second implementation of bucket arithmetic and a second answer to "how is
// a label value escaped" — where the second answer is the dangerous one: get
// it wrong and Prometheus rejects the WHOLE scrape, so every family in the
// process leaves the dashboard together.
//
// Bounds are the caller's, deliberately. A route's latency and a model call's
// latency do not share a scale, and a shared bucket set would put every model
// call in the top bucket of one and every route in the bottom of the other.

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
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
	if h.counts == nil {
		h.counts = make([]uint64, len(h.bounds)+1)
	}
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
	if h.counts != nil {
		out.counts = make([]uint64, len(h.counts))
		copy(out.counts, h.counts)
	}
	return out
}

// WriteSeries renders this histogram's three series groups under name, with
// labels already rendered as a `k=v,k=v` fragment (empty for none).
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
		var seen uint64
		if h.counts != nil {
			seen = h.counts[i]
		}
		writeLine(w, "%s_bucket{%sle=%s} %d\n",
			name, prefix(labels), Label(strconv.FormatFloat(bound, 'g', -1, 64)), seen)
	}
	writeLine(w, "%s_bucket{%sle=\"+Inf\"} %d\n", name, prefix(labels), h.count)
	writeLine(w, "%s_sum{%s} %g\n", name, labels, h.sum)
	writeLine(w, "%s_count{%s} %d\n", name, labels, h.count)
}

// prefix answers labels with the separator a further label needs after it, so
// a family with no labels of its own still renders `{le="..."}` correctly
// rather than `{,le="..."}`.
func prefix(labels string) string {
	if labels == "" {
		return ""
	}
	return labels + ","
}

// writeLine is the ONE place this package's hand-rolled families discard a
// write error, and the exposition writer is why it is sound: it holds the
// first refusal, no-ops after it, and is asked once per scrape.
//
//craft:ignore swallowed-errors the exposition writer holds the first error and no-ops after it; the handler asks it once per scrape
func writeLine(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}

// Label renders a Prometheus label VALUE with only the three escapes the text
// format defines: backslash, double quote, and newline.
//
// Not %q. strconv.Quote is Go's escaping, not Prometheus', and the two agree
// only by coincidence on ordinary input: %q also emits \t, \r, \xNN and
// \uNNNN, and Prometheus' parser rejects those as an invalid escape sequence.
// It rejects the WHOLE SCRAPE when it does, not the offending line — so one
// stray byte in one label would take every family in this process off the
// dashboard at once.
//
// Exported because the reachable case is no longer hypothetical. Route comes
// from a compile-time template and method from a closed set, but the AI
// router labels by a tier binding's model id, which an operator types.
//
// Anything else that is not printable is dropped rather than escaped, because
// a control byte in a label value is not information an operator can use.
func Label(value string) string {
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')
	for _, r := range value {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r == '\n':
			b.WriteString(`\n`)
		case r < 0x20 || r == 0x7f:
			// Dropped, per the note above.
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
