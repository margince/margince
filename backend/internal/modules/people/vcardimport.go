// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Turning parsed cards into people.
//
// A handed-over card is first-party data: the person gave it, which is what
// justifies storing their contact details, and a human pressed the button,
// which is what lets this path WRITE rather than stage. That is the same rule
// the site read applies from the other side — an automatic read proposes, a
// human click writes.
//
// Three outcomes per card, and the third is the one that matters. An exact
// match applies what the card states by recency, the way a signature does; no
// match creates; and a card that merely
// LOOKS like somebody is returned for a human to judge rather than merged.
// Guessing there is how one person becomes two records, or two people become
// one, and neither is recoverable by the reader who imported the file.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// vcardSource is the provenance every row this import writes carries.
const vcardSource = "vcard_import"

// vcardFieldStated marks a field this import answered. The image says WHICH
// field moved and never what it moved to: audit_log is append-only, and this
// path writes a phone number and a postal address off a file somebody handed
// over.
const vcardFieldStated = "stated"

// VCardOutcome is what became of one card.
type VCardOutcome string

const (
	// VCardCreated means nobody matched, so the card became a person.
	VCardCreated VCardOutcome = "created"
	// VCardUpdated means an exact match, updated by recency: a card is the
	// contact stating their own details, so what it says replaces what the
	// record holds and the replaced value is kept for one click of undo.
	VCardUpdated VCardOutcome = "updated"
	// VCardNeedsReview means something close enough to be the same person and not
	// close enough to be sure. The card is not written; the candidate is
	// named so a human can open it.
	VCardNeedsReview VCardOutcome = "needs_review"
	// VCardSkipped means the card could not be written: it states no name, or
	// it matches a contact this caller may not write.
	VCardSkipped VCardOutcome = "skipped"
)

// VCardResult is one card's outcome, in the order the file listed them.
type VCardResult struct {
	Index    int
	FullName string
	Outcome  VCardOutcome
	// PersonID is the person created, updated, or — for a review — the
	// candidate the card resembles.
	PersonID *ids.PersonID
	// Reason says why a card was skipped, in words a reader can act on.
	Reason string
}

// ImportVCards writes every card in one file and reports what became of each.
//
// One transaction per CARD rather than one for the file: a file of forty cards
// with one bad row should import thirty-nine people, and a reader who has to
// find and fix the one row before any of it lands is a reader who gives up. The
// per-card outcome list is what makes that legible.
func (s *Store) ImportVCards(ctx context.Context, entries []VCardEntry) ([]VCardResult, error) {
	return s.importCardsFrom(ctx, entries, ids.Nil)
}

// ImportVCardsFromMessage imports cards that arrived as an attachment, holding
// every card's write against the message they came from.
//
// The difference from ImportVCards is the reason the second entry point exists:
// a browser upload is a human holding a file, and no source row's audience can
// change under it. A MAILED card has one, and the gap between reading the
// attachment and writing the person is real — an object-store fetch per
// attachment, then a transaction per card. A narrowing, a restriction or an
// erasure committing in that gap would otherwise land the card anyway, and what
// lands is a name, a number and a postal address on a record every seat reads,
// which the narrowing does not reach.
//
// The source is re-read INSIDE each card's own transaction, as the first
// statement there. A check taken before the transaction is a check with a gap
// after it; this one holds for as long as the write it guards.
func (s *Store) ImportVCardsFromMessage(ctx context.Context, entries []VCardEntry, source ids.UUID) ([]VCardResult, error) {
	// A zero id would silently import with no guard at all, which is the one
	// outcome this entry point exists to prevent — so it refuses rather than
	// falling back to the unguarded path.
	if source.IsZero() {
		return nil, errors.New("people: importing cards from a message that names no message")
	}
	return s.importCardsFrom(ctx, entries, source)
}

