// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RedactMarkedWeeklyAuditPageTx uses the approved editable erasure marker.
// Only an already-cleared item qualifies; later working text disqualifies it.
func RedactMarkedWeeklyAuditPageTx(ctx context.Context, tx pgx.Tx, after, until ids.UUID, limit int, _ CountryReader) (HistoricalAuditPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return HistoricalAuditPage{}, err
	}
	held, err := historicalAuditUnderHold(ctx, tx, ids.UUID{}, "weekly_plan_commitment", ids.UUID{}, FloorPolicy{})
	if err != nil {
		return HistoricalAuditPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id,label=@marker AND linked_record_type IS NULL AND linked_record_id IS NULL
 AND help_requested='' AND manager_response='' AND manager_user_id IS NULL AND responded_at IS NULL
 FROM weekly_plan_commitment WHERE id>@after AND id<@until ORDER BY id LIMIT @limit FOR UPDATE`, pgx.StrictNamedArgs{"marker": erasedMention, "after": after, "until": until, auditPageLimitParameter: limit})
	if err != nil {
		return HistoricalAuditPage{}, err
	}
	type item struct {
		id      ids.UUID
		cleared bool
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (item, error) {
		var i item
		err := row.Scan(&i.id, &i.cleared)
		return i, err
	})
	if err != nil {
		return HistoricalAuditPage{}, err
	}
	page := HistoricalAuditPage{}
	for _, i := range items {
		page.Scanned = append(page.Scanned, i.id)
		if !i.cleared {
			continue
		}
		if held {
			continue
		}
		changed, err := redactMarkedWeeklyRecord(ctx, tx, i.id)
		if err != nil {
			return HistoricalAuditPage{}, err
		}
		if changed {
			page.Changed++
		}
	}
	return page, nil
}

func redactMarkedWeeklyRecord(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	var needed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_log history
 WHERE history.entity_type='weekly_plan_commitment' AND history.entity_id=@id AND `+storekit.AuditRedactionNeededSQL+`)
 OR EXISTS(SELECT 1 FROM event_outbox WHERE envelope->'entity'->>'type'='weekly_plan_commitment'
 AND envelope->'entity'->>'id'=@id::uuid::text)`, pgx.StrictNamedArgs{"id": id}).Scan(&needed); err != nil {
		return false, err
	}
	if !needed {
		return false, nil
	}
	proof, err := storekit.AuditWithEvidence(ctx, tx, actionErase, "weekly_plan_commitment", id, nil, nil, map[string]any{
		evidenceKeyCause: subjectRedactionCause, evidenceKeyReason: subjectRedactionReason, "policy": "weekly_marker_cleanup",
	})
	if err != nil {
		return false, err
	}
	return true, storekit.RedactAuditHistory(ctx, tx, proof)
}
