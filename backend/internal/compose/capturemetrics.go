// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The capture pipeline's counter family: what this process decided about the
// messages it captured.
//
// Until now /metrics said nothing at all about capture — an operator could see
// the outbox backlog and the job queue, but not that every message from a
// mailbox had been dropped as internal since somebody registered a domain.

import (
	"io"
	"log/slog"
	"sort"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// writeCaptureMetrics renders one counter per traced outcome.
//
// The family is declared even before the first trace, so a scrape can tell a
// process that renders it from one that does not; no zero sample is written,
// because a zero would claim a pipeline ran and decided nothing.
//
// Sorted, because Prometheus does not care but a human reading a scrape by hand
// does, and an unordered map would reshuffle the block on every request.
func writeCaptureMetrics(w io.Writer, totals map[string]uint64) {
	httpserver.WriteLine(w, "# HELP margince_capture_outcomes_total What the capture pipeline decided about each message, since process start.\n")
	httpserver.WriteLine(w, "# TYPE margince_capture_outcomes_total counter\n")
	outcomes := make([]string, 0, len(totals))
	for outcome := range totals {
		outcomes = append(outcomes, outcome)
	}
	sort.Strings(outcomes)
	for _, outcome := range outcomes {
		httpserver.WriteLine(w, "margince_capture_outcomes_total{outcome=%s} %d\n", httpserver.Label(outcome), totals[outcome])
	}
}

// WriteCaptureProcessMetrics renders this process's capture counters: what it
// decided about the messages it captured, and the provider calls and mailbox
// imports it ran. Both roles render it, because both run capture.
func WriteCaptureProcessMetrics(w io.Writer) {
	writeCaptureMetrics(w, capture.TraceOutcomeTotals())
	capturemetrics.WriteProcessMetrics(w)
}

// writeCaptureSection is the fan-out's entry point.
func (Server) writeCaptureSection(w io.Writer) { WriteCaptureProcessMetrics(w) }

// rateLimitAttr is the reason and HTTP status of the rate limit err carries,
// for the capture lanes' WARN lines; empty, and so omitted, for any other fault.
func rateLimitAttr(err error) slog.Attr { return connector.RateLimitLogAttr(err) }
