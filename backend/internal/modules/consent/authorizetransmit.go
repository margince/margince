// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The decision taken immediately before a message reaches a provider, and the
// record of it.
//
// A staging decision answers "may this be written down and queued". It cannot
// answer "may this go out NOW", because the two are separated by a queue: a
// person can withdraw consent, object, or have their address hard-bounce in
// between, and a delivery that waited a day on a retry ladder was authorized
// against a world that no longer exists. So the question is asked again here,
// and the answer is persisted before any provider I/O rather than after — a
// decision written afterwards records what was sent, not what permitted it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// AuthorizeTransmit answers whether this delivery may go out now, and writes
// the answer down.
//
// It runs in its own short transaction, which commits BEFORE the provider is
// called. That ordering is the point: the row says "this was permitted at
// attempt N", and a send that happens without one is a send nobody can account
// for. The transaction is its own rather than the caller's because there is no
// caller transaction to join — provider I/O cannot sit inside one.
func (g *Gate) AuthorizeTransmit(ctx context.Context, req commsauthz.TransmitRequest) (commsauthz.TransmitTicket, error) {
	for _, r := range req.Recipients {
		if err := r.Validate(); err != nil {
			// A malformed recipient is a caller DEFECT, not an answer about
			// anybody. The legacy gate refuses the same shape for the same
			// reason: a recipient carrying both an address and a channel
			// identity would be answered about on the channel arm alone, and
			// the address — which may be the one that objected — would never
			// be looked at.
			return commsauthz.TransmitTicket{}, fmt.Errorf(
				"consent: this recipient cannot be put to the engine: %w", err)
		}
	}
	if len(req.Recipients) == 0 {
		// A caller fault, not an answer about anybody: reported as the defect
		// it is rather than dressed up as a suppression.
		return commsauthz.TransmitTicket{}, fmt.Errorf("consent: a transmit decision needs at least one recipient: %w",
			apperrors.ErrInvalidArgument)
	}
	setID := ids.NewV7()
	ticket := commsauthz.TransmitTicket{DeliveryID: req.DeliveryID, Attempt: req.Attempt, DecisionSetID: setID}
	// Counted after the commit, not beside the insert. Two arms below return an
	// error once the decisions are already written — an unanswerable legacy
	// gate, and the wording comparison — and each rolls the rows back. A
	// counter incremented inside the transaction would keep those, and report
	// decisions the record does not hold.
	var recorded []commsauthz.Decision

	// The legacy gate's answer, recorded beside the engine's on every row so a
	// disagreement is readable in the record rather than only in a counter that
	// dies with the process.
	//
	// IT NO LONGER DECIDES, with one exception the rollback lever depends on:
	// Effective consults it only when NO recipient's category is enforced,
	// which the shipped posture never produces but an operator moving a
	// category back to observe deliberately does. The dispatcher used to ask it
	// a second time after the ticket already said yes; that call is gone.
	legacyErr := g.RequireGrantedForRecipients(ctx, req.Recipients, req.PurposeKey)
	// Three outcomes, and only two of them are answers: granted, not granted,
	// and anything else. That third one is NOT a no — whether it matters
	// depends on the posture, and the posture is not known until the modes are
	// read inside the transaction below, so it is carried there as a value
	// rather than decided here.
	legacyUnanswerable := legacyErr != nil && !errors.Is(legacyErr, apperrors.ErrConsentNotGranted)
	legacyAllowed := legacyErr == nil

	err := g.store.db.Tx(ctx, func(tx pgx.Tx) error {
		// Read inside the transaction that binds the decision, so the posture
		// a row records is the one that was live when it was taken. Reading it
		// before the transaction would let a rollout change between the read
		// and the write, leaving a row stamped with a mode that never decided
		// it.
		modes, err := settings.ApplyTx(ctx, tx, AuthorizationModes)
		if err != nil {
			return err
		}
		modeFor := func(c commsauthz.Category) commsauthz.Mode { return ModeFor(modes, c) }
		set, err := g.decideRecipients(ctx, tx, req, legacyAllowed, modeFor)
		if err != nil {
			return err
		}
		written, err := g.recordDecisions(ctx, tx, req, setID, set)
		if err != nil {
			return err
		}
		recorded = written
		// AN UNANSWERABLE LEGACY GATE IS ONLY FATAL WHERE ITS ANSWER IS USED.
		//
		// Effective consults legacyAllowed only when no recipient's category is
		// enforced, which the shipped posture never produces. Where it IS
		// consulted — an operator has moved a category back to observe — a gate
		// that failed to answer must retry rather than refuse: a refusal parks
		// with a reason a human can act on, and a failure to LEARN the answer
		// is not one. Treating the outage as a refusal would destroy every
		// in-flight delivery in the window instead of deferring them.
		// An absolute denial needs no second opinion: it refuses in every mode,
		// so an unanswerable legacy gate changes nothing and retrying would
		// hold a message the subject already stopped.
		if legacyUnanswerable && !set.HasAbsoluteDenial() && !set.HasEnforcedRecipient(modeFor) {
			return legacyErr
		}
		ticket.Allowed = set.Effective(modeFor, legacyAllowed)
		ticket.Reason = refusalReason(set, legacyAllowed)
		// The message that goes must be the message that was authorized.
		//
		// Asked LAST, and only of a send the engine would otherwise allow: a
		// delivery already refused is parked with a reason about the recipient,
		// and replacing that with "the wording changed" would tell an operator
		// to re-approve a message somebody had objected to. A changed body on
		// an allowed send is its own refusal, and it parks rather than denying
		// forever — the wording is a thing a human can look at and re-send.
		if ticket.Allowed {
			changed, err := g.wordingDiffersFromStaging(ctx, tx, req)
			if err != nil {
				return err
			}
			if changed {
				ticket.Allowed = false
				ticket.Reason = "this message was edited after it was authorized, so the wording that " +
					"was checked is not the wording that would go"
			}
		}
		return nil
	})
	if err != nil {
		return commsauthz.TransmitTicket{}, err
	}
	for _, d := range recorded {
		countDecision(d)
	}
	return ticket, nil
}

