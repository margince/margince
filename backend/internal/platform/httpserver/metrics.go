// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

// The /metrics endpoint: the exposition handler, the sections this package
// owns, and the input struct the composition layer fills in. What each number
// MEANS is documented beside the section that renders it; what to count is
// injected, because this package owns no domain state.

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OverlayMetrics is the overlay sync-health section /metrics adds when
// this role has an incumbent connection surface wired (design.md §4.7):
// per-object-class source lag (the fleet-wide worst-case staleness),
// plus the inbound sync-rate and conflict-rate counters. Nil means this
// role never wired an overlay keyvault (WithKeyvault absent) — the same
// "declared or absent" posture backlog/published already establish for
// the outbox relay, never a silent zero-valued section.
type OverlayMetrics struct {
	// SourceLag answers, per canonical object class, now minus the
	// oldest last_synced_at seen anywhere in the fleet for that class.
	SourceLag func(context.Context) (map[string]time.Duration, error)
	// SyncedTotal answers the process's inbound mirror-sync counter.
	SyncedTotal func() uint64
	// ConflictTotal answers the process's mirror.conflict counter.
	ConflictTotal func() uint64
	// DeletedTotal answers the process's mirror.deleted counter (records
	// purged from the mirror by the continuous-sync deletion feed).
	DeletedTotal func() uint64
}

// MetricsInput is everything a role wires into its /metrics endpoint. It is a
// struct rather than a parameter list because the list ran out of room, and
// because the fields divide into two kinds that a positional call could not
// keep straight.
//
// FLEET-WIDE fields read shared state — the outbox table, the job table, the
// mirror — and any role holding a pool answers them identically. A second role
// publishing its own copy would add a duplicate series that agrees with the
// first except for a scrape interval, so exactly ONE role wires them and every
// other passes nil. Today that role is the api.
//
// PER-PROCESS state is the opposite: it is read out of THIS runtime, differs on
// every target, and a role that leaves it unwired publishes nothing about
// itself while still doing the work. It is not a field here at all — the
// runtime collector below is unconditional, and the AI counters reach the
// handler through Extra, which every role that resolves a model path must wire.
//
// A nil field means "this role does not answer this", never "the answer is
// zero": a gauge reporting rows it did not count reads as a drained outbox.
type MetricsInput struct {
	// Pool renders the connection-pool gauges. Per-process despite naming a
	// shared database: each role holds its own pool against it.
	Pool *pgxpool.Pool
	// Backlog answers committed outbox rows the relay has not shipped.
	// Fleet-wide.
	Backlog func(context.Context) (int64, error)
	// Published answers this process's relay counter. Per-process — both
	// roles run a relay, and zero on one while the other publishes is a
	// different fault from zero on both.
	Published func() uint64
	// Extra renders the counter families a role wires itself, the AI
	// router's among them. Per-process: these are in-memory counters this
	// process incremented, so a role that resolves a model path and leaves
	// this nil counts its own calls and tells nobody.
	Extra func(io.Writer)
	// JobStats renders the job-runtime section. Fleet-wide, and unlike
	// Extra it takes a context because it queries at scrape time — it is
	// handed THIS handler's deadline-bound ctx rather than the request's,
	// because an unbounded job read is what the 2s budget exists to stop.
	JobStats func(context.Context, io.Writer) error
	// Overlay renders the mirror sync-health section. Fleet-wide, and nil
	// for a role with no overlay surface wired.
	Overlay *OverlayMetrics
}

// Metrics serves the Prometheus text exposition format. The margince_*
// families are hand-rolled text — the format is a stable, trivially-emitted
// contract and the sections know their own domain — while the go_* and
// process_* runtime families come from client_golang's collectors, which
// measure things (CPU, RSS, GC pause, threads, descriptors) that no amount of
// hand-rolling reaches. The two halves share no family name, which is what
// lets them be concatenated into one exposition.
//
// A section that could not MEASURE writes nothing and returns nil, so the
// rest of the exposition still serves; the error return is reserved for a
// refused WRITE, which in practice means the scraper's connection is
// already gone. The handler logs it and stops rather than writing further
// sections into a socket that is not there.
func Metrics(in MetricsInput) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		out := &exposition{w: w}

		// Always first, and never injected: this section measures the
		// PROCESS answering the scrape, so it is the one part of the
		// exposition that means something different on every target and
		// cannot be assembled anywhere but here.
		writeRuntimeMetrics(r.Context(), out)

		writeOutboxBacklog(ctx, out, in.Backlog)
		writeRelayPublished(out, in.Published)
		writePoolGauges(out, in.Pool)

		if in.Extra != nil && !out.gone() {
			in.Extra(out)
		}
		if in.JobStats != nil && !out.gone() {
			// Its error is a refused write, by the same contract every section
			// here follows — a section that could not MEASURE writes nothing
			// and returns nil. Recorded on the exposition rather than acted on
			// here, so one place decides what a refused write means.
			if err := in.JobStats(ctx, out); err != nil && out.err == nil {
				out.err = err
			}
		}
		if in.Overlay != nil && !out.gone() {
			// The deadline-bound ctx, not the request's: this section queries
			// at scrape time like the job one above it, and the 2s budget
			// exists so a stalled read cannot hold the handler and its
			// database work open for as long as the client keeps the socket.
			writeOverlayMetrics(ctx, out, in.Overlay)
		}
		// Asked ONCE, about the whole exposition. A refused write means the
		// scraper is already gone, so this cannot be answered to the caller —
		// it is logged because a target that keeps failing to deliver its
		// scrape looks, from Prometheus' side, exactly like a target that is
		// down, and this line is the difference.
		if out.err != nil {
			slog.ErrorContext(r.Context(), "metrics: the exposition was not written in full; the scrape it belongs to is incomplete", "err", out.err)
		}
	}
}

