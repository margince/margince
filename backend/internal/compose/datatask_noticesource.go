// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	dataTaskCapArg    = "cap"
	dataTaskAfterArg  = "after"
	dataTaskStatesArg = "states"
)

func noticeSourceDataTask(now func() time.Time) datatask.Task {
	return datatask.Task{
		ID: "core/1793514621_notice_source_assessment", Version: 1,
		Title: "Reviewing unverified notice sources", Requires: []string{"core/1793514621"},
		Steps: []datatask.Step{{
			Name: "review_unverified_sources", Limit: 200, MinLimit: 10, EmptyWhenDone: true,
			Audit: datatask.AuditPolicy{PerRecord: true}, Estimate: estimateNoticeSources,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return reviewNoticeSourcesBatch(ctx, tx, in, now())
			},
		}},
	}
}

func noticeSourceScopeSQL() string {
	return ` FROM privacy_notice_case n
    JOIN contact_acquisition_evidence e ON e.id = n.acquisition_id
    WHERE n.state = ANY(@states)
    AND n.state <> 'source_review'
    AND (` + consent.UnverifiedNoticeSourceSQL("n", "e") + `)`
}

func estimateNoticeSources(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT n.id`+noticeSourceScopeSQL()+` LIMIT @cap) candidates`,
		pgx.NamedArgs{dataTaskStatesArg: consent.SettleableNoticeStates(), dataTaskCapArg: maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}

func reviewNoticeSourcesBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn, now time.Time) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT n.id`+noticeSourceScopeSQL()+` AND n.id > @after ORDER BY n.id LIMIT @cap`,
		pgx.NamedArgs{dataTaskStatesArg: consent.SettleableNoticeStates(), dataTaskAfterArg: after, dataTaskCapArg: in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	cases, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	changed := 0
	for _, id := range cases {
		moved, err := consent.ReviewUnverifiedNoticeTx(ctx, tx, id, now)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if moved {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, cases, changed)
}
