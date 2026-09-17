// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The extractor's rule, answered for ONE conversation, for the member reading
// the pipeline trace.
//
// It composes the same named arms the queue composes (signalextractrule.go),
// so what a member is told and what the pass decided are the same sentence. The
// trace asks through an interface it declares and this implements, because the
// rule belongs here: compose/pipelinetrace explains rules and owns none.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/pipelinetrace"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ThreadReadings answers the trace's per-conversation question.
type ThreadReadings struct {
	db    *database.DB
	clock func() time.Time
}

// NewThreadReadings wires the reader. The clock is injected because two of the
// arms are about time — settled, and a park that expires — and a test that
// could not move it would have to sleep.
func NewThreadReadings(db *database.DB, clock func() time.Time) *ThreadReadings {
	return &ThreadReadings{db: db, clock: clock}
}

// ReadThread answers what the extractor's rule says about this conversation,
// and whether a reading has already happened over it.
func (r *ThreadReadings) ReadThread(
	ctx context.Context, threadKey, activityID string,
) (pipelinetrace.ThreadReading, error) {
	cited, err := ids.Parse(activityID)
	if err != nil {
		return pipelinetrace.ThreadReading{}, fmt.Errorf("thread reading: the citing message id: %w", err)
	}
	now := r.clock()
	var out pipelinetrace.ThreadReading
	err = r.db.Tx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, threadOfferQuery,
			now.Add(-extractSettleHours*time.Hour), threadOfferPassCap,
			extractRefusalCap, now.Add(-extractParkFor), threadKey)
		switch err := row.Scan(&out.ReachesOneAccount, &out.IsOneBodyOfWork,
			&out.IsFullyOpen, &out.HasANamedReader, &out.HasSettled,
			&out.IsParked, &out.HasMoved, &out.Scanned); {
		case err == pgx.ErrNoRows:
			// The message carries a thread key and the extractor sees no
			// conversation under it. Known stays false and the rung says so.
			return nil
		case err != nil:
			return fmt.Errorf("thread reading: the extractor's offer: %w", err)
		}
		out.Known = true
		out.Cited, err = signals.CitesActivity(ctx, tx, cited)
		return err
	})
	if err != nil {
		return pipelinetrace.ThreadReading{}, err
	}
	return out, nil
}
