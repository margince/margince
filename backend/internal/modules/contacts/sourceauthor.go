// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Recording who created a contact, a company or a lead in the system it was
// imported from.
//
// THREE FUNCTIONS, NOT ONE, and the repetition is deliberate. A single function
// taking the table as a parameter reads better and is wrong here twice over:
// the ownership census cannot read a table name out of a variable, so such a
// write is attributed to no owner at all (gates/tableownership_test.go), and
// auth.EnsureWritable needs the table as a literal for the same reason. An
// earlier draft took `object string` and carried a whitelist to keep a
// request-supplied name out of SQL — with literals there is no name to
// validate and no whitelist to keep in step.
//
// What the three genuinely share — the input, the outcomes, the "is this
// already the answer" comparison, the before-image read — is in storekit, which
// is where the sharing belongs.
//
// None of the three carries a retention hold or a per-row audience, so each is
// the ordinary lock and the ordinary patch: the savepoint and the
// indistinguishable-refusal dance that activities.SetSourceAuthorTx needs have
// no subject here.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SetContactSourceAuthorTx records the author on one contact, inside the
// caller's transaction, and answers what it did.
//
// In the caller's transaction so the repair's ledger row and this write stand
// or fall together: a ledger that said "attributed" about a rolled-back write
// would tell the next run to skip the record forever.
//
// It has the same SHAPE as its company and lead siblings below and cannot share
// a body with them — see SetCompanySourceAuthorTx for why every table name here
// must be a literal.
//
//nolint:dupl // forced by the census above; see the paragraph.
func (s *Store) SetContactSourceAuthorTx(
	ctx context.Context, tx pgx.Tx, id ids.ContactID, in storekit.SourceAuthorInput,
) (storekit.SourceAuthorOutcome, string, error) {
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return storekit.SourceAuthorSkipped, "", err
	}
	if !in.Named() {
		return storekit.SourceAuthorSkipped, storekit.SourceAuthorUnnamedReason, nil
	}
	lock, err := storekit.LockRow(ctx, tx, "contact", id.UUID, storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return storekit.SourceAuthorSkipped, storekit.SourceAuthorMissingReason("contact"), nil
	}
	if err != nil {
		return storekit.SourceAuthorSkipped, "", err
	}
	// The row probe, after the object grant and before the write. The grant says
	// this caller may edit contacts at all; this says they may edit THIS one.
	//
	// ITS ErrNotFound IS A SKIP, not an error, and in the SAME words the missing
	// row gets. EnsureWritable asks EnsureVisible first, which applies capture
	// privacy on top of row scope — so an owner-private contact answers "not
	// found" even to an admin whose ordinary scope is unbounded. Returned as an
	// error it would abort the whole batch transaction, discarding the rows
	// already attributed before it and doing so again on every resumed run.
	if err := auth.EnsureWritable(ctx, tx, "contact", id.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return storekit.SourceAuthorSkipped, storekit.SourceAuthorMissingReason("contact"), nil
		}
		return storekit.SourceAuthorSkipped, "", err
	}
	before, reason, err := storekit.AttributableNow(ctx, tx, "contact", id.UUID, in)
	if err != nil || reason != "" {
		return storekit.SourceAuthorSkipped, reason, err
	}
	if before.Same(in) {
		return storekit.SourceAuthorUnchanged, "", nil
	}
	p := authorPatch(before, in)
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return storekit.SourceAuthorSkipped, "", fmt.Errorf("contacts: attributing the contact: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "import", "contact", id.UUID, p.Before(), p.After()); err != nil {
		return storekit.SourceAuthorSkipped, "", fmt.Errorf("contacts: auditing the attribution: %w", err)
	}
	return storekit.SourceAuthorApplied, "", nil
}

