// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The job-runtime section of /metrics: OPS-MET-2's queue depth and the
// gauges beside it, plus the two sweep pairs. Every number here is read from
// river_job at scrape time rather than counted in process, because the work
// itself happens in cmd/worker: an in-process counter incremented there is
// one replica's tally, so a fleet-wide number kept that way would report
// whatever the scraped process happened to have done. cmd/worker does serve
// its own /metrics (--observe-addr), and deliberately carries only what IS
// process-local — its runtime, its pool, its relay counter — leaving every
// reading of this shared table to the one reader here.
//
// The two families in jobdeclared.go beside it are the exception, and the
// reason they exist: a table scan can only ever name a kind that HAS rows,
// so it reads a declared-but-idle kind and a kind nobody wired identically.
// Those two are read against the compiled declaration instead.

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// queueDepthStates are the states a job is WAITING in. A job backing off
// toward its next attempt is queued work nobody has done, so retryable
// belongs here — OPS-MET-2 asks how much work is outstanding, and a queue
// full of retrying jobs is the case the metric exists to surface. pending
// is River's staged state (its migration 004): nothing in this tree sets
// it today, and it is counted rather than dropped because a state the
// renderer has never heard of vanishing is the posture this whole phase
// exists to remove.
var queueDepthStates = map[string]bool{
	"available": true,
	"scheduled": true,
	"retryable": true,
	"pending":   true,
}

// queueKey identifies one series of the per-queue gauges: the age gauge,
// which is legitimately per-queue rather than per-kind — "how long has the
// oldest runnable job in this queue waited" does not split by which kind
// that job is.
type queueKey struct{ queue string }

// queueKindKey identifies one series of the depth/running gauges. Kind is
// on the wire alongside queue because most kinds share the "default" queue
// (opts_owner: caller and every kind without its own pool), so a queue-only
// breakdown collapses dozens of unrelated kinds into one series named
// "default" — useless for "which kind is backed up" or "which kind is
// actually running".
type queueKindKey struct{ kind, queue string }

// kindKey identifies one series of the per-kind gauges. Dead work is keyed
// by kind rather than by queue because "which work will never run" is a
// question about the kind, not about the lane it happened to sit in.
type kindKey struct{ kind string }

// stateKey identifies one series of the unrecognised-state gauge, which
// carries the state itself so an operator can see what appeared.
type stateKey struct{ state, queue string }

// The river_job_state values this renderer classifies by name. Named so
// the spelling that decides which gauge a row lands on exists once.
const (
	stateRunning   = "running"
	stateDiscarded = "discarded"
	stateCancelled = "cancelled"
)

// jobSeries is one pass of the snapshot, bucketed per family.
type jobSeries struct {
	depth        map[queueKindKey]int64
	running      map[queueKindKey]int64
	discarded    map[kindKey]int64
	cancelled    map[kindKey]int64
	oldest       map[queueKey]float64
	unrecognised map[stateKey]int64
}

// aggregate buckets every row exactly once. Split out of writeJobMetrics so
// the classification reads as one list of cases rather than as a preamble to
// seven writes.
func aggregate(rows []jobs.StateRow) jobSeries {
	a := jobSeries{
		depth:        map[queueKindKey]int64{},
		running:      map[queueKindKey]int64{},
		discarded:    map[kindKey]int64{},
		cancelled:    map[kindKey]int64{},
		oldest:       map[queueKey]float64{},
		unrecognised: map[stateKey]int64{},
	}
	for _, r := range rows {
		qk := queueKey{queue: r.Queue}
		qkk := queueKindKey{kind: r.Kind, queue: r.Queue}
		kk := kindKey{kind: r.Kind}
		switch {
		case queueDepthStates[r.State]:
			a.depth[qkk] += r.Count
		case r.State == stateRunning:
			a.running[qkk] += r.Count
		case r.State == stateDiscarded:
			a.discarded[kk] += r.Count
		case r.State == stateCancelled:
			// Reported apart from discarded on purpose. River cancels
			// deliberately, without spending every attempt, so folding the
			// two together would make the discarded gauge's own HELP text
			// false for half the rows under it.
			a.cancelled[kk] += r.Count
		default:
			a.unrecognised[stateKey{state: r.State, queue: r.Queue}] += r.Count
		}
		a.ageFrom(qk, r)
	}
	return a
}

