// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The one answer to "may we write to this person for this purpose", and the
// reason for it.
//
// It exists so the composer's preview and the dispatcher's transmit-time check
// cannot drift. Two implementations of the same question are two questions, and
// the one that stops matching looks exactly like the one that still does — a
// preview that says "allowed" over a send that refuses is worse than no preview
// at all, because the rep has already written the message.
//
// ADR-0098 is the ruling this encodes: not every purpose is consent-gated.
// Individual business correspondence is not advertising under UWG §7, and its
// lawful basis is Art 6(1)(b)/(f). Consent, with its German evidence standard,
// belongs to marketing. Treating a reply to somebody who wrote to us as a
// consent violation is a frame that is legally wrong and that every rep
// correctly ignores, which is worse than useless.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Class decides which question a purpose answers to.
type Class string

const (
	// ClassBusinessCorrespondence is 1:1 mail: replies, meeting follow-ups,
	// answers to their own question. Allowed on a qualifying event, no consent
	// object consulted.
	ClassBusinessCorrespondence Class = "business_correspondence"
	// ClassTransactional is invoices and account notices — allowed while the
	// business relationship exists.
	ClassTransactional Class = "transactional"
	// ClassMarketing is »Werbung« read broadly: newsletters, campaigns, and
	// feedback requests. Express consent with double-opt-in proof, or the
	// §7(3) existing-customer flag.
	ClassMarketing Class = "marketing"
	// ClassPhoneOutreach is specced and dormant — no call provider is wired.
	// The purpose exists so the model is complete, not because a path uses it.
	ClassPhoneOutreach Class = "phone_outreach"
)

// Verdict is the effective answer for one person and one purpose.
type Verdict struct {
	// State is allowed, blocked, or unknown. Unknown is not a soft block: it
	// means no decision is recorded, and the offered action is to REQUEST
	// consent rather than to send anyway.
	State string
	// Reason is the answer in the reader's words — "she wrote to you on 2 May",
	// "opt-out 12 Jul", "no consent recorded". A verdict a rep cannot explain
	// to the person in front of them is not usable.
	Reason string
	// Qualifying names the event that flipped correspondence to allowed, when
	// one did. Recording WHICH event is what makes the Art 6(1)(f) balancing
	// accountable rather than merely asserted.
	Qualifying *QualifyingEvent
	// Code names the block in a form code can read, because the engine has to
	// tell an Art. 21 objection from a withdrawal from a deployment fact — and
	// matching on Reason, which is an operator-facing sentence, means an
	// ordinary copy edit silently reclassifies a legal fact.
	Code string
	// QualifyingDerived marks a Qualifying that was READ OFF the timeline
	// rather than read from a stored row.
	//
	// The distinction is the accountability one. A derived event is
	// re-computed on every call and leaves no trace of what authorized a
	// particular send; ADR-0098 D2 requires the flip be "stamped with which
	// event and when", so the transmit path persists a derived event before it
	// relies on it. The guard preview does not — a preview authorizes nothing,
	// and writing a record because somebody opened a drawer would put a legal
	// fact on the record for a message nobody sent.
	QualifyingDerived bool
}

const (
	// VerdictAllowed proceeds.
	VerdictAllowed = "allowed"
	// VerdictBlocked refuses, and Reason says why.
	VerdictBlocked = "blocked"
	// VerdictUnknown has no decision recorded either way.
	VerdictUnknown = "unknown"
)

// QualifyingEvent is the deterministic thing on the record that makes business
// correspondence lawful.
type QualifyingEvent struct {
	Kind             string
	OccurredAt       time.Time
	SourceEntityType string
	SourceEntityID   string
	Note             string
}

// PurposeRow is one configured purpose, as both callers read it.
type PurposeRow struct {
	ID          string
	Key         string
	Label       string
	Class       Class
	RequiresDOI bool
}

// VerdictForPerson is THE decision. Both the guard endpoint and the transmit
// gate call it, so a preview and a send answer with the same code.
//
// The order of the checks is the ruling, and it is not rearrangeable: an
// objection is evaluated FIRST and overrides every basis below it, including
// legitimate interest, including a qualifying event, including §7(3). For
// direct marketing Art 21(2)–(3) is absolute — there is no balancing and no
// override toggle, so there is no branch here that can reach past a
// suppression.
func VerdictForPerson(ctx context.Context, tx pgx.Tx, personID string, purpose PurposeRow) (Verdict, error) {
	suppressed, at, err := objectionStands(ctx, tx, personID, purpose.ID)
	if err != nil {
		return Verdict{}, err
	}
	if suppressed {
		return Verdict{State: VerdictBlocked, Reason: objectionReason(at), Code: BlockObjection}, nil
	}

	switch purpose.Class {
	case ClassTransactional:
		// Art 6(1)(b): the contract itself is the basis. Nothing to consult.
		return Verdict{State: VerdictAllowed, Reason: "account and contract notices need no consent"}, nil

	case ClassBusinessCorrespondence:
		return correspondenceVerdict(ctx, tx, personID, purpose)

	case ClassPhoneOutreach:
		// Dormant by decision: the purpose exists so the model is complete. A
		// surface that offered it would offer a path nothing implements.
		return Verdict{State: VerdictBlocked, Reason: "no call path is configured", Code: BlockNoChannel}, nil

	default:
		return marketingVerdict(ctx, tx, personID, purpose)
	}
}

