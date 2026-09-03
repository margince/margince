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
	"crypto/sha256"
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

	// The legacy gate still rules while the engine is in observe mode, so its
	// answer is taken first and carried onto every row. A disagreement is then
	// readable in the record rather than only in a counter that dies with the
	// process.
	legacyErr := g.RequireGrantedForRecipients(ctx, req.Recipients, req.PurposeKey)
	switch {
	case legacyErr == nil:
	case errors.Is(legacyErr, apperrors.ErrConsentNotGranted):
	default:
		// NOT an answer. The question could not be asked, so nothing has been
		// learned and nothing is recorded — the dispatcher retries.
		return commsauthz.TransmitTicket{}, legacyErr
	}
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
		if err := g.recordDecisions(ctx, tx, req, setID, set); err != nil {
			return err
		}
		ticket.Allowed = set.Effective(modeFor, legacyAllowed)
		ticket.Reason = refusalReason(set, legacyAllowed)
		return nil
	})
	if err != nil {
		return commsauthz.TransmitTicket{}, err
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
	for _, r := range req.Recipients {
		d, err := g.decideOne(ctx, tx, r, req.PurposeKey)
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
// suppresses them, and what the purpose class says.
func (g *Gate) decideOne(ctx context.Context, tx pgx.Tx, r connector.Recipient, purposeKey string) (commsauthz.Decision, error) {
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
		return g.decideLead(ctx, tx, r, purposeKey, d)
	}
	parsed, err := ids.Parse(personID)
	if err != nil {
		return commsauthz.Decision{}, fmt.Errorf("consent: the resolved subject is not an id: %w", err)
	}
	d.SubjectKind, d.SubjectID = entityPerson, parsed

	suppressed, kind, err := liveSuppression(ctx, tx, personID, r)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	if suppressed {
		d.Verdict = commsauthz.VerdictDeny
		d.ReasonCode = kind
		d.Suppression = kind
		return d, nil
	}
	return classVerdict(ctx, tx, personID, purposeKey, d)
}

// classVerdict reads the purpose the delivery was staged under and answers in
// the engine's vocabulary.
//
// It calls VerdictForPerson rather than reimplementing the class model, which
// is what keeps the engine, the legacy transmit gate and the guard endpoint
// answering with one body of code about one person. A second implementation
// here would be a second answer, and the one that stopped matching would look
// exactly like the one that still did.
func classVerdict(ctx context.Context, tx pgx.Tx, personID, purposeKey string, d commsauthz.Decision) (commsauthz.Decision, error) {
	purpose, defined, err := purposeRowFor(ctx, tx, purposeKey)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	if !defined {
		d.Verdict = commsauthz.VerdictDeny
		d.ReasonCode = commsauthz.ReasonUnknownPurpose
		return d, nil
	}
	d.Resolved = categoryForClass(purpose.Class)

	verdict, err := VerdictForPerson(ctx, tx, personID, purpose)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	switch verdict.State {
	case VerdictAllowed:
		d.Verdict = commsauthz.VerdictAllow
		d.ReasonCode = commsauthz.ReasonAllowed
		d.Basis = basisForClass(purpose.Class)
	case VerdictBlocked:
		d.Verdict = commsauthz.VerdictDeny
		d.ReasonCode = blockedReasonCode(verdict)
	default:
		d.Verdict = commsauthz.VerdictReview
		d.ReasonCode = commsauthz.ReasonNoMarketingConsent
	}
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
func (g *Gate) recordDecisions(ctx context.Context, tx pgx.Tx, req commsauthz.TransmitRequest, setID ids.UUID, set commsauthz.DecisionSet) error {
	sum := sha256.Sum256([]byte(req.Subject + "\x00" + req.Body))
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
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
		if _, err := tx.Exec(ctx, `
			INSERT INTO communication_decision
			  (delivery_id, attempt, decision_set_id, recipient_address, subject_kind, subject_id,
			   phase, resolved_category, verdict, reason_code, basis, suppression,
			   content_fingerprint, legacy_verdict, mode, actor)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
			ON CONFLICT (decision_set_id, recipient_address, phase) DO NOTHING`,
			req.DeliveryID, req.Attempt, setID, decisionRecipientKey(d.Recipient),
			subjectKind, subjectID, string(d.Phase), string(d.Resolved), string(d.Verdict),
			d.ReasonCode, nullableBasis(d.Basis), nullableText(d.Suppression),
			sum[:], d.LegacyVerdict, string(d.Mode), by); err != nil {
			return fmt.Errorf("consent: record the transmit decision: %w", err)
		}
	}
	return nil
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
func liveSuppression(ctx context.Context, tx pgx.Tx, personID string, r connector.Recipient) (bool, string, error) {
	var kind string
	// STRONGEST first, not newest. Ordering by time would let a weaker
	// suppression recorded later mask an objection recorded earlier, and the
	// reason code is what decides whether a refusal survives observe mode — so
	// a masked objection is a message that goes out to somebody who said stop.
	// This is also the only reader in the tree that applies these rows: the
	// legacy gate reads person_consent and never looks here.
	err := tx.QueryRow(ctx, `
		SELECT kind FROM communication_suppression
		 WHERE revoked_at IS NULL
		   AND (person_id = $1
		        OR lead_id = $1
		        OR (address IS NOT NULL AND $2 <> '' AND lower(address) = lower($2)))
		 ORDER BY CASE kind
		            WHEN 'marketing_objection'    THEN 0
		            WHEN 'processing_restriction' THEN 1
		            WHEN 'subject_request'        THEN 2
		            WHEN 'hard_bounce'            THEN 3
		            ELSE 4
		          END, recorded_at DESC
		 LIMIT 1`, personID, r.Email).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("consent: read the recipient's suppressions: %w", err)
	}
	switch kind {
	case "marketing_objection":
		return true, commsauthz.ReasonObjection, nil
	case "processing_restriction":
		return true, commsauthz.ReasonRestricted, nil
	case "hard_bounce":
		return true, commsauthz.ReasonHardBounce, nil
	default:
		// A kind this code does not recognise still SUPPRESSES, and absolutely.
		// Somebody added a row shape to the constraint and not to this switch;
		// treating the unknown case as the weaker one would let the next kind
		// added to the table be the one that sends.
		return true, commsauthz.ReasonRestricted, nil
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
