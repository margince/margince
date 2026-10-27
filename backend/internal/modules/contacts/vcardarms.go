// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What one card does to the record it resolved to.
//
// Its own file for the reason the arms differ at all: one has an existing
// subject to hold and the other does not, and that difference decides the LOCK
// ORDER each has to keep. Reading them side by side is what makes the ordering
// checkable; buried inside the dedupe switch they read as two similar blocks.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// updateMatchedByCard applies a card to the contact it matched exactly.
//
// HOLD rather than probe, and the order of the two gates is load-bearing. This
// arm goes on to WRITE the contact, and the source check below takes a lock of
// its own — the eraser is subject-first (privacy/erasure.go anonymizes the
// subject's rows before deleting what hangs off it), so this transaction must
// hold the subject before it holds anything else or the two deadlock and an
// Art. 17 fulfilment fails on a path an ordinary seat can drive.
func (s *Store) updateMatchedByCard(
	ctx context.Context, tx pgx.Tx, match ContactResolution,
	entry VCardEntry, source ids.UUID, result *VCardResult,
) error {
	contactID := match.ContactID
	// The card names somebody who exists, so the caller's authority over THAT
	// ROW decides whether this card may touch it — the contact:create grant that
	// let them start the import says nothing about a record they cannot see.
	//
	// Reported as a skip rather than skipped silently: an import that quietly
	// leaves one card out is an import nobody can audit, and the reader can act
	// on "you may not write this one".
	if err := auth.HoldWritableLive(ctx, tx, entityContact, contactID.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) && match.MatchedLane == LaneEmail && len(entry.Emails) > 0 {
			// A colleague's contact the importer cannot see holds the address:
			// the held refusal, which fails this card's transaction.
			if refusal := claimedEmailRefusal(ctx, tx, entry.Emails[0].Value, contactID); !isDuplicateEmail(refusal) {
				return refusal
			}
		}
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			// Deliberately says nothing about WHAT was matched. The two refusals
			// are one sentence because telling them apart would confirm that the
			// address on this card belongs to a real contact in this workspace,
			// to somebody who may not see that contact.
			result.Outcome = VCardSkipped
			result.Reason = "this card could not be written here"
			return nil
		}
		return fmt.Errorf("probing write authority over the matched contact: %w", err)
	}
	narrowed, err := sourceIsNarrowed(ctx, tx, source)
	if err != nil {
		return err
	}
	if narrowed {
		result.Outcome = VCardSkipped
		result.Reason = vcardSourceNarrowed
		return nil
	}
	result.Outcome = VCardUpdated
	result.ContactID = &contactID
	st, err := s.fillFromVCard(ctx, tx, contactID, entry)
	if err != nil {
		return err
	}
	address, err := fillAddressFromCard(ctx, tx, contactID, entry, st.corrected[fieldAddress])
	if err != nil {
		return err
	}
	held, err := liveEmails(ctx, tx, contactID)
	if err != nil {
		return err
	}
	result.Fields = updatedReport(entry, st, held, address)
	return nil
}

// createFromCard turns a card nobody matched into a contact.
//
// The source is checked AFTER the contact is minted, and that ordering is the
// deadlock rule rather than a preference. This arm has no prior subject to hold
// — the contact does not exist yet — so it cannot open with a subject lock the
// way updateMatchedByCard does. What it can do is take its rows in the same
// DIRECTION the eraser does: privacy/erasure.go deletes contact_email (its
// `email = ANY(...)` arm reaches an address held by an archived record) and only
// then locks the subject's activities. Taking the activity lock first would put
// this transaction holding the message and waiting on an email the eraser holds
// while that eraser waits on the message — and the loser of a 40P01 there can be
// the Art. 17 fulfilment, which fails on a path ordinary inbound mail drives.
//
// The guarantee is unchanged by the move: the check and the write are still one
// transaction, so a narrowing cannot commit between them. Only the order in
// which this transaction takes its two locks changes, and the rollback that
// follows a refusal undoes the contact as if it had never been minted.
func (s *Store) createFromCard(
	ctx context.Context, tx pgx.Tx, entry VCardEntry,
	source ids.UUID, result *VCardResult,
) error {
	contact, err := s.CreateContactTx(ctx, tx, contactFromVCard(entry), CustomColumns{})
	if err != nil {
		return err
	}
	narrowed, err := sourceIsNarrowed(ctx, tx, source)
	if err != nil {
		return err
	}
	if narrowed {
		// The contact minted above goes with the transaction. Reported as a skip
		// so the caller sees a refusal rather than a create it cannot find.
		result.Outcome = VCardSkipped
		result.Reason = vcardSourceNarrowed
		return errCardSourceNarrowed
	}
	created := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	result.Outcome = VCardCreated
	result.ContactID = &created
	url, err := s.completeCardCreate(ctx, tx, contact, entry)
	if err != nil {
		return err
	}
	result.Fields = createdReport(contact, entry, url)
	return nil
}

