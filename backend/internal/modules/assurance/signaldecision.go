// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SignalDecisionFunc applies subject changes inside the resolution transaction.
type SignalDecisionFunc func(context.Context, pgx.Tx, ids.UUID, Resolution) error

// WithSignalDecision binds the signal owner without importing a sibling module.
func (s *Store) WithSignalDecision(decide SignalDecisionFunc) *Store {
	wired := *s
	wired.signalDecision = decide
	return &wired
}

func (s *Store) applySignalDecision(ctx context.Context, tx pgx.Tx, exception ids.UUID, in Resolution) error {
	var kind string
	var subject ids.UUID
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`SELECT subject_kind, subject_id FROM assurance_exception WHERE id = $%d`, arg(exception))
	if err := tx.QueryRow(ctx, query, args...).Scan(&kind, &subject); err != nil {
		return err
	}
	if kind != subjectKindSignal {
		if in.DealID != nil {
			return httperr.Validation("deal_id", "not_a_signal", "Only an unlinked signal can be filed on a deal.")
		}
		return nil
	}
	if in.DealID != nil && in.Outcome != OutcomeFixedRecord {
		return httperr.Validation("deal_id", "invalid_outcome", "Linking a deal needs the fixed record outcome.")
	}
	if in.Outcome == OutcomeFixedRecord && in.DealID == nil {
		return httperr.Validation("deal_id", "required", "Choose the deal that handles this request.")
	}
	if s.signalDecision == nil {
		if in.DealID != nil {
			return fmt.Errorf("assurance: signal decision is not wired")
		}
		return nil
	}
	return s.signalDecision(ctx, tx, subject, in)
}
