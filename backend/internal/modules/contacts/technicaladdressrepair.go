// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Retiring contacts a technical address made. A contact whose every address is
// technical names no one and is archived, human-authored data and all, which
// Undo brings back. A human who also holds a real address keeps the record and
// loses only the technical one. A contact on legal hold, or one a live phone or
// channel identity may still name, is kept for review.

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

// TechnicalAddressPage is one page of the technical-address correction.
type TechnicalAddressPage struct {
	// Scanned holds every contact the page read, in id order, so the cursor
	// moves past contacts that hold only human addresses.
	Scanned []ids.UUID
	// Archived counts technical-only contacts archived, Retired the mixed
	// contacts whose technical addresses were retired.
	Archived, Retired int
	// Held are technical-only contacts kept for review.
	Held []ids.UUID
}

// technicalCandidate is one contact on the page with what decides its group.
type technicalCandidate struct {
	id      ids.ContactID
	version int64
	emails  []string
	// legalHold means nothing about the contact may change. It is only tagged.
	legalHold bool
	// otherIdentity is a live phone or channel identity that may name the
	// human the technical address does not.
	otherIdentity bool
}

// technicalOutcome is what the correction did to one contact.
type technicalOutcome int

const (
	technicalUntouched technicalOutcome = iota
	technicalArchived
	technicalRetired
	technicalHeld
)

// technicalWriteAttempts bounds the re-reads of one contact edited while the
// page runs. A contact still changing after that fails the batch, which rolls
// back and runs again, so no contact is passed over.
const technicalWriteAttempts = 3

// beforeTechnicalWrite runs between reading a contact and writing it. Tests
// set it to edit the contact in that window; production leaves it nil.
var beforeTechnicalWrite func(ctx context.Context, id ids.ContactID)

// RetireTechnicalAddressContactsTx corrects one page of live contacts after the
// given id. Each write goes through the domain's own archive and address
// writers at the version read. A contact edited meanwhile is read again and
// judged on what it now holds. The cursor never moves past a contact the
// correction did not decide. System principal only: it reads every contact.
func (s *Store) RetireTechnicalAddressContactsTx(
	ctx context.Context, tx pgx.Tx, after ids.UUID, limit int,
) (TechnicalAddressPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return TechnicalAddressPage{}, err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionDelete); err != nil {
		return TechnicalAddressPage{}, err
	}
	candidates, scanned, err := technicalCandidatePage(ctx, tx, after, limit, nil)
	if err != nil {
		return TechnicalAddressPage{}, err
	}
	page := TechnicalAddressPage{Scanned: scanned}
	for _, contact := range candidates {
		outcome, err := s.correctTechnicalContact(ctx, tx, contact)
		if err != nil {
			return TechnicalAddressPage{}, err
		}
		switch outcome {
		case technicalArchived:
			page.Archived++
		case technicalRetired:
			page.Retired++
		case technicalHeld:
			page.Held = append(page.Held, contact.id.UUID)
		case technicalUntouched:
		}
	}
	return page, nil
}

// correctTechnicalContact decides one contact and writes it. A write refused
// for a version skew re-reads the contact and decides again. A contact that
// left the live set meanwhile needs nothing.
func (s *Store) correctTechnicalContact(
	ctx context.Context, tx pgx.Tx, contact technicalCandidate,
) (technicalOutcome, error) {
	for attempt := 1; ; attempt++ {
		if beforeTechnicalWrite != nil {
			beforeTechnicalWrite(ctx, contact.id)
		}
		outcome, err := s.writeTechnicalOutcome(ctx, tx, contact)
		switch {
		case err == nil:
			return outcome, nil
		case errors.Is(err, apperrors.ErrNotFound):
			return technicalUntouched, nil
		case !errors.Is(err, apperrors.ErrVersionSkew) || attempt == technicalWriteAttempts:
			return technicalUntouched, fmt.Errorf("contacts: correcting a technical address's contact: %w", err)
		}
		reread, _, err := technicalCandidatePage(ctx, tx, ids.Nil, 1, &contact.id)
		if err != nil {
			return technicalUntouched, err
		}
		if len(reread) == 0 {
			return technicalUntouched, nil
		}
		contact = reread[0]
	}
}

