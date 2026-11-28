// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What River's encoder writes into river_job.args, read back off a real
// Postgres. An installation holds one company, so no job names a workspace.
// The key is read off the encoded row because the decoded Go struct agrees
// with itself whatever the encoder wrote.

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestNoJobArgsShipAWorkspaceKey(t *testing.T) {
	e := Setup(t)
	ApplyRiverSchema(t)
	inserter, err := jobs.NewInserter(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("jobs.NewInserter: %v", err)
	}
	ctx := context.Background()

	batch := []river.JobArgs{
		compose.SendEmailArgs{DeliveryID: ids.NewV7().String()},
		compose.CaptureBackfillArgs{BackfillID: ids.NewV7().String()},
		compose.TelegramIngestArgs{
			ConnectionID: ids.NewV7().String(),
			BotID:        "bot", RawCaptureID: ids.NewV7().String(),
		},
		compose.TelegramPollArgs{ConnectionID: ids.NewV7().String()},
		compose.VoiceBuildArgs{
			ProfileID: ids.NewV7().String(),
			BuildID:   ids.NewV7().String(), RequestedBy: "human:" + e.Rep1.String(),
		},
		compose.CaptureSyncArgs{ConnectionID: ids.NewV7().String(), Provider: "gmail"},
		compose.TelegramPollSweepArgs{},
	}

	// The high-water mark fences the read to rows this test inserted, so the
	// assertion does not depend on the table arriving empty.
	var highWater int64
	if err := e.Pool.QueryRow(ctx, `SELECT coalesce(max(id), 0) FROM river_job`).Scan(&highWater); err != nil {
		t.Fatalf("reading the river_job high-water mark: %v", err)
	}
	for _, args := range batch {
		if err := inserter.Enqueue(ctx, args, nil); err != nil {
			t.Fatalf("enqueueing %s: %v", args.Kind(), err)
		}
	}

	rows, err := e.Pool.Query(ctx, `SELECT kind, args ? 'workspace_id' FROM river_job WHERE id > $1`, highWater)
	if err != nil {
		t.Fatalf("reading back the enqueued jobs: %v", err)
	}
	type wireRow struct {
		Kind   string
		HasKey bool
	}
	wire, err := pgx.CollectRows(rows, pgx.RowToStructByPos[wireRow])
	if err != nil {
		t.Fatalf("collecting the enqueued jobs: %v", err)
	}
	if len(wire) != len(batch) {
		t.Fatalf("river_job holds %d of this batch's rows, want %d: an insert that never landed would make the assertion vacuous", len(wire), len(batch))
	}
	for _, row := range wire {
		if row.HasKey {
			t.Errorf("%s: args carry a workspace_id key; no job names a workspace in an installation of one company", row.Kind)
		}
	}
}
