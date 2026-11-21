// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The contact object's own write and collision paths.
//
// Split from csvwriters.go and csvcollision.go for the reason csvcontactfields.go
// is split from csvfields.go: a contact's identity is an email held in a child
// table with estate-wide uniqueness behind it, so both paths answer differently
// from the company ones beside them. A duplicate company name creates a twin and
// files a review pair; a duplicate contact email is REFUSED, because one is a real
// key and the other is not. Keeping the two apart is what stops either answer
// being read as the general rule.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/importkeys"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// createContact lands one row as a contact, through the same CreateContactTx seam
// the flip's own bulk contact import uses — so the row runs the identity ladder,
// and the contact and its identity-map row commit or roll back together.
//
// An address that already belongs to someone else is a SKIP with a reason, never
// an automatic merge: two records that look like one contact is a question for a
// human, and answering it by writing is the one move no undo reverses.
// different seam, a different required field and a different reason for
// skipping. Folding them together would put one branch between the two
// record types and hide which rule each row was judged by.
//
//nolint:dupl // createCompany has the same SHAPE and not the same job: a
func (w *csvWriters) createContact(ctx context.Context, row migration.Row, typed map[string]any) (migration.EnsureResult, error) {
	in, err := w.contactInputForRow(ctx, row, typed)
	if err != nil {
		return migration.EnsureResult{}, err
	}
	var employer contacts.EmployerAttached
	in.EmployerResult = &employer
	err = w.land(ctx, row, func(tx pgx.Tx) (ids.UUID, error) {
		if err := w.authors.resolve(ctx, tx, &in.Author); err != nil {
			return ids.UUID{}, err
		}
		contact, err := w.contacts.CreateContactTx(ctx, tx, in, w.cols.store)
		if err != nil {
			return ids.UUID{}, fmt.Errorf("import: creating contact %s: %w", row.ExternalID, err)
		}
		if err := w.recordCreatedEmployer(ctx, tx, employer); err != nil {
			return ids.UUID{}, err
		}
		return ids.UUID(contact.Id), nil
	})
	// The row's own transaction has ended, so a colleague holding the address
	// is asked to share it, as on every other create door.
	err = w.contacts.AskHolderToShare(ctx, err)
	_, leadHolds := errors.AsType[*contacts.LeadHoldsIdentityError](err)
	if contacts.IsClaimedEmail(err) || leadHolds {
		// The same sentence the preview used, so an approval decided on
		// "skipped, the address is held" is not answered by different words.
		// A live lead's address is held too: an import never converts a lead.
		return skippedFor(contactEmailClaimedWhy), nil
	}
	if refusedAsErased(err) {
		return skippedFor(addressErasedWhy), nil
	}
	if why, refused := employerRefusalWhy(err); refused {
		return skippedFor(why), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	result := migration.EnsureResult{Created: true}
	if warning := importedContactChannelWarning(in); warning != nil {
		result.Warnings = []migration.SkippedRow{*warning}
	}

	return result, nil
}

// applyContact writes a contact's changed fields. It merges the file's address,
// email and phone onto what the contact holds, because the store replaces each
// of those wholesale. The store merges the LinkedIn handle itself.
func (w *csvWriters) applyContact(
	ctx context.Context, id ids.UUID, in contacts.UpdateContactInput, changed map[string]string, current []byte, source string,
) error {
	in.IfVersion = w.reviewedVersion(id)
	// Emails and phones added by an update carry the provenance the create
	// path stamps; without it their source lands empty.
	in.Source = source
	merged, given, err := addressMergedOnto(current, in.Address, changed)
	if err != nil {
		return err
	}
	if given {
		in.Address = merged
	}
	mergedEmails, given, err := emailsMergedOnto(current, in.Emails)
	if err != nil {
		return err
	}
	if given {
		in.Emails = mergedEmails
	}
	mergedPhones, given, err := phonesMergedOnto(current, in.Phones)
	if err != nil {
		return err
	}
	if given {
		in.Phones = mergedPhones
	}
	_, err = w.contacts.UpdateContact(ctx, ids.From[ids.ContactKind](id), in)
	return err
}

// contactEmailIsClaimed answers whether the estate already holds this row's
// address, WITHOUT regard to who holds it.
//
// It is deliberately not visibility-filtered, and that is the opposite of the
// rule the collision check below follows. The reason is that the two answer
// different questions. A duplicate company NAME is a judgement — the commit
// creates a twin and files a review pair — so telling a caller about an
// incumbent they cannot see would disclose one. A duplicate email is not a
// judgement: uq_contact_email_dedupe is estate-wide, so the commit REFUSES the
// row whoever holds the address, and a preview that promised a create would
// simply be wrong.
//
// So the preview reports the skip that will happen. What it must not do is say
// why, and it does not: the reason names the row's own address, which the caller
// supplied, and never the incumbent or their existence beyond the fact that this
// address is spoken for.
func (w *csvWriters) contactEmailAlreadyHeld(ctx context.Context, row migration.Row) (bool, error) {
	if w.object != migration.ObjectContact {
		return false, nil
	}
	email := strings.TrimSpace(textFields(row.Fields)[fieldEmail])
	if email == "" {
		return false, nil
	}
	if claimed, read := w.heldEmails[email]; read {
		return claimed, nil
	}
	held, err := importkeys.EmailsHeld(ctx, w.pool, []string{email})
	if err != nil {
		return false, err
	}
	return held[email], nil
}

// readPageEmails answers, in one query, which addresses of a dry run's page a
// live contact holds. Nothing writes during a dry run, so the answer holds
// for every row of the page.
func (w *csvWriters) readPageEmails(ctx context.Context, rows []migration.Row) error {
	if w.object != migration.ObjectContact {
		return nil
	}
	var emails []string
	for _, row := range rows {
		if email := strings.TrimSpace(textFields(row.Fields)[fieldEmail]); email != "" {
			emails = append(emails, email)
		}
	}
	held, err := importkeys.EmailsHeld(ctx, w.pool, emails)
	if err != nil {
		return err
	}
	w.heldEmails = held
	return nil
}

// contactCollides answers whether this row names someone the caller can already
// see, running the same ladder the create path runs.
//
// Like the company arm, it asks about every ranked match. An invisible record
// ranked first must not hide a visible one.
func (w *csvWriters) contactCollides(ctx context.Context, row migration.Row) (bool, error) {
	fields := textFields(row.Fields)
	email := strings.TrimSpace(fields[fieldEmail])
	name := contactFullName(fields)
	if email == "" && name == "" {
		return false, nil
	}
	var visible bool
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		// The workspace's own free-mail list, read through capture.MatcherTx. On
		// the shipped baseline alone, a domain an admin declared a company's
		// would score as free mail, and the preview would part from the commit.
		consumerMail, err := w.consumerMailList(ctx, tx)
		if err != nil {
			return err
		}
		asked := w.newContactOf(fields)
		candidate := contacts.ContactCandidate{FullName: asked.FullName, Emails: asked.Emails, ConsumerMail: consumerMail}
		match, err := contacts.DedupeContact(ctx, tx, candidate)
		if match = match.Without(w.links.batch.Changes); err != nil || match.Decision == contacts.DecisionNoMatch {
			return err
		}
		visible, err = contacts.AnyMatchVisible(ctx, tx, match)
		return err
	}); err != nil {
		named := email
		if named == "" {
			named = name
		}
		return false, fmt.Errorf("import: checking %q against the contacts already held: %w", named, err)
	}
	return visible, nil
}

