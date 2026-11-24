// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// foldSeriesSplitsTx folds each meeting a split series replaced at meeting's
// slot on seat's calendar onto the split series' meeting. Calendar capture and
// the repair task share it. It answers how many it folded.
func foldSeriesSplitsTx(
	ctx context.Context, tx pgx.Tx, meetings *activities.Store, meeting ids.ActivityID, seat ids.UUID,
) (int, error) {
	splits, err := activities.SeriesSplitsTx(ctx, tx, meeting, seat)
	if err != nil {
		return 0, err
	}
	folded := 0
	for _, split := range splits {
		done, err := foldSeriesSplitTx(ctx, tx, meetings, split, seat)
		if err != nil {
			return folded, err
		}
		if done {
			folded++
		}
	}
	return folded, nil
}

// foldSeriesSplitTx locks the pair and asks the rule again before it folds,
// because the pair was found before the lock. A pair that no longer holds, or
// whose replaced row does not retire cleanly, stays.
func foldSeriesSplitTx(
	ctx context.Context, tx pgx.Tx, meetings *activities.Store, split activities.SeriesSplit, seat ids.UUID,
) (bool, error) {
	live, err := lockFoldPair(ctx, tx, split.Replaced, split.Survivor)
	if err != nil || !live {
		return false, err
	}
	stands, err := activities.SeriesSplitStandsTx(ctx, tx, split, seat)
	if err != nil || !stands {
		return false, err
	}
	folded, err := foldMeetingDuplicateTx(ctx, tx, meetings, split.Replaced, split.Survivor, seat, "")
	if err == nil && !folded {
		slog.WarnContext(ctx, "compose: a meeting split into a new series stays stored twice because the old row does not retire cleanly",
			"old", split.Replaced.String(), "new", split.Survivor.String())
	}
	return folded, err
}

// foldCapturedSeriesSplits is the sink's capture.SeriesSplitFolder.
func foldCapturedSeriesSplits(meetings *activities.Store) capture.SeriesSplitFolder {
	return func(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, seat ids.UUID) error {
		_, err := foldSeriesSplitsTx(ctx, tx, meetings, meeting, seat)
		return err
	}
}
