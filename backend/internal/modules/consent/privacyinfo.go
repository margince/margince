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
	// AcquiredAt is when the acquisition happened. Where the creating door did
	// not record a time, the evidence row's own write stands in — the same
	// coalesce the notice-case deadline runs on, so the page and the duty date
	// the acquisition alike. Nil only where there is no evidence row at all.
	AcquiredAt *time.Time
	// Purposes are what the installation uses this contact's data for, by
	// published name.
	Purposes []string
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
// It reads the EARLIEST acquisition, because that is the one whose duty is
// being discharged: a contact obtained from a list in March and met again at a
// conference in June was first obtained in March, and the disclosure is about
// how they came to be in the system at all.
//
// It carries no name, no employer, no address and no phone. The message said we
// hold information about you and here is what and why; it did not offer to show
// somebody their file, and a link that arrives unbidden should disclose the
// minimum the duty requires rather than the maximum the system knows.
func (s *Store) assembleDisclosure(
	ctx context.Context, contactID ids.ContactID,
) (PrivacyInformation, error) {
	out := PrivacyInformation{Rights: subjectRights(), Purposes: []string{}}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT kind, coalesce(occurred_at, captured_at)
			  FROM contact_acquisition_evidence
			 WHERE contact_id = $1
			 ORDER BY coalesce(occurred_at, captured_at), id
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
		rows, err := tx.Query(ctx, `
			SELECT label FROM consent_purpose
			 WHERE archived_at IS NULL
			 ORDER BY label`)
		if err != nil {
			return fmt.Errorf("consent: reading what this installation uses data for: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var label string
			if err := rows.Scan(&label); err != nil {
				return err
			}
			out.Purposes = append(out.Purposes, label)
		}
		return rows.Err()
	})
	return out, err
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
	return s.assembleDisclosure(ctx, ref.ContactID)
}
