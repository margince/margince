// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What the installation holds about somebody, told to them.
//
// Art. 14 requires the controller to say, within a month of obtaining data from
// anywhere but the subject: that we hold it, where it came from, what we use it
// for, and the rights they have over it. It requires no answer.
//
// The record-confirmation page discharged the same duty and did more: it shows
// the contact's employer, phone, address and provenance trail, and asks whether
// they want to hear from us. Showing somebody their file is a different act
// from telling them one exists, and the marketing question inside a legal
// obligation is the arrangement a supervisory authority reads as consent
// obtained under pressure.
//
// This page is the narrower thing the duty actually asks for.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PrivacyInformation is one contact's Art. 14 disclosure.
type PrivacyInformation struct {
	// AcquiredAs is how this contact was obtained, in
	// contact_acquisition_evidence's own closed vocabulary. Art. 14(2)(f)
	// requires naming the source.
	AcquiredAs string
	// AcquiredAt is the actual acquisition date, including a later source assessment.
	// Capture time orders unknown history but does not stand in for acquisition.
	AcquiredAt *time.Time
	// Purposes are what the installation uses this contact's data for, by
	// published name. PurposeKeys holds each one's key at the same index, so a
	// page can name the purposes the product seeds in its reader's language.
	Purposes    []string
	PurposeKeys []string
	// Rights are the codes a page renders in its own language.
	Rights []string
}

// subjectRights is what Art. 14(2)(c)-(e) requires naming, in the order a
// reader meets them: see it, fix it, remove it, pause it, object to it, and
// complain to somebody who is not us.
//
// A literal rather than derived from what the product implements, and that is
// deliberate: these rights exist whether or not this installation has built a
// button for each, and a disclosure that listed only the implemented ones would
// be telling the subject they have fewer rights than the law gives them.
func subjectRights() []string {
	return []string{
		"access", "rectification", "erasure",
		"restriction", "objection", "complain_to_authority",
	}
}

// assembleDisclosure builds the Art. 14 answer for one contact.
//
// It reads the earliest known source, using an officer's assessment where present.
// Unknown history cannot displace a confirmed source in the disclosure.
//
// It carries no name, no employer, no address and no phone. The message said we
// hold information about you and here is what and why; it did not offer to show
// somebody their file, and a link that arrives unbidden should disclose the
// minimum the duty requires rather than the maximum the system knows.
func (s *Store) assembleDisclosure(
	ctx context.Context, contactID ids.ContactID,
) (PrivacyInformation, error) {
	out := PrivacyInformation{Rights: subjectRights(), Purposes: []string{}, PurposeKeys: []string{}}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT coalesce(n.source_kind, e.kind),
                   CASE WHEN n.source_kind IS NOT NULL THEN n.source_obtained_at ELSE e.occurred_at END
              FROM contact_acquisition_evidence e LEFT JOIN privacy_notice_case n ON n.acquisition_id = e.id
             WHERE e.contact_id = $1
             ORDER BY (coalesce(n.source_kind, e.kind) = 'unknown_legacy'),
                      coalesce(n.source_obtained_at, e.occurred_at, e.captured_at), e.id
			 LIMIT 1`, contactID).Scan(&out.AcquiredAs, &out.AcquiredAt)
		if errors.Is(err, pgx.ErrNoRows) {
			// A contact with no acquisition row predates the doors that write
			// one. The honest answer is the vocabulary's own word for it rather
			// than an empty string a page would render as a gap: we do not know
			// how this contact arrived, and saying so is the disclosure.
			out.AcquiredAs, out.AcquiredAt = acquiredUnknownLegacy, nil
		} else if err != nil {
			return fmt.Errorf("consent: reading how this contact was obtained: %w", err)
		}
		return readDisclosedPurposes(ctx, tx, &out)
	})
	return out, err
}

// assembleLeadDisclosure is assembleDisclosure for a website lead. Only a lead
// read off a company's public website holds a notice link. So that is its
// source, dated when the lead was captured.
func (s *Store) assembleLeadDisclosure(ctx context.Context, leadID ids.LeadID) (PrivacyInformation, error) {
	out := PrivacyInformation{
		Rights: subjectRights(), Purposes: []string{}, PurposeKeys: []string{},
		AcquiredAs: acquiredFromAPublicSource,
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var capturedAt time.Time
		if err := tx.QueryRow(ctx, `SELECT created_at FROM lead WHERE id = $1`, leadID).
			Scan(&capturedAt); err != nil {
			return fmt.Errorf("consent: reading when this lead was obtained: %w", err)
		}
		out.AcquiredAt = &capturedAt
		return readDisclosedPurposes(ctx, tx, &out)
	})
	return out, err
}

// acquiredFromAPublicSource mirrors contacts.AcquiredPublicOrBusinessSource:
// a company's public website is such a source.
//
// Held by: TestThePublicSourceAcquisitionIsSpelledTheSameOnBothSides
// (backend/gates/acquisitionvocabulary_test.go)
const acquiredFromAPublicSource = "public_or_business_source"

// readDisclosedPurposes fills in what the installation uses data for, which a
// contact's and a lead's disclosure state alike.
func readDisclosedPurposes(ctx context.Context, tx pgx.Tx, out *PrivacyInformation) error {
	rows, err := tx.Query(ctx, `
		SELECT key, label FROM consent_purpose
		 WHERE archived_at IS NULL
		 ORDER BY label`)
	if err != nil {
		return fmt.Errorf("consent: reading what this installation uses data for: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, label string
		if err := rows.Scan(&key, &label); err != nil {
			return err
		}
		out.Purposes = append(out.Purposes, label)
		out.PurposeKeys = append(out.PurposeKeys, key)
	}
	return rows.Err()
}

// acquiredUnknownLegacy mirrors contacts.AcquiredUnknownLegacy, which this
// module may not import — consent and contacts are siblings.
//
// Held by: TestTheUnknownAcquisitionIsSpelledTheSameOnBothSides
// (backend/gates/acquisitionvocabulary_test.go)
const acquiredUnknownLegacy = "unknown_legacy"

// PrivacyInformationFor answers the disclosure behind a notice link.
//
// Gated by the LINK rather than by a seat: whoever holds the plaintext proved
// the mailbox, which is the same authority every other confirm page runs on.
// The caller resolves the token and hands the reference in, so this function
// never sees a token itself.
//
// It therefore CANNOT enforce that a token was presented, and does not claim
// to. What it checks is the kind, so a reference for a record or consent link
// cannot be turned into an Art. 14 disclosure. The one HTTP path that reaches
// it resolves a token first (handlers_confirm.go), and that resolution is what
// makes the mailbox claim true.
func (s *Store) PrivacyInformationFor(
	ctx context.Context, ref ConfirmRef,
) (PrivacyInformation, error) {
	if ref.Kind != LinkPrivacyNotice {
		// Defensive, and it matters: this assembles a disclosure for whoever
		// holds the link, so reaching it with a link of another kind would
		// serve the Art. 14 page to somebody whose mail described something
		// else. The handler checks too; a second check costs nothing.
		return PrivacyInformation{}, apperrors.ErrNotFound
	}
	if !ref.LeadID.IsZero() {
		return s.assembleLeadDisclosure(ctx, ref.LeadID)
	}
	return s.assembleDisclosure(ctx, ref.ContactID)
}