// completeCardCreate writes what the contact create does not carry: the
// employer the card names, and a URL the social slot did not take.
//
// That URL is usually the company site, and it lands as evidence as it does on
// a matched contact. It returns that URL's outcome, or nil when none was filed.
func (s *Store) completeCardCreate(ctx context.Context, tx pgx.Tx, contact crmcontracts.Contact, entry VCardEntry) (*statedField, error) {
	created := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	if err := s.attachVCardEmployer(ctx, tx, created, entry); err != nil {
		return nil, err
	}
	url, ok := cardURLField(entry)
	if !ok || (contact.Social != nil && (*contact.Social)[profileURLField] != nil) {
		return nil, nil //nolint:nilnil // a card whose URL the social slot took files nothing more
	}
	card, err := readyObservedCard(ctx, tx, entry)
	if err != nil {
		return nil, err
	}
	outcome, err := applyObservedField(ctx, tx, created, observedField{
		Field: url.name, Value: url.value, Evidence: card.Evidence, SourceRef: card.SourceRef,
		Source: card.Source, CapturedBy: card.CapturedBy, ObservedAt: card.ObservedAt,
	})
	if err != nil {
		return nil, err
	}
	if outcome.landed() {
		if err := auditCardFields(ctx, tx, created, []string{url.name}); err != nil {
			return nil, err
		}
	}
	return &statedField{cardField: url, outcome: outcome}, nil
}

// errCardSourceNarrowed rolls back the contact a refused card had already minted.
//
// It never leaves importOneVCard: the create arm has to write before it may take
// the source lock (see above), so the only way to undo that write is to fail the
// transaction. The caller recognises this one error and reports the card's own
// outcome, which the arm has already filled in.
var errCardSourceNarrowed = errors.New("contacts: the message this card arrived on is no longer readable here")

// vcardRecordAddress is the card's address in the record's parts. A card
// staged before the parts were read carries only the one line, which goes
// through the splitter every other line-1 writer uses.
func vcardRecordAddress(entry VCardEntry) *crmcontracts.Address {
	if parts := addressOrNil(entry.PostalAddress); parts != nil {
		return parts
	}
	line := strings.TrimSpace(entry.Address)
	if line == "" {
		return nil
	}
	if split, ok := SplitAddressText(line); ok {
		return &split
	}
	return &crmcontracts.Address{Line1: &line}
}

// skipHeldCard answers a card whose address a colleague's contact holds. The
// colleague is asked to share it, and the card is skipped with the refusal's
// own words. Any other error passes through. A mailed card is the machine's
// work: a live lead's card is skipped with the reason and offers no convert.
func (s *Store) skipHeldCard(ctx context.Context, err error, index int, name string, mailed bool) (VCardResult, error) {
	err = s.AskHolderToShare(ctx, err)
	if held, ok := errors.AsType[*ContactHeldByColleagueError](err); ok {
		return VCardResult{Index: index, FullName: name, Outcome: VCardSkipped, Reason: held.Error()}, nil
	}
	if _, erased := errors.AsType[*storekit.ErasedIdentifierError](err); erased {
		return VCardResult{Index: index, FullName: name, Outcome: VCardSkipped, Reason: vcardErasedAddress}, nil
	}
	if held, ok := errors.AsType[*LeadHoldsIdentityError](err); ok {
		if mailed {
			return VCardResult{Index: index, FullName: name, Outcome: VCardSkipped, Reason: vcardLeadHoldsMailed}, nil
		}
		skipped := VCardResult{Index: index, FullName: name, Outcome: VCardSkipped, Reason: vcardLeadHolds}
		if len(held.LeadIDs) > 0 {
			skipped.LeadID = &held.LeadIDs[0]
		}
		return skipped, nil
	}
	return VCardResult{}, err
}

// vcardLeadHolds is the reason a card for somebody who is already a lead creates
// nothing. Somebody is a lead or a contact, never both.
const vcardLeadHolds = "this card names somebody who is already a lead; convert the lead instead"

