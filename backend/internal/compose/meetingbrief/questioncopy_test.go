// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestQuestionLanguageKeepsGapsAndEvidence(t *testing.T) {
	langcopytest.Census(t, questionWords)
	empty := Input{ActivityID: meetingID, Company: "Original Company"}
	typ := MeetingType{Value: crmcontracts.MeetingPlanTypeValueMeetingPlanTypeUnknown}
	english := unknownsFor(empty, typ, nil)
	for _, language := range []string{"en", "de", "vi"} {
		in := empty
		in.Language = language
		gaps := unknownsFor(in, typ, nil)
		if len(gaps) != len(english) {
			t.Fatal("language changed missing facts")
		}
		for i, gap := range gaps {
			if gap.Kind != english[i].Kind || gap.Question == "" || (language != "en" && gap.Question == english[i].Question) {
				t.Fatalf("%s gap: %+v", language, gap)
			}
		}
		original := questionsFor(empty, english, rankClaims(empty))
		got := questionsFor(in, gaps, rankClaims(in))
		if len(got) != len(original) || len(got) == 0 {
			t.Fatal("language changed questions")
		}
		for i, question := range got {
			if !reflect.DeepEqual(question.Evidence, original[i].Evidence) || !strings.Contains(question.Why, in.Company) {
				t.Fatal("language changed question basis")
			}
		}
	}
}

func TestClosedClaimsAreNotDescribedAsOutstanding(t *testing.T) {
	for _, language := range []string{"en", "de", "vi"} {
		said := briefCopyFor(language)
		for _, kind := range []string{kindCommitmentOurs, kindCommitmentTheirs, kindOpenQuestion, kindObjection} {
			claim := ClaimIn{Kind: kind, ContactName: "Original Name", Body: "Original quotation", SourceID: activityID, Status: statusOpen}
			openLedger := said.commitmentLine(claim)
			for _, status := range []string{"done", "dismissed", "future_status", ""} {
				claim.Status = status
				ledger := said.commitmentLine(claim)
				if ledger == openLedger || !strings.Contains(ledger, said.claimStatus(status)) {
					t.Fatalf("%s %s lost status %q", language, kind, status)
				}
				if !strings.Contains(ledger, claim.Body) {
					t.Fatal("status wording lost source text")
				}
				if language == "en" && (strings.Contains(ledger, "We owe") || strings.Contains(ledger, "owes us")) {
					t.Fatalf("closed claim described as outstanding: %s", ledger)
				}
			}
		}
	}
}

func TestClosedClaimsDoNotBecomeFreshMeetingPrompts(t *testing.T) {
	for _, language := range []string{"en", "de", "vi"} {
		in := fullInput()
		in.Language = language
		in.Commitments = nil
		for _, status := range []string{"done", "dismissed", ""} {
			for _, kind := range []string{kindCommitmentOurs, kindCommitmentTheirs, kindOpenQuestion, kindObjection} {
				in.Commitments = append(in.Commitments, ClaimIn{Kind: kind, Status: status, Body: "Closed matter", SourceID: activityID})
			}
		}
		if got := likelyAsksFor(in, rankClaims(in)); len(got) != 0 {
			t.Fatalf("%s surfaced closed likely asks: %+v", language, got)
		}
		if got := questionsFor(in, nil, rankClaims(in)); len(got) != 0 {
			t.Fatalf("%s surfaced closed questions: %+v", language, got)
		}
		points := briefCopyFor(language).talkingPointsSection(rankClaims(in))
		for _, point := range points {
			if point.Text == fmt.Sprintf(briefCopyFor(language).Say(sectionWords.TheirPromise), "", "Closed matter") {
				t.Fatal("closed buyer commitment became an active prompt")
			}
		}
		in.Commitments = []ClaimIn{{Kind: kindCommitmentTheirs, Status: "done", Body: "Closed matter", SourceID: activityID}, {Kind: kindPriority, Status: statusOpen, Body: "Current priority", SourceID: activityID}}
		points = briefCopyFor(language).talkingPointsSection(rankClaims(in))
		if len(points) != 1 || !strings.Contains(points[0].Text, "Current priority") {
			t.Fatal("closed claim displaced current priority")
		}
	}
}
