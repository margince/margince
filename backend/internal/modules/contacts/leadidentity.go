// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A lead's exact identity keys and what a create does when one is already
// claimed. ADR-0008 keeps leads out of contact matching, so these keys — a live
// email address and a LinkedIn profile URL — are the whole of lead dedupe, and
// each refusal is the 409 contract with the incumbent's id.
//
// The QUESTION is storekit's (LiveLeadByEmail / LiveLeadByLinkedInURL), shared
// with the captured-lead write shape in the capture module; only the ANSWER
// differs between the two, and it lives here.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DuplicateLeadError carries the live lead already holding an email
// (uq_lead_email_dedupe → 409, features/01 §6.2).
type DuplicateLeadError struct {
	Email      string
	ExistingID ids.LeadID
}

func (e *DuplicateLeadError) Error() string { return "lead with email " + e.Email + " already exists" }

// Is maps the claim onto the shared conflict sentinel, so every caller that
// only cares that the create was refused keeps working unchanged.
func (e *DuplicateLeadError) Is(target error) bool { return target == apperrors.ErrConflict }

// DuplicateLeadLinkedInError is the same 409 contract on the LinkedIn key
// (E12.11): one profile URL, one lead.
type DuplicateLeadLinkedInError struct {
	URL        string
	ExistingID ids.LeadID
}

func (e *DuplicateLeadLinkedInError) Error() string {
	return "lead with linkedin url " + e.URL + " already exists"
}

// Is maps the claim onto the shared conflict sentinel, so every caller that
// only cares that the create was refused keeps working unchanged.
func (e *DuplicateLeadLinkedInError) Is(target error) bool { return target == apperrors.ErrConflict }

// ensureLeadEmailUnclaimed answers the live-email dedupe probe with the
// contract's 409, disclosing the existing id only when the caller could
// read that row.
func ensureLeadEmailUnclaimed(ctx context.Context, tx pgx.Tx, email *string) error {
	if email == nil {
		return nil
	}
	existing, found, err := storekit.LiveLeadByEmail(ctx, tx, *email, nil)
	if err != nil || !found {
		return err
	}
	named, err := nameableLead(ctx, tx, existing)
	if err != nil {
		return err
	}
	return &DuplicateLeadError{Email: *email, ExistingID: named}
}

// lockLeadLinkedInIdentity takes the LinkedIn write identity BEFORE either
// probe reads.
//
// The LinkedIn key has no unique index to fall back on — idx_lead_linkedin is
// deliberately non-UNIQUE, because a workspace may already hold duplicates from
// before this refusal existed and merging those is a human decision — so the
// advisory lock is the whole race guard, not a nicety.
//
// Only this key is locked; the email key needs no lock because
// uq_lead_email_dedupe decides its race and leadUniqueViolation maps the
// violation back to the same 409. The lock is still taken before BOTH probes:
// two creates sharing an address AND a profile would otherwise interleave
// between the two probes and each report whichever key it happened to lose,
// answering `duplicate_email` on one run and `duplicate_linkedin_url` on the
// next.
func lockLeadLinkedInIdentity(ctx context.Context, tx pgx.Tx, url *string) error {
	if url == nil {
		return nil
	}
	return storekit.LockWriteIdentity(ctx, tx, "lead_linkedin", *url)
}

// ensureLeadLinkedInUnclaimed is the same refusal on the OTHER exact key a
// lead carries. A profile URL names one human as firmly as an address does,
// and the probe that answers it existed while nothing called it — so two
// imports of the same contact under different addresses each minted a lead.
func ensureLeadLinkedInUnclaimed(ctx context.Context, tx pgx.Tx, url *string) error {
	if url == nil {
		return nil
	}
	existing, found, err := storekit.LiveLeadByLinkedInURL(ctx, tx, *url, nil)
	if err != nil || !found {
		return err
	}
	named, err := nameableLead(ctx, tx, existing)
	if err != nil {
		return err
	}
	return &DuplicateLeadLinkedInError{URL: *url, ExistingID: named}
}

// DuplicateContactLeadError refuses a second live lead worked from one
// contact. ExistingID is set only when the caller may read that lead.
type DuplicateContactLeadError struct {
	ExistingID ids.LeadID
}

func (e *DuplicateContactLeadError) Error() string {
	return "a live lead is already worked from this contact"
}

// Is maps the refusal onto the shared conflict sentinel, as its siblings do.
func (e *DuplicateContactLeadError) Is(target error) bool { return target == apperrors.ErrConflict }

// ensureContactNotWorked is the contact key's refusal. A contact with no email
// and no LinkedIn profile has no other key, so without it a retried "Work as a
// lead" mints a second lead. Racing writers queue on the contact row itself:
// demote already holds it, so a separate lock taken after it could deadlock
// against a create, and NO KEY UPDATE leaves the lead's foreign-key check free.
// uq_lead_from_contact_live backs it up. A lead coming back to life passes
// itself as except, so it is not refused over its own row.
func ensureContactNotWorked(ctx context.Context, tx pgx.Tx, contactID *ids.ContactID, except *ids.LeadID) error {
	if contactID == nil {
		return nil
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM contact WHERE id = $1 FOR NO KEY UPDATE`, contactID); err != nil {
		return fmt.Errorf("lock the contact a lead is worked from: %w", err)
	}
	var existing ids.LeadID
	err := tx.QueryRow(ctx,
		`SELECT id FROM lead WHERE from_contact_id = $1 AND archived_at IS NULL
		   AND ($2::uuid IS NULL OR id <> $2)`, contactID, except).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("probe lead worked from contact: %w", err)
	}
	named, err := nameableLead(ctx, tx, existing)
	if err != nil {
		return err
	}
	return &DuplicateContactLeadError{ExistingID: named}
}

// nameableLead is the lead a duplicate refusal may name: the incumbent when the
// caller could read it, and the zero id otherwise, so the 409 never discloses a
// lead outside their sight. It decides nothing about the row being written.
func nameableLead(ctx context.Context, tx pgx.Tx, existing ids.LeadID) (ids.LeadID, error) {
	visible, err := auth.VisibleTo(ctx, tx, "lead", existing.UUID)
	if err != nil || !visible {
		return ids.LeadID{}, err
	}
	return existing, nil
}

// workedFromContact is the contact a lead row was worked from, or nil. Read off
// the row: the wire lead withholds a contact its reader cannot open, and the
// writers that need the link must see it either way.
func workedFromContact(ctx context.Context, tx pgx.Tx, leadID ids.LeadID) (*ids.ContactID, error) {
	var contact *ids.ContactID
	if err := tx.QueryRow(ctx, `SELECT from_contact_id FROM lead WHERE id = $1`, leadID).Scan(&contact); err != nil {
		return nil, fmt.Errorf("read the contact a lead was worked from: %w", err)
	}
	return contact, nil
}

// reopenedLeadContactFree refuses bringing a closed lead back to life while
// another live lead is worked from its contact.
func reopenedLeadContactFree(ctx context.Context, tx pgx.Tx, leadID ids.LeadID) error {
	contact, err := workedFromContact(ctx, tx, leadID)
	if err != nil {
		return err
	}
	return ensureContactNotWorked(ctx, tx, contact, &leadID)
}
