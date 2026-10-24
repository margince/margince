// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How far behind the confidentiality pass is, read from the thread ledger at
// scrape time beside the job gauges. The job gauges cannot show it. A pass that
// runs on time and judges its cap each run looks healthy there while mail piles
// up behind it. Every api replica answers the same numbers, so read them with
// max, never sum.

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// writeThreadBacklog renders the two gauges. Nothing waiting is a measured
// zero, not an absent series, so an alert on the age never reads a gap as
// healthy.
func writeThreadBacklog(w io.Writer, backlog capture.ThreadBacklog) error {
	if err := writeFamilyHeader(w, "margince_capture_confidentiality_waiting_threads",
		"Held mail threads the confidentiality pass has not answered yet."); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "margince_capture_confidentiality_waiting_threads %d\n", backlog.Waiting); err != nil {
		return err
	}
	if err := writeFamilyHeader(w, "margince_capture_confidentiality_oldest_wait_seconds",
		"How long the longest-held unanswered thread has been invisible to the team, counted from the capture of the message that opened it; 0 when none waits."); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "margince_capture_confidentiality_oldest_wait_seconds %.0f\n", backlog.OldestWait.Seconds())
	return err
}

// threadBacklogSection binds the gauges to a reader. A failed read writes
// nothing, for the reason jobMetricsSection gives: a fabricated zero reads as
// a pass that has caught up.
func threadBacklogSection(read func(context.Context) (capture.ThreadBacklog, error)) func(context.Context, io.Writer) error {
	return func(ctx context.Context, w io.Writer) error {
		backlog, err := read(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "metrics: confidentiality backlog read failed", "err", err)
			return nil
		}
		return writeThreadBacklog(w, backlog)
	}
}

// threadBacklogReader reads the backlog through the installation's workspace
// binding, the store path every capture read takes.
func threadBacklogReader(pool *pgxpool.Pool) func(context.Context) (capture.ThreadBacklog, error) {
	db := InstallationDB(pool)
	return func(ctx context.Context) (capture.ThreadBacklog, error) {
		return capture.ReadThreadBacklog(ctx, db)
	}
}