func (s *Store) importCardsFrom(ctx context.Context, entries []VCardEntry, source ids.UUID) ([]VCardResult, error) {
	// Both grants, up front. The import CREATES people and it UPDATES the ones
	// a card already matches, and those are two different permissions: the
	// create grant that lets somebody start an import says nothing about
	// changing a record that already exists, which is what UpdatePerson asks
	// for on every other path into the same rows.
	if err := auth.Require(ctx, entityPerson, principal.ActionCreate); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, entityPerson, principal.ActionUpdate); err != nil {
		return nil, err
	}
	if len(entries) > vcardMaxCards {
		return nil, &TooManyCardsError{Limit: vcardMaxCards}
	}
	results := make([]VCardResult, 0, len(entries))
	for i, entry := range entries {
		result, err := s.importOneVCard(ctx, i, entry, source)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Store) importOneVCard(ctx context.Context, index int, entry VCardEntry, source ids.UUID) (VCardResult, error) {
	result := VCardResult{Index: index, FullName: strings.TrimSpace(entry.FullName)}
	if result.FullName == "" {
		result.Outcome = VCardSkipped
		result.Reason = "the card states no name"
		return result, nil
	}

	err := s.tx(ctx, func(tx pgx.Tx) error {
		// The dedupe first, and it takes no row lock — it is a read, so it
		// orders nothing. What comes after it is ordered, and strictly: the
		// SUBJECT before the source, on every arm.
		decision, err := DedupePerson(ctx, tx, vcardCandidate(entry))
		if err != nil {
			return err
		}
		switch decision.Decision {
		case DecisionExactCollision:
			// The card names somebody who exists, so the caller's authority
			// over THAT ROW decides whether this card may touch it — the
			// person:create grant that let them start the import says nothing
			// about a record they cannot see.
			//
			// Reported as a skip rather than skipped silently: an import that
			// quietly leaves one card out is an import nobody can audit, and
			// the reader can act on "you may not write this one".
			return s.updateMatchedByCard(ctx, tx, decision.PersonID, entry, source, &result)
		case DecisionFuzzyReview, DecisionNameCollisionReview:
			// Named, not merged, and not created either: creating beside a
			// near-match is how a file of forty cards quietly doubles a
			// contact list.
			//
			// The candidate is named only when the caller may READ it. The
			// dedupe lanes search the whole workspace by design — that is what
			// makes them able to find a duplicate at all — so handing the id
			// back unchecked would turn an upload into a lookup for records
			// outside the caller's scope.
			// The source, before the candidate is even named. This arm writes
			// nothing itself, but what it RETURNS becomes a durable proposal
			// carrying the whole card — so a narrowed message would reach a
			// review queue instead of a person record, which is the same leak
			// in a different table.
			narrowed, err := sourceIsNarrowed(ctx, tx, source)
			if err != nil {
				return err
			}
			if narrowed {
				result.Outcome = VCardSkipped
				result.Reason = vcardSourceNarrowed
				return nil
			}
			result.Outcome = VCardNeedsReview
			if err := auth.EnsureVisible(ctx, tx, entityPerson, decision.PersonID.UUID); err != nil {
				if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
					result.Reason = "this card resembles an existing contact"
					return nil
				}
				return fmt.Errorf("probing visibility of the candidate: %w", err)
			}
			result.PersonID = &decision.PersonID
			return nil
		default:
			return s.createFromCard(ctx, tx, entry, source, &result)
		}
	})
	// The create arm signals a refusal by failing its transaction, because the
	// person it had to mint before it could take the source lock must not
	// survive. The outcome it filled in is the answer; the error is only how the
	// rollback was reached.
	if errors.Is(err, errCardSourceNarrowed) {
		return result, nil
	}
	if err != nil {
		return VCardResult{}, fmt.Errorf("people: importing card %d: %w", index+1, err)
	}
	return result, nil
}

// vcardCandidate is the card as the dedupe lanes read it: the email is the
// exact key, the name and company are what the fuzzy tier scores.
func vcardCandidate(entry VCardEntry) PersonCandidate {
	candidate := PersonCandidate{
		FullName: strings.TrimSpace(entry.FullName),
		// Name collisions ARE wanted here: this caller creates, so two records
		// written the same way is a question worth putting to a human. The
		// opt-in exists to keep routing callers away from it, and an import is
		// not one — it never delivers a message to the person it matched.
		QueueNameCollisions: true,
	}
	// Every address the card states, not just the first: the exact tier checks
	// them all, and a person reachable two ways matches on either.
	for _, email := range entry.Emails {
		if v := strings.ToLower(strings.TrimSpace(email.Value)); v != "" {
			candidate.Emails = append(candidate.Emails, v)
		}
	}
	return candidate
}

// ValidateVCardContacts answers whether a reviewed card's addresses and
// numbers are ones the create will accept, without writing anything.
//
// It asks by BUILDING the create's own input and running the create's own
// parser over it, rather than restating the rules: a second copy of "what is
// a valid number here" is the thing that drifts, and the whole point of
// asking early is that the answer matches what the writer will say.
//
// Nothing is normalised for the caller. The parse mutates the input it is
// given, and the input here is built fresh and thrown away — the proposal on
// the row is left exactly as the reviewer sees it.
func ValidateVCardContacts(entry VCardEntry) error {
	person := personFromVCard(entry)
	return parsePersonContacts(person.Emails, person.Phones)
}

