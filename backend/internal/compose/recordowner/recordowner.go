// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package recordowner reads who owns a shareable record and what it is called,
// for the requests a colleague puts to that owner.
package recordowner

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordOwners read the owner and the name of each record type a colleague may
// ask its owner about. A lead may carry only an address, so its name falls back
// to the address.
var recordOwners = map[string]string{
	"contact": `SELECT owner_id, full_name FROM contact WHERE id = $1`,
	"company": `SELECT owner_id, display_name FROM company WHERE id = $1`,
	"lead":    `SELECT owner_id, COALESCE(NULLIF(btrim(full_name), ''), email::text, '') FROM lead WHERE id = $1`,
	"deal":    `SELECT owner_id, name FROM deal WHERE id = $1`,
	"project": `SELECT owner_id, name FROM project WHERE id = $1`,
}

// Known reports whether ReadTx can read a record of recordType.
func Known(recordType string) bool {
	_, ok := recordOwners[recordType]
	return ok
}

// ReadTx answers the record's owner, nil for nobody, and its name. It asks no
// grant: the caller has already admitted the reader to the record.
func ReadTx(ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID) (*ids.UUID, string, error) {
	statement, ok := recordOwners[recordType]
	if !ok {
		return nil, "", fmt.Errorf("recordowner: a %q has no owner to read", recordType)
	}
	var owner *ids.UUID
	var name string
	err := tx.QueryRow(ctx, statement, id).Scan(&owner, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", apperrors.ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("recordowner: reading the owner of a %s: %w", recordType, err)
	}
	return owner, name, nil
}
