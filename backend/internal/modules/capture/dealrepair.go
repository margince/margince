// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unfiledCapturedMailSQL selects the mail the deal repair inspects: live mail
// a connector captured, on no deal. A connector's captured_by is the recorded
// provenance that a machine filed the mail; mail a human logged or filed on a
// deal is never touched. Callers append the cursor and order.
const unfiledCapturedMailSQL = `
	SELECT a.id FROM activity a
	 WHERE a.kind = 'email' AND a.channel_provider IS NULL
	   AND a.captured_by LIKE 'connector:%'
	   AND a.archived_at IS NULL AND a.restricted_at IS NULL
	   AND NOT EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND l.entity_type = 'deal')`

// RepairDealLinksTx files mail captured before the deal rule under the deal
// the rule finds for it now, through AttributeDealTx, so a repaired link is
// exactly the link capture would write today. It asks nobody: old mail does
// not become a new question. A second run changes nothing, because a filed
// mail leaves the selection and an unfiled one decides the same way.
func RepairDealLinksTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn, candidates DealCandidates) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, unfiledCapturedMailSQL+` AND a.id > $1 ORDER BY a.id LIMIT $2`, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("capture: listing mail for the deal repair: %w", err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("capture: listing mail for the deal repair: %w", err)
	}
	linked := 0
	for _, id := range scanned {
		verdict, err := AttributeDealTx(ctx, tx, ids.From[ids.ActivityKind](id), candidates, nil)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if verdict.Outcome == DealLinked {
			linked++
		}
	}
	return datatask.AdvanceUUID(in, scanned, linked)
}

// EstimateDealRepair is the preview: how many captured mails the repair will
// inspect, at most maximum. Each is linked only where the rule finds one deal.
func EstimateDealRepair(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (`+unfiledCapturedMailSQL+` LIMIT $1) AS candidates`, maximum+1).Scan(&count)
	return min(count, maximum), count > maximum, err
}
