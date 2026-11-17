// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import "testing"

// TestEveryReasonDeclaresItsRemedy fails when a Reason constant has no entry in
// the remedy table, so a new refusal cannot be a silent dead end.
func TestEveryReasonDeclaresItsRemedy(t *testing.T) {
	t.Parallel()
	for _, reason := range reasonConstants(t) {
		if _, declared := remedies[reason]; !declared {
			t.Errorf("reason %q declares no remedy; write its entry in remedies, even an empty one", reason)
		}
	}
	for reason := range remedies {
		found := false
		for _, constant := range reasonConstants(t) {
			found = found || constant == reason
		}
		if !found {
			t.Errorf("remedies names %q, which no Reason constant declares", reason)
		}
	}
}


// TestNoAnswerOrFactSuppliesConsentOrLiftsAStop holds that advertising and every
// stop stay out of reach of a sender's answer or a recorded fact.
func TestNoAnswerOrFactSuppliesConsentOrLiftsAStop(t *testing.T) {
	t.Parallel()
	for _, reason := range []string{
		ReasonNoMarketingConsent, ReasonObjection, ReasonRestricted, ReasonSubjectRequest,
		ReasonConsentWithdrawn, ReasonHardBounce, ReasonUnconfirmedDOI, ReasonFrequencyCapReached,
		ReasonOptedOutAfterRequest, ReasonMarketingConsentRequired,
	} {
		if HasRemedy(reason, RemedyAnswer) || HasRemedy(reason, RemedyFact) {
			t.Errorf("%q must not be resolvable by an answer or a recorded fact", reason)
		}
	}
}

func TestRemediesForReturnsACopyAndNothingForAnUnknownReason(t *testing.T) {
	t.Parallel()
	got := RemediesFor(ReasonNoEvidence)
	got[0] = RemedyAdminException
	if !HasRemedy(ReasonNoEvidence, RemedyAnswer) {
		t.Error("mutating the returned slice changed the table")
	}
	if len(RemediesFor("made_up")) != 0 {
		t.Error("an undeclared reason offers a remedy")
	}
}


// A bounced address stops the message, whoever recorded the exception.
func TestNoStaffExceptionAnswersAHardBounce(t *testing.T) {
	t.Parallel()
	missing := Decision{Verdict: VerdictDeny, ReasonCode: ReasonNoMarketingConsent}
	bounced := Decision{Verdict: VerdictDeny, ReasonCode: ReasonHardBounce}
	if !(DecisionSet{Decisions: []Decision{missing}}).ExceptionMayAnswer(false) {
		t.Error("a rep's exception does not answer missing marketing consent")
	}
	if (DecisionSet{Decisions: []Decision{missing, bounced}}).ExceptionMayAnswer(false) {
		t.Error("a rep's exception answers a hard bounce")
	}
	if (DecisionSet{Decisions: []Decision{missing, bounced}}).ExceptionMayAnswer(true) {
		t.Error("an admin's exception answers a hard bounce")
	}
}

// A rep may send one message past every policy refusal the registry lists. A
// dead address is no policy answer, so it stops the message whoever decided.
func TestARepMaySendPastEveryPolicyRefusalButNotADeadAddress(t *testing.T) {
	t.Parallel()
	for _, reason := range RefusalCodes() {
		refused := Decision{Verdict: VerdictDeny, ReasonCode: reason}
		mayPass := reason != ReasonHardBounce
		if got := (DecisionSet{Decisions: []Decision{refused}}).ExceptionMayAnswer(false); got != mayPass {
			t.Errorf("a rep's exception over %q = %v, want %v", reason, got, mayPass)
		}
		if got := refused.OverrulableForOneMessage(); got != mayPass {
			t.Errorf("%q offered for one message = %v, want %v", reason, got, mayPass)
		}
	}
	stop := Decision{Verdict: VerdictDeny, ReasonCode: ReasonObjection}
	bounced := Decision{Verdict: VerdictDeny, ReasonCode: ReasonHardBounce}
	if (DecisionSet{Decisions: []Decision{stop, bounced}}).ExceptionMayAnswer(true) {
		t.Error("an exception answers a set that holds a dead address")
	}
	if (DecisionSet{}).ExceptionMayAnswer(true) {
		t.Error("an exception answers an empty check")
	}
	allowed := Decision{Verdict: VerdictAllow, ReasonCode: ReasonAllowed}
	if allowed.OverrulableForOneMessage() {
		t.Error("an allowed recipient is offered an override")
	}
}
