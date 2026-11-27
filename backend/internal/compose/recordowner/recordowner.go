// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package recordowner reads who owns a shareable record and what it is called,
// and files the requests a colleague puts to that owner.
package recordowner

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// recordOwners read the owner and the name of each record type a colleague may
// ask its owner about. The share lock keeps the owner from moving until the
// reader's transaction ends. A lead may carry only an address, so its name
// falls back to the address.
var recordOwners = map[string]string{
	"contact": `SELECT owner_id, full_name FROM contact WHERE id = $1 FOR SHARE`,
	"company": `SELECT owner_id, display_name FROM company WHERE id = $1 FOR SHARE`,
	"lead":    `SELECT owner_id, COALESCE(NULLIF(btrim(full_name), ''), email::text, '') FROM lead WHERE id = $1 FOR SHARE`,
	"deal":    `SELECT owner_id, name FROM deal WHERE id = $1 FOR SHARE`,
	"project": `SELECT owner_id, name FROM project WHERE id = $1 FOR SHARE`,
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

// NameForTx is the record's name when member may still read it, and the words
// for an unnamed record otherwise. A line to member then never names a record
// they lost sight of, or one that was erased.
func NameForTx(ctx context.Context, tx pgx.Tx, member ids.UUID, recordType string, id ids.UUID, lang textlang.Lang) (string, error) {
	// Every record type here is a core one, so the core vocabulary decides.
	members, err := identity.MemberPrincipalsTx(ctx, tx, identity.RbacObjects{}, []ids.UUID{member})
	if err != nil {
		return "", err
	}
	unnamed := systemtext.Copy("editAccess.UnnamedRecord", lang)
	reader, live := members[member]
	if !live {
		return unnamed, nil
	}
	visible, err := auth.VisibleToExisting(principal.WithActor(ctx, reader), tx, recordType, id)
	if err != nil || !visible {
		return unnamed, err
	}
	_, name, err := ReadTx(ctx, tx, recordType, id)
	if errors.Is(err, apperrors.ErrNotFound) {
		return unnamed, nil
	}
	return name, err
}

// ColleagueNameTx is a member's display name, for a sentence naming them.
func ColleagueNameTx(ctx context.Context, tx pgx.Tx, member ids.UUID) (string, error) {
	var name string
	if err := tx.QueryRow(ctx, `SELECT display_name FROM app_user WHERE id = $1`, member).Scan(&name); err != nil {
		return "", fmt.Errorf("recordowner: naming a colleague: %w", err)
	}
	return name, nil
}
