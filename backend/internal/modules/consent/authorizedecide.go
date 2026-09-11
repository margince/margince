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
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// resolveAndRecord answers the half of the question that rests on evidence, for
// EITHER subject kind, and writes the ground down when it holds.
//
// Shared by the contact arm and the lead arm because it is one invariant: a
// category borne out by the record allows on that ground, and the ground is
// recorded before the send relies on it. Two spellings of that would be two
// answers, and the lead one is the copy that would rot — leads reach this code
// far less often than contacts do.
//
// The returned resolution's Supported reports whether it settled. When it did
// not, the caller asks its own subject kind's grant, which is the ONLY part
// that differs: a contact's is VerdictForContact and a lead's is grantedForLead,
// and each reads a column the other's query does not.
func (g *Gate) resolveAndRecord(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef, phase commsauthz.Phase, suppressed bool) (resolution, error) {
	res, err := g.resolveCategory(ctx, tx, req, subject)
	if err != nil {
		return resolution{}, err
	}
	if !res.Supported {
		return res, nil
	}
	// The ground goes on the record BEFORE the message is permitted on it.
	// A basis written afterwards would be describing a send that already
	// happened, and one never written at all is the gap that made the
	// subject-access export answer "we relied on nothing".
	// RECORDED AT STAGING ONLY, and the phase is why. A basis is scoped
	// to the conversation it was earned on, and that scope comes from the
	// anchor — which communication_decision does not store, so the transmit
	// phase cannot recover it. A transmit-phase write would therefore be an
	// UNSCOPED row matching every other unscoped one, collapsing the thread
	// separation staging established. Transmit still re-checks the
	// evidence; what it does not do is write a second, weaker record of it.
	// NOT WHILE A SUPPRESSION STANDS. A basis row asserts we hold a lawful
	// ground to write to this contact; writing one about somebody whose
	// processing is restricted is itself processing, and it lands in their own
	// Art. 15 export as a claim made after they said stop. The category is
	// still resolved — the decision row records what the message was — but the
	// ground is not written down.
	if phase == commsauthz.PhaseStaging && !suppressed {
		w, err := g.store.packRulesFor(ctx, tx)
		if err != nil {
			return resolution{}, err
		}
		if err := recordBasis(ctx, tx, subject, res, req, w); err != nil {
			return resolution{}, err
		}
	}
	return res, nil
}

// allowOn stamps a decision with a resolution the record bore out.
//
// This is the arm the old model had no way to reach: a reply to a thread the
// subject started needed no consent row, and the purpose gate had no way to
// know it was a reply.
func allowOn(d commsauthz.Decision, res resolution) commsauthz.Decision {
	d.Resolved = res.Category
	d.Verdict = commsauthz.VerdictAllow
	d.ReasonCode = commsauthz.ReasonAllowed
	d.Basis = res.Basis
	return d
}

