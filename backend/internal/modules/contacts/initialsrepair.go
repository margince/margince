// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Showing an initials-only address as its local part on contacts capture named
// before the parser kept "dv@" as "dv". A guess from the address and a folded
// header "DV" both store "Dv". Only the mail that minted the contact can tell
// them apart, so a contact without one such mail keeps its name.

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// InitialsNameCandidate is a contact whose display may be capture's guess from
// an initials-only address, with the one mail whose capture minted it.
type InitialsNameCandidate struct {
	ContactID ids.ContactID
	// Address is the minting mail's counterparty, still live on the contact.
	Address      string
	RawCaptureID ids.UUID
	// Connector names the format of the stored original, read from the
	// activity's captured_by.
	Connector string
}

// InitialsNamePage is one page of the walk: every contact it inspected, and
// the candidates among them.
type InitialsNamePage struct {
	Scanned    []ids.UUID
	Candidates []InitialsNameCandidate
}

// InitialsNamePageTx inspects up to limit live contacts with an address, after
// the given id, and answers which may show capture's initials guess. Its three
// statements read only those contacts, however few need a repair.
func InitialsNamePageTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (InitialsNamePage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return InitialsNamePage{}, err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return InitialsNamePage{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT c.id FROM contact c
		 WHERE c.archived_at IS NULL AND c.id > $1
		   AND EXISTS (SELECT 1 FROM contact_email e WHERE e.contact_id = c.id AND e.archived_at IS NULL)
		 ORDER BY c.id
		 LIMIT $2`, after, limit)
	if err != nil {
		return InitialsNamePage{}, fmt.Errorf("contacts: paging contacts for the initials repair: %w", err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil || len(scanned) == 0 {
		return InitialsNamePage{Scanned: scanned}, err
	}
	rule := displayRule{unsplitMaxLen: maxInitialsDisplayLen, among: scanned}
	guessed, err := captureGuessedDisplayNamesTx(ctx, tx, nil, nil, rule, len(scanned))
	if err != nil || len(guessed) == 0 {
		return InitialsNamePage{Scanned: scanned}, err
	}
	candidates, err := mintingMailsTx(ctx, tx, guessed)
	return InitialsNamePage{Scanned: scanned, Candidates: candidates}, err
}

// mintingMailsTx answers the mail whose capture created each contact. Its
// activity link is written in the contact's create transaction, so the two
// created_at values are equal. A contact is left out when it has no such link
// or more than one. It is also left out when the original is gone or the
// minting address is no longer live on it.
func mintingMailsTx(ctx context.Context, tx pgx.Tx, contactIDs []ids.ContactID) ([]InitialsNameCandidate, error) {
	keys := make([]ids.UUID, 0, len(contactIDs))
	for _, id := range contactIDs {
		keys = append(keys, id.UUID)
	}
	rows, err := tx.Query(ctx, `
		SELECT p.id, a.counterparty_email, rc.id, split_part(a.captured_by, ':', 2)
		  FROM contact p
		  JOIN activity_link l
		    ON l.entity_type = 'contact' AND l.contact_id = p.id AND l.created_at = p.created_at
		  JOIN activity a ON a.id = l.activity_id AND a.kind = 'email'
		  JOIN raw_capture rc
		    ON rc.id = a.raw_capture_id
		    OR (a.raw_capture_id IS NULL
		        AND rc.source_system = a.source_system AND rc.source_id = a.source_id)
		 WHERE p.id = ANY($1::uuid[])
		   AND EXISTS (
		       SELECT 1 FROM contact_email e
		        WHERE e.contact_id = p.id AND e.archived_at IS NULL
		          AND lower(e.email) = lower(a.counterparty_email))
		   AND NOT EXISTS (
		       SELECT 1 FROM activity_link o
		        WHERE o.entity_type = 'contact' AND o.contact_id = p.id
		          AND o.created_at = p.created_at AND o.id <> l.id)
		 ORDER BY p.id`, keys)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the mail that minted a contact: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (InitialsNameCandidate, error) {
		var c InitialsNameCandidate
		return c, row.Scan(&c.ContactID, &c.Address, &c.RawCaptureID, &c.Connector)
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the mail that minted a contact: %w", err)
	}
	return candidates, nil
}

// RepairInitialsDisplayNameTx shows the local part of the candidate's minting
// address in place of capture's guess. headerDisplayName is the name that
// mail's header gave the address, and it must name nobody. It answers whether
// it wrote.
// Every check is asked again under the row lock, so a name typed since the page
// was read stays.
func RepairInitialsDisplayNameTx(
	ctx context.Context, tx pgx.Tx, candidate InitialsNameCandidate, headerDisplayName string,
) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	var previous string
	var split bool
	err := tx.QueryRow(ctx, `
		SELECT full_name, first_name IS NOT NULL OR last_name IS NOT NULL
		  FROM contact WHERE id = $1 FOR UPDATE`, candidate.ContactID).Scan(&previous, &split)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: reading the name contact %s shows: %w", candidate.ContactID, err)
	}
	if split || utf8.RuneCountInString(previous) > maxInitialsDisplayLen {
		return false, nil
	}
	local, ok := initialsDisplayReplacement(previous, headerDisplayName, candidate.Address)
	if !ok {
		return false, nil
	}
	rule := displayRule{unsplitMaxLen: maxInitialsDisplayLen}
	guessed, err := captureGuessedDisplayNamesTx(ctx, tx, &candidate.ContactID, nil, rule, 1)
	if err != nil || len(guessed) != 1 {
		return false, err
	}
	live, err := mintingMailsTx(ctx, tx, guessed)
	if err != nil || len(live) != 1 || live[0] != candidate {
		return false, err
	}
	return rewriteFullNameTx(ctx, tx, candidate.ContactID, previous, local)
}
