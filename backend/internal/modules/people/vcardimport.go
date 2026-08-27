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
					result.Outcome = VCardSkipped
					result.Reason = "this card matches a contact you may not write"
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
			result.Outcome = VCardNeedsReview
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
	org, err := s.CreateOrganizationTx(ctx, tx, CreateOrganizationInput{
		DisplayName: name,
		Source:      vcardSource,
	})
	if err != nil {
		return err
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))
	role := strings.TrimSpace(entry.Title)
	edge := CreateRelationshipInput{
		Kind:           employmentKind,
		PersonID:       &personID,
		OrganizationID: &orgID,
		Source:         vcardSource,
	}
	if role != "" {
		edge.Role = &role
	}
	_, err = s.CreateRelationshipTx(ctx, tx, edge)
	return err
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
