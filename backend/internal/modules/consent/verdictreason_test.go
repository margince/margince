// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Every reason code reaches the wire as a value the contract names, and every
// one still reads as an English sentence for an agent.
func TestEveryReasonCodeIsAContractValueWithASentence(t *testing.T) {
	codes := []string{ReasonQualifyingEvent}
	for code := range reasonSentences {
		codes = append(codes, code)
	}
	event := QualifyingEvent{Kind: "inbound_message", OccurredAt: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC)}
	for _, code := range codes {
		if !crmcontracts.ContactConsentGuardEntryReasonCode(code).Valid() {
			t.Errorf("%q is not a reason_code the contract names", code)
		}
		if said(Verdict{ReasonCode: code, Qualifying: &event}).Reason == "" {
			t.Errorf("%q has no sentence", code)
		}
	}
}

func TestAnObjectionQuotesItsDateOnlyWhenTheRecordHasOne(t *testing.T) {
	at := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	dated := said(Verdict{ReasonCode: ReasonObjection, ReasonAt: knownDate(at)})
	if dated.Reason != "they objected on 12 Jul 2026, and an objection overrides every other basis" {
		t.Errorf("dated objection reads %q", dated.Reason)
	}
	undated := said(Verdict{ReasonCode: ReasonObjection, ReasonAt: knownDate(time.Time{})})
	if undated.ReasonAt != nil || undated.Reason != reasonSentences[ReasonObjection] {
		t.Errorf("an objection with no proof row reads %q at %v", undated.Reason, undated.ReasonAt)
	}
}
