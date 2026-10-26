// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// colleagueSenderDataTask re-checks every copy capture read as a colleague's
// sent mail before it required the colleague's login address to be at a
// trusted own domain. A stamp lets its colleague open the copy, so a copy that
// fails the rule is read as received mail again. The read grant waits for this
// task's step to finish (auth.SenderStampsCheckedStep).
func colleagueSenderDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792679687_colleague_sender_domains_rechecked", Version: 1,
		Title: "Re-checking mail read as a colleague's sent mail", Requires: []string{"core/1791798498"},
		Steps: []datatask.Step{{
			Name: auth.SenderStampsCheckedStep, Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateColleagueStamps, Batch: recheckColleagueSendersBatch,
		}},
	}
}

func recheckColleagueSendersBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	own, err := capture.TrustedOwnDomainsTx(ctx, tx)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	stamps, err := activities.ColleagueStampsTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(stamps))
	changed := 0
	for _, stamp := range stamps {
		scanned = append(scanned, stamp.ID)
		if own.Covers(stamp.Address) {
			continue
		}
		turned, err := activities.TurnBackColleagueCopyTx(ctx, tx, stamp)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if turned {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

func estimateColleagueStamps(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := activities.CountColleagueStamps(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
