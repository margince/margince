// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The single-record writes a bulk change drives over deals, on the caller's
// transaction. Each asks every gate its single-record twin asks and reaches
// the same body, so a deal the bulk change hands on is handed on exactly as a
// PATCH of its owner would hand it on.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BulkRow is what a bulk change reads about one deal before it changes it.
type BulkRow struct {
	Label   string
	Version int64
	OwnerID *ids.UUID
}

// LockDealForBulkTx takes the row lock a bulk change holds for its whole
// transaction and answers the deal as it stands under that lock. A deal the
// caller cannot see, or one already archived, answers apperrors.ErrNotFound.
func (s *Store) LockDealForBulkTx(ctx context.Context, tx pgx.Tx, id ids.DealID) (BulkRow, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return BulkRow{}, err
	}
	if err := auth.EnsureReadable(ctx, tx, dealTable, id.UUID); err != nil {
		return BulkRow{}, err
	}
	var row BulkRow
	err := tx.QueryRow(ctx,
		`SELECT name, version, owner_id FROM deal WHERE id = $1 AND archived_at IS NULL FOR UPDATE`,
		id).Scan(&row.Label, &row.Version, &row.OwnerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return BulkRow{}, apperrors.ErrNotFound
	}
	if err != nil {
		return BulkRow{}, fmt.Errorf("lock deal %s for a bulk change: %w", id, err)
	}
	return row, nil
}

// ReassignDealTx hands one deal to owner, conditioned on ifVersion — the write
// UpdateDeal makes when owner_id is the only field named, owner hand-over of
// its pending proposals included.
func (s *Store) ReassignDealTx(ctx context.Context, tx pgx.Tx, id ids.DealID, owner ids.UserID, ifVersion *int64) error {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return err
	}
	_, err := s.updateDealInTx(ctx, tx, id, UpdateDealInput{OwnerID: &owner, IfVersion: ifVersion}, nil)
	return err
}