// decideRecipients asks the engine about every addressee.
//
// Every recipient is asked even once one has refused, because the record is
// per recipient: an operator answering "why did this not go" is owed the whole
// answer, not the first line of it.
func (g *Gate) decideRecipients(ctx context.Context, tx pgx.Tx, req commsauthz.TransmitRequest, legacyAllowed bool, modeFor func(commsauthz.Category) commsauthz.Mode) (commsauthz.DecisionSet, error) {
	legacy := commsauthz.VerdictDeny
	if legacyAllowed {
		legacy = commsauthz.VerdictAllow
	}
	set := commsauthz.DecisionSet{}
	now := g.store.now().UTC()
	// What each recipient's message was staged as, so the transmit phase asks
	// about the same message rather than about a purpose key. Per recipient,
	// because one message can be several things at once.
	claims, err := stagedClaims(ctx, tx, req.DeliveryID)
	if err != nil {
		return commsauthz.DecisionSet{}, err
	}
	// The conversation this delivery belongs to. Staging asked the thread arm
	// with the anchor the caller named; nothing carries that anchor forward, so
	// transmit names the same thread from the delivery row instead.
	threadKey, err := deliveryThreadKey(ctx, tx, req.DeliveryID)
	if err != nil {
		return commsauthz.DecisionSet{}, err
	}
	// The records this delivery's activity is filed under — the live-deal
	// arm's own evidence, absent from TransmitRequest for deliveryThreadKey's
	// reason.
	links, err := deliveryLinks(ctx, tx, req.DeliveryID)
	if err != nil {
		return commsauthz.DecisionSet{}, err
	}
	// Every address's cap lock, sorted, before the first recipient is counted.
	// Taking them inside the loop would order them by the caller's To list, and
	// two messages naming the same pair in opposite orders would deadlock.
	if err := g.lockCapAddresses(ctx, tx, req.Recipients); err != nil {
		return commsauthz.DecisionSet{}, err
	}
	for _, r := range req.Recipients {
		d, err := g.decideOne(ctx, tx, r, stagedRequestFor(req, r, claims, threadKey, links), commsauthz.PhaseTransmit)
		if err != nil {
			return commsauthz.DecisionSet{}, err
		}
		// The ceiling is applied HERE and not at staging, because it counts
		// what has already been delivered and that number moves while a message
		// waits in the queue. A cap checked at staging would be answering about
		// a moment that has passed by the time the message goes.
		d, err = g.applyFrequencyCap(ctx, tx, d, now)
		if err != nil {
			return commsauthz.DecisionSet{}, err
		}
		d.Phase = commsauthz.PhaseTransmit
		// Stamped from the category the engine RESOLVED, not the one the
		// caller claimed: the mode that decided this row is the one belonging
		// to what the message actually is.
		d.Mode = modeFor(d.Resolved)
		d.LegacyVerdict = string(legacy)
		set.Decisions = append(set.Decisions, d)
	}
	return set, nil
}

