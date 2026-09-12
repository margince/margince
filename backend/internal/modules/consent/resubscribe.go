// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Subscribing again, from the subject's own preference page.
//
// A withdrawal is not meant to be permanent. refuseWithdrawnPurposeTx refuses
// to let anybody ELSE restart the conversation — an operator pressing the
// double-opt-in verb, a booking naming a withdrawn subject — and its own
// comment says why that is safe: "a withdrawal is not permanent for the
// SUBJECT — they may re-subscribe through the preference centre, which is their
// own mailbox and their own choice".
//
// That sentence was not true. The preference centre took the grant and wrote
// `granted` on the row, and the send gate then refused every message anyway,
// because a purpose requiring double opt-in is only effective once a
// confirmation round trip has happened (authorizelead.go, recordedStateFor) and
// nothing on this path performed one.
//
// So the subject pressed subscribe, the page said yes, and nothing ever
// arrived. Their only remaining route was to ask somebody at the company to
// restart it for them — which is precisely what the guard above refuses, and
// rightly.
//
// WHAT THIS DOES. A grant for a double-opt-in purpose from the preference
// centre no longer writes a dead row. It mints the same confirmation link the
// ordinary subscribe flow uses, mails it to the address the token already
// proves, and tells the page the answer is pending rather than done.
//
// THE GRANT IS NOT WRITTEN YET, and that is the point rather than an
// optimisation. Writing `granted` and then waiting for a confirmation would put
// a row on the record saying the subject consented, at a moment they had only
// asked to be asked — and every reader of contact_consent would believe it. The
// consent is recorded when the link is spent, by the path that already records
// one, with the mailbox proof that makes it demonstrable.
//
// A NON-DOI PURPOSE IS UNCHANGED. Nothing about it needs a round trip, the
// grant is effective when written, and sending a confirmation nobody asked for
// would be a mail this contact did not need.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReasonConfirmationSent says the subject's subscribe was taken and a
// confirmation link is on its way to them.
//
// AN OUTCOME, NOT AN ERROR, and it rides the same list a refusal does. What the
// page has to show is different from both "saved" and "declined": the answer is
// pending on an action the subject has still to take, in a mailbox this page
// cannot see. A save reported as plain success would leave them expecting mail
// that will not come until they click.
const ReasonConfirmationSent = "confirmation_sent"

// ReasonConfirmationUnavailable says the subscribe could not be started because
// this installation cannot mail the confirmation.
//
// Distinguished from ReasonCannotGrant, which is about the SUBJECT — an erased
// record whose capability is gone. This one is about US: no relay is wired, or
// the address the link would go to is not one we can reach. Telling somebody
// their choice was declined when the truth is that our mail is broken would
// send them looking for a fault on their side.
const ReasonConfirmationUnavailable = "confirmation_unavailable"

// resubscribeTx starts a double-opt-in subscribe from the preference centre.
//
// It answers (outcome, handled, error): handled is false when this purpose
// needs no round trip and the ordinary write should proceed.
func (s *Store) resubscribeTx(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, purposeID ids.PurposeID, purposeKey string,
) (ChoiceOutcome, bool, error) {
	_, requiresDOI, err := loadConsentPurpose(ctx, tx, purposeID)
	if err != nil {
		return ChoiceOutcome{}, false, err
	}
	if !requiresDOI {
		return ChoiceOutcome{}, false, nil
	}
	// ALREADY CONFIRMED, so there is nothing to confirm. A subject re-ticking a
	// box they already hold is not asking for anything, and mailing them a link
	// would be a message they did not need about a decision they already made.
	confirmed, err := grantIsConfirmedTx(ctx, tx,
		subject{entityType: entityContact, column: subjectColumnContact, id: contactID.UUID}, purposeID)
	if err != nil {
		return ChoiceOutcome{}, false, err
	}
	if confirmed {
		return ChoiceOutcome{}, false, nil
	}
	if err := s.mailResubscribeConfirmationTx(ctx, tx, contactID, purposeID); err != nil {
		if errors.Is(err, errNoConfirmationLane) {
			return ChoiceOutcome{
				PurposeKey: purposeKey, Reason: ReasonConfirmationUnavailable,
			}, true, nil
		}
		return ChoiceOutcome{}, false, err
	}
	return ChoiceOutcome{PurposeKey: purposeKey, Reason: ReasonConfirmationSent}, true, nil
}

