// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Turning a resolution into a decision: what the engine concluded the message
// is, and whether that alone permits it.
//
// Split from authorizetransmit.go for the file-length ceiling, and the seam is
// the honest one — that file answers "who is this recipient and what stops
// them", and this one answers "what is this message and does the record carry
// it". The two are asked in order and are separate questions.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// decideResolved answers about a person once nothing suppresses them: what the
// record says this message is, and whether that is supported.
//
// The resolution decides the CATEGORY and the legacy verdict decides the
// PERMISSION, and both are recorded. Keeping them separate is what makes the
// engine measurable: a row can say "this is a reply, and the old gate refused
// it", which is exactly the disagreement a rollout needs to see before anybody
// flips a mode.
func (g *Gate) decideResolved(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef, d commsauthz.Decision) (commsauthz.Decision, error) {
	res, err := g.resolveCategory(ctx, tx, req, subject)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	if res.Supported {
		// The ground goes on the record BEFORE the message is permitted on it.
		// A basis written afterwards would be describing a send that already
		// happened, and one never written at all is the gap that made the
		// subject-access export answer "we relied on nothing".
		w, err := g.windowsFor(ctx, tx)
		if err != nil {
			return commsauthz.Decision{}, err
		}
		if err := recordBasis(ctx, tx, subject, res, req, w); err != nil {
			return commsauthz.Decision{}, err
		}
		// The record bears the category out on its own evidence, so the engine
		// allows on that ground and names it. This is the arm the old model had
		// no way to reach: a reply to a thread the subject started needed no
		// consent row, and the purpose gate had no way to know it was a reply.
		d.Resolved = res.Category
		d.Verdict = commsauthz.VerdictAllow
		d.ReasonCode = commsauthz.ReasonAllowed
		d.Basis = res.Basis
		return d, nil
	}
	// AN UNSUPPORTED CLAIM IS RECORDED, NEVER RESOLVED TO.
	//
	// Resolved steers two things that decide whether mail goes out: which
	// rollout mode applies (Effective reads modeFor(d.Resolved)) and whether
	// the jurisdiction's advertising ceiling is counted at all
	// (applyFrequencyCap returns early unless Resolved is marketing). Letting
	// an unproven claim set it would hand a caller both — a marketing send
	// claiming active_deal_followup would skip the ceiling, and its decision
	// row would then not count against the next send either, degrading the
	// ceiling for that address permanently.
	//
	// So the claim goes to Requested, which is exactly the column that exists
	// to record what somebody asked for, and Resolved stays with what the
	// engine itself worked out.
	//
	// TWO GUARDS HOLD THIS, and either alone is sufficient: not assigning here,
	// and legacyVerdictFor assigning Resolved from the purpose class below.
	// Said out loud because a mutation check on either one PASSES — the other
	// covers it — so a reader who deletes one sees a green suite and concludes
	// it was redundant. Only reverting both together fails
	// TestAnUnsupportedClaimIsRecordedButNeverResolvedTo, which is what that
	// test's own comment records.
	d.Requested = res.Category
	return legacyVerdictFor(ctx, tx, subject.ID, req.LegacyPurposeKey, res, d)
}

// legacyVerdictFor answers on the old purpose model when the record supports no
// category on its own.
//
// It calls VerdictForPerson rather than reimplementing the class model, which
// is what keeps the engine, the legacy transmit gate and the guard endpoint
// answering with one body of code about one person. A second implementation
// here would be a second answer, and the one that stopped matching would look
// exactly like the one that still did.
func legacyVerdictFor(ctx context.Context, tx pgx.Tx, personID, purposeKey string, res resolution, d commsauthz.Decision) (commsauthz.Decision, error) {
	purpose, defined, err := purposeRowFor(ctx, tx, purposeKey)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	if !defined {
		d.Verdict = commsauthz.VerdictDeny
		d.ReasonCode = commsauthz.ReasonUnknownPurpose
		return d, nil
	}
	// The engine's OWN reading of what this message is, derived from the
	// purpose row rather than from anything a caller said.
	//
	// The second of the two guards named in decideResolved: this assignment
	// alone would correct a claim that reached Resolved, and not assigning
	// there alone would keep it out. Neither is redundant — they fail in
	// different directions, and a decision that never reaches this function
	// (the supported arm) is covered only by the first.
	d.Resolved = resolutionForClass(purpose.Class).Category

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
		// The resolution's own reason, not a blanket "no marketing consent".
		// An unevidenced operational claim and an unconsented marketing send
		// are different problems with different fixes, and a reader who is
		// told the wrong one goes looking in the wrong place.
		d.Verdict = commsauthz.VerdictReview
		d.ReasonCode = res.Reason
	}
	return d, nil
}