// decideOne answers about a single recipient: who they are, whether anything
// suppresses them, and what the record says this message is.
//
// It takes the whole request rather than a purpose key, because the category is
// now RESOLVED from the anchor and the links (authorizeresolve.go) instead of
// read off the caller's label. The purpose key is still in there and still
// consulted, as the weakest of the four grounds.
func (g *Gate) decideOne(ctx context.Context, tx pgx.Tx, r connector.Recipient, req commsauthz.Request, phase commsauthz.Phase) (commsauthz.Decision, error) {
	d := commsauthz.Decision{Recipient: r, Resolved: commsauthz.CategoryMarketing}
	personID, found, err := resolvePerson(ctx, tx, r)
	if err != nil {
		// Ambiguity refuses rather than picking, and that is an ANSWER about
		// this send: no verdict can be about one person.
		if errors.Is(err, apperrors.ErrConsentNotGranted) {
			d.Verdict = commsauthz.VerdictDeny
			d.ReasonCode = commsauthz.ReasonNoSubject
			return d, nil
		}
		return commsauthz.Decision{}, err
	}
	if !found {
		// No person: this may still be a LEAD, which is a subject the engine
		// can answer about. Without this arm every lead-only recipient came
		// back `review`, so a category moved to enforce would refuse exactly
		// the sends the legacy gate allows — an inversion rather than a
		// tightening, and it would have arrived the day somebody flipped a
		// mode rather than the day this code was written.
		return g.decideLead(ctx, tx, r, req, d, phase)
	}
	parsed, err := ids.Parse(personID)
	if err != nil {
		return commsauthz.Decision{}, fmt.Errorf("consent: the resolved subject is not an id: %w", err)
	}
	d.SubjectKind, d.SubjectID = entityPerson, parsed

	// READ FIRST, APPLY AFTER THE CATEGORY IS KNOWN. What a suppression binds
	// depends on what the message is, and nothing knows that until the record
	// has been resolved — see applySuppression.
	kinds, err := liveSuppression(ctx, tx, personID, r)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	if kind, absolute := bindsEveryCategory(kinds); absolute {
		// Nothing a category could say would change this answer, so the record
		// is not resolved at all. An objection and a restriction do NOT come
		// through here — both need the category before they can be applied.
		d.Verdict = commsauthz.VerdictDeny
		d.ReasonCode = kind
		d.Suppression = kind
		return d, nil
	}
	address, channelProvider, channelUserID := recipientSubjectAddress(r)
	d, err = g.decideResolved(ctx, tx, req, subjectRef{
		Kind: entityPerson, ID: personID, Address: address,
		ChannelProvider: channelProvider, ChannelUserID: channelUserID,
	}, d, phase, len(kinds) > 0)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	d = applySuppression(d, kinds)
	return d, nil
}

