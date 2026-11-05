// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// errBackfillEnqueue marks a start the queue refused. It is its own answer
// because nothing was written, so the caller can say "try again".
var errBackfillEnqueue = errors.New("backfill: the paging job could not be scheduled")

// begin opens a past-email import and schedules its pager: the one engine
// behind both the start endpoint and the import a connect asked for. The
// estimate is the caller's count, stored as the run's progress denominator.
func (h backfillHandlers) begin(ctx context.Context, ws ids.UUID, provider string, userID ids.UserID, months int, estimate connector.BackfillEstimate, startOver bool) (capture.BackfillRun, error) {
	// Continuing a run that stopped on an error is the default: starting over
	// re-reads hours of provider pages for mail already captured.
	start := h.registry.StartBackfill
	if startOver {
		start = h.registry.StartBackfillOver
	}
	// Kept apart from the run's own error so a refused queue keeps its own
	// answer instead of the generic connector-fault mapping.
	var enqueueErr error
	run, err := start(ctx, provider, userID, months, estimate,
		func(ctx context.Context, tx pgx.Tx, backfillID ids.UUID) error {
			enqueueErr = h.inserter.EnqueueTx(ctx, tx, CaptureBackfillArgs{
				Workspace: ws, BackfillID: backfillID.String(),
			}, &river.InsertOpts{
				// One attempt: the backfill row owns the outcome (api/jobs.yaml), and
				// a River retry would re-page a run the engine already ended.
				MaxAttempts: rowOwnedMaxAttempts,
				UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
			})
			return enqueueErr
		})
	if enqueueErr != nil {
		return capture.BackfillRun{}, fmt.Errorf("%w: %w", errBackfillEnqueue, enqueueErr)
	}
	return run, err
}

// importOnConnect is the callback's view of this engine: the same registry's
// count and the same begin the start endpoint calls.
func (h backfillHandlers) importOnConnect() connectImport {
	if h.registry == nil || h.inserter == nil {
		return connectImport{}
	}
	return connectImport{estimate: h.registry.EstimateBackfill, begin: h.begin}
}
