// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The single-record writes a bulk change drives, on the caller's transaction.
//
// A bulk change is these writes N times over, in one commit. Each entry point
// here asks every gate its single-record twin asks and reaches the same body,
// so a contact the bulk change hands on is handed on exactly as a PATCH of its
// owner would hand it on: the same write check, the same assignee rule, the
// same audit row and event.

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

// BulkRow is what a bulk change reads about one record before it changes it.
type BulkRow struct {
	Label   string
	Version int64
	OwnerID *ids.UUID
}

// LockContactForBulkTx takes the row lock a bulk change holds for its whole
// transaction and answers the row as it stands under that lock. A contact the
// caller cannot see, or one already archived, answers apperrors.ErrNotFound.
func (s *Store) LockContactForBulkTx(ctx context.Context, tx pgx.Tx, id ids.ContactID) (BulkRow, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return BulkRow{}, err
	}
	return lockForBulk(ctx, tx, "contact", id.UUID,
		`SELECT full_name, version, owner_id FROM contact WHERE id = $1 AND archived_at IS NULL FOR UPDATE`)
}

// LockCompanyForBulkTx is LockContactForBulkTx for a company.
func (s *Store) LockCompanyForBulkTx(ctx context.Context, tx pgx.Tx, id ids.CompanyID) (BulkRow, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return BulkRow{}, err
	}
	return lockForBulk(ctx, tx, "company", id.UUID,
		`SELECT display_name, version, owner_id FROM company WHERE id = $1 AND archived_at IS NULL FOR UPDATE`)
}

func lockForBulk(ctx context.Context, tx pgx.Tx, table string, id ids.UUID, statement string) (BulkRow, error) {
	if err := auth.EnsureReadable(ctx, tx, table, id); err != nil {
		return BulkRow{}, err
	}
	var row BulkRow
	err := tx.QueryRow(ctx, statement, id).Scan(&row.Label, &row.Version, &row.OwnerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return BulkRow{}, apperrors.ErrNotFound
	}
	if err != nil {
		return BulkRow{}, fmt.Errorf("lock %s %s for a bulk change: %w", table, id, err)
	}
	return row, nil
}

// ReassignContactTx hands one contact to owner, conditioned on ifVersion — the
// write UpdateContact makes when owner_id is the only field named.
func (s *Store) ReassignContactTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, owner ids.UserID, ifVersion *int64) error {
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return err
	}
	_, err := s.updateContactInTx(ctx, tx, id, UpdateContactInput{OwnerID: &owner, IfVersion: ifVersion}, nil)
	return err
}

// ReassignCompanyTx is ReassignContactTx for a company.
func (s *Store) ReassignCompanyTx(ctx context.Context, tx pgx.Tx, id ids.CompanyID, owner ids.UserID, ifVersion *int64) error {
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return err
	}
	_, err := s.updateCompanyInTx(ctx, tx, id, UpdateCompanyInput{OwnerID: &owner, IfVersion: ifVersion}, nil)
	return err
}

// LockLeadForBulkTx is LockContactForBulkTx for a lead.
func (s *Store) LockLeadForBulkTx(ctx context.Context, tx pgx.Tx, id ids.LeadID) (BulkRow, error) {
	if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
		return BulkRow{}, err
	}
	// A lead may carry only an address, so its label falls back to the email
	// and then to nothing rather than scanning a NULL into the label.
	return lockForBulk(ctx, tx, "lead", id.UUID,
		`SELECT COALESCE(NULLIF(btrim(full_name), ''), email::text, ''), version, owner_id
		   FROM lead WHERE id = $1 AND archived_at IS NULL FOR UPDATE`)
}

// ReassignLeadTx hands one lead to owner, conditioned on ifVersion — the write
// UpdateLead makes when owner_id is the only field named, so an ownerless lead
// in the queue can be handed on exactly as a single assignment hands it on.
// An owner-only patch touches no custom field, so no catalog is needed.
func (s *Store) ReassignLeadTx(ctx context.Context, tx pgx.Tx, id ids.LeadID, owner ids.UserID, ifVersion *int64) error {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return err
	}
	_, err := s.updateLeadTx(ctx, tx, id, UpdateLeadInput{OwnerID: &owner, IfVersion: ifVersion}, nil)
	return err
}