// SetCompanySourceAuthorTx records the author on one company.
//
// It has the same SHAPE as its contact and lead siblings and cannot share a
// body with them: every table name here must be a LITERAL, because the
// ownership census cannot attribute a write whose table it reads from a
// variable, and auth.EnsureWritable needs the same. A helper taking the table
// as a parameter is precisely the design this file replaced after
// gates/tableownership_test.go refused it. What can be shared is shared — the
// input, the outcomes, the comparison and the before-image read are all in
// storekit, and the patch is authorPatch below.
//
//nolint:dupl // forced by the census above; see the paragraph.
func (s *Store) SetCompanySourceAuthorTx(
	ctx context.Context, tx pgx.Tx, id ids.CompanyID, in storekit.SourceAuthorInput,
) (storekit.SourceAuthorOutcome, string, error) {
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return storekit.SourceAuthorSkipped, "", err
	}
	if !in.Named() {
		return storekit.SourceAuthorSkipped, storekit.SourceAuthorUnnamedReason, nil
	}
	lock, err := storekit.LockRow(ctx, tx, "company", id.UUID, storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return storekit.SourceAuthorSkipped, storekit.SourceAuthorMissingReason("company"), nil
	}
	if err != nil {
		return storekit.SourceAuthorSkipped, "", err
	}
	// Its ErrNotFound is a skip in the same words the missing row gets, for the
	// reason SetContactSourceAuthorTx states: capture privacy hides an
	// owner-private row from EnsureVisible, and an error here would take the
	// whole batch down with it.
	if err := auth.EnsureWritable(ctx, tx, "company", id.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return storekit.SourceAuthorSkipped, storekit.SourceAuthorMissingReason("company"), nil
		}
		return storekit.SourceAuthorSkipped, "", err
	}
	before, reason, err := storekit.AttributableNow(ctx, tx, "company", id.UUID, in)
	if err != nil || reason != "" {
		return storekit.SourceAuthorSkipped, reason, err
	}
	if before.Same(in) {
		return storekit.SourceAuthorUnchanged, "", nil
	}
	p := authorPatch(before, in)
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return storekit.SourceAuthorSkipped, "", fmt.Errorf("contacts: attributing the company: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "import", "company", id.UUID, p.Before(), p.After()); err != nil {
		return storekit.SourceAuthorSkipped, "", fmt.Errorf("contacts: auditing the attribution: %w", err)
	}
	return storekit.SourceAuthorApplied, "", nil
}

// SetLeadSourceAuthorTx records the author on one lead.
//
// The same as its two siblings above: the table name must be a literal at
// LockRow, EnsureWritable and Audit, so the three cannot be one function. See
// SetCompanySourceAuthorTx for the full reason.
//
//nolint:dupl // forced by the census above; see the paragraph.
func (s *Store) SetLeadSourceAuthorTx(
	ctx context.Context, tx pgx.Tx, id ids.LeadID, in storekit.SourceAuthorInput,
) (storekit.SourceAuthorOutcome, string, error) {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return storekit.SourceAuthorSkipped, "", err
	}
	if !in.Named() {
		return storekit.SourceAuthorSkipped, storekit.SourceAuthorUnnamedReason, nil
	}
	lock, err := storekit.LockRow(ctx, tx, "lead", id.UUID, storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return storekit.SourceAuthorSkipped, storekit.SourceAuthorMissingReason("lead"), nil
	}
	if err != nil {
		return storekit.SourceAuthorSkipped, "", err
	}
	// Its ErrNotFound is a skip in the same words the missing row gets — see
	// SetContactSourceAuthorTx.
	if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return storekit.SourceAuthorSkipped, storekit.SourceAuthorMissingReason("lead"), nil
		}
		return storekit.SourceAuthorSkipped, "", err
	}
	before, reason, err := storekit.AttributableNow(ctx, tx, "lead", id.UUID, in)
	if err != nil || reason != "" {
		return storekit.SourceAuthorSkipped, reason, err
	}
	if before.Same(in) {
		return storekit.SourceAuthorUnchanged, "", nil
	}
	p := authorPatch(before, in)
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return storekit.SourceAuthorSkipped, "", fmt.Errorf("contacts: attributing the lead: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "import", "lead", id.UUID, p.Before(), p.After()); err != nil {
		return storekit.SourceAuthorSkipped, "", fmt.Errorf("contacts: auditing the attribution: %w", err)
	}
	return storekit.SourceAuthorApplied, "", nil
}

// authorPatch is the two-column patch all three write. It names its columns and
// nothing else: `captured_by` answers who recorded the row HERE, and this write
// must not be able to reach it.
func authorPatch(before storekit.SourceAuthorBefore, in storekit.SourceAuthorInput) *storekit.Patch {
	p := storekit.NewPatch()
	p.Set("source_author_id", before.ID, in.AuthorID)
	p.Set("source_author_name", before.Name, in.AuthorName)
	return p
}