// blockedReasonCode translates the verdict's own code into the engine's
// vocabulary.
//
// It reads Verdict.Code and never the operator sentence beside it. Matching on
// prose meant an ordinary copy edit in verdict.go could silently reclassify a
// legal fact, and it collapsed three different blocks into "objection" — a
// withdrawal under Art. 7(3), and a purpose class this installation has no
// transport for, both recorded as though the person had objected. A subject
// access request discloses these rows, so a wrong label there is a false
// statement about somebody.
//
// The default is the STRONGEST code, not the weakest: an unrecognised block is
// a block this function has not been taught about, and guessing leniently is
// how a new refusal becomes the one that sends.
func blockedReasonCode(v Verdict) string {
	switch v.Code {
	case BlockUnconfirmedDOI:
		return commsauthz.ReasonUnconfirmedDOI
	case BlockWithdrawn:
		return commsauthz.ReasonConsentWithdrawn
	case BlockNoChannel:
		return commsauthz.ReasonNoEvidence
	case BlockSuppressed:
		// v.Suppression carries the kind that actually bound, so a caller
		// reading Code alone gets the record's own reason rather than the
		// unrelated default below — the same mislabelling this file's own
		// doc comment exists to prevent.
		return v.Suppression
	default:
		return commsauthz.ReasonObjection
	}
}

// categoryForClass maps the legacy purpose class onto the closed vocabulary,
// conservatively. The old transactional class becomes account_notice rather
// than anything broader: it is the narrowest member that can carry a genuine
// operational message, and the engine records what it was ASKED alongside it.
func categoryForClass(class Class) commsauthz.Category {
	switch class {
	case ClassBusinessCorrespondence:
		return commsauthz.CategoryReplyToInbound
	case ClassTransactional:
		return commsauthz.CategoryAccountNotice
	default:
		return commsauthz.CategoryMarketing
	}
}

func basisForClass(class Class) commsauthz.Basis {
	switch class {
	case ClassBusinessCorrespondence:
		return commsauthz.BasisSubjectInitiatedCorrespondence
	case ClassTransactional:
		return commsauthz.BasisContract
	default:
		return commsauthz.BasisConsent
	}
}

// refusalReason renders the operator-facing sentence for a parked delivery. It
// names the category and the reason code, never the recipient's consent
// history: the caller supplied the addresses, so nothing here discloses
// anything they did not already hold.
func refusalReason(set commsauthz.DecisionSet, legacyAllowed bool) string {
	denied := set.Denied()
	if len(denied) == 0 {
		if legacyAllowed {
			return ""
		}
		return "the consent gate refused this delivery"
	}
	return fmt.Sprintf("%d of %d recipients are not authorized for this message (%s)",
		len(denied), len(set.Decisions), denied[0].ReasonCode)
}

