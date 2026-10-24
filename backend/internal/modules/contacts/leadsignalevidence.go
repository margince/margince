// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ManualSignalNoEvidence is what a stored manual signal holds once its stock
// sentence has been taken away. The screen shows it as "No evidence recorded".
// SetLeadManualSignal refuses it, so only the repair below can write it.
const ManualSignalNoEvidence = "No evidence recorded"

// stockManualSignalReasons are the sentences the lead form once invented when
// a rep typed no evidence, one per shipped locale. They are frozen here: a
// released data task must not change when a locale file later does.
var stockManualSignalReasons = []string{
	"No evidence given. Entered manually.",
	"Kein Beleg angegeben. Manuell erfasst.",
	"Không nêu bằng chứng. Nhập thủ công.",
}

// StockManualSignalReasons returns a copy of the frozen list.
func StockManualSignalReasons() []string {
	return append([]string(nil), stockManualSignalReasons...)
}

// unevidencedSignal is one stored row the repair rewrites, with the audit
// image of the row before and after.
type unevidencedSignal struct {
	id            ids.UUID
	before, after map[string]any
}

// MarkUnevidencedManualSignalsTx replaces the stock sentence on one page of
// signals with ManualSignalNoEvidence and audits it once per lead. Band,
// points, kind, confidence, set_by and set_at stay as they were, so the score
// does not move and no recompute runs. Superseded rows are included because
// the history list shows them. A row a human later re-saved with real text no
// longer matches, and a second run finds nothing.
func MarkUnevidencedManualSignalsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (scanned []ids.UUID, changed int, err error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return nil, 0, err
	}
	scanned, hits, err := readUnevidencedPage(ctx, tx, after, limit)
	if err != nil {
		return nil, 0, err
	}
	before := map[ids.LeadID][]map[string]any{}
	marked := map[ids.LeadID][]map[string]any{}
	var order []ids.LeadID
	for _, h := range hits {
		// Only a row still holding the stock sentence is rewritten.
		var lead ids.LeadID
		err := tx.QueryRow(ctx,
			`UPDATE lead_manual_signal SET reason = $2 WHERE id = $1 AND reason = $3 RETURNING lead_id`,
			h.id, ManualSignalNoEvidence, h.before[fieldKeyReason]).Scan(&lead)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, 0, fmt.Errorf("contacts: marking a manual signal without evidence: %w", err)
		}
		changed++
		if _, seen := marked[lead]; !seen {
			order = append(order, lead)
		}
		before[lead] = append(before[lead], h.before)
		marked[lead] = append(marked[lead], h.after)
	}
	for _, lead := range order {
		auditID, err := storekit.Audit(ctx, tx, "update", "lead", lead.UUID,
			map[string]any{auditKeyManualSignals: before[lead]},
			map[string]any{auditKeyManualSignals: marked[lead]})
		if err != nil {
			return nil, 0, err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, lead.UUID, crmcontracts.PublicEventLeadUpdated{
			ChangedFields: map[string]any{eventKeyDelta: map[string]any{auditKeyManualSignals: ManualSignalNoEvidence}},
		}); err != nil {
			return nil, 0, err
		}
	}
	return scanned, changed, nil
}

func readUnevidencedPage(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, []unevidencedSignal, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, factor, band, points, reason FROM lead_manual_signal
		  WHERE id > $1 ORDER BY id LIMIT $2 FOR UPDATE`, after, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("contacts: reading manual signals to repair: %w", err)
	}
	defer rows.Close()
	var scanned []ids.UUID
	var hits []unevidencedSignal
	for rows.Next() {
		var s unevidencedSignal
		var factor, band, reason string
		var points int
		if err := rows.Scan(&s.id, &factor, &band, &points, &reason); err != nil {
			return nil, nil, fmt.Errorf("contacts: reading a manual signal to repair: %w", err)
		}
		scanned = append(scanned, s.id)
		if isStockManualSignalReason(reason) {
			s.before = manualSignalImage(factor, band, points, reason)
			s.after = manualSignalImage(factor, band, points, ManualSignalNoEvidence)
			hits = append(hits, s)
		}
	}
	return scanned, hits, rows.Err()
}

func isStockManualSignalReason(reason string) bool {
	return slices.Contains(stockManualSignalReasons, reason)
}

// CountUnevidencedManualSignals counts, up to limit+1, the signals the repair
// will rewrite.
func CountUnevidencedManualSignals(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM lead_manual_signal WHERE reason = ANY($1) LIMIT $2) stale`,
		stockManualSignalReasons, limit+1).Scan(&count)
	return count, err
}
