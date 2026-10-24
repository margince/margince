// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/introduction"
)

// PreviewIntroCarryTx shares duplicate and self-route closure rules with the merge writer.
func (s *Store) PreviewIntroCarryTx(ctx context.Context, tx pgx.Tx, from, to ids.ContactID) ([]introduction.CarriedRequest, error) {
	if err := admitIntroCarry(ctx, tx, from, to); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "introduction", principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, introCarryClosures+`, recorded_losers AS (`+introCarryRecordedLosers+`)
 SELECT r.id,r.version,r.contact_id,`+introCarryContact+`,r.through_contact_id,`+introCarryThrough+`,
 r.status,
 CASE WHEN l.id IS NOT NULL THEN 'cancelled' WHEN rl.id IS NOT NULL THEN 'archived' ELSE r.status END,
 CASE WHEN l.id IS NOT NULL THEN CASE WHEN l.self_route THEN @self_reason ELSE @duplicate_reason END
 WHEN rl.id IS NOT NULL THEN @recorded_reason ELSE '' END
 FROM intro_request r LEFT JOIN losers l ON l.id=r.id LEFT JOIN recorded_losers rl ON rl.id=r.id
 WHERE `+introCarrySource+` OR rl.id IS NOT NULL ORDER BY r.id LIMIT 501`, introCarryArgs(from, to))
	if err != nil {
		return nil, fmt.Errorf("introductions: previewing carried requests: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (introduction.CarriedRequest, error) {
		var change introduction.CarriedRequest
		err := row.Scan(&change.ID, &change.Version, &change.ContactBefore, &change.ContactAfter, &change.ThroughBefore, &change.ThroughAfter, &change.StatusBefore, &change.StatusAfter, &change.CancellationReason)
		return change, err
	})
	if err != nil {
		return nil, fmt.Errorf("introductions: reading carried request effects: %w", err)
	}
	if len(out) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	if len(out) == 0 {
		return out, nil
	}
	for _, change := range out {
		if _, err := s.load(ctx, tx, change.ID); err != nil {
			return nil, err
		}
	}
	if err := s.authorizeIntroCarryWitnesses(ctx, tx, from, to); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) authorizeIntroCarryWitnesses(ctx context.Context, tx pgx.Tx, from, to ids.ContactID) error {
	rows, err := tx.Query(ctx, introCarryClosures+` SELECT DISTINCT witness FROM collisions ORDER BY witness LIMIT 501`, introCarryArgs(from, to))
	if err != nil {
		return fmt.Errorf("introductions: reading merge collision witnesses: %w", err)
	}
	witnesses, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("introductions: reading merge collision witnesses: %w", err)
	}
	if len(witnesses) > 500 {
		return apperrors.ErrInvalidArgument
	}
	for _, id := range witnesses {
		if _, err := s.load(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}
