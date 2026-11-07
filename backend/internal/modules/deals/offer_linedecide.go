// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AcceptOfferLineItem flips a staged (AI-proposed) line to accepted, at
// which point it starts counting toward the offer totals (E03.21a). The
// contract does not expose proposal_state yet, so this is the store seam
// the future drafting surface confirms through; accepting an already
// accepted line is a no-op because the desired end-state already holds.
func (s *Store) AcceptOfferLineItem(ctx context.Context, offerID ids.OfferID, lineID ids.UUID) (crmcontracts.Offer, error) {
	if err := auth.Require(ctx, "offer", principal.ActionUpdate); err != nil {
		return crmcontracts.Offer{}, err
	}
	var out crmcontracts.Offer
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		current, _, err := visibleOfferLocked(ctx, tx, offerID, storekit.LiveOnly)
		if err != nil {
			return err
		}
		if err := ensureTypedDraft(current); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`UPDATE offer_line_item SET proposal_state = $3 WHERE id = $1 AND offer_id = $2 AND proposal_state = $4`,
			lineID, offerID, ProposalAccepted, ProposalStaged)
		if err != nil {
			return fmt.Errorf("accept offer line: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// Missing line is 404; an existing accepted one is idempotent.
			var exists bool
			if err := tx.QueryRow(ctx,
				`SELECT EXISTS (SELECT 1 FROM offer_line_item WHERE id = $1 AND offer_id = $2)`,
				lineID, offerID).Scan(&exists); err != nil {
				return fmt.Errorf("probe offer line: %w", err)
			}
			if !exists {
				return apperrors.ErrNotFound
			}
			out, err = readOfferWithLines(ctx, tx, offerID, storekit.LiveOnly)
			return err
		}
		totals, err := recomputeOfferTotals(ctx, tx, offerID)
		if err != nil {
			return err
		}
		// Acceptance is what makes the line count, so the offer's money moves
		// here; the state the line left behind is context about the write.
		auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "offer", offerID.UUID,
			offerTotalsImage(totals.Before), offerTotalsImage(totals.After),
			lineFieldEvidence(lineID,
				map[string]any{"proposal_state": ProposalStaged},
				map[string]any{"proposal_state": ProposalAccepted}))
		if err != nil {
			return fmt.Errorf("audit line accept: %w", err)
		}
		if err := announceOfferEdit(ctx, tx, auditID, current, lineEditFields(totals.Before, totals.After)); err != nil {
			return err
		}
		if out, err = readOfferWithLines(ctx, tx, offerID, storekit.LiveOnly); err != nil {
			return fmt.Errorf("read offer after line accept: %w", err)
		}
		return nil
	})
	return out, err
}

func (s *Store) RemoveOfferLineItem(ctx context.Context, offerID ids.OfferID, lineID ids.UUID) (crmcontracts.Offer, error) {
	if err := auth.Require(ctx, "offer", principal.ActionUpdate); err != nil {
		return crmcontracts.Offer{}, err
	}
	var out crmcontracts.Offer
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		current, _, err := visibleOfferLocked(ctx, tx, offerID, storekit.LiveOnly)
		if err != nil {
			return err
		}
		if err := ensureTypedDraft(current); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM offer_line_item WHERE id = $1 AND offer_id = $2`, lineID, offerID)
		if err != nil {
			return fmt.Errorf("delete offer line: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.ErrNotFound
		}
		totals, err := recomputeOfferTotals(ctx, tx, offerID)
		if err != nil {
			return err
		}
		// Which line went is context about the write; what the removal did to
		// the offer is the three money columns.
		auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "offer", offerID.UUID,
			offerTotalsImage(totals.Before), offerTotalsImage(totals.After),
			map[string]any{lineIDKey: lineID, "line_removed": true})
		if err != nil {
			return fmt.Errorf("audit line remove: %w", err)
		}
		if err := announceOfferEdit(ctx, tx, auditID, current, lineEditFields(totals.Before, totals.After)); err != nil {
			return err
		}
		if out, err = readOfferWithLines(ctx, tx, offerID, storekit.LiveOnly); err != nil {
			return fmt.Errorf("read offer after line remove: %w", err)
		}
		return nil
	})
	return out, err
}
