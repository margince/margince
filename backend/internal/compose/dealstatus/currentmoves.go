// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CurrentMoves chooses from batched owning reads, without model calls or cached operands.
func (s *Service) CurrentMoves(ctx context.Context, dealIDs []ids.UUID) (map[ids.UUID]crmcontracts.DealStatusCardMove, error) {
	out := make(map[ids.UUID]crmcontracts.DealStatusCardMove, len(dealIDs))
	if len(dealIDs) == 0 {
		return out, nil
	}
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	if _, err := actingUser(ctx); err != nil {
		return nil, err
	}
	err := database.WithWorkspaceSnapshot(ctx, s.pool, func(ctx context.Context) error {
		return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
			current, err := s.currentMoveFacts(ctx, tx, dealIDs)
			if err != nil {
				return err
			}
			for id, facts := range current {
				move, err := assessMove(facts, decideMove(facts))
				if err != nil {
					return err
				}
				if move.Action != ActionNone {
					out[id] = move
				}
			}
			return admitCurrentMoves(ctx, tx, out)
		})
	})
	if err != nil {
		return nil, fmt.Errorf("check the current deal moves: %w", err)
	}
	s.refreshMoveCoverage(ctx, out)
	return out, nil
}

func admitCurrentMoves(ctx context.Context, tx pgx.Tx, moves map[ids.UUID]crmcontracts.DealStatusCardMove) error {
	readable, err := readableActivities(ctx, tx, namedActivities(moves))
	if err != nil {
		return err
	}
	contacts, err := contactsStillFileable(ctx, tx, contactsAcrossMoves(moves))
	if err != nil {
		return err
	}
	setAside, err := MovesSetAside(ctx, tx, moves)
	if err != nil {
		return err
	}
	for id, move := range moves {
		activity, named := NamedActivity(move)
		if (named && !readable[activity]) || !everyNamedContactStillFileable(move, contacts) || setAside[id] {
			delete(moves, id)
		}
	}
	return nil
}
