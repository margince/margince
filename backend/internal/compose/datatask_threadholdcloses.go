// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// threadHoldClosesDataTask closes the messages a thread had already opened when
// its seat's verdict on the thread became a closing one. The verdict path now
// does this itself; the task repairs the rows it left open before.
//
// The page limit stays at or under the 500 rows one RecordOutcomeOnThreadTx
// call takes, so one call per seat covers that seat's rows on the page.
func threadHoldClosesDataTask(pool *pgxpool.Pool) datatask.Task {
	threads := capture.NewThreadVerdictStore(InstallationDB(pool))
	return datatask.Task{
		ID: "core/1794240000_held_threads_close_opened_messages", Version: 1,
		Title: "Closing messages left open in held threads",
		Steps: []datatask.Step{{
			Name: "close_opened_messages", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: capture.EstimateOpenUnderHeldThreads, Batch: closeHeldThreadsBatch(threads),
			EmptyWhenDone: true,
		}},
	}
}

func closeHeldThreadsBatch(threads *capture.ThreadVerdictStore) func(context.Context, pgx.Tx, datatask.BatchIn) (datatask.BatchOut, error) {
	return func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		if err := auth.RequireSystem(ctx); err != nil {
			return datatask.BatchOut{}, err
		}
		after, err := datatask.UUIDCursor(in.Cursor)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		page, seats, err := capture.OpenUnderHeldThreadsTx(ctx, tx, after, in.Limit)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		stamped, err := closeHeldThreadSeatsTx(ctx, tx, threads, seats)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if err := activities.RecomputeAudiencesTx(ctx, tx, asActivityIDs(stamped)); err != nil {
			return datatask.BatchOut{}, err
		}
		changed := 0
		for _, id := range page {
			if slices.Contains(stamped, id) {
				changed++
			}
		}
		return datatask.AdvanceUUID(in, page, changed)
	}
}

// closeHeldThreadSeatsTx applies each seat's own verdict to its thread, under
// the locks the verdict engine takes. The page only read rows, so it waits for
// every thread lock before it locks any ledger row.
func closeHeldThreadSeatsTx(
	ctx context.Context, tx pgx.Tx, threads *capture.ThreadVerdictStore, seats []capture.HeldThreadSeat,
) ([]ids.UUID, error) {
	keys := make([]string, 0, len(seats))
	for _, seat := range seats {
		keys = append(keys, seat.ThreadKey)
	}
	if err := storekit.WaitThreadKeysTx(ctx, tx, keys); err != nil {
		return nil, err
	}
	var stamped []ids.UUID
	for _, seat := range seats {
		if err := capture.LockThreadHoldTx(ctx, tx, seat.ThreadKey); err != nil {
			return nil, err
		}
		locked, ok, err := threads.LockSettledThreadTx(ctx, tx, seat.ThreadKey, seat.UserID)
		if err != nil {
			return nil, err
		}
		if !ok || !capture.ClosesOpenMessages(locked.Status) {
			continue
		}
		outcome, err := threads.RecordOutcomeOnThreadTx(
			ctx, tx, locked.ThreadKey, locked.UserID, locked.Status, locked.Kind, locked.Seen)
		if err != nil {
			return nil, err
		}
		stamped = append(stamped, outcome.Stamped...)
	}
	return stamped, nil
}