// ageFrom folds one row into the per-queue oldest-runnable age.
//
// The age is the WORST case across the kinds sharing a queue; reporting the
// last row read would make the number depend on the order the database
// returned groups in.
//
// Only WAITING rows contribute a series at all. The gauge measures the
// oldest runnable-and-UNCLAIMED job, so a running row is not its subject —
// it has already been claimed — and a discarded one never will be. A queue
// holding nothing but those has no such job, and emitting a zero for it
// would answer the gauge's own question with a number about work that is
// not waiting. It is also what keeps this gauge agreeing with the endpoint,
// which reports null for exactly these rows.
//
// A zero from a queue that DOES hold waiting work is a measured value: the
// work is there and none of it is late yet.
func (a jobSeries) ageFrom(qk queueKey, r jobs.StateRow) {
	if !queueDepthStates[r.State] || r.OldestRunnableAgeSeconds == nil {
		return
	}
	if _, seen := a.oldest[qk]; !seen || *r.OldestRunnableAgeSeconds > a.oldest[qk] {
		a.oldest[qk] = *r.OldestRunnableAgeSeconds
	}
}

// writeJobMetrics renders the job-runtime section. Pure over snap so the
// exposition text is provable without a database.
//
// The series carry no workspace label: an installation holds one company, so
// every row of river_job belongs to it.
//
// It returns an error because an io.Writer can fail. A scrape that
// swallowed a refused write would serve a truncated exposition, which
// parses as a smaller fleet rather than as a broken one.
func writeJobMetrics(w io.Writer, snap jobs.Snapshot) error {
	a := aggregate(snap.Rows)
	depth, running := a.depth, a.running
	discarded, cancelled := a.discarded, a.cancelled
	oldest, unrecognised := a.oldest, a.unrecognised

	// The catalogue leads: everything below it is a projection of whatever
	// rows happen to exist, and this is the closed set they are read against.
	if err := writeDeclaredInfo(w); err != nil {
		return err
	}
	if err := writeQueueKindGauge(w, "margince_job_queue_depth",
		"Jobs waiting to run (available + scheduled + retryable + pending) per kind and queue.",
		depth); err != nil {
		return err
	}
	if err := writeQueueKindGauge(w, "margince_job_running",
		"Jobs currently executing per kind and queue.",
		running); err != nil {
		return err
	}
	if err := writeKindGauge(w, "margince_job_discarded",
		"Jobs that exhausted every attempt and will never run without intervention, per kind.",
		discarded); err != nil {
		return err
	}
	if err := writeKindGauge(w, "margince_job_cancelled",
		"Jobs stopped deliberately before their attempts ran out, per kind. Counted apart from discarded because the operator story differs -- a discarded job spent every attempt, a cancelled one was stopped. Both are work that will not happen, which is why the sweep pair counts either as a pass missed.",
		cancelled); err != nil {
		return err
	}
	if err := writeAgeGauge(w, oldest); err != nil {
		return err
	}
	if err := writeSweepGauges(w, snap.Sweeps); err != nil {
		return err
	}
	if err := writeSweepUnitGauges(w, snap.Units); err != nil {
		return err
	}
	if err := writeJobFailureGauge(w, snap.Failures); err != nil {
		return err
	}
	if err := writeUnrecognisedStateGauge(w, unrecognised); err != nil {
		return err
	}
	return writeUnrecognisedKindGauge(w, snap.Rows)
}

func writeQueueKindGauge(w io.Writer, name, help string, series map[queueKindKey]int64) error {
	if err := writeFamilyHeader(w, name, help); err != nil {
		return err
	}
	keys := sortedKeysOf(series, func(a, b queueKindKey) int {
		return cmp.Or(cmp.Compare(a.kind, b.kind), cmp.Compare(a.queue, b.queue))
	})
	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "%s{kind=%s,queue=%s} %d\n",
			name, label(k.kind), label(k.queue), series[k]); err != nil {
			return err
		}
	}
	return nil
}

func writeKindGauge(w io.Writer, name, help string, series map[kindKey]int64) error {
	if err := writeFamilyHeader(w, name, help); err != nil {
		return err
	}
	keys := sortedKeysOf(series, func(a, b kindKey) int {
		return cmp.Compare(a.kind, b.kind)
	})
	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "%s{kind=%s} %d\n",
			name, label(k.kind), series[k]); err != nil {
			return err
		}
	}
	return nil
}

