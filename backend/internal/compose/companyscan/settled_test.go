// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const nudgeQuote = "did the sample reports get held up somewhere?"

func TestAFindingThatSaysAnAskIsOwedIsRefusedOnASettledMessage(t *testing.T) {
	in, nudge := scanInput()
	in.Messages[0].Settled = true
	companyID := ids.New[ids.CompanyKind]()
	for _, kind := range []string{"question_unanswered", "commitment_unmet"} {
		raw := finding(nudge, nudgeQuote)
		raw["kind"] = kind
		kept, refused, err := ParseFindings(reply(raw), companyID, in, "en")
		if err != nil || len(kept) != 0 || len(refused) != 1 {
			t.Errorf("%s on a settled message: kept %d, refused %v, err %v; want it refused", kind, len(kept), refused, err)
		}
	}
	risk := finding(nudge, nudgeQuote)
	risk["kind"] = "risk_raised"
	if kept, _, err := ParseFindings(reply(risk), companyID, in, "en"); err != nil || len(kept) != 1 {
		t.Errorf("a risk on a settled message was refused: kept %d, err %v", len(kept), err)
	}
}

func TestAnOpenMessageStillCarriesItsAsk(t *testing.T) {
	in, nudge := scanInput()
	kept, refused, err := ParseFindings(reply(finding(nudge, nudgeQuote)), ids.New[ids.CompanyKind](), in, "en")
	if err != nil || len(kept) != 1 || len(refused) != 0 {
		t.Errorf("kept %d, refused %v, err %v; want the open ask kept", len(kept), refused, err)
	}
}

func TestAPartnerAccountIsNeverAdvisedToOpenADeal(t *testing.T) {
	in, nudge := scanInput()
	in.Partner = true
	need := finding(nudge, nudgeQuote)
	need["kind"] = "need_raised"
	kept, refused, err := ParseFindings(reply(need), ids.New[ids.CompanyKind](), in, "en")
	if err != nil || len(kept) != 0 || len(refused) != 1 {
		t.Errorf("a need on a partner account: kept %d, refused %v, err %v; want it refused", len(kept), refused, err)
	}
	if kept, _, err := ParseFindings(reply(finding(nudge, nudgeQuote)), ids.New[ids.CompanyKind](), in, "en"); err != nil || len(kept) != 1 {
		t.Errorf("a partner's open question was refused: kept %d, err %v", len(kept), err)
	}
}

func TestOnlyTheModelsOwedFindingsAreHeldToTheThread(t *testing.T) {
	in, nudge := scanInput()
	kept, _, err := ParseFindings(reply(finding(nudge, nudgeQuote)), ids.New[ids.CompanyKind](), in, "en")
	if err != nil || len(kept) != 1 {
		t.Fatalf("kept %d, err %v", len(kept), err)
	}
	if id, ok := owedCitation(kept[0]); !ok || id != nudge.ID {
		t.Errorf("owedCitation = %v, %v; want the cited message", id, ok)
	}
	rule := kept[0]
	rule.WrittenBy = nil
	if _, ok := owedCitation(rule); ok {
		t.Error("a rule row is held to the thread's verdict; only the model's findings are")
	}
}
