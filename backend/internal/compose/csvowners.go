// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A record's owner, named in the file by the seat's email. Import never
// creates a seat: an address no seat holds leaves the importer as the owner,
// and the report says so.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const csvTargetOwner = "owner_email"

// unknownOwnerReason is the report's word for an address no seat holds.
const unknownOwnerReason = "no user has this email, so you become the owner"

// seatsByEmail answers the seat each address belongs to, keyed by the folded
// address, in one query. An address no seat holds is absent.
func seatsByEmail(ctx context.Context, tx pgx.Tx, emails []string) (map[string]ids.UUID, error) {
	folded := make([]string, 0, len(emails))
	for _, email := range emails {
		folded = append(folded, foldedEmail(email))
	}
	rows, err := tx.Query(ctx, `SELECT id, lower(email) FROM app_user WHERE lower(email) = ANY($1)`, folded)
	if err != nil {
		return nil, fmt.Errorf("import: finding the seats these emails belong to: %w", err)
	}
	defer rows.Close()
	seats := make(map[string]ids.UUID, len(folded))
	for rows.Next() {
		var id ids.UUID
		var email string
		if err := rows.Scan(&id, &email); err != nil {
			return nil, fmt.Errorf("import: reading a seat: %w", err)
		}
		seats[email] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("import: finding the seats these emails belong to: %w", err)
	}
	return seats, nil
}

func foldedEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ownerResolver holds the seat of every owner address a run names, read once.
type ownerResolver struct {
	seats map[string]ids.UUID
}

// newOwnerResolver resolves every distinct owner address of the run.
func newOwnerResolver(ctx context.Context, pool *pgxpool.Pool, emails []string) (*ownerResolver, error) {
	o := &ownerResolver{seats: map[string]ids.UUID{}}
	if len(emails) == 0 {
		return o, nil
	}
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		o.seats, err = seatsByEmail(ctx, tx, emails)
		return err
	})
	return o, err
}

// ownerFor is the seat an owner cell names. An address no seat holds answers
// nil, so the store makes the importer the owner, and the warning saying so.
func (o *ownerResolver) ownerFor(raw string) (*ids.UserID, *migration.SkippedRow) {
	seat, known := o.seats[foldedEmail(raw)]
	if !known {
		return nil, &migration.SkippedRow{Kind: migration.ProblemUnknownOwner, Value: strings.TrimSpace(raw), Reason: unknownOwnerReason}
	}
	owner := ids.From[ids.UserKind](seat)
	return &owner, nil
}

// ownerOf resolves a row's owner cell, with the warning placed on the row's
// own line and column.
func (w *csvWriters) ownerOf(row migration.Row) (*ids.UserID, *migration.SkippedRow) {
	raw := strings.TrimSpace(textFields(row.Fields)[csvTargetOwner])
	if raw == "" {
		return nil, nil
	}
	owner, warning := w.owners.ownerFor(raw)
	if warning != nil {
		warning.ExternalID, warning.Line, warning.Column = row.ExternalID, row.Line, w.headerFor(csvTargetOwner)
	}
	return owner, warning
}

// landingCells is checkedCells with the row's owner: a known seat rides in
// typed, and an unknown address is the warning the row lands with.
func (w *csvWriters) landingCells(row migration.Row) (map[string]any, *migration.SkippedRow, *migration.SkippedRow) {
	typed, invalid := w.checkedCells(row)
	if invalid != nil {
		return nil, invalid, nil
	}
	owner, warning := w.ownerOf(row)
	if owner != nil {
		if typed == nil {
			typed = map[string]any{}
		}
		typed[csvTargetOwner] = owner.UUID
	}
	return typed, nil, warning
}

// ownerFrom is the seat a create or an update names, nil when the file named
// none this run knows.
func ownerFrom(fields map[string]string, typed map[string]any) *ids.UserID {
	if _, given := fields[csvTargetOwner]; !given {
		return nil
	}
	seat, known := typed[csvTargetOwner].(ids.UUID)
	if !known {
		return nil
	}
	owner := ids.From[ids.UserKind](seat)
	return &owner
}
