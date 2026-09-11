// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The decision taken as a message is written down, in the same transaction that
// writes it.
//
// The transmit decision beside this one answers "may this go out now" and is
// the last word. It cannot be the only word: it runs in a worker, minutes or
// days later, and its refusal reaches a parked row and an operator's lane
// rather than the contact who typed the message. A staging decision is what
// makes a refusal answerable — the rep is still there, and the message has not
// yet been promised to anybody.
//
// It commits with the activity, the delivery row, the audit entry, the outbox
// event and the job. All of them or none: a decision recorded for a delivery
// that rolled back would describe a message nobody sent, and a delivery staged
// with no decision is the gap the transmit ticket exists to catch.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// AuthorizeStagingTx records why this message was allowed to be queued, one row
// per recipient, on the caller's transaction.
//
// It does NOT open its own: the whole point is to commit with the delivery it
// describes. That also means it must not acquire a connection of its own, which
// is why every read below runs on the passed tx.
func (g *Gate) AuthorizeStagingTx(ctx context.Context, tx pgx.Tx, deliveryID ids.UUID, req commsauthz.Request) (commsauthz.DecisionSet, error) {
	return g.stageDecisions(ctx, tx, deliveryID, req, nil)
}

// AuthorizeStagingWithDecisionTx is the same staging, told where to look for a
// recorded decision that may authorize a refusal.
//
// SEPARATE ENTRY POINT rather than a wider one, so the ordinary call cannot
// acquire this behaviour by accident. Three callers stage messages; exactly one
// of them resumes a held message, and it is the one that says so here.
//
// The instruction is resolved BEFORE the decision rows are written, because the
// authority is part of the finding each row records and those rows are never
// updated afterwards — migration 1788529047 revoked UPDATE on them from the
// runtime role, on the ground that a proof the application can silently edit is
// not a proof.
func (g *Gate) AuthorizeStagingWithDecisionTx(
	ctx context.Context, tx pgx.Tx, deliveryID ids.UUID, req commsauthz.Request, intentID ids.UUID, authored [32]byte,
) (commsauthz.DecisionSet, ids.UUID, error) {
	directed := &directedExecution{intentID: intentID, authored: authored, wanted: true}
	set, err := g.stageDecisions(ctx, tx, deliveryID, req, directed)
	if err != nil {
		return commsauthz.DecisionSet{}, ids.UUID{}, err
	}
	return set, directed.instructionID, nil
}

func (g *Gate) stageDecisions(ctx context.Context, tx pgx.Tx, deliveryID ids.UUID, req commsauthz.Request, directed *directedExecution) (commsauthz.DecisionSet, error) {
	for _, r := range req.Recipients {
		if err := r.Validate(); err != nil {
			return commsauthz.DecisionSet{}, fmt.Errorf(
				"consent: this recipient cannot be put to the engine: %w", err)
		}
	}
	if len(req.Recipients) == 0 {
		return commsauthz.DecisionSet{}, fmt.Errorf(
			"consent: a staging decision needs at least one recipient: %w", apperrors.ErrInvalidArgument)
	}

	// The posture, read on the caller's transaction — the same one the
	// decision rows commit on, so a row's stamped mode is the one that was
	// live when it was taken.
	modes, err := settings.ApplyTx(ctx, tx, AuthorizationModes)
	if err != nil {
		return commsauthz.DecisionSet{}, err
	}

	setID := ids.NewV7()
	set := commsauthz.DecisionSet{}
	for _, r := range req.Recipients {
		d, err := g.decideOne(ctx, tx, r, req, commsauthz.PhaseStaging)
		if err != nil {
			return commsauthz.DecisionSet{}, err
		}
		d.Phase = commsauthz.PhaseStaging
		// From the RESOLVED category, so the authority stamped on the row is
		// the one belonging to what the engine decided the message is.
		d.Mode = ModeFor(modes, d.Resolved)
		d.Requested = req.Context
		set.Decisions = append(set.Decisions, d)
	}
	// THE AUTHORITY IS DECIDED BEFORE THE ROWS ARE WRITTEN, because it is part
	// of what each row records and those rows are never updated afterwards.
	//
	// Only when the engine actually refuses. A message the engine allows goes
	// out on that permission and spends nobody's decision — asking otherwise
	// would consume an instruction on a send that never needed one.
	if directed != nil && directed.wanted && refusesAnyRecipient(set) {
		instruction, err := g.AuthorizeDirectedExecutionTx(
			ctx, tx, directed.intentID, deliveryID, directed.authored)
		if err != nil {
			return commsauthz.DecisionSet{}, err
		}
		directed.instructionID = instruction
	}
	if err := g.recordStagingDecisions(ctx, tx, deliveryID, setID, req, set, directedAuthority(directed)); err != nil {
		return commsauthz.DecisionSet{}, err
	}
	return set, nil
}