// correspondenceVerdict answers for ordinary business correspondence: the
// person's own recorded answer first, and only then what the record implies.
//
// THE ORDER IS THE FIX. This arm used to read qualifying events alone, so an
// explicit `granted` row authorized nothing: a person who had said in as many
// words that we may write to them was refused until they happened to send an
// inbound message, while one inbound message opened every unrelated send. A
// recorded answer is the strongest thing a subject can give us about their own
// correspondence, and reading it second — or, as here, not at all — inverted
// that.
//
// A withdrawal needs no branch here. objectionStands runs before this for every
// class and every purpose, so a withdrawn row has already blocked.
//
// The implied arm below is unchanged and stays second: it is the Art 6(1)(f)
// reading of a relationship the subject started, and it is what answers for the
// overwhelming majority of people, who never record an answer either way.
func correspondenceVerdict(ctx context.Context, tx pgx.Tx, personID string, purpose PurposeRow) (Verdict, error) {
	// requiresDOI is passed as the purpose declares it rather than as a
	// constant false. Correspondence does not demand the round trip today, and
	// hard-coding that here would silently ignore an installation that turned
	// it on for this purpose.
	state, granted, err := recordedState(ctx, tx, personID, purpose.ID, purpose.RequiresDOI)
	if err != nil {
		return Verdict{}, err
	}
	if granted {
		return Verdict{State: VerdictAllowed, Reason: "they said we may write to them"}, nil
	}
	if state == string(StateGranted) && purpose.RequiresDOI {
		// Granted but never confirmed, on an installation that demands the
		// round trip for this purpose. Falling through to the implied arm
		// would let the timeline supply what the confirmation did not.
		return Verdict{
			State:  VerdictBlocked,
			Reason: "consent was recorded but never confirmed by the double opt-in",
			Code:   BlockUnconfirmedDOI,
		}, nil
	}
	event, source, err := latestQualifyingEvent(ctx, tx, personID)
	if err != nil {
		return Verdict{}, err
	}
	if source == sourceNone {
		// No answer on file, no inbound, no inquiry, no deal, no recorded
		// exchange. There is nothing here to balance, so this is not the easy
		// Art 6(1)(f) case and the honest answer is that nobody has decided.
		return Verdict{
			State:  VerdictUnknown,
			Reason: "they have never written to you and no deal or inquiry connects you",
		}, nil
	}
	return Verdict{
		State:             VerdictAllowed,
		Reason:            qualifyingReason(event),
		Qualifying:        &event,
		QualifyingDerived: source == sourceDerived,
	}, nil
}

// marketingVerdict is the strict arm, unchanged in strictness by ADR-0098:
// express consent with the DOI round-trip, or the §7(3) existing-customer flag
// with all four of its conditions on the record. There is no legitimate-interest
// escape for marketing email, B2C or B2B, and the product does not offer the
// toggle.
func marketingVerdict(ctx context.Context, tx pgx.Tx, personID string, purpose PurposeRow) (Verdict, error) {
	state, granted, err := recordedState(ctx, tx, personID, purpose.ID, purpose.RequiresDOI)
	if err != nil {
		return Verdict{}, err
	}
	if granted {
		return Verdict{State: VerdictAllowed, Reason: "they gave consent, with the confirmation on file"}, nil
	}
	if state == string(StateWithdrawn) {
		return Verdict{State: VerdictBlocked, Reason: "they withdrew consent for this purpose", Code: BlockWithdrawn}, nil
	}
	if state == string(StateGranted) && purpose.RequiresDOI {
		// Granted but never confirmed. The BGH evidence standard is the whole
		// point of double opt-in, so an unconfirmed grant is not proof and does
		// not send.
		return Verdict{
			State:  VerdictBlocked,
			Reason: "consent was recorded but never confirmed by the double opt-in",
			Code:   BlockUnconfirmedDOI,
		}, nil
	}
	flagged, err := existingCustomerFlag(ctx, tx, personID)
	if err != nil {
		return Verdict{}, err
	}
	if flagged {
		return Verdict{State: VerdictAllowed, Reason: "existing customer under UWG §7(3), with the sale and opt-out notice on file"}, nil
	}
	return Verdict{State: VerdictUnknown, Reason: "no consent recorded"}, nil
}