// recordDecisions writes one immutable row per recipient.
//
// The content fingerprint is a hash of subject and body, never the text: it
// exists so a later reader can tell whether the message that went is the
// message that was authorized, and storing the words themselves would make the
// decision a second copy of the mail.
func (g *Gate) recordDecisions(ctx context.Context, tx pgx.Tx, req commsauthz.TransmitRequest, setID ids.UUID, set commsauthz.DecisionSet) ([]commsauthz.Decision, error) {
	var written []commsauthz.Decision
	sum := SendingDigest(req.Subject, req.Body, req.HTMLBody)
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range set.Decisions {
		// Both or neither, which the table's own CHECK also demands: a
		// subject_kind naming a row with no id describes nothing.
		subjectKind := nullableText(d.SubjectKind)
		var subjectID *ids.UUID
		if d.SubjectKind != "" {
			id := d.SubjectID
			subjectID = &id
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO communication_decision
			  (delivery_id, attempt, decision_set_id, recipient_address, subject_kind, subject_id,
			   phase, resolved_category, verdict, reason_code, basis, suppression,
			   content_fingerprint, legacy_verdict, mode, actor)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
			ON CONFLICT (decision_set_id, recipient_address, phase) DO NOTHING`,
			req.DeliveryID, req.Attempt, setID, decisionRecipientKey(d.Recipient),
			subjectKind, subjectID, string(d.Phase), string(d.Resolved), string(d.Verdict),
			d.ReasonCode, nullableBasis(d.Basis), nullableText(d.Suppression),
			sum[:], d.LegacyVerdict, string(d.Mode), by)
		if err != nil {
			return nil, fmt.Errorf("consent: record the transmit decision: %w", err)
		}
		if tag.RowsAffected() > 0 {
			written = append(written, d)
		}
	}
	return written, nil
}

// nullableBasis and nullableText carry the difference between "no value" and
// "the empty string" to Postgres. A *string is what pgx reads as NULL, and the
// distinction matters on both columns: a decision with no basis recorded is not
// the same fact as one whose basis is blank.
func nullableBasis(b commsauthz.Basis) *string {
	if b == "" {
		return nil
	}
	v := string(b)
	return &v
}

func nullableText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// liveSuppression reads what stops a message reaching this recipient
// independently of any consent grant.
//
// Two shapes, and the address arm matters as much as the person arm: a hard
// bounce is a fact about a MAILBOX, so it is recorded against the address and
// keeps applying when the same address later appears on a different record.
// The person arm carries objections and restrictions, which follow the human.
func liveSuppression(ctx context.Context, tx pgx.Tx, personID string, r connector.Recipient) ([]string, error) {
	// EVERY live kind, not the strongest one.
	//
	// An earlier version took one row ordered by a fixed strength, which was
	// sound while every kind refused everything: whichever won, the answer was
	// the same. It stopped being sound when reach became category-dependent —
	// a marketing objection sorts first and binds the LEAST, so a person
	// carrying both an objection and a hard bounce had the bounce masked and
	// their invoice sent to a dead mailbox. Strength is no longer a total
	// order, so the caller is given all of them and applies each.
	//
	// Reading the row is not applying it: what a suppression BINDS depends on
	// the category, which is not known here. applySuppression decides that,
	// after resolution.
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT kind FROM communication_suppression
		 WHERE revoked_at IS NULL
		   AND (person_id = $1
		        OR lead_id = $1
		        OR (address IS NOT NULL AND $2 <> '' AND lower(address) = lower($2)))`,
		personID, r.Email)
	if err != nil {
		return nil, fmt.Errorf("consent: read the recipient's suppressions: %w", err)
	}
	defer rows.Close()
	var kinds []string
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			return nil, fmt.Errorf("consent: read the recipient's suppressions: %w", err)
		}
		kinds = append(kinds, reasonForSuppressionKind(kind))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("consent: read the recipient's suppressions: %w", err)
	}
	return kinds, nil
}

// reasonForSuppressionKind maps a stored kind onto the reason code a decision
// row carries.
//
// A kind this code does not recognise gets its OWN code rather than being
// folded onto a known one: suppressionBinds refuses an unrecognised code
// outright, and folding it onto a recognised one would hand it that code's
// narrower reach. That is exactly how subject_request came to permit five
// categories of mail while wearing the statutory restriction's name.
func reasonForSuppressionKind(kind string) string {
	switch kind {
	case "marketing_objection":
		return commsauthz.ReasonObjection
	case "processing_restriction":
		return commsauthz.ReasonRestricted
	case "subject_request":
		return commsauthz.ReasonSubjectRequest
	case "hard_bounce":
		return commsauthz.ReasonHardBounce
	default:
		return "unrecognised_suppression:" + kind
	}
}

// decisionRecipientKey is the stored identity of one recipient, and it is
// deliberately NOT recipientLabel.
//
// recipientLabel exists to name a refused recipient in an operator's error
// message, where a channel account id is withheld on purpose — the caller never
// supplied it, so a refusal must not hand it back. That is right for a sentence
// and wrong for a key: every channel recipient would store the same words, so
// two recipients on one delivery would collide on the uniqueness index and the
// second decision — possibly the refusal — would be dropped.
//
// A channel identity is therefore stored structurally. It stays inside the
// installation, where the timeline already holds the same id.
func decisionRecipientKey(r connector.Recipient) string {
	if r.Channel != nil {
		return r.Channel.Provider + ":" + r.Channel.ChannelUserID
	}
	return r.Email
}
