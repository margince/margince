// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestRecommendationChecksPreserveAHumanDismissal(t *testing.T) {
	f := facts{deal: crmcontracts.Deal{Name: "Platform renewal", Status: crmcontracts.DealStatusOpen}, now: testNow, loc: time.UTC, lang: "en"}
	original := decideMove(f)
	checked, err := assessMove(f, original)
	if err != nil {
		t.Fatal(err)
	}
	if checked.Recommendation == nil || checked.Recommendation.BasisFingerprint == "" {
		t.Fatal("the checked action lost its source basis")
	}
	if fingerprintOf(checked) != fingerprintOf(original) || moveFingerprint(checked) != moveFingerprint(original) {
		t.Fatal("checking a suggestion undid its dismissal identity")
	}
	f.now = f.now.Add(time.Minute)
	checkedAgain, err := assessMove(f, decideMove(f))
	if err != nil {
		t.Fatal(err)
	}
	if checkedAgain.Recommendation.CheckedAt.Equal(checked.Recommendation.CheckedAt) ||
		!sameCurrentBasis(checked, checkedAgain) || moveFingerprint(checkedAgain) != moveFingerprint(original) {
		t.Fatal("a later observation changed unchanged advice or its dismissal")
	}
	receipt := *checkedAgain.Recommendation
	receipt.PolicyVersion = "a different policy"
	args := semanticArguments(*checkedAgain.Arguments)
	args["recommendation"] = receipt
	checkedAgain.Recommendation, checkedAgain.Arguments = &receipt, &args
	if sameCurrentBasis(checked, checkedAgain) || moveFingerprint(checkedAgain) != moveFingerprint(original) {
		t.Fatal("policy acceptance and human dismissal were coupled")
	}
}

func TestAnActivityRevisionChangesTheRecommendationBasis(t *testing.T) {
	version := crmcontracts.RowVersion(1)
	f := facts{
		deal: crmcontracts.Deal{Status: crmcontracts.DealStatusOpen}, now: testNow, loc: time.UTC,
		currentRequests: []crmcontracts.Activity{{Kind: crmcontracts.ActivityKindEmail, Version: &version}},
	}
	before, err := assessMove(f, decideMove(f))
	if err != nil {
		t.Fatal(err)
	}
	version++
	after, err := assessMove(f, decideMove(f))
	if err != nil {
		t.Fatal(err)
	}
	if sameCurrentBasis(before, after) || before.Recommendation.BasisFingerprint == after.Recommendation.BasisFingerprint {
		t.Fatal("a source edit retained the old recommendation basis")
	}
}

func TestTheChosenReadableRequestRevisionChangesTheBasis(t *testing.T) {
	withheld := crmcontracts.ActivityContentStateWithheld
	first, chosen := crmcontracts.RowVersion(1), crmcontracts.RowVersion(2)
	for _, current := range []bool{true, false} {
		f := facts{deal: openDeal(), now: testNow, loc: time.UTC}
		rows := []crmcontracts.Activity{
			{Kind: crmcontracts.ActivityKindEmail, ContentState: &withheld, Version: &first},
			{Kind: crmcontracts.ActivityKindEmail, Version: &chosen},
		}
		if current {
			f.currentRequests = rows
		} else {
			f.requests = rows
		}
		before, err := moveBasisFingerprint(f)
		if err != nil {
			t.Fatal(err)
		}
		chosen++
		after, err := moveBasisFingerprint(f)
		if err != nil {
			t.Fatal(err)
		}
		if before == after {
			t.Fatal("an edit of the chosen readable request retained the receipt basis")
		}
	}
}

func TestPrivateCaptureObservationsDoNotChangeTheAcceptanceBasis(t *testing.T) {
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, mailCoverage: &crmcontracts.MailCaptureCoverage{State: crmcontracts.MailCaptureCoverageStateUnknown}}
	before, err := assessMove(f, decideMove(f))
	if err != nil {
		t.Fatal(err)
	}
	f.mailCoverage = &crmcontracts.MailCaptureCoverage{State: crmcontracts.MailCaptureCoverageStateUnavailable, EvaluatedAt: testNow.Add(time.Hour)}
	after, err := assessMove(f, decideMove(f))
	if err != nil {
		t.Fatal(err)
	}
	if !sameCurrentBasis(before, after) || moveFingerprint(before) != moveFingerprint(after) {
		t.Fatal("private capture state changed acceptance or a human dismissal")
	}
}
