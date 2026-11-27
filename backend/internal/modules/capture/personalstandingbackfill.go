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
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// settledPrivateVerdictSQL selects a seat's latest settled `personal` verdict
// that a lasting decision may rest on (verdictMayStandSQL). Restoring a standing
// after a withdrawn correction reads the same rows as the backfill.
var settledPrivateVerdictSQL = `
	FROM capture_pending_counterparty p
	JOIN activity a ON a.id = p.activity_id
	WHERE a.restricted_at IS NULL AND ` + verdictMayStandSQL("a", "p.email") + ` AND p.status = '` + PendingStatusNoise + `' AND p.kind = '` + KindPersonal + `'
	  AND p.owner_id IS NOT NULL AND ` + latestResolvedSenderVerdict("p")

// personalWithoutStandingSQL adds the seats with no decision of any kind.
var personalWithoutStandingSQL = settledPrivateVerdictSQL + `
	  AND NOT EXISTS (SELECT 1 FROM capture_sender_override o
	                   WHERE o.user_id = p.owner_id AND o.address = p.email)`

// BackfillPersonalStandingsTx records the standing decision for one page of
// `personal` verdicts reached before the verdict recorded it. It pages over the
// ledger row id and reports every row it inspected, so the cursor advances past
// a row whose seat already decided.
func BackfillPersonalStandingsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (scanned []ids.UUID, written int, err error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT p.id, p.owner_id, p.email `+personalWithoutStandingSQL+`
		AND p.id > $1 ORDER BY p.id LIMIT $2`, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("capture: reading personal verdicts without a standing decision: %w", err)
	}
	type verdict struct {
		id, owner ids.UUID
		email     string
	}
	var page []verdict
	for rows.Next() {
		var v verdict
		if err := rows.Scan(&v.id, &v.owner, &v.email); err != nil {
			rows.Close()
			return nil, 0, fmt.Errorf("capture: reading a personal verdict: %w", err)
		}
		page = append(page, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for _, v := range page {
		scanned = append(scanned, v.id)
		if err := RecordPersonalStandingTx(ctx, tx, v.owner, v.email); err != nil {
			return nil, 0, err
		}
		written++
	}
	return scanned, written, nil
}

// CountPersonalWithoutStanding bounds the backfill's estimate.
func CountPersonalWithoutStanding(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+personalWithoutStandingSQL+` LIMIT $1) unmarked`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("capture: counting personal verdicts without a standing decision: %w", err)
	}
	return count, nil
}

// PersonalStandingDataTask records the standing decision for every `personal`
// verdict reached before verdicts recorded one. Those senders then stay
// withdrawn when the seat writes to them again.
func PersonalStandingDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795688930_private_senders_stand", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1795688930_private_senders_stand"), Requires: []string{"core/1795766214"},
		Steps: []datatask.Step{{
			Name: "private_senders", Limit: 100, MinLimit: 5,
			Audit: datatask.AuditPolicy{PerRecord: true},
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				count, err := CountPersonalWithoutStanding(ctx, q, maximum)
				return min(count, maximum), count > maximum, err
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				scanned, written, err := BackfillPersonalStandingsTx(ctx, tx, after, in.Limit)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				return datatask.AdvanceUUID(in, scanned, written)
			},
		}},
	}
}