// writeTechnicalOutcome sorts one contact into its group and writes it. Legal
// hold comes first: a held contact keeps every address, technical ones too.
func (s *Store) writeTechnicalOutcome(
	ctx context.Context, tx pgx.Tx, contact technicalCandidate,
) (technicalOutcome, error) {
	technical := technicalAddresses(contact.emails)
	switch {
	case len(technical) == 0:
		return technicalUntouched, nil
	case contact.legalHold:
		return technicalHeld, nil
	case len(technical) < len(contact.emails):
		if err := s.retireTechnicalEmails(ctx, tx, contact, technical); err != nil {
			return technicalUntouched, err
		}
		return technicalRetired, nil
	case contact.otherIdentity:
		return technicalHeld, nil
	default:
		if err := archiveContactInTx(ctx, tx, contact.id, &contact.version, writeOptions{}); err != nil {
			return technicalUntouched, err
		}
		return technicalArchived, nil
	}
}

// technicalAddresses are the addresses refuseMachineAddress refuses, the rule
// every create path asks.
func technicalAddresses(emails []string) map[string]bool {
	out := map[string]bool{}
	for _, email := range emails {
		if refuseMachineAddress(email) != nil {
			out[email] = true
		}
	}
	return out
}

// retireTechnicalEmails keeps the human and every other address, through the
// contact's own edit: one audit row and one contact.updated per contact. A
// retired primary hands primacy to the next address.
func (s *Store) retireTechnicalEmails(
	ctx context.Context, tx pgx.Tx, contact technicalCandidate, technical map[string]bool,
) error {
	rows, err := tx.Query(ctx, `SELECT email, email_type, is_primary, position, NOT from_correspondence FROM contact_email
		WHERE contact_id = $1 AND archived_at IS NULL ORDER BY position, email`, contact.id)
	if err != nil {
		return fmt.Errorf("contacts: reading a contact's addresses: %w", err)
	}
	held, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ContactEmailInput])
	if err != nil {
		return fmt.Errorf("contacts: reading a contact's addresses: %w", err)
	}
	kept := make([]ContactEmailInput, 0, len(held))
	lostPrimary := false
	for _, email := range held {
		if technical[email.Email] {
			lostPrimary = lostPrimary || email.IsPrimary
			continue
		}
		kept = append(kept, email)
	}
	if lostPrimary && len(kept) > 0 {
		kept[0].IsPrimary = true
	}
	// No custom columns: the edit replaces only the address set, and the
	// catalog read would take a second connection inside the caller's
	// transaction. The audit and event images carry only the addresses.
	_, err = s.updateContactInTx(ctx, tx, contact.id, UpdateContactInput{Emails: kept, IfVersion: &contact.version}, nil)
	if err != nil {
		return fmt.Errorf("contacts: retiring a contact's technical address: %w", err)
	}
	return nil
}

// technicalCandidatePage reads one page of live, unmerged contacts holding a
// live address, with what decides each one's group. With only set, it reads
// that one contact again.
func technicalCandidatePage(
	ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, only *ids.ContactID,
) ([]technicalCandidate, []ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.id, c.version, array_agg(e.email ORDER BY e.email), c.legal_hold,
		       EXISTS (SELECT 1 FROM contact_phone p WHERE p.contact_id = c.id AND p.archived_at IS NULL)
		       OR EXISTS (SELECT 1 FROM contact_channel_identity i WHERE i.contact_id = c.id AND i.archived_at IS NULL)
		  FROM contact c
		  JOIN contact_email e ON e.contact_id = c.id AND e.archived_at IS NULL
		 WHERE c.archived_at IS NULL AND c.merged_into_id IS NULL AND c.id > $1
		   AND ($3::uuid IS NULL OR c.id = $3)
		 GROUP BY c.id
		 ORDER BY c.id
		 LIMIT $2`, after, limit, only)
	if err != nil {
		return nil, nil, fmt.Errorf("contacts: paging contacts for the technical-address correction: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (technicalCandidate, error) {
		var c technicalCandidate
		err := row.Scan(&c.id, &c.version, &c.emails, &c.legalHold, &c.otherIdentity)
		return c, err
	})
	if err != nil {
		return nil, nil, fmt.Errorf("contacts: reading contacts for the technical-address correction: %w", err)
	}
	scanned := make([]ids.UUID, len(candidates))
	for i, c := range candidates {
		scanned[i] = c.id.UUID
	}
	return candidates, scanned, nil
}
