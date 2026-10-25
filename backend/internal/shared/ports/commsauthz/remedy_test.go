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

// TestOnlyAnAdminMayTakeAnExceptionPastASubjectsStop holds that a subject's own
// stop lists the admin exception and never the plain one every sender holds.
func TestOnlyAnAdminMayTakeAnExceptionPastASubjectsStop(t *testing.T) {
	t.Parallel()
	for _, reason := range reasonConstants(t) {
		if LevelForReason(reason) != LevelSubject {
			continue
		}
		if HasRemedy(reason, RemedyException) {
			t.Errorf("%q is the subject's own stop but lists the exception any sender may take", reason)
		}
		if !HasRemedy(reason, RemedyAdminException) {
			t.Errorf("%q is the subject's own stop and an admin may still send past it, yet it lists no admin exception", reason)
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