// decideResolved answers about a contact once nothing suppresses them: what the
// record says this message is, and whether that is supported.
//
// The resolution decides the CATEGORY and the legacy verdict decides the
// PERMISSION, and both are recorded. Keeping them separate is what makes the
// engine measurable: a row can say "this is a reply, and the old gate refused
// it", which is exactly the disagreement a rollout needs to see before anybody
// flips a mode.
func (g *Gate) decideResolved(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef, d commsauthz.Decision, phase commsauthz.Phase, suppressed bool) (commsauthz.Decision, error) {
	res, err := g.resolveAndRecord(ctx, tx, req, subject, phase, suppressed)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	// ONLY EVIDENCE THIS CALLER WAS SHOWN TO HOLD travels any further.
	//
	// refuseUnreadableEvidence runs inside validate, and validate runs only for
	// arm 3 of resolution — the thread and live-deal arms answer before it. So
	// a message allowed by one of those two carries evidence ids nobody has
	// checked this caller may see, and writing them to the decision row would
	// hand them to the transmit phase. That phase runs under the system
	// principal, for which auth.Require returns nil, so an unchecked id there
	// becomes an authorization the sender could never have obtained: name any
	// invoice in the installation, get allowed by an open deal at staging, and
	// have the invoice arm allow it at transmit.
	//
	// EvidenceChecked is the resolution's own record of having passed that
	// check, so what reaches the row is the intersection of "named" and
	// "readable by the contact who named it".
	if res.EvidenceChecked {
		d.Evidence = req.Evidence
	}
	if res.Supported {
		// The record bears the category out — and the subject may still have
		// said stop. The evidence arms never read contact_consent, so this is
		// the only place a withdrawal is put to them.
		stopped, err := withdrawalCovers(ctx, tx, subject, res.Category)
		if err != nil {
			return commsauthz.Decision{}, err
		}
		if stopped {
			d.Resolved = res.Category
			d.Verdict = commsauthz.VerdictDeny
			d.ReasonCode = commsauthz.ReasonConsentWithdrawn
			return d, nil
		}
		return allowOn(d, res), nil
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
	return g.legacyVerdictFor(ctx, tx, subject.ID, req.LegacyPurposeKey, res, d, suppressed)
}

// legacyVerdictFor answers on the old purpose model when the record supports no
// category on its own.
//
// It calls VerdictForContact rather than reimplementing the class model, which
// is what keeps the engine, the legacy transmit gate and the guard endpoint
// answering with one body of code about one contact. A second implementation
// here would be a second answer, and the one that stopped matching would look
// exactly like the one that still did.
func (g *Gate) legacyVerdictFor(ctx context.Context, tx pgx.Tx, contactID, purposeKey string, res resolution, d commsauthz.Decision, suppressed bool) (commsauthz.Decision, error) {
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

	w, err := g.store.packRulesFor(ctx, tx)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	// NO Advertises. Nothing on a send names the GOODS it advertises: the
	// nearest field, Request.MarketingPurpose, is a consent purpose key
	// ("newsletter"), and similar_goods_note is free text a rep typed about a
	// sale ("espresso machines"). Comparing the two is not a weak check, it is
	// a satisfiable one — a rep who types the purpose key into the note field
	// would hold §7(3) authority for that contact forever, which is the hole
	// this file exists to close, relocated.
	//
	// So an exception requiring similarity refuses here, exactly as it does for
	// the legacy gate and the guard, until a caller can honestly say what a
	// message advertises.
	verdict, err := VerdictForContact(ctx, tx, contactID, purpose, time.Now().Add(-w.reply),
		MarketingContext{Exception: w.marketingException})
	if err != nil {
		return commsauthz.Decision{}, err
	}
	switch verdict.State {
	case VerdictAllowed:
		// THE TRANSACTIONAL CLASS DOES NOT CARRY ITSELF ON THE SEND PATH.
		//
		// VerdictForContact allows ClassTransactional unconditionally, because
		// Art 6(1)(b) really does mean the contract is the basis and the guard
		// endpoint has to say so about a contact who has an invoice coming. But
		// that answer is about a CONTACT, and this function is about a MESSAGE:
		// we are only here because resolveCategory found nothing supporting
		// this one — no thread, no live deal, no accepted claim — and
		// resolutionForClass already said so with legacy_transactional_unevidenced.
		//
		// Taking the allow anyway is what let any message calling itself
		// operational become one, on nothing but the purpose key. The lead arm
		// closed this hole already (TestALeadTakesNoAuthorityFromATransactionalPurpose);
		// this is the same hole for contacts, and the reason code is the one the
		// resolution had picked before the legacy gate overrode it.
		//
		// The guard endpoint and the legacy gate keep VerdictForContact's answer
		// untouched: an invoice with real evidence resolves as supported and
		// returns from decideResolved's supported arm, never reaching here.
		if purpose.Class == ClassTransactional {
			d.Verdict = commsauthz.VerdictReview
			d.ReasonCode = commsauthz.ReasonLegacyTransactionalUnevidenced
			break
		}
		// A basis this call DERIVED is written down before the send relies on
		// it (Art. 5(2)). The engine is the only authority now, so it is the
		// only thing left that can make that record: grantedForRecipient used
		// to stamp here, and nothing calls it on the send path any more.
		// Not while a suppression stands, for recordBasis's reason: the stamp
		// writes a consent_qualifying_event asserting a ground to correspond,
		// and the send is about to be refused anyway.
		if err := stampDerivedBasis(ctx, tx, contactID, verdict, suppressed); err != nil {
			return commsauthz.Decision{}, err
		}
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