// grantIsConfirmedTx asks whether this subject already holds a grant the SEND
// GATE would honour.
//
// MIRRORS THE GATE'S OWN CONDITION exactly (authorizelead.go,
// recordedStateFor): the state row says granted, and the proof ledger carries a
// grant with both a confirmation moment AND an issuance trigger. A check that
// asked for less would call a grant confirmed that the gate still refuses, and
// the subject would get no confirmation link and no mail — the silent failure
// this whole file exists to end, reappearing one condition narrower.
//
// The trigger is the second half because it names the mailbox proof the
// confirmation rests on. A row carrying an old operator-set timestamp and no
// trigger authorizes nothing, and there are such rows: the retired
// operator-token endpoint wrote them.
//
// NOT COVERED BY A TEST, and said here rather than left to look covered. The
// shape needs a proof row with a confirmation moment and no trigger, which
// nothing in this build writes any more — so a test would have to seed it, and
// a seeded row proves the query I wrote matches the query I wrote. What holds
// this is that the condition is copied from recordedStateFor: if that one
// changes and this does not, they disagree, and the symptom is the silent
// failure above.
func grantIsConfirmedTx(
	ctx context.Context, tx pgx.Tx, sub subject, purposeID ids.PurposeID,
) (bool, error) {
	var confirmed bool
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1 FROM contact_consent pc
			 WHERE pc.%[1]s = $1 AND pc.purpose_id = $2 AND pc.state = 'granted')
		   AND EXISTS (
			SELECT 1 FROM consent_event ce
			 WHERE ce.%[1]s = $1 AND ce.purpose_id = $2
			   AND ce.new_state = 'granted' AND ce.double_opt_in_confirmed_at IS NOT NULL
			   AND ce.issuance_trigger IS NOT NULL)`, sub.column),
		sub.id, purposeID).Scan(&confirmed)
	if err != nil {
		return false, fmt.Errorf("consent: asking whether this subscription is already confirmed: %w", err)
	}
	return confirmed, nil
}

// errNoConfirmationLane reports that this installation cannot mail the link.
var errNoConfirmationLane = errors.New("consent: no confirmation lane is wired")

// mailResubscribeConfirmationTx mints and stages the link, on the caller's
// transaction.
//
// THROUGH THE ORDINARY MINT, not a second one. issueLinkTx is what records the
// token, pins which question it asks and stages the mail; a path of its own
// here would be a second way to create a consent link, free to drift from the
// one every other door uses.
func (s *Store) mailResubscribeConfirmationTx(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, purposeID ids.PurposeID,
) error {
	if s.confirmSender == nil || s.vault == nil {
		return errNoConfirmationLane
	}
	token, err := newConfirmToken()
	if err != nil {
		return err
	}
	// ASKED BY THE SUBJECT, which lifts the re-solicitation guard: they are
	// holding their own preference token and asking for their own mailbox.
	//
	// The plaintext is hashed and turned into a URL HERE, and neither the mint
	// nor anything it calls ever sees it — see mintRequest.
	issued, err := s.issueLinkTx(ctx, tx, mintRequest{
		contactID:         contactID,
		kind:              LinkConsentConfirmation,
		purposeID:         purposeID,
		tokenHash:         hashPublicToken(token),
		link:              s.confirmLink(token),
		askedByTheSubject: true,
	})
	if err != nil {
		// EVERY REFUSAL THE MINT IS ENTITLED TO MAKE becomes an outcome, not an
		// error, and the distinction decides whether an unrelated withdrawal
		// survives this request.
		//
		// PublicSaveChoices orders withdrawals first so a refused grant cannot
		// cost the suppression saved beside it. This mint runs inside that same
		// transaction, so an error returned here rolls the whole PUT back —
		// including a stop the subject asked for in the same press. A contact
		// with no live address, an archived record, a purpose that cannot be
		// confirmed: none of those is a reason to discard the rest of what
		// somebody just chose.
		//
		// A ValidationError is exactly that class. Anything else is a genuine
		// fault and still rolls back.
		var invalid *ValidationError
		if errors.As(err, &invalid) {
			return errNoConfirmationLane
		}
		return err
	}
	if !issued.staged {
		// The token exists and no mail carries it. An installation with no
		// relay wired mints links nobody receives, and reporting success would
		// leave the subject waiting for a confirmation that was never sent.
		return errNoConfirmationLane
	}
	return nil
}
