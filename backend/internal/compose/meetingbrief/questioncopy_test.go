// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestQuestionLanguageKeepsGapsAndEvidence(t *testing.T) {
	asked, notes := questionCopyByVoice(t)
	langcopytest.Census(t, asked,
		langcopytest.OwnVoice("a question is what the rep asks the buyer aloud, so I and our side are the rep's"))
	langcopytest.Census(t, notes)
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

// A questionWords field in neither list would escape both censuses, so it fails here.
func questionCopyByVoice(t *testing.T) (asked, notes map[string]langcopy.Phrase) {
	t.Helper()
	askedFields := []string{"Intent", "Funding", "Approvers", "PriorContact", "Outstanding", "Background", "Joining", "Changed"}
	noteFields := []string{
		"Answer", "Progress", "Prepare", "Objection", "Evidence", "PromiseOpen", "RaisedOpen", "Named",
		"ListenPriority", "WhyIntent", "WhyDecision", "WhyDeal", "ListenProblem", "ListenRoles", "ListenContact",
	}
	asked, notes = map[string]langcopy.Phrase{}, map[string]langcopy.Phrase{}
	voiceOf := map[string]map[string]langcopy.Phrase{}
	for _, name := range askedFields {
		voiceOf[name] = asked
	}
	for _, name := range noteFields {
		if _, both := voiceOf[name]; both {
			t.Fatalf("questionWords.%s is listed both as a question asked aloud and as a note to the rep", name)
		}
		voiceOf[name] = notes
	}
	table := reflect.ValueOf(questionWords)
	for i := range table.NumField() {
		name := table.Type().Field(i).Name
		voice, listed := voiceOf[name]
		if !listed {
			t.Fatalf("questionWords.%s is neither a question asked aloud nor a note to the rep, so no census reads it", name)
		}
		phrase, ok := reflect.TypeAssert[langcopy.Phrase](table.Field(i))
		if !ok {
			t.Fatalf("questionWords.%s is not a langcopy.Phrase", name)
		}
		voice[name] = phrase
		delete(voiceOf, name)
	}
	for name := range voiceOf {
		t.Fatalf("%s is listed as question copy but questionWords has no such field", name)
	}
	return asked, notes
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
