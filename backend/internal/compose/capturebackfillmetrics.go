// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The mailbox imports as the fleet sees them, read from capture_backfill at
// scrape time beside the job gauges. Every api replica answers the same
// numbers, so a dashboard reads them with max, never sum. How fast an import
// moves is the worker's per-process families (capturemetrics); this is where
// it stands.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// writeBackfillFleet renders the two gauges. A status the column admits but no
// run holds is a measured zero, so every one is written.
func writeBackfillFleet(w io.Writer, fleet capture.BackfillFleet) error {
	if err := writeFamilyHeader(w, "margince_capture_backfill_runs",
		"Mailbox history imports per status, installation-wide."); err != nil {
		return err
	}
	var unlisted []string
	for status := range fleet.Runs {
		if !slices.Contains(capture.BackfillStatuses, status) {
			unlisted = append(unlisted, status)
		}
	}
	slices.Sort(unlisted)
	for _, status := range slices.Concat(capture.BackfillStatuses, unlisted) {
		if _, err := fmt.Fprintf(w, "margince_capture_backfill_runs{status=%s} %d\n",
			label(status), fleet.Runs[status]); err != nil {
			return err
		}
	}

	if err := writeFamilyHeader(w, "margince_capture_backfill_progress",
		"Counters summed over the queued and running imports: scanned, captured and skipped are each the committed count plus the running page's live tally; total_estimate is the preview's estimate, a floor where the preview said so."); err != nil {
		return err
	}
	for _, field := range []struct {
		name  string
		value int64
	}{
		{"scanned", fleet.Live.Scanned},
		{"captured", fleet.Live.Captured},
		{"skipped", fleet.Live.Skipped},
		{"total_estimate", fleet.Live.TotalEstimate},
	} {
		if _, err := fmt.Fprintf(w, "margince_capture_backfill_progress{field=%s} %d\n",
			label(field.name), field.value); err != nil {
			return err
		}
	}
	return nil
}

// backfillFleetSection binds the gauges to a reader. A failed read writes
// nothing, for the reason jobMetricsSection gives: a fabricated zero reads as
// no import running.
func backfillFleetSection(read func(context.Context) (capture.BackfillFleet, error)) func(context.Context, io.Writer) error {
	return func(ctx context.Context, w io.Writer) error {
		fleet, err := read(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "metrics: capture backfill read failed", "err", err)
			return nil
		}
		return writeBackfillFleet(w, fleet)
	}
}

// backfillFleetReader reads the runs through the installation's workspace
// binding, the store path every capture read takes.
func backfillFleetReader(pool *pgxpool.Pool) func(context.Context) (capture.BackfillFleet, error) {
	db := InstallationDB(pool)
	return func(ctx context.Context) (capture.BackfillFleet, error) { return capture.ReadBackfillFleet(ctx, db) }
}

// fleetSections renders each fleet-wide section in turn, stopping at the first
// refused write.
func fleetSections(sections ...func(context.Context, io.Writer) error) func(context.Context, io.Writer) error {
	return func(ctx context.Context, w io.Writer) error {
		for _, section := range sections {
			if err := section(ctx, w); err != nil {
				return err
			}
		}
		return nil
	}
}