func personFromVCard(entry VCardEntry) CreatePersonInput {
	person := CreatePersonInput{
		FullName: strings.TrimSpace(entry.FullName),
		Source:   vcardSource,
	}
	if title := strings.TrimSpace(entry.Title); title != "" {
		person.Title = &title
	}
	// Only a LinkedIn URL goes in the social slot, whose key IS "linkedin". A
	// card's URL is usually the company site, and filing that here showed a
	// company's home page as the person's LinkedIn profile. The website lands
	// on the profile-field sidecar instead, by the same split the update path
	// makes — one rule, so which outcome a card gets cannot change what it
	// means.
	if u := strings.TrimSpace(entry.URL); u != "" && isLinkedinURL(u) {
		person.Social = map[string]any{profileURLField: u}
	}
	// The card's postal address, as the one line it was reduced to. Parsing a
	// field and then dropping it is the quieter defect: a reader who sees an
	// address on the card they just imported expects to find it on the record.
	if street := strings.TrimSpace(entry.Address); street != "" {
		person.Address = &crmcontracts.Address{Line1: &street}
	}
	for i, email := range entry.Emails {
		person.Emails = append(person.Emails, PersonEmailInput{
			Email:     strings.ToLower(strings.TrimSpace(email.Value)),
			EmailType: email.Kind,
			IsPrimary: i == 0,
			Position:  i,
			// A card STATES an address; it is not correspondence, and the mail
			// ladder must not read it as a settled verdict about whether this
			// person writes to us.
			VouchedNotCorresponded: true,
		})
	}
	for i, phone := range entry.Phones {
		person.Phones = append(person.Phones, PersonPhoneInput{
			Phone:     strings.TrimSpace(phone.Value),
			PhoneType: phone.Kind,
			IsPrimary: i == 0,
			Position:  i,
		})
	}
	return person
}

// attachVCardEmployer creates the company the card names and the employment
// edge to it. A card with no ORG leaves the person unemployed, which is a
// person.
func (s *Store) attachVCardEmployer(ctx context.Context, tx pgx.Tx, personID ids.PersonID, entry VCardEntry) error {
	name := strings.TrimSpace(entry.Organization)
	if name == "" {
		return nil
	}
	orgID, err := s.employerByName(ctx, tx, name)
	if err != nil {
		return err
	}
	role := strings.TrimSpace(entry.Title)
	edge := CreateRelationshipInput{
		Kind:           employmentKind,
		PersonID:       &personID,
		OrganizationID: orgID,
		Source:         vcardSource,
	}
	if role != "" {
		edge.Role = &role
	}
	_, err = s.CreateRelationshipTx(ctx, tx, edge)
	return err
}

// employerByName finds the company the card names, or creates it.
//
// Without the lookup, two cards from the same company create two companies:
// every employee of Acme arrives with `ORG:Acme`, and a create-only path turns
// a ten-card export into ten Acmes that a human then has to merge.
func (s *Store) employerByName(ctx context.Context, tx pgx.Tx, name string) (*ids.OrganizationID, error) {
	var existing ids.OrganizationID
	err := tx.QueryRow(ctx, `
		SELECT id FROM organization
		 WHERE lower(display_name) = lower($1) AND archived_at IS NULL AND merged_into_id IS NULL
		 ORDER BY created_at
		 LIMIT 1`, name).Scan(&existing)
	switch {
	case err == nil:
		return &existing, nil
	case errors.Is(err, pgx.ErrNoRows):
		org, createErr := s.CreateOrganizationTx(ctx, tx, CreateOrganizationInput{
			DisplayName: name,
			Source:      vcardSource,
		})
		if createErr != nil {
			return nil, createErr
		}
		made := ids.From[ids.OrganizationKind](ids.UUID(org.Id))
		return &made, nil
	default:
		return nil, fmt.Errorf("looking for the card's employer: %w", err)
	}
}