// directedExecution carries the question "is there a recorded decision standing
// over this refusal" into staging, and the answer back out.
type directedExecution struct {
	intentID ids.UUID
	authored [32]byte
	wanted   bool
	// instructionID is the decision that was spent, set on the way out. Zero
	// when nothing was decided, which is the ordinary case.
	instructionID ids.UUID
}

// directedAuthority answers the instruction to stamp on the refused rows, or
// zero when the message goes out on the engine's own permission.
func directedAuthority(directed *directedExecution) ids.UUID {
	if directed == nil {
		return ids.UUID{}
	}
	return directed.instructionID
}

// refusesAnyRecipient reports whether the engine said no to anybody, which is
// the only case a recorded decision has anything to authorize.
func refusesAnyRecipient(set commsauthz.DecisionSet) bool {
	for _, d := range set.Decisions {
		if d.Verdict != commsauthz.VerdictAllow {
			return true
		}
	}
	return false
}

// recordStagingDecisions writes the rows.
//
// attempt is 0 and means "before any attempt": the delivery has not been picked
// up yet, and every transmit row that follows carries the attempt it belonged
// to. The fingerprint is of the message as staged, so a later reader can tell
// whether what went out is what was authorized.
func (g *Gate) recordStagingDecisions(ctx context.Context, tx pgx.Tx, deliveryID, setID ids.UUID, req commsauthz.Request, set commsauthz.DecisionSet, instruction ids.UUID) error {
	sum := SendingDigest(req.Subject, req.Body, req.HTMLBody)
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	for _, d := range set.Decisions {
		// The ids THIS RECIPIENT'S decision was taken on, so the transmit phase
		// can put the same questions to the same records.
		//
		// From the decision and not from the request: decideOne copies the
		// request's evidence onto the decision only when the resolution passed
		// refuseUnreadableEvidence, so an id the caller was never shown to hold
		// is absent here rather than being handed to a phase that runs as the
		// system principal. See authorizeevidencecarry.go, and decideOne's own
		// note on why the two differ.
		evidence, err := evidenceJSON(d.Evidence)
		if err != nil {
			return err
		}
		subjectKind := nullableText(d.SubjectKind)
		var subjectID *ids.UUID
		if d.SubjectKind != "" {
			id := d.SubjectID
			subjectID = &id
		}
		// NOT COUNTED. See decisioncounter.go: this runs on a transaction this
		// function does not own, and an enforced refusal is delivered by
		// rolling that transaction back (compose/commsstager.go refuseAtStaging),
		// so a staging deny leaves no row. A counter incremented here would
		// report refusals the record does not hold, and would do it under the
		// shipped posture rather than in some corner.
		//
		// A denial under observe or warn DOES commit, and those rows are real.
		// They are read from the table, not from a counter that would mean one
		// thing in one posture and another in the next.
		// THE AUTHORITY THIS RECIPIENT'S MESSAGE GOES OUT UNDER, written with
		// the finding rather than stamped on afterwards: these rows are never
		// updated (migration 1788529047), and a row that had to be corrected
		// later would be a proof the application can edit.
		//
		// Named on the REFUSED rows only. A message to three people may be
		// allowed for two of them and directed for the third, and saying all
		// three went out on somebody's decision would overstate what was
		// decided — the human was shown one refusal and signed for that one.
		authority, instructionID := AuthoritySupported, (*ids.UUID)(nil)
		if !instruction.IsZero() && d.Verdict != commsauthz.VerdictAllow {
			id := instruction
			authority, instructionID = AuthorityInstruction, &id
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO communication_decision
			  (delivery_id, attempt, decision_set_id, recipient_address, subject_kind, subject_id,
			   phase, requested_category, resolved_category, verdict, reason_code, basis, suppression,
			   content_fingerprint, mode, actor, evidence, execution_authority, instruction_id)
			VALUES ($1,0,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
			ON CONFLICT (decision_set_id, recipient_address, phase) DO NOTHING`,
			deliveryID, setID, decisionRecipientKey(d.Recipient),
			subjectKind, subjectID, string(d.Phase), nullableCategory(d.Requested),
			string(d.Resolved), string(d.Verdict), d.ReasonCode,
			nullableBasis(d.Basis), nullableText(d.Suppression),
			sum[:], string(d.Mode), by, evidence, authority, instructionID)
		if err != nil {
			return fmt.Errorf("consent: record the staging decision: %w", err)
		}
	}
	return nil
}

// nullableCategory keeps the requested category NULL when the caller claimed
// none, which is the honest record: a door that said nothing is different from
// one that named a category the engine then disagreed with.
func nullableCategory(c commsauthz.Category) *string {
	if c == "" {
		return nil
	}
	v := string(c)
	return &v
}
