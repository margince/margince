// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DecideAssuranceSignal settles the cited matter through its owning writers.
func DecideAssuranceSignal(ctx context.Context, tx pgx.Tx, signal ids.UUID, in assurance.Resolution) error {
	if in.Outcome == assurance.OutcomeNotRelevant {
		if err := dismissCoveredSuggestion(ctx, tx, signal); err != nil {
			return err
		}
		_, err := signals.DismissTx(ctx, tx, signal, "assurance")
		return err
	}
	if in.DealID == nil {
		return nil
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`SELECT resolved_company_id FROM signal WHERE id = $%d`, arg(signal))
	var company ids.UUID
	if err := tx.QueryRow(ctx, query, args...).Scan(&company); err != nil {
		return err
	}
	if err := auth.EnsureVisible(ctx, tx, "company", company); err != nil {
		return err
	}
	suggestions, err := coveredSuggestions(ctx, tx, signal, true)
	if err != nil {
		return err
	}
	if err := deals.LockMatchingDealTx(ctx, tx, company, *in.DealID); err != nil {
		return err
	}
	return linkSignalSources(ctx, tx, signal, *in.DealID, suggestions)
}

func linkSignalSources(ctx context.Context, tx pgx.Tx, signal, deal ids.UUID, suggestions []ids.UUID) error {
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`SELECT DISTINCT `+storekit.CitedActivityID("cite")+`
 FROM signal s CROSS JOIN LATERAL jsonb_array_elements(s.evidence) cite
 WHERE s.id = $%d AND cite->>'source_type' = 'activity'`, arg(signal))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	sources, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	effects := dealSuggestionEffects{}
	for _, source := range sources {
		if err := effects.LinkActivityToDeal(ctx, tx, source, deal); err != nil {
			return err
		}
	}
	if _, err := signals.AcknowledgeTx(ctx, tx, signal, "assurance"); err != nil {
		return err
	}
	for _, suggestion := range suggestions {
		if _, err := deals.LinkSuggestionTx(ctx, tx, suggestion, deal, effects); err != nil {
			if _, decided := errors.AsType[*deals.SuggestionDecidedError](err); !decided {
				return err
			}
		}
	}
	return nil
}

func dismissCoveredSuggestion(ctx context.Context, tx pgx.Tx, signal ids.UUID) error {
	suggestions, err := coveredSuggestions(ctx, tx, signal, false)
	if err != nil {
		return err
	}
	for _, suggestion := range suggestions {
		if _, err := deals.DismissSuggestionTx(ctx, tx, suggestion, "not_a_deal"); err != nil {
			if _, decided := errors.AsType[*deals.SuggestionDecidedError](err); !decided {
				return err
			}
		}
	}
	return nil
}

func coveredSuggestions(ctx context.Context, tx pgx.Tx, signal ids.UUID, linkExisting bool) ([]ids.UUID, error) {
	if err := deals.RequireSuggestionDecision(ctx, linkExisting); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, nil
		}
		return nil, err
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	visible, err := deals.ReadableSuggestionClause(ctx, arg)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT s.id FROM deal_suggestion s
 WHERE EXISTS (SELECT 1 FROM deal_suggestion_evidence e WHERE e.suggestion_id = s.id AND e.signal_id = $%d)
 AND s.state = 'open' AND `+visible+` ORDER BY s.id FOR UPDATE OF s`, arg(signal))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
