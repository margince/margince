// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// Refusals park only this reading, so a model refusal never replays the whole job.
func (x *SignalExtractor) deferRefusedReading(
	ctx context.Context, thread settledThread, now time.Time, refusal error,
) error {
	var refusals int
	if markErr := withRetryingWorkspaceTx(ctx, x.pool, func(tx pgx.Tx) error {
		refusals = 0

		if err := storekit.WaitThreadKeysTx(ctx, tx, []string{thread.Key}); err != nil {
			return err
		}
		counted, countErr := recordThreadRefusal(ctx, tx, thread, now)
		refusals = counted
		return countErr
	}); markErr != nil {
		return markErr
	}
	if refusals >= extractRefusalCap {
		x.log.WarnContext(ctx, "signal extract: parking a conversation nothing could read",
			"thread_key", thread.Key, "refusals", refusals,
			"parked_for", extractParkFor.String(), "error", clampToken(refusal.Error()))
		return nil
	}
	x.log.InfoContext(ctx, "signal extract: refusing the model's reading, will try again",
		"thread_key", thread.Key, "refusals", refusals,
		"of", extractRefusalCap, "error", clampToken(refusal.Error()))
	return nil
}
