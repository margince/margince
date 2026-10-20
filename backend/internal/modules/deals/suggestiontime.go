// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func suggestionSourceTimes(ctx context.Context, tx pgx.Tx, evidence []SuggestionEvidence) (bool, error) {
	for _, item := range evidence {
		if item.SignalID == nil {
			continue
		}
		var sourceTime *time.Time
		var args []any
		arg := func(value any) int { args = append(args, value); return len(args) }
		query := fmt.Sprintf(`SELECT `+storekit.CitedActivityTime("s.evidence")+`
		 FROM signal s WHERE s.id = $%d AND s.archived_at IS NULL`, arg(*item.SignalID))
		err := tx.QueryRow(ctx, query, args...).Scan(&sourceTime)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("deals: reading suggestion source time: %w", err)
		}
		if sourceTime == nil || !sourceTime.Equal(item.OccurredAt) {
			return false, nil
		}
	}
	return true, nil
}

func supersedeIfEvidenceChanged(ctx context.Context, tx pgx.Tx, current Suggestion) (bool, error) {
	evidence := make([]SuggestionEvidence, 0, len(current.Evidence))
	for _, item := range current.Evidence {
		evidence = append(evidence, item.SuggestionEvidence)
	}
	valid, err := suggestionSourceTimes(ctx, tx, evidence)
	if err != nil || valid {
		return false, err
	}
	return true, supersedeTx(ctx, tx, current.ID)
}

func (s *Store) supersedeIfBuyerChanged(ctx context.Context, tx pgx.Tx, current Suggestion) (bool, error) {
	valid, err := s.suggestionEffects.BuyerStillVerified(ctx, tx, current, s.clock())
	if err != nil || valid {
		return false, err
	}
	return true, supersedeTx(ctx, tx, current.ID)
}
