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

// TestNobodySendsPastASubjectsStop holds that a subject's own stop lists no
// remedy at all: no answer, no fact, and no exception, an admin's included.
func TestNobodySendsPastASubjectsStop(t *testing.T) {
	t.Parallel()
	stops := 0
	for _, reason := range reasonConstants(t) {
		if LevelForReason(reason) != LevelSubject {
			continue
		}
		stops++
		if got := RemediesFor(reason); len(got) != 0 {
			t.Errorf("%q is the subject's own stop and lists %v; nobody may send past it", reason, got)
		}
	}
	if stops != 5 {
		t.Errorf("found %d subject-level stops, want the five: objection, withdrawal, restriction, request to stop, opt-out after a request", stops)
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

func TestAnExceptionAnswersNoSetThatHoldsASubjectsStop(t *testing.T) {
	t.Parallel()
	missing := Decision{Verdict: VerdictDeny, ReasonCode: ReasonNoMarketingConsent}
	bounced := Decision{Verdict: VerdictDeny, ReasonCode: ReasonHardBounce}
	allowed := Decision{Verdict: VerdictAllow, ReasonCode: ReasonAllowed}
	if (DecisionSet{Decisions: []Decision{missing, bounced, allowed}}).ExceptionMayAnswer(true) {
		t.Error("an admin's exception answers a bounced address")
	}
	for _, stop := range []string{ReasonObjection, ReasonConsentWithdrawn, ReasonRestricted, ReasonSubjectRequest} {
		set := DecisionSet{Decisions: []Decision{missing, {Verdict: VerdictDeny, ReasonCode: stop}}}
		if set.ExceptionMayAnswer(true) {
			t.Errorf("an admin's exception answers a set holding %q", stop)
		}
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

// Where the recipient's country requires prior consent, nobody records an
// exception in its place. The country's advertising ceiling is not lifted for
// one message either.
func TestNoExceptionStandsInForConsentWhereTheCountryRequiresIt(t *testing.T) {
	t.Parallel()
	for _, reason := range []string{ReasonMarketingConsentRequired, ReasonFrequencyCapReached, ReasonOptedOutAfterRequest} {
		if got := RemediesFor(reason); len(got) != 0 {
			t.Errorf("%q lists %v; nobody may send past it, an admin included", reason, got)
		}
		set := DecisionSet{Decisions: []Decision{{Verdict: VerdictDeny, ReasonCode: reason}}}
		if set.ExceptionMayAnswer(true) {
			t.Errorf("an admin's exception answers %q", reason)
		}
	}
	if !HasRemedy(ReasonNoMarketingConsent, RemedyException) {
		t.Error("outside such a country missing marketing consent keeps the exception")
	}
}

func TestAnExceptionCannotWaiveAnAbsoluteRefusal(t *testing.T) {
	t.Parallel()
	for reason := range absoluteDenials {
		set := DecisionSet{Decisions: []Decision{{Verdict: VerdictDeny, ReasonCode: reason}}}
		if set.ExceptionMayAnswer(false) || set.ExceptionMayAnswer(true) {
			t.Errorf("an exception waives the absolute refusal %s", reason)
		}
	}
	if (DecisionSet{}).ExceptionMayAnswer(true) {
		t.Error("an exception answers an empty check")
	}
}
