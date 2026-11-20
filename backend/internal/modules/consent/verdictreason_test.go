// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"strings"
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

// The no-connection sentence tells a rep what would change the answer, so it
// may name only evidence latestQualifyingEvent reads. A deal is read on the
// send path alone, through the message's own deal links.
func TestTheNoConnectionSentenceNamesOnlyEvidenceTheGuardReads(t *testing.T) {
	read := map[string]bool{kindInboundMessage: true, KindInquiry: true, KindMeeting: true}
	for kind := range handRecordedKinds {
		read[kind] = true
	}
	wordsFor := map[crmcontracts.ConsentQualifyingEventKind][]string{
		crmcontracts.ConsentQualifyingEventKindActiveDeal:         {"deal"},
		crmcontracts.ConsentQualifyingEventKindInPerson:           {"met", "in person"},
		crmcontracts.ConsentQualifyingEventKindInboundMessage:     {"written", "wrote"},
		crmcontracts.ConsentQualifyingEventKindInquiry:            {"inquiry", "asked"},
		crmcontracts.ConsentQualifyingEventKindMeeting:            {"met", "meeting"},
		crmcontracts.ConsentQualifyingEventKindRequestedBySubject: {"asked"},
	}
	for kind := range read {
		if _, named := wordsFor[crmcontracts.ConsentQualifyingEventKind(kind)]; !named {
			t.Errorf("the guard reads %q, and this test has no words for it", kind)
		}
	}
	sentence := said(Verdict{ReasonCode: ReasonNoConnection}).Reason
	for kind, words := range wordsFor {
		if read[string(kind)] {
			continue
		}
		for _, word := range words {
			if strings.Contains(sentence, word) {
				t.Errorf("%q names %q, evidence of kind %s that the guard never reads", sentence, word, kind)
			}
		}
	}
}
