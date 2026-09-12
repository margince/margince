// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Why a contact exists, recorded where the contact is created.
//
// contact.source already says where a record came from — "manual", "capture", an
// import — and that is routinely mistaken for permission. It answers who typed
// it in, not what the contact did. A contact a rep entered from a business card
// and a contact who filled in a form are both `manual` to that column, and
// nothing on either row says which of them asked to hear from us.
//
// These kinds record the ACT instead, and they are deliberately not lawful
// bases: each names something that happened, which a basis would later be
// argued from. Keeping the two apart is the whole point — "they filled in a
// form" is a fact, "we may market to them" is a conclusion, and a vocabulary
// that mixed them would let a creation surface assert the conclusion.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The closed vocabulary of how a contact was obtained.
const (
	// AcquiredSubjectInitiated is the strongest: they wrote to us first.
	AcquiredSubjectInitiated = "subject_initiated"
	// AcquiredCustomerContract is a contact on a live customer relationship.
	AcquiredCustomerContract = "customer_contract"
	// AcquiredRequestedQuoteOrMeeting is somebody who asked for something.
	AcquiredRequestedQuoteOrMeeting = "requested_quote_or_meeting"
	// AcquiredInPersonPermission is a conversation somebody recorded by hand.
	AcquiredInPersonPermission = "in_person_permission"
	// AcquiredReferral is a third party naming them.
	AcquiredReferral = "referral"
	// AcquiredEventOrForm is a form, booking or event registration.
	AcquiredEventOrForm = "event_or_form"
	// AcquiredPublicOrBusinessSource is a directory, a website, a public profile.
	AcquiredPublicOrBusinessSource = "public_or_business_source"
	// AcquiredPurchasedOrImported is a list somebody brought in.
	AcquiredPurchasedOrImported = "purchased_or_imported"
	// AcquiredUnknownLegacy is the honest answer where the door cannot say.
	//
	// It exists so a creation path that does not know is not forced to guess.
	// Writing nothing would be worse: the absence of a row reads as "nobody has
	// looked", which is exactly what this one says out loud, and it keeps the
	// census over creation doors complete.
	AcquiredUnknownLegacy = "unknown_legacy"
)

// Acquisition is what a creation door says about why this contact exists, and
// when.
//
// The table also carries columns for the source entity and the purpose a
// surface claimed. No door populates those yet, and a Go field with no writer
// is a shape callers can pass that nothing produces, so they arrive with the
// door that has something to put in them.
type Acquisition struct {
	// Kind is one of the constants above. Empty means the door did not say,
	// and recordAcquisition writes unknown_legacy rather than nothing.
	Kind string
	// OccurredAt is when the ACT happened, which is not when this row was
	// written. A message captured on Tuesday may have arrived on Friday, and a
	// contact created when a human finally answers a triage question was
	// acquired when the message came in, not when somebody got round to it.
	//
	// This is not bookkeeping. The Art. 14 disclosure deadline runs one month
	// from the ACQUISITION, and compose/noticecaseopen.go dates the duty from
	// `coalesce(occurred_at, captured_at)` — so a door that leaves this empty
	// gives every contact it makes a deadline starting the day the row was
	// written. For anything captured in arrears that deadline is late, and the
	// lateness is invisible: the case looks perfectly on time.
	//
	// Nil means the door genuinely does not know, and captured_at then stands
	// in. That is the honest fallback rather than a guess, and it is what every
	// door did before this field existed.
	OccurredAt *time.Time
}

// recordAcquisition writes one evidence row beside a contact that was just
// created, on the same transaction.
//
// Called from createContact, which every production creation door goes through,
// so a new door gets this without knowing it exists — and a door that says
// nothing still leaves a row saying nobody knows, rather than leaving the
// question unasked.
func recordAcquisition(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, in Acquisition, capturedBy string) error {
	kind := in.Kind
	if kind == "" {
		kind = AcquiredUnknownLegacy
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
		VALUES ($1, $2, $3, $4)`,
		contactID, kind, in.OccurredAt, capturedBy); err != nil {
		return fmt.Errorf("contacts: recording how this contact was acquired: %w", err)
	}
	return nil
}
