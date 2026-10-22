// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// MachineAddressCode is the field-fault code for an address whose local part
// names a sending system. Doors that name the address differently read it.
const MachineAddressCode = "email_names_no_one"

// refuseMachineAddress refuses an address nobody answers at, by the same
// local-part rule capture applies before it mints a contact from mail.
func refuseMachineAddress(email string) error {
	if !mailidentity.HasMachineLocalpart(email) {
		return nil
	}
	return &values.ParseError{
		Field: "emails", Code: MachineAddressCode,
		Message: "this address belongs to a sending system, not a human; add the contact's own address",
	}
}

const vcardMachineAddress = "the card's address belongs to a sending system, not a human"

// cardNamesASendingSystem reports whether any address on a card is one
// refuseMachineAddress refuses. The import asks it before the dedupe match, so
// a card matching an existing contact cannot carry the address onto it either.
func cardNamesASendingSystem(entry VCardEntry) bool {
	for _, email := range entry.Emails {
		if refuseMachineAddress(strings.TrimSpace(email.Value)) != nil {
			return true
		}
	}
	return false
}

// MachineAddressPage is one page of live contacts that hold an address.
type MachineAddressPage struct {
	// Scanned holds the contacts the page read, in id order, so a cursor can
	// move past contacts that hold only personal addresses.
	Scanned []ids.UUID
	// Machine is the subset holding a live address refuseMachineAddress refuses.
	Machine []ids.UUID
}

// MachineAddressContactsTx pages live contacts with a live address after the
// given id and names those a sending system's address made. The check runs in
// Go, through the same rule every create path asks, so the words list has one
// home. System principal only: it reads every contact in the workspace.
func MachineAddressContactsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (MachineAddressPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return MachineAddressPage{}, err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return MachineAddressPage{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT c.id, array_agg(e.email)
		  FROM contact c
		  JOIN contact_email e ON e.contact_id = c.id AND e.archived_at IS NULL
		 WHERE c.archived_at IS NULL AND c.id > $1
		 GROUP BY c.id
		 ORDER BY c.id
		 LIMIT $2`, after, limit)
	if err != nil {
		return MachineAddressPage{}, fmt.Errorf("contacts: paging contact addresses: %w", err)
	}
	var page MachineAddressPage
	for rows.Next() {
		var id ids.UUID
		var emails []string
		if err := rows.Scan(&id, &emails); err != nil {
			rows.Close()
			return MachineAddressPage{}, fmt.Errorf("contacts: reading contact addresses: %w", err)
		}
		page.Scanned = append(page.Scanned, id)
		for _, email := range emails {
			if refuseMachineAddress(email) != nil {
				page.Machine = append(page.Machine, id)
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return MachineAddressPage{}, fmt.Errorf("contacts: paging contact addresses: %w", err)
	}
	return page, nil
}

// CountContactsWithAddress counts, up to limit+1, the contacts
// MachineAddressContactsTx pages, so an estimate never reads past its cap.
func CountContactsWithAddress(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM contact c
		 WHERE c.archived_at IS NULL
		   AND EXISTS (SELECT 1 FROM contact_email e WHERE e.contact_id = c.id AND e.archived_at IS NULL)
		 LIMIT $1) candidates`, limit+1).Scan(&count)
	return count, err
}
