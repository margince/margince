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
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BulkRow is what a bulk change reads about one deal before it changes it.
type BulkRow struct {
	Label   string
	Version int64
	OwnerID *ids.UUID
	// Archived and Lock are set by HoldDealForBulkTx only.
	Archived bool
	Lock     storekit.RowLock
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

// HoldDealForBulkTx is LockDealForBulkTx for the lock a bulk change takes on
// every row before it changes any. It locks an archived deal too, because an
// undo restores one, and answers the lock for the write that follows.
func (s *Store) HoldDealForBulkTx(ctx context.Context, tx pgx.Tx, id ids.DealID) (BulkRow, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return BulkRow{}, err
	}
	if err := auth.EnsureReadable(ctx, tx, dealTable, id.UUID); err != nil {
		return BulkRow{}, err
	}
	var row BulkRow
	lock, err := storekit.LockRowReading(ctx, tx, dealTable, id.UUID, storekit.IncludeArchived,
		"name, version, owner_id, archived_at IS NOT NULL", &row.Label, &row.Version, &row.OwnerID, &row.Archived)
	if err != nil {
		return BulkRow{}, err
	}
	row.Lock = lock
	return row, nil
}

// ReassignDealTx hands one deal to owner, conditioned on ifVersion. It is the
// write UpdateDeal makes when owner_id is the only field named, the hand-over of
// pending proposals included. Nothing is read back.
func (s *Store) ReassignDealTx(ctx context.Context, tx pgx.Tx, id ids.DealID, owner ids.UserID, ifVersion *int64) error {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return err
	}
	_, err := s.lockAndWriteDealInTx(ctx, tx, id, UpdateDealInput{OwnerID: &owner, IfVersion: ifVersion}, nil)
	return err
}

// ReassignHeldDealTx is ReassignDealTx on the lock HoldDealForBulkTx took, so
// the row is not locked a second time.
func (s *Store) ReassignHeldDealTx(ctx context.Context, tx pgx.Tx, held storekit.RowLock, owner ids.UserID, ifVersion *int64) error {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return err
	}
	if !held.Holds(dealTable, held.ID()) {
		return errors.New("deals: a held reassignment needs a deal row lock")
	}
	_, err := s.writeDealInTx(ctx, tx, held, UpdateDealInput{OwnerID: &owner, IfVersion: ifVersion}, nil)
	return err
}
