// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package transcriptaudience runs the sweep that keeps each meeting
// transcript readable by the readers of its meeting and no one else.
package transcriptaudience

import (
	"cmp"
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sweepBatch bounds one transaction of a workspace's pass.
const sweepBatch = 200

// Sweep gives each meeting transcript the readers its meeting has now. A guest
// added to or taken off the meeting, by any writer, reaches the transcript here.
type Sweep struct {
	Pool *pgxpool.Pool
	// Batch is the rows one transaction takes. Zero is sweepBatch.
	Batch int
}

// Workspace runs one workspace's pass, as the system. It pages through every
// drifted transcript, so a guest taken off a meeting loses its transcript
// within one cadence however many drifted. The job's timeout bounds the pass.
func (s Sweep) Workspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.SystemActing(ctx, "system:transcript-audience")
	var after ids.ActivityID
	for {
		var page activities.DriftedPage
		if err := database.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
			var err error
			page, err = activities.FollowDriftedTranscriptsTx(ctx, tx, after, s.batch())
			return err
		}); err != nil {
			return err
		}
		if page.Next.IsZero() {
			return nil
		}
		after = page.Next
	}
}

func (s Sweep) batch() int { return cmp.Or(s.Batch, sweepBatch) }
