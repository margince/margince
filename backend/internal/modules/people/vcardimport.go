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
// match fills only what is empty; no match creates; and a card that merely
// LOOKS like somebody is returned for a human to judge rather than merged.
// Guessing there is how one person becomes two records, or two people become
// one, and neither is recoverable by the reader who imported the file.

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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// vcardSource is the provenance every row this import writes carries.
const vcardSource = "vcard_import"

// VCardOutcome is what became of one card.
type VCardOutcome string

const (
	// VCardCreated means nobody matched, so the card became a person.
	VCardCreated VCardOutcome = "created"
	// VCardUpdated means an exact match, filled only where it was empty. A value a
	// human typed is never overwritten by a card.
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
		result, err := s.importOneVCard(ctx, i, entry)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Store) importOneVCard(ctx context.Context, index int, entry VCardEntry) (VCardResult, error) {
	result := VCardResult{Index: index, FullName: strings.TrimSpace(entry.FullName)}
	if result.FullName == "" {
		result.Outcome = VCardSkipped
		result.Reason = "the card states no name"
		return result, nil
	}

	err := s.tx(ctx, func(tx pgx.Tx) error {
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
			if err := auth.EnsureWritableLive(ctx, tx, entityPerson, decision.PersonID.UUID); err != nil {
				if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
					// Deliberately says nothing about WHAT was matched. The two
					// refusals are one sentence because telling them apart
					// would confirm that the address on this card belongs to a
					// real contact in this workspace, to somebody who may not
					// see that contact.
					result.Outcome = VCardSkipped
					result.Reason = "this card could not be written here"
					return nil
				}
				return fmt.Errorf("probing write authority over the matched person: %w", err)
			}
			result.Outcome = VCardUpdated
			result.PersonID = &decision.PersonID
			return s.fillFromVCard(ctx, tx, decision.PersonID, entry)
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
			person, err := s.CreatePersonTx(ctx, tx, personFromVCard(entry))
			if err != nil {
				return err
			}
			created := ids.From[ids.PersonKind](ids.UUID(person.Id))
			result.Outcome = VCardCreated
			result.PersonID = &created
			return s.attachVCardEmployer(ctx, tx, created, entry)
		}
	})
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

func personFromVCard(entry VCardEntry) CreatePersonInput {
	person := CreatePersonInput{
		FullName: strings.TrimSpace(entry.FullName),
		Source:   vcardSource,
	}
	if title := strings.TrimSpace(entry.Title); title != "" {
		person.Title = &title
	}
	if url := strings.TrimSpace(entry.URL); url != "" {
		person.Social = map[string]any{profileURLField: url}
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
// exists, filling only what is empty.
//
// Modelled on ApplySitePersonFields, and for the same reason: a human's own
// entry outranks anything a machine read or a file stated, so a value already
// on the record stays. What differs is that the human pressed the button, so
// this writes rather than staging.
func (s *Store) fillFromVCard(ctx context.Context, tx pgx.Tx, personID ids.PersonID, entry VCardEntry) error {
	fields := SitePersonFields{
		Name:        strings.TrimSpace(entry.FullName),
		Role:        strings.TrimSpace(entry.Title),
		LinkedinURL: strings.TrimSpace(entry.URL),
		// The card itself is the evidence, and there is no page to quote: the
		// snippet is what the card stated, which is what an Art. 15 answer has
		// to be able to show.
		EvidenceSnippet: vcardEvidence(entry),
		SourceURL:       vcardSource,
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	sourceRef := vcardSource
	applied, previous, values, err := fillSitePersonFields(ctx, tx, personID, sourceRef, by, vcardSource, fields)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		// The card said nothing this record did not already hold. There is no
		// change to audit, and an audit row with an empty after-image would
		// report a write that did not happen.
		return nil
	}
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

// CreateFromVCardReview creates the person a reviewed card proposed, after a
// human decided the near-match is somebody else.
//
// The dedupe pass deliberately does NOT run again: the decision being executed
// IS the answer to the question that pass would re-ask, and re-asking it would
// stage a second review of a review. The creating authority is the caller's —
// the deciding human — so the store's own create gate answers for them.
func (s *Store) CreateFromVCardReview(ctx context.Context, entry VCardEntry) (ids.PersonID, error) {
	var created ids.PersonID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		person, err := s.CreatePersonTx(ctx, tx, personFromVCard(entry))
		if err != nil {
			return err
		}
		created = ids.From[ids.PersonKind](ids.UUID(person.Id))
		return s.attachVCardEmployer(ctx, tx, created, entry)
	})
	if err != nil {
		return ids.PersonID{}, fmt.Errorf("people: creating the reviewed card's person: %w", err)
	}
	return created, nil
}