// fillFromVCard writes the card's stated fields onto a person who already
// exists, by the same recency rule the signature pass obeys.
//
// A card is the contact stating their own details, exactly as a signature is,
// so it goes through the writer they now share. It used to fill only blanks and
// only three fields, which meant a handed-over card could not correct a number
// that had changed — and whether a stale value got fixed depended on whether
// the details arrived by mail or on paper.
//
// The card's own date is when it was handed over, which is now: unlike a mail,
// a .vcf carries no timestamp of its own, and the moment a human chose to
// import it is the most honest thing available. That also makes a card the
// newest statement on the record, which is the right answer — somebody is
// holding it and typing it in.
func (s *Store) fillFromVCard(ctx context.Context, tx pgx.Tx, personID ids.PersonID, entry VCardEntry) error {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	// The card itself is the evidence, and there is no page to quote: the
	// snippet is what the card stated, which is what an Art. 15 answer has to
	// be able to show.
	evidence := vcardEvidence(entry)
	// The card's own REV when it states one it can be read from, so importing
	// the same file twice states the same thing twice. Dated from the clock
	// instead, a re-upload would be a NEWER statement than everything since it,
	// and a reader re-uploading a file they were unsure landed would put back a
	// detail a signature had already corrected.
	//
	// A card with no REV, an unreadable one, or one dated in the FUTURE takes
	// the import's own clock — the behaviour every card had before this. The
	// future case is the security-relevant one and the check below says why.
	card := observedCard{
		Entry: entry, Evidence: evidence, SourceRef: vcardSource,
		Source: vcardSource, CapturedBy: by,
	}
	// A card's REV is written by whoever exported it, so it is attacker-supplied
	// like every other field on the card — and unlike the others it decides
	// what this card OUTRANKS. A REV in 2099 would win against every statement
	// the contact ever makes afterwards, permanently, from one file somebody
	// mailed in. A future date is therefore not read as a date: the import
	// falls back to its own clock, which is exactly the answer a card with no
	// REV already gets.
	if entry.Revised != nil {
		var now time.Time
		if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
			return fmt.Errorf("people: dating the card against the clock: %w", err)
		}
		if !entry.Revised.After(now) {
			card.ObservedAt = *entry.Revised
		}
	}
	applied, err := applyObservedCard(ctx, tx, personID, card)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		// The card said nothing this record did not already hold. There is no
		// change to audit, and an audit row with an empty after-image would
		// report a write that did not happen.
		return nil
	}
	// Named, not quoted, on both sides. This writes a phone number and a
	// postal address off a file somebody handed over, and audit_log is
	// append-only — a value placed in an image outlives the erasure that
	// clears the record it describes.
	previous := map[string]any{}
	values := map[string]any{}
	for _, f := range applied {
		previous[f] = nil
		values[f] = vcardFieldStated
	}
	sourceRef := vcardSource
	// The source travels in EVIDENCE rather than in the after-image: an
	// after-image key projects as a field change in the record's own history,
	// and "source" is not a field of a person.
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", entityPerson, personID.UUID,
		previous, values, map[string]any{auditKeySource: vcardSource, auditKeySourceRef: sourceRef})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, personID.UUID, crmcontracts.PublicEventPersonUpdated{
		ChangedFields: map[string]any{auditKeyFields: applied, auditKeySource: vcardSource},
	})
}

func vcardEvidence(entry VCardEntry) string {
	parts := []string{entry.FullName}
	if entry.Title != "" {
		parts = append(parts, entry.Title)
	}
	if entry.Organization != "" {
		parts = append(parts, entry.Organization)
	}
	return strings.Join(parts, " · ")
}

// CreateFromVCardReviewTx creates the person a reviewed card proposed, after
// a human decided the near-match is somebody else. It writes in the CALLER's
// transaction so the approval's redemption and the create it releases commit
// together — an approval consumed without its person, or a person without its
// consumed approval, are both states nothing can repair.
//
// The dedupe pass deliberately does NOT run again: the decision being executed
// IS the answer to the question that pass would re-ask, and re-asking it would
// stage a second review of a review. The creating authority is the caller's —
// the deciding human — so the store's own create gate answers for them.
func (s *Store) CreateFromVCardReviewTx(ctx context.Context, tx pgx.Tx, entry VCardEntry) (ids.PersonID, error) {
	person, err := s.CreatePersonTx(ctx, tx, personFromVCard(entry))
	if err != nil {
		return ids.PersonID{}, fmt.Errorf("people: creating the reviewed card's person: %w", err)
	}
	created := ids.From[ids.PersonKind](ids.UUID(person.Id))
	if err := s.attachVCardEmployer(ctx, tx, created, entry); err != nil {
		return ids.PersonID{}, fmt.Errorf("people: attaching the reviewed card's employer: %w", err)
	}
	return created, nil
}
