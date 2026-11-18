// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import "testing"

func TestFooterOmissionsNeedAnInstructionEvenWhenCorrespondenceIsAllowed(t *testing.T) {
	d := Decision{Verdict: VerdictAllow, ReasonCode: ReasonAllowed}
	d = d.WithFooterFindings([]FooterFinding{{Kind: "objection_route"}})
	if d.Verdict != VerdictReview || d.ReasonCode != ReasonFooterIncomplete || !d.OverrulableForOneMessage() {
		t.Fatalf("footer finding is not an overridable review: %+v", d)
	}
	if !(DecisionSet{Decisions: []Decision{d}}).ExceptionMayAnswer(false) {
		t.Fatal("a rep cannot answer the footer warning")
	}
}

func TestFooterFindingsDoNotEraseAnExistingRefusal(t *testing.T) {
	d := Decision{Verdict: VerdictDeny, ReasonCode: ReasonNoEvidence}
	d = d.WithFooterFindings([]FooterFinding{{Kind: "objection_route"}})
	if d.ReasonCode != ReasonNoEvidence || len(d.Evidence.FooterFindings) != 1 {
		t.Fatalf("combined findings lost: %+v", d)
	}
}

func TestFooterOmissionsRequireAReasonInEveryMode(t *testing.T) {
	for _, base := range []Decision{
		{Verdict: VerdictAllow, ReasonCode: ReasonAllowed},
		{Verdict: VerdictDeny, ReasonCode: ReasonNoEvidence},
		{Verdict: VerdictDeny, ReasonCode: ReasonNoMarketingConsent},
	} {
		t.Run(base.ReasonCode, func(t *testing.T) {
			d := base.WithFooterFindings([]FooterFinding{{Kind: "objection_route"}})
			set := DecisionSet{Decisions: []Decision{d}}
			for _, mode := range []Mode{ModeObserve, ModeWarn, ModeEnforce} {
				if !d.WouldRefuse(mode) || set.Effective(func(Category) Mode { return mode }, true) {
					t.Fatalf("footer omission silently passed under %s", mode)
				}
			}
			if d.CanBeOverruledByCategory() {
				t.Fatal("a standing category override hid a footer omission")
			}
			if !set.ExceptionMayAnswer(false) {
				t.Fatal("the rep cannot confirm the omissions for this message")
			}
		})
	}
}