// vcardLeadHoldsMailed is the same skip for a card that came with mail, which
// nobody is there to answer. The lead is left as it is.
const vcardLeadHoldsMailed = "this card names somebody who is already a lead, so it was not imported"

// vcardExistsReason is what a model-read card gets when its address already
// belongs to a contact. A reading of a photo is a guess about the print, and a
// guess must not overwrite a record.
const vcardExistsReason = "this contact already exists; a card read from a photo never changes an existing contact"

// skipModelReadMatch reports a model-read card whose address matches a contact.
// The contact is named only when the caller may read it, as in the near-match
// arm of importOneVCard.
func skipModelReadMatch(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, result *VCardResult) error {
	result.Outcome = VCardSkipped
	result.Reason = vcardExistsReason
	visible, err := auth.VisibleTo(ctx, tx, entityContact, contactID.UUID)
	if err != nil {
		return fmt.Errorf("contacts: probing a matched contact's visibility: %w", err)
	}
	if visible {
		result.ContactID = &contactID
	}
	return nil
}

// proposeModelReadCard reports a model-read card nobody matched. Nothing is
// written: the staging call that follows turns it into an approval, and the
// contact exists only once a human accepts that.
func proposeModelReadCard(ctx context.Context, tx pgx.Tx, source ids.UUID, result *VCardResult) error {
	narrowed, err := sourceIsNarrowed(ctx, tx, source)
	if err != nil {
		return err
	}
	if narrowed {
		result.Outcome = VCardSkipped
		result.Reason = vcardSourceNarrowed
		return nil
	}
	result.Outcome = VCardProposed
	return nil
}

// attachVCardEmployer creates the company the card names and the employment
// edge to it. A card with no COMPANY leaves the contact unemployed, which is a
// contact.
func (s *Store) attachVCardEmployer(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, entry VCardEntry) error {
	name := strings.TrimSpace(entry.Company)
	if name == "" {
		return nil
	}
	companyID, err := s.employerByName(ctx, tx, name)
	if err != nil {
		return err
	}
	return s.ensureEmployment(ctx, tx, contactID, *companyID, entry.Title, vcardSource)
}

// ensureEmployment gives a contact a live employment at a company, unless it
// already holds one there. The vCard import and a lead's promotion write the
// edge through here.
func (s *Store) ensureEmployment(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, companyID ids.CompanyID, role, source string) error {
	held, err := holdsLiveEmployment(ctx, tx, contactID, companyID)
	if err != nil || held {
		return err
	}
	edge := CreateRelationshipInput{
		Kind:      employmentKind,
		ContactID: &contactID,
		CompanyID: &companyID,
		Source:    source,
	}
	if role = strings.TrimSpace(role); role != "" {
		edge.Role = &role
	}
	_, err = s.CreateRelationshipTx(ctx, tx, edge)
	return err
}

// holdsLiveEmployment reports whether a contact holds a live employment at a company.
func holdsLiveEmployment(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, companyID ids.CompanyID) (bool, error) {
	var held bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM relationship r
		                WHERE r.contact_id = $1 AND r.company_id = $2 AND `+employment.LiveSlotSQL("r")+`)`,
		contactID, companyID).Scan(&held); err != nil {
		return false, fmt.Errorf("looking for the contact's employment at the company: %w", err)
	}
	return held, nil
}

// employerByName finds the company the card names, or creates it.
//
// Without the lookup, two cards from one company create two companies.
// Every employee of Acme arrives with `ORG:Acme`. A create-only path turns a
// ten-card export into ten Acmes that a human then has to merge.
func (s *Store) employerByName(ctx context.Context, tx pgx.Tx, name string) (*ids.CompanyID, error) {
	var existing ids.CompanyID
	err := tx.QueryRow(ctx, `
		SELECT id FROM company
		 WHERE lower(display_name) = lower($1) AND archived_at IS NULL AND merged_into_id IS NULL
		 ORDER BY created_at
		 LIMIT 1`, name).Scan(&existing)
	switch {
	case err == nil:
		return &existing, nil
	case errors.Is(err, pgx.ErrNoRows):
		company, createErr := s.CreateCompanyTx(ctx, tx, CreateCompanyInput{
			DisplayName: name,
			Source:      vcardSource,
		}, CustomColumns{})
		if createErr != nil {
			return nil, createErr
		}
		made := ids.From[ids.CompanyKind](ids.UUID(company.Id))
		return &made, nil
	default:
		return nil, fmt.Errorf("looking for the card's employer: %w", err)
	}
}
