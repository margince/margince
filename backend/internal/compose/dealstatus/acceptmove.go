// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ValidateRecommendationTx rechecks the rule, source and reader before its writer commits.
func (s *Service) ValidateRecommendationTx(ctx context.Context, tx pgx.Tx, receipt crmcontracts.RecommendationReceipt) (crmcontracts.DealStatusCardMove, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return crmcontracts.DealStatusCardMove{}, err
	}
	var isolation string
	if err := tx.QueryRow(ctx, "SHOW transaction_isolation").Scan(&isolation); err != nil {
		return crmcontracts.DealStatusCardMove{}, err
	}
	if isolation != "serializable" {
		return crmcontracts.DealStatusCardMove{}, fmt.Errorf("create suggested work through an isolated task write or a serializable transaction: %w", apperrors.ErrInvalidArgument)
	}
	id := ids.UUID(receipt.DealId)
	currentFacts, err := s.currentMoveFacts(ctx, tx, []ids.UUID{id})
	if err != nil {
		return crmcontracts.DealStatusCardMove{}, err
	}
	f, visible := currentFacts[id]
	if !visible {
		return crmcontracts.DealStatusCardMove{}, apperrors.ErrNotFound
	}

	current, err := assessMove(f, decideMove(f))
	if err != nil {
		return crmcontracts.DealStatusCardMove{}, err
	}
	definition, known := moveRules[moveRule(receipt.Rule)]
	prior := current
	prior.Recommendation = &receipt
	if !known || definition.validate == nil || !definition.validate(prior, current) {
		return crmcontracts.DealStatusCardMove{}, fmt.Errorf("the suggested step changed; refresh it before accepting: %w", apperrors.ErrVersionSkew)
	}
	setAside, err := MovesSetAside(ctx, tx, map[ids.UUID]crmcontracts.DealStatusCardMove{id: current})
	if err != nil {
		return crmcontracts.DealStatusCardMove{}, err
	}
	if setAside[id] {
		return crmcontracts.DealStatusCardMove{}, fmt.Errorf("this suggested step was set aside; refresh the record: %w", apperrors.ErrVersionSkew)
	}
	return current, nil
}
