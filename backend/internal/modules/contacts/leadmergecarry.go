// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// carryLeadWorkToLead moves the merged-away lead's hand-offs, the factors a
// rep set by hand, and the earlier merges that point at it. Left behind, each
// hangs off an archived lead that no read of the survivor returns. Every row
// moved or dropped gets an audit row with what it held before.
func carryLeadWorkToLead(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.LeadID) error {
	if err := carryLeadManualSignals(ctx, tx, sourceID, targetID); err != nil {
		return err
	}
	args := pgx.NamedArgs{mergeSourceKey: sourceID, mergeTargetKey: targetID}
	for _, step := range []struct{ what, entity, column, stmt string }{
		{
			"hand-offs", tableSDRHandoff, "lead_id",
			`UPDATE sdr_handoff SET lead_id = @target WHERE lead_id = @source RETURNING id`,
		},
		// The redirect chain stays one hop deep, as the contact merge keeps it.
		{
			"earlier merges", entityLead, "merged_into_id",
			`UPDATE lead SET merged_into_id = @target WHERE merged_into_id = @source RETURNING id`,
		},
	} {
		rows, err := tx.Query(ctx, step.stmt, args)
		if err != nil {
			return fmt.Errorf("carry the merged-away lead's %s: %w", step.what, err)
		}
		moved, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return fmt.Errorf("carry the merged-away lead's %s: %w", step.what, err)
		}
		if err := storekit.AuditEach(ctx, tx, "update", step.entity, moved,
			map[string]any{step.column: sourceID}, map[string]any{step.column: targetID}); err != nil {
			return fmt.Errorf("audit the merged-away lead's %s: %w", step.what, err)
		}
	}
	return nil
}

// movedManualSignal is one hand-set factor a merge dropped or moved.
type movedManualSignal struct {
	Factor, Band, Reason string
	Points               int
}

// carryLeadManualSignals keeps one live factor per lead. The survivor's own
// answer stands, as every other merge collision is settled, and the retired
// copy goes. Each change is audited on its lead in the shape a rep's own edit
// takes.
func carryLeadManualSignals(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.LeadID) error {
	args := pgx.NamedArgs{mergeSourceKey: sourceID, mergeTargetKey: targetID}
	dropped, err := collectManualSignals(ctx, tx, `DELETE FROM lead_manual_signal a
		  WHERE a.lead_id = @source AND a.superseded_at IS NULL AND EXISTS (
		    SELECT 1 FROM lead_manual_signal b
		     WHERE b.lead_id = @target AND b.factor = a.factor AND b.superseded_at IS NULL)
		 RETURNING factor, band, reason, points`, args)
	if err != nil {
		return fmt.Errorf("drop the merged-away lead's colliding manual factors: %w", err)
	}
	moved, err := collectManualSignals(ctx, tx,
		`UPDATE lead_manual_signal SET lead_id = @target WHERE lead_id = @source RETURNING factor, band, reason, points`, args)
	if err != nil {
		return fmt.Errorf("carry the merged-away lead's manual factors: %w", err)
	}
	for _, m := range slices.Concat(dropped, moved) {
		image := map[string]any{auditKeyManualSignal: manualSignalImage(m.Factor, m.Band, m.Points, m.Reason)}
		if _, err := storekit.Audit(ctx, tx, "update", entityLead, sourceID.UUID, image, map[string]any{auditKeyManualSignal: nil}); err != nil {
			return fmt.Errorf("audit the merged-away lead's manual factors: %w", err)
		}
	}
	for _, m := range moved {
		image := map[string]any{auditKeyManualSignal: manualSignalImage(m.Factor, m.Band, m.Points, m.Reason)}
		if _, err := storekit.Audit(ctx, tx, "update", entityLead, targetID.UUID, map[string]any{auditKeyManualSignal: nil}, image); err != nil {
			return fmt.Errorf("audit the surviving lead's carried manual factors: %w", err)
		}
	}
	return nil
}

func collectManualSignals(ctx context.Context, tx pgx.Tx, stmt string, args pgx.NamedArgs) ([]movedManualSignal, error) {
	rows, err := tx.Query(ctx, stmt, args)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[movedManualSignal])
}