// objectionReason states the refusal, with the date when the proof ledger
// carries one. A zero time means the state row stands without a proof row
// behind it, and inventing "1 Jan 0001" for a rep to read out is worse than
// saying plainly that the objection is recorded.
func objectionReason(at time.Time) string {
	if at.IsZero() {
		return "they objected, and an objection overrides every other basis"
	}
	return fmt.Sprintf("they objected on %s, and an objection overrides every other basis",
		at.Format("2 Jan 2006"))
}

func qualifyingReason(event QualifyingEvent) string {
	when := event.OccurredAt.Format("2 Jan")
	switch event.Kind {
	case "inbound_message":
		return fmt.Sprintf("they wrote to you on %s", when)
	case "inquiry":
		return fmt.Sprintf("they made an inquiry on %s", when)
	case "active_deal":
		return "an open deal connects you"
	default:
		return fmt.Sprintf("a recorded exchange on %s: %s", when, event.Note)
	}
}

// objectionStands asks whether an opt-out, unsubscribe or Art 21 objection is
// on the record for this purpose.
//
// The state row says THAT they objected; the append-only proof ledger says
// when. The date is read from the ledger's newest withdrawal rather than from
// the state row, which carries only the capture time of whatever decision is
// current — on a person who granted after withdrawing, that timestamp names
// the grant, and a refusal quoting it would cite the wrong day back to a rep
// standing in front of the customer.
//
// A missing ledger row does not soften the answer: the state is the authority
// for the verdict, and a zero time renders as an objection whose date this
// installation cannot evidence.
func objectionStands(ctx context.Context, tx pgx.Tx, personID, purposeID string) (bool, time.Time, error) {
	var state string
	err := tx.QueryRow(ctx, `
		SELECT pc.state FROM person_consent pc
		WHERE pc.person_id = $1 AND pc.purpose_id = $2`, personID, purposeID).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, fmt.Errorf("read the objection state: %w", err)
	}
	if state != string(StateWithdrawn) {
		return false, time.Time{}, nil
	}
	var at time.Time
	err = tx.QueryRow(ctx, `
		SELECT ce.captured_at FROM consent_event ce
		WHERE ce.person_id = $1 AND ce.purpose_id = $2 AND ce.new_state = 'withdrawn'
		ORDER BY ce.captured_at DESC
		LIMIT 1`, personID, purposeID).Scan(&at)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, time.Time{}, fmt.Errorf("read the objection's proof: %w", err)
	}
	return true, at, nil
}

// recordedState reads the person's own decision for this purpose, and whether
// it satisfies the DOI round-trip when the purpose demands one.
//
// issuance_trigger IS NOT NULL is what separates a confirmation from a claim of
// one. It is set where a mailbox proof stood in for the round trip: the subject
// spent a link that had been mailed to their own live primary address. That
// proof can only be claimed by the confirm submit, past the spend of the link
// that earns it.
//
// Held by: TestOnlyTheConfirmSubmitClaimsAProvenMailbox
// (backend/gates/mailboxproofwriters_test.go)
//
// It is NULL on the rows the retired operator-token endpoint produced, which
// returned the plaintext to the operator and accepted it straight back, so one
// person could mint and redeem a confirmation the subject never saw. Those rows
// stay on the proof log as the history they are, and authorize no send.
func recordedState(ctx context.Context, tx pgx.Tx, personID, purposeID string, requiresDOI bool) (string, bool, error) {
	return recordedStateFor(ctx, tx, subjectColumnPerson, personID, purposeID, requiresDOI)
}

// existingCustomerFlag reads the UWG §7(3) flag. The DDL already refuses a row
// without the opt-out notice, so a live row here IS all four conditions.
func existingCustomerFlag(ctx context.Context, tx pgx.Tx, personID string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM consent_existing_customer_flag
		  WHERE person_id = $1 AND revoked_at IS NULL)`, personID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("read the existing-customer flag: %w", err)
	}
	return exists, nil
}

// PurposesForGuard lists the purposes a guard read reports on, in a fixed order
// so the rail card does not reshuffle between visits.
func PurposesForGuard(ctx context.Context, tx pgx.Tx) ([]PurposeRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, key, label, class, requires_double_opt_in
		FROM consent_purpose
		WHERE archived_at IS NULL
		ORDER BY CASE class
		           WHEN 'business_correspondence' THEN 0
		           WHEN 'transactional' THEN 1
		           WHEN 'marketing' THEN 2
		           ELSE 3
		         END, key`)
	if err != nil {
		return nil, fmt.Errorf("list the consent purposes: %w", err)
	}
	defer rows.Close()
	var out []PurposeRow
	for rows.Next() {
		var p PurposeRow
		if err := rows.Scan(&p.ID, &p.Key, &p.Label, &p.Class, &p.RequiresDOI); err != nil {
			return nil, fmt.Errorf("scan a consent purpose: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list the consent purposes: %w", err)
	}
	return out, nil
}
