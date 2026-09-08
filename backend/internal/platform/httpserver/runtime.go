// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

// The runtime section of /metrics: what the SCRAPED PROCESS is doing, as
// opposed to what the installation is doing.
//
// Every other section of the exposition reads shared state, so a wedged replica
// is arithmetically invisible in it — the fleet-wide numbers are the same
// whichever process served the scrape. This section is the one that differs per
// target, and it is why the worker serves a listener at all.
//
// It used to be four numbers read by hand out of runtime.MemStats, under a
// margince_process_* prefix whose stated reason was to avoid colliding with
// client_golang's go_* collector "if client_golang is ever adopted". It has
// been, here, so the hedge would now BE the collision: two spellings of one
// number, and — because both halves call runtime.ReadMemStats — two
// stop-the-world pauses per scrape per replica to produce them. The four
// families are gone; go_goroutines, go_memstats_heap_alloc_bytes,
// go_memstats_heap_sys_bytes and go_gc_duration_seconds are their successors,
// and the collectors bring CPU, RSS, threads, descriptors, network and process
// start time besides, none of which a hand-rolled section ever reached.

import (
	"context"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/common/expfmt"
)

// runtimeRegistry is this process's collector set, and it is deliberately
// PRIVATE rather than prometheus.DefaultRegisterer.
//
// The default registerer is process-global mutable state: a second Register of
// the same collector panics, which under `go test` means one package's handler
// construction can fail another's, and a test binary that builds two handlers
// fails on the second. A registry owned by this package registers exactly once,
// at init, and cannot be reached to have anything else added to it.
var runtimeRegistry = newRuntimeRegistry()

func newRuntimeRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	// MustRegister, not Register: a duplicate here is a programming error in
	// this file — the registry is private and nothing else can write to it —
	// and a process whose runtime section is silently missing looks exactly
	// like a healthy one to everything downstream.
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return reg
}

// writeRuntimeMetrics renders the collector families into the exposition.
//
// The encoder is pinned to the text format rather than negotiated from the
// request's Accept header, which is why promhttp's handler is not used here:
// it would negotiate, a Prometheus scraper asks for OpenMetrics first, and the
// two halves of this response would then be in two different formats — one
// exposition that parses as neither.
//
// A gather failure writes NOTHING and logs, the same posture every measuring
// section takes: a partial runtime section is a process misreporting its own
// health, which is worse than a target that has no runtime section at all.
func writeRuntimeMetrics(ctx context.Context, out *exposition) {
	if out.gone() {
		return
	}
	families, err := runtimeRegistry.Gather()
	if err != nil {
		slog.ErrorContext(ctx, "metrics: runtime collectors could not be gathered", "err", err)
		return
	}
	enc := expfmt.NewEncoder(out, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, family := range families {
		// Encode's error is either the exposition's own refused write, which
		// the writer has already remembered and which every later write
		// no-ops on, or a malformed family, which cannot arise from a
		// registry this file is the only writer of. Either way the loop stops
		// here rather than assembling into a socket that is gone.
		if err := enc.Encode(family); err != nil {
			return
		}
	}
}
