// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestSectionCopyPreservesClaimsAndSources(t *testing.T) {
	langcopytest.Census(t, sectionWords)
	for _, language := range []string{"en", "de", "vi"} {
		said := briefCopyFor(language)
		for _, kind := range []string{kindOpenQuestion, kindDecision, kindCommitmentOurs, kindObjection, kindDecisionProcess, kindSuccessCriterion, kindCommitmentTheirs, kindPriority} {
			claim := ClaimIn{Kind: kind, ContactName: "Original Name", Body: "Original quotation", Status: statusOpen, DueAt: new(at(8))}
			lines := []string{said.goalLine(claim, at(10)), said.talkingPointLine(claim), said.dealStateLine(claim)}
			if risk, ok := said.riskLine(claim, at(10)); ok {
				lines = append(lines, risk)
			}
			for _, line := range lines {
				if !strings.Contains(line, claim.ContactName) || !strings.Contains(line, claim.Body) || strings.Contains(line, "%!") {
					t.Fatalf("%s %s lost source words: %s", language, kind, line)
				}
			}
		}
		in := fullInput()
		in.Commitments = append(in.Commitments, ClaimIn{Kind: kindObjection, ContactName: "Other Name", Body: "Original objection", Status: statusOpen, SourceID: activityID})
		english := Deterministic(in)
		in.Language = language
		localized := Deterministic(in)
		for _, kind := range []crmcontracts.MeetingBriefSectionKind{crmcontracts.MeetingBriefSectionKindGoal, crmcontracts.MeetingBriefSectionKindRisks, crmcontracts.MeetingBriefSectionKindDealState} {
			before, after := sectionOf(t, english, kind), sectionOf(t, localized, kind)
			if len(before.Sentences) != len(after.Sentences) || len(after.Sentences) == 0 {
				t.Fatalf("%s changed coverage of %s", language, kind)
			}
			for i, sentence := range after.Sentences {
				if !reflect.DeepEqual(sentence.Evidence, before.Sentences[i].Evidence) || sentence.Nature != before.Sentences[i].Nature {
					t.Fatalf("%s changed evidence or nature", language)
				}
				if language != "en" && sentence.Text == before.Sentences[i].Text {
					t.Fatalf("%s left %s untranslated", language, kind)
				}
			}
		}
		risk := sectionOf(t, localized, crmcontracts.MeetingBriefSectionKindRisks).Sentences[0]
		plan := DeterministicPlanFor(in)
		if plan.TopRisk == nil || plan.TopRisk.Text.Text != risk.Text || !reflect.DeepEqual(plan.TopRisk.Text.Evidence, risk.Evidence) {
			t.Fatalf("%s plan disagrees with the brief's risk", language)
		}
	}
}