// writeOutboxBacklog renders the committed-but-unshipped gauge.
//
// The backlog is a fleet-wide reading of a shared table, so a role that would
// only duplicate another target's copy of it passes nil rather than querying —
// the same "declared or absent" posture every section here takes, and the
// reason a FAILED read writes nothing: a gauge reporting rows it did not count
// reads as a drained outbox.
func writeOutboxBacklog(ctx context.Context, out *exposition, backlog func(context.Context) (int64, error)) {
	if backlog == nil || out.gone() {
		return
	}
	n, err := backlog(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "metrics: outbox backlog query failed", "err", err)
		return
	}
	out.printf("# HELP margince_outbox_unpublished Committed outbox rows the relay has not shipped yet.\n")
	out.printf("# TYPE margince_outbox_unpublished gauge\n")
	out.printf("margince_outbox_unpublished %d\n", n)
}

// writeRelayPublished renders this process's relay counter.
//
// Guarded on out.gone like the sections around it, and for the same rule
// rather than for its cost: printf goes quiet after a refusal, but Go
// evaluates the argument first, so the supplier would still run. This one is
// an atomic load and the pool read below is cheap too — the point is that
// "nothing is measured for a scrape that has gone" is either true of every
// supplier here or it is a claim a reader has to check one section at a time.
func writeRelayPublished(out *exposition, published func() uint64) {
	if published == nil || out.gone() {
		return
	}
	out.printf("# HELP margince_relay_published_total Outbox rows shipped to the bus since process start.\n")
	out.printf("# TYPE margince_relay_published_total counter\n")
	out.printf("margince_relay_published_total %d\n", published())
}

// writePoolGauges renders this process's own connection pool.
//
// Omitted rather than zeroed when no pool was injected: a gauge reporting
// connections it did not measure reads as an idle pool.
func writePoolGauges(out *exposition, pool *pgxpool.Pool) {
	if pool == nil || out.gone() {
		return
	}
	stat := pool.Stat()
	out.printf("# HELP margince_pgxpool_conns Connection pool state by class.\n")
	out.printf("# TYPE margince_pgxpool_conns gauge\n")
	out.printf("margince_pgxpool_conns{state=\"acquired\"} %d\n", stat.AcquiredConns())
	out.printf("margince_pgxpool_conns{state=\"idle\"} %d\n", stat.IdleConns())
	out.printf("margince_pgxpool_conns{state=\"total\"} %d\n", stat.TotalConns())
	out.printf("margince_pgxpool_conns{state=\"max\"} %d\n", stat.MaxConns())
}

// writeOverlayMetrics renders the overlay sync-health section — split
// out of Metrics so that function's own top-to-bottom read stays one
// section per line, not buried behind a nested nil-check.
func writeOverlayMetrics(ctx context.Context, out *exposition, overlay *OverlayMetrics) {
	if lag, err := overlay.SourceLag(ctx); err == nil {
		out.printf("# HELP margince_overlay_source_lag_seconds Seconds since the mirror's oldest last sync per object class (worst case across the fleet).\n")
		out.printf("# TYPE margince_overlay_source_lag_seconds gauge\n")
		for _, objectClass := range sortedKeys(lag) {
			out.printf("margince_overlay_source_lag_seconds{object_class=%s} %.0f\n", Label(objectClass), lag[objectClass].Seconds())
		}
	} else {
		slog.Error("metrics: overlay source-lag query failed", "err", err)
	}
	// The lag section may be what discovers the writer is gone. printf goes
	// quiet from here, but the three counter suppliers below would still be
	// called to build arguments for writes that go nowhere.
	if out.gone() {
		return
	}

	out.printf("# HELP margince_overlay_mirror_synced_total Mirror rows ingested (push+pull) since process start.\n")
	out.printf("# TYPE margince_overlay_mirror_synced_total counter\n")
	out.printf("margince_overlay_mirror_synced_total %d\n", overlay.SyncedTotal())

	out.printf("# HELP margince_overlay_mirror_conflict_total mirror.conflict events emitted (incumbent-wins divergence) since process start.\n")
	out.printf("# TYPE margince_overlay_mirror_conflict_total counter\n")
	out.printf("margince_overlay_mirror_conflict_total %d\n", overlay.ConflictTotal())

	out.printf("# HELP margince_overlay_mirror_deleted_total mirror.deleted events emitted (incumbent-deleted records purged from the mirror) since process start.\n")
	out.printf("# TYPE margince_overlay_mirror_deleted_total counter\n")
	out.printf("margince_overlay_mirror_deleted_total %d\n", overlay.DeletedTotal())
}

// sortedKeys answers lag's object-class keys in a stable order — a
// Prometheus scrape target's series order should not flap between
// scrapes for no reason, and map iteration order is not stable.
func sortedKeys(lag map[string]time.Duration) []string {
	keys := make([]string, 0, len(lag))
	for k := range lag {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