// consumerMailList is the workspace's consumer-mail list, read on first use and
// kept for the run.
func (w *csvWriters) consumerMailList(ctx context.Context, tx pgx.Tx) (*freemail.Matcher, error) {
	if w.consumerMail == nil {
		matcher, err := capture.MatcherTx(ctx, tx)
		if err != nil {
			return nil, err
		}
		w.consumerMail = matcher
	}
	return w.consumerMail, nil
}

// leadKeyHeld names the skip for a row refused over a key a live lead holds.
// That is a lead row with another lead's address, or a contact row with a
// lead's. An import never converts, so the lead keeps its address.
func leadKeyHeld(err error) (migration.Reason, bool) {
	if _, ok := errors.AsType[*contacts.DuplicateLeadError](err); ok {
		return naturalKeyTakenWhy, true
	}
	if _, ok := errors.AsType[*contacts.LeadHoldsIdentityError](err); ok {
		return contactEmailClaimedWhy, true
	}
	return migration.Reason{}, false
}

func importedContactChannelWarning(in contacts.CreateContactInput) *migration.SkippedRow {
	if contacts.HasContactChannel(in) {
		return nil
	}
	why := migration.Why(migration.ReasonContactChannelMissing)
	return &migration.SkippedRow{Kind: migration.ProblemIncompleteIdentity, Reason: why.English(), Code: why.Code}
}
