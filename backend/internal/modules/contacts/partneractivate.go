// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Activating a partner: the stamp that lets it stand on a stage meaning active.
// An accepted partner lead and a won deal the partner sourced both reach it.
// Both go through activatePartnerTx, the one writer of the stamp.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The bases a door stamps, as partner_activation_basis_check names them.
const (
	// eventKeyPartner is the company.updated delta key a partner change rides.
	eventKeyPartner        = "partner"
	activationLeadAccepted = "lead_accepted"
	activationCustomerWon  = "customer_won"
)

// partnerActivationEvidence is what activated a partner: the basis, who, and
// the lead or deal it came through.
type partnerActivationEvidence struct {
	basis  string
	by     string
	leadID *ids.UUID
	dealID *ids.UUID
}

// activatePartnerTx stamps the company's live partner as activated, once. A
// partner already activated, or a company with no live programme, is left as
// it is and answers false. A partner whose stage means candidate, signed or
// ready moves to the first active stage meaning active, so the stamp and the
// ladder never disagree.
func activatePartnerTx(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, ev partnerActivationEvidence) (bool, error) {
	var basis *string
	var meaning string
	err := tx.QueryRow(ctx, `
		SELECT p.activation_basis, s.meaning
		  FROM partner p JOIN partner_stage s ON s.key = p.relationship_stage
		 WHERE p.company_id = $1 AND `+livePartnerSQL("p")+`
		   FOR UPDATE OF p`, companyID).Scan(&basis, &meaning)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && basis != nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read the partner to activate: %w", err)
	}
	before, err := readPartnerImage(ctx, tx, companyID)
	if err != nil {
		return false, err
	}
	target, err := activationStage(ctx, tx, crmcontracts.PartnerStageMeaning(meaning))
	if err != nil {
		return false, err
	}
	row, err := scanPartner(tx.QueryRow(ctx, `
		UPDATE partner SET activated_at = now(), activation_basis = $2, activated_by = $3,
		       activation_lead_id = $4, activation_deal_id = $5,
		       relationship_stage = coalesce($6, relationship_stage)
		 WHERE company_id = $1
		RETURNING `+partnerColumns, companyID, ev.basis, ev.by, ev.leadID, ev.dealID, target))
	if err != nil {
		return false, fmt.Errorf("stamp the partner's activation: %w", err)
	}
	changedFrom, changedTo := storekit.ChangedColumns(before, partnerAuditImage(row))
	auditID, err := storekit.Audit(ctx, tx, "update", "company", companyID.UUID, changedFrom, changedTo)
	if err != nil {
		return false, err
	}
	return true, storekit.EmitEvent(ctx, tx, auditID, companyID.UUID, crmcontracts.PublicEventCompanyUpdated{
		ChangedFields: map[string]any{eventKeyDelta: map[string]any{eventKeyPartner: map[string]any{
			"activation_basis": ev.basis, partnerFieldStage: row.RelationshipStage,
		}}},
	})
}

// activationStage is where an activated partner stands: unchanged when its
// stage already means active, dormant or ended, else the first active stage
// meaning active. Nil keeps the stage; so does a ladder with no active stage.
func activationStage(ctx context.Context, tx pgx.Tx, meaning crmcontracts.PartnerStageMeaning) (*string, error) {
	switch meaning {
	case crmcontracts.PartnerStageMeaningCandidate, crmcontracts.PartnerStageMeaningSigned, crmcontracts.PartnerStageMeaningReady:
	default:
		return nil, nil //nolint:nilnil // nil is "keep the stage", not a failure
	}
	var key string
	err := tx.QueryRow(ctx, `
		SELECT key FROM partner_stage WHERE active AND meaning = 'active'
		 ORDER BY sort_order, key LIMIT 1 FOR SHARE`).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // a ladder with no active stage keeps the partner where it is
	}
	if err != nil {
		return nil, fmt.Errorf("read the first active partner stage: %w", err)
	}
	return &key, nil
}

// ActivatePartnerForWonDeal activates the partner that sourced a won deal. The
// partneractivation consumer calls it; a partner already activated is left as
// it is, so a replayed win changes nothing.
func (s *Store) ActivatePartnerForWonDeal(ctx context.Context, partnerID ids.CompanyID, dealID ids.DealID) (bool, error) {
	if err := auth.Require(ctx, partnerMaskObject, principal.ActionUpdate); err != nil {
		return false, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	deal := dealID.UUID
	var activated bool
	err = s.tx(ctx, func(tx pgx.Tx) error {
		activated, err = activatePartnerTx(ctx, tx, partnerID,
			partnerActivationEvidence{basis: activationCustomerWon, by: by, dealID: &deal})
		return err
	})
	return activated, err
}
