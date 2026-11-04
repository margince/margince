// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Ending a confidentiality hold on a contact capture made.
//
// The verdict keeps a `contact` owner-scoped while the thread it came from is
// held (NarrowedConfidentialityHold). Nothing here decides whether that hold
// has ended: the evidence is capture's ledger and the thread's messages, read
// by the release in compose. This file only answers which contacts the release
// may look at, and makes the one write it decides on.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// confidentialityHeldSQL is the test that contact p is a live, unmerged contact
// capture made, kept to its owner for a confidentiality hold and nothing else.
// A contact a human, an import or an API caller made fails it, and so does one
// whose reason is another or unknown.
func confidentialityHeldSQL() string {
	quoted := func(s string) string { return "'" + s + "'" }
	return `(p.visibility = '` + visibilityOwner + `'
		AND p.narrowing_reason = '` + string(NarrowedConfidentialityHold) + `'
		AND p.archived_at IS NULL AND p.merged_into_id IS NULL
		AND ` + captureWriterSQL("p.captured_by",
		quoted(CaptureVerdictActor), quoted(CaptureAcceptActor), quoted(DisplayNameRepairActor)) + `)`
}

// ConfidentialityHeldContact is a contact kept to its owner for a confidentiality hold, with
// the live addresses its release reads the ledger by.
type ConfidentialityHeldContact struct {
	ID     ids.ContactID
	Emails []string
}

// LockConfidentialityHeldTx locks contact id and returns it when it is still
// held for confidentiality alone. ok is false for any other contact, which the
// release leaves as it is.
//
// The release takes this row lock after the hold locks of the contact's
// addresses (capture.LockHoldEvidenceTx), the order capture's writers take
// them in. The reply promotion and the verdict's ensure lock the same row, so
// two writers over one contact queue and the second re-reads the first.
func LockConfidentialityHeldTx(ctx context.Context, tx pgx.Tx, id ids.ContactID) (ConfidentialityHeldContact, bool, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return ConfidentialityHeldContact{}, false, err
	}
	var locked ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT p.id FROM contact p
		 WHERE p.id = $1 AND `+confidentialityHeldSQL()+`
		 FOR UPDATE`, id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfidentialityHeldContact{}, false, nil
	}
	if err != nil {
		return ConfidentialityHeldContact{}, false, fmt.Errorf("contacts: locking a contact held for confidentiality: %w", err)
	}
	emails, err := ContactAddressesTx(ctx, tx, id)
	if err != nil {
		return ConfidentialityHeldContact{}, false, err
	}
	return ConfidentialityHeldContact{ID: id, Emails: emails}, true, nil
}

// HeldEvidence is capture's half of whether a confidentiality hold has ended,
// as SQL tests over an address column. Compose hands it in, so this module
// names none of capture's tables.
type HeldEvidence struct {
	// EveryAddress must hold for each live address of the contact.
	EveryAddress func(column string) string
	// SomeAddress must hold for at least one of them.
	SomeAddress func(column string) string
}

// releasableSQL is the test that contact p is held for confidentiality alone
// and that ev says its hold has ended.
func releasableSQL(ev HeldEvidence) string {
	return `(` + confidentialityHeldSQL() + `
		AND EXISTS (SELECT 1 FROM contact_email se
		             WHERE se.contact_id = p.id AND se.archived_at IS NULL AND ` + ev.SomeAddress("se.email") + `)
		AND NOT EXISTS (SELECT 1 FROM contact_email ee
		                 WHERE ee.contact_id = p.id AND ee.archived_at IS NULL
		                   AND NOT ` + ev.EveryAddress("ee.email") + `))`
}

// ConfidentialityReleasableTx reports whether contact id may leave its hold.
func ConfidentialityReleasableTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, ev HeldEvidence) (bool, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return false, err
	}
	var releasable bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contact p WHERE p.id = $1 AND `+releasableSQL(ev)+`)`,
		id).Scan(&releasable)
	if err != nil {
		return false, fmt.Errorf("contacts: reading whether a held contact may be released: %w", err)
	}
	return releasable, nil
}

// ReleasableConfidentialityHeldTx lists up to limit contacts that may leave
// their hold, in id order. A contact leaves the list once released, so a pass
// needs no position of its own.
func ReleasableConfidentialityHeldTx(ctx context.Context, tx pgx.Tx, ev HeldEvidence, limit int) ([]ids.ContactID, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT p.id FROM contact p WHERE `+releasableSQL(ev)+` ORDER BY p.id LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("contacts: listing held contacts that may be released: %w", err)
	}
	held, err := pgx.CollectRows(rows, pgx.RowTo[ids.ContactID])
	if err != nil {
		return nil, fmt.Errorf("contacts: listing held contacts that may be released: %w", err)
	}
	return held, nil
}