func writeAgeGauge(w io.Writer, series map[queueKey]float64) error {
	const name = "margince_job_oldest_queued_age_seconds"
	if err := writeFamilyHeader(w, name,
		"Seconds the oldest runnable-and-unclaimed job has waited per queue. A job scheduled for the future is not counted; it is not late."); err != nil {
		return err
	}
	for _, k := range sortedQueueKeys(series) {
		if _, err := fmt.Fprintf(w, "%s{queue=%s} %.0f\n",
			name, label(k.queue), series[k]); err != nil {
			return err
		}
	}
	return nil
}

// writeUnrecognisedStateGauge reports work sitting in a state no arm above
// claimed — a River release that adds one, or a state this renderer was
// never taught. Its header is written ONLY when there is something to
// report: unlike the families above, a permanently empty series here would
// be noise on every dashboard, and its whole purpose is to be absent.
func writeUnrecognisedStateGauge(w io.Writer, series map[stateKey]int64) error {
	if len(series) == 0 {
		return nil
	}
	const name = "margince_job_unrecognised_state"
	if err := writeFamilyHeader(w, name,
		"Jobs in a state this exposition does not classify. Present only when such work exists; investigate rather than graph."); err != nil {
		return err
	}
	keys := sortedKeysOf(series, func(a, b stateKey) int {
		return cmp.Or(
			cmp.Compare(a.state, b.state),
			cmp.Compare(a.queue, b.queue))
	})
	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "%s{state=%s,queue=%s} %d\n",
			name, label(k.state), label(k.queue), series[k]); err != nil {
			return err
		}
	}
	return nil
}

func writeFamilyHeader(w io.Writer, name, help string) error {
	_, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
	return err
}

// label renders one label value for the Prometheus text format, which
// defines exactly three escapes — backslash, double quote and newline —
// and no others.
//
// Go's %q is not that format. It emits \t, \r and \xNN for control
// characters, and a parser meeting an escape the format does not define
// rejects the whole exposition rather than the one series. That matters
// here because kind and queue are river_job columns verbatim, in a table
// with no constraint on them and direct app-role CRUD. A single raw-inserted
// row could otherwise poison every scrape for every metric.
// A control character outside those three is rewritten as a printable
// <0xNN>, never dropped. Dropping is lossy: two malformed kind names that
// differ only in a control byte would collapse to one label set, and
// duplicate series make a scrape unparseable.
//
// A literal '<' is escaped the same way, and that is what makes the mapping
// INJECTIVE rather than merely printable. Without it, a value containing
// the literal six characters "<0x09>" would render identically to one
// containing a real tab — reintroducing exactly the collision this function
// exists to remove. Every '<' in the output therefore begins an escape.
func label(value string) string {
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
		case r < ' ' || r == 0x7f || r == '<':
			fmt.Fprintf(&b, "<0x%02x>", r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// sortedQueueKeys orders the per-queue series. A scrape target's series
// order should not flap between scrapes for no reason, and map iteration
// order is not stable.
func sortedQueueKeys[V any](series map[queueKey]V) []queueKey {
	return sortedKeysOf(series, func(a, b queueKey) int {
		return cmp.Compare(a.queue, b.queue)
	})
}

// sortedKeysOf answers a series map's keys in the caller's order.
func sortedKeysOf[K comparable, V any](series map[K]V, order func(a, b K) int) []K {
	keys := make([]K, 0, len(series))
	for k := range series {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, order)
	return keys
}

// jobMetricsSection binds the job gauges to a snapshot reader. A failed
// read logs and writes NOTHING for the section: a scrape that emits zeroes
// it did not measure is worse than a gap, because a gap is visible on a
// graph while a fabricated zero reads as a healthy empty queue. This is the
// same posture Metrics already takes for the outbox backlog.
//
// The reader is a closure rather than the pool itself, matching how the
// outbox backlog is injected next door — so that posture is provable
// without a database, which is the only way it stays true.
//
// The ctx handed in is the exposition handler's own budget context, NOT the
// request's. The overlay section next door passes r.Context() and so runs
// unbounded; this read is the one that needs the budget, because it scans a
// table no index covers.
func jobMetricsSection(read func(context.Context) (jobs.Snapshot, error)) func(context.Context, io.Writer) error {
	return func(ctx context.Context, w io.Writer) error {
		snap, err := read(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "metrics: job stats query failed", "err", err)
			// A section that could not be measured is absent, not fatal: the
			// rest of the exposition is still true and still worth serving.
			return nil
		}
		return writeJobMetrics(w, snap)
	}
}
