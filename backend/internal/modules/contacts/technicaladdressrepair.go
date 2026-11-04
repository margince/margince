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
	// held is a legal hold, or a live phone or channel identity that may name
	// the human the technical address does not.
	held bool
}

// RetireTechnicalAddressContactsTx corrects one page of live contacts after the
// given id. Each write goes through the domain's own archive and address
// writers at the version read. A contact edited meanwhile is skipped and met
// again on a later run. System principal only: it reads every contact.
func (s *Store) RetireTechnicalAddressContactsTx(
	ctx context.Context, tx pgx.Tx, after ids.UUID, limit int,
) (TechnicalAddressPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return TechnicalAddressPage{}, err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionDelete); err != nil {
		return TechnicalAddressPage{}, err
	}
	candidates, scanned, err := technicalCandidatePage(ctx, tx, after, limit)
	if err != nil {
		return TechnicalAddressPage{}, err
	}
	page := TechnicalAddressPage{Scanned: scanned}
	for _, contact := range candidates {
		technical := technicalAddresses(contact.emails)
		switch {
		case len(technical) == 0:
			continue
		case len(technical) < len(contact.emails):
			retired, err := s.retireTechnicalEmails(ctx, tx, contact, technical)
			if err != nil {
				return TechnicalAddressPage{}, err
			}
			if retired {
				page.Retired++
			}
		case contact.held:
			page.Held = append(page.Held, contact.id.UUID)
		default:
			err := archiveContactInTx(ctx, tx, contact.id, &contact.version, writeOptions{})
			if skippedWrite(err) {
				continue
			}
			if err != nil {
				return TechnicalAddressPage{}, fmt.Errorf("contacts: archiving a technical address's contact: %w", err)
			}
			page.Archived++
		}
	}
	return page, nil
}

// skippedWrite is a contact that changed or left since the page was read. It is
// left alone now and judged again by a later run.
func skippedWrite(err error) bool {
	return errors.Is(err, apperrors.ErrVersionSkew) || errors.Is(err, apperrors.ErrNotFound)
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
) (bool, error) {
	rows, err := tx.Query(ctx, `SELECT email, email_type, is_primary, position, NOT from_correspondence FROM contact_email
		WHERE contact_id = $1 AND archived_at IS NULL ORDER BY position, email`, contact.id)
	if err != nil {
		return false, fmt.Errorf("contacts: reading a contact's addresses: %w", err)
	}
	held, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ContactEmailInput])
	if err != nil {
		return false, fmt.Errorf("contacts: reading a contact's addresses: %w", err)
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
	if skippedWrite(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: retiring a contact's technical address: %w", err)
	}
	return true, nil
}

// technicalCandidatePage reads one page of live, unmerged contacts holding a
// live address, with what decides each one's group.
func technicalCandidatePage(
	ctx context.Context, tx pgx.Tx, after ids.UUID, limit int,
) ([]technicalCandidate, []ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.id, c.version, array_agg(e.email ORDER BY e.email),
		       c.legal_hold
		       OR EXISTS (SELECT 1 FROM contact_phone p WHERE p.contact_id = c.id AND p.archived_at IS NULL)
		       OR EXISTS (SELECT 1 FROM contact_channel_identity i WHERE i.contact_id = c.id AND i.archived_at IS NULL)
		  FROM contact c
		  JOIN contact_email e ON e.contact_id = c.id AND e.archived_at IS NULL
		 WHERE c.archived_at IS NULL AND c.merged_into_id IS NULL AND c.id > $1
		 GROUP BY c.id
		 ORDER BY c.id
		 LIMIT $2`, after, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("contacts: paging contacts for the technical-address correction: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (technicalCandidate, error) {
		var c technicalCandidate
		err := row.Scan(&c.id, &c.version, &c.emails, &c.held)
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