// ContactAddressesTx reads a contact's live addresses in order, without a
// lock. The release takes the hold locks of these addresses before it locks
// the contact.
func ContactAddressesTx(ctx context.Context, tx pgx.Tx, id ids.ContactID) ([]string, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT email FROM contact_email
		 WHERE contact_id = $1 AND archived_at IS NULL
		 ORDER BY email`, id)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a contact's addresses: %w", err)
	}
	emails, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a contact's addresses: %w", err)
	}
	return emails, nil
}

// ConfidentialityHeldByEmailTx lists up to limit contacts held for
// confidentiality alone that carry one of the addresses, in id order.
//
// It reads the correspondence address index, because a contact capture made
// carries the address it was captured from.
func ConfidentialityHeldByEmailTx(ctx context.Context, tx pgx.Tx, emails []string, limit int) ([]ids.ContactID, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return nil, err
	}
	if len(emails) == 0 || limit <= 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.id
		  FROM contact_email pe
		  JOIN contact p ON p.id = pe.contact_id
		 WHERE pe.email = ANY($1) AND pe.from_correspondence AND pe.archived_at IS NULL
		   AND `+confidentialityHeldSQL()+`
		 ORDER BY p.id
		 LIMIT $2`, emails, limit)
	if err != nil {
		return nil, fmt.Errorf("contacts: listing contacts held for confidentiality: %w", err)
	}
	held, err := pgx.CollectRows(rows, pgx.RowTo[ids.ContactID])
	if err != nil {
		return nil, fmt.Errorf("contacts: listing contacts held for confidentiality: %w", err)
	}
	return held, nil
}

// ConfidentialityHeldPage is one page of contacts in primary key order.
type ConfidentialityHeldPage struct {
	// Scanned lists the contacts the page read, held or not, so the caller's
	// cursor moves past contacts that need nothing.
	Scanned []ids.UUID
	// Held is the part of Scanned held for confidentiality alone.
	Held []ids.ContactID
}

// ConfidentialityHeldPageTx reads the next limit contacts after `after` by
// primary key and marks which are held for confidentiality alone.
//
// The page is bounded by contacts read, not by contacts found. A million
// contacts with three held ones still read limit rows a page.
func ConfidentialityHeldPageTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (ConfidentialityHeldPage, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return ConfidentialityHeldPage{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT p.id, `+confidentialityHeldSQL()+`
		  FROM contact p
		 WHERE p.id > $1
		 ORDER BY p.id
		 LIMIT $2`, after, limit)
	if err != nil {
		return ConfidentialityHeldPage{}, fmt.Errorf("contacts: paging contacts for confidentiality holds: %w", err)
	}
	defer rows.Close()
	var page ConfidentialityHeldPage
	for rows.Next() {
		var id ids.UUID
		var held bool
		if err := rows.Scan(&id, &held); err != nil {
			return ConfidentialityHeldPage{}, fmt.Errorf("contacts: paging contacts for confidentiality holds: %w", err)
		}
		page.Scanned = append(page.Scanned, id)
		if held {
			page.Held = append(page.Held, ids.From[ids.ContactKind](id))
		}
	}
	if err := rows.Err(); err != nil {
		return ConfidentialityHeldPage{}, fmt.Errorf("contacts: paging contacts for confidentiality holds: %w", err)
	}
	return page, nil
}

// CountContactsToInspect counts the contacts a full pass reads, up to maximum
// plus one so a caller can tell a capped count.
func CountContactsToInspect(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, maximum int64,
) (int64, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM contact LIMIT $1) inspected`, maximum+1).Scan(&count)
	return count, err
}

// EndConfidentialityHoldTx ends contact id's confidentiality hold and reports
// whether it changed. An empty next publishes the contact to the workspace.
// NarrowedOutboundNoAnswer keeps it the owner's, for want of an answer.
//
// Both writes match only a row still held for confidentiality. A second
// release, or one racing a human's "make private", writes nothing.
func (s *Store) EndConfidentialityHoldTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, next NarrowingReason) (bool, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return false, err
	}
	switch next {
	case "":
		return widenTx(ctx, tx, id, widening{lifts: []NarrowingReason{NarrowedConfidentialityHold}})
	case NarrowedOutboundNoAnswer:
		return renarrowTx(ctx, tx, id, NarrowedConfidentialityHold, next)
	default:
		return false, fmt.Errorf("contacts: a confidentiality hold cannot end in %q", next)
	}
}

// renarrowTx replaces one narrowing reason with another on an owner-scoped
// contact, with the audit row and event any reason change carries.
func renarrowTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, from, to NarrowingReason) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE contact SET narrowing_reason = $3
		 WHERE id = $1 AND visibility = $4 AND narrowing_reason = $2 AND archived_at IS NULL`,
		id, from, to, visibilityOwner)
	if err != nil {
		return false, fmt.Errorf("contacts: changing why a contact stays its owner's: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	auditID, err := storekit.Audit(ctx, tx, "update", entityContact, id.UUID,
		map[string]any{fieldNarrowingReason: from},
		map[string]any{fieldNarrowingReason: to})
	if err != nil {
		return false, fmt.Errorf("contacts: auditing why a contact stays its owner's: %w", err)
	}
	return true, storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventContactUpdated{
		ChangedFields: map[string]any{fieldNarrowingReason: to},
	})
}
