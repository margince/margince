// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Store) linkSuggestionTx(ctx context.Context, tx pgx.Tx, current Suggestion, in AcceptSuggestionInput) (SuggestionAcceptance, error) {
	if in.Name != nil || in.NoAmount || in.AmountMinor != nil || in.Currency != nil || in.StageID != nil || in.OwnerID != nil || in.CloseDate != nil {
		return SuggestionAcceptance{}, httperr.Validation("existing_deal_id", "conflicting_corrections", "Link an existing deal without changing its fields.")
	}
	if err := LockMatchingDealTx(ctx, tx, current.CompanyID, *in.ExistingDealID); err != nil {
		return SuggestionAcceptance{}, err
	}
	out := SuggestionAcceptance{Suggestion: current, DealID: *in.ExistingDealID}
	var err error
	out.Unlinked, out.Acknowledged, err = s.applyAcceptEffects(ctx, tx, current, out.DealID)
	if err != nil {
		return SuggestionAcceptance{}, err
	}
	out.Suggestion.State = SuggestionAccepted
	return out, recordDecision(ctx, tx, current.ID, SuggestionAccepted, &out.DealID)
}

// LinkSuggestionTx settles a covered proposal alongside its signal review.
func LinkSuggestionTx(ctx context.Context, tx pgx.Tx, suggestion, deal ids.UUID, effects SuggestionEffects) (SuggestionAcceptance, error) {
	if err := RequireSuggestionDecision(ctx, true); err != nil {
		return SuggestionAcceptance{}, err
	}
	current, err := openSuggestionForDecision(ctx, tx, suggestion)
	if err != nil {
		return SuggestionAcceptance{}, err
	}
	store := Store{suggestionEffects: effects}
	return store.linkSuggestionTx(ctx, tx, current, AcceptSuggestionInput{ExistingDealID: &deal})
}

// LockMatchingDealTx keeps the authorized target on the cited company through the link transaction.
func LockMatchingDealTx(ctx context.Context, tx pgx.Tx, company, deal ids.UUID) error {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return err
	}
	if err := auth.EnsureWritable(ctx, tx, dealTable, deal); err != nil {
		return err
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`SELECT id FROM deal WHERE id = $%d AND company_id = $%d AND archived_at IS NULL FOR UPDATE`, arg(deal), arg(company))
	var locked ids.UUID
	if err := tx.QueryRow(ctx, query, args...).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return err
	}
	return nil
}
