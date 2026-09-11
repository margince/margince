// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The decision taken immediately before a message reaches a provider, and the
// record of it.
//
// A staging decision answers "may this be written down and queued". It cannot
// answer "may this go out NOW", because the two are separated by a queue: a
// contact can withdraw consent, object, or have their address hard-bounce in
// between, and a delivery that waited a day on a retry ladder was authorized
// against a world that no longer exists. So the question is asked again here,
// and the answer is persisted before any provider I/O rather than after — a
// decision written afterwards records what was sent, not what permitted it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

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
		// THIS REFUSAL IS ABOUT THE RECIPIENTS, which is the one a recorded
		// human decision can answer — they were shown the engine's verdict
		// about the contacts, and that is what they signed for.
		//
		// Set here, from the engine's own answer, and never widened below: the
		// wording check that follows refuses for a reason NOBODY has looked at,
		// so it must not inherit this flag. See TransmitTicket.ConsentRefused.
		ticket.ConsentRefused = !ticket.Allowed
		// The message that goes must be the message that was authorized.
		//
		// ASKED OF EVERY MESSAGE THAT MIGHT ACTUALLY GO, which is not the same
		// as every message the engine allows. A delivery the engine refused can
		// still leave on a named human's recorded decision, and that decision
		// was about specific words — so skipping this check for a refused
		// delivery would let a directed send carry whatever the payload holds
		// now rather than what was acknowledged. That was the defect: the check
		// ran only under `if ticket.Allowed`, and a directed send is never
		// allowed.
		//
		// It does not REPLACE the recipient refusal for a message that is not
		// going anyway: an operator reading "the wording changed" about a
		// message somebody had objected to would be told to re-approve the
		// wrong thing. So the reason is only rewritten when the recipient
		// verdict would otherwise have let it through, and the flag is cleared
		// either way — a wording refusal is nobody's to waive.
		changed, err := g.wordingDiffersFromStaging(ctx, tx, req)
		if err != nil {
			return err
		}
		applyWordingVerdict(&ticket, changed)
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
	contactID, found, err := resolveContact(ctx, tx, r)
	if err != nil {
		// Ambiguity refuses rather than picking, and that is an ANSWER about
		// this send: no verdict can be about one contact.
		if errors.Is(err, apperrors.ErrConsentNotGranted) {
			d.Verdict = commsauthz.VerdictDeny
			d.ReasonCode = commsauthz.ReasonNoSubject
			return d, nil
		}
		return commsauthz.Decision{}, err
	}
	if !found {
		// No contact: this may still be a LEAD, which is a subject the engine
		// can answer about. Without this arm every lead-only recipient came
		// back `review`, so a category moved to enforce would refuse exactly
		// the sends the legacy gate allows — an inversion rather than a
		// tightening, and it would have arrived the day somebody flipped a
		// mode rather than the day this code was written.
		return g.decideLead(ctx, tx, r, req, d, phase)
	}
	parsed, err := ids.Parse(contactID)
	if err != nil {
		return commsauthz.Decision{}, fmt.Errorf("consent: the resolved subject is not an id: %w", err)
	}
	d.SubjectKind, d.SubjectID = entityContact, parsed

	// READ FIRST, APPLY AFTER THE CATEGORY IS KNOWN. What a suppression binds
	// depends on what the message is, and nothing knows that until the record
	// has been resolved — see applySuppression.
	kinds, err := liveSuppression(ctx, tx, contactID, r)
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
		Kind: entityContact, ID: contactID, Address: address,
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
// transport for, both recorded as though the contact had objected. A subject
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

// applyWordingVerdict folds the wording answer into the ticket.
//
// Split out so the rule can be stated without a database behind it, and because
// it is the rule a directed send hangs on: a message edited after it was
// checked is refused for a reason NOBODY has looked at, so the flag that lets a
// recorded decision waive the recipient refusal must not survive it.
//
// The REASON is only rewritten when the recipient verdict would otherwise have
// let the message through. An operator reading "the wording changed" about a
// message somebody had objected to would be told to re-approve the wrong thing.
func applyWordingVerdict(ticket *commsauthz.TransmitTicket, changed bool) {
	if !changed {
		return
	}
	if ticket.Allowed {
		ticket.Reason = "this message was edited after it was authorized, so the wording that " +
			"was checked is not the wording that would go"
	}
	ticket.Allowed = false
	ticket.ConsentRefused = false
}
