// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"context"
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestObjectiveLanguageKeepsRecommendationsAndEvidence(t *testing.T) {
	langcopytest.Census(t, objectiveWords)
	kinds := []crmcontracts.MeetingPlanTypeValue{crmcontracts.MeetingPlanTypeValueMeetingPlanTypeUnknown, crmcontracts.MeetingPlanTypeValueMeetingPlanTypeFollowupDiscovery}
	for _, family := range subjectFamilies {
		kinds = append(kinds, family.value)
	}
	for _, language := range []string{"en", "de", "vi"} {
		for _, kind := range kinds {
			in := fullInput()
			in.Commitments = nil
			typ := MeetingType{Value: kind}
			english := objectiveFor(in, typ, rankClaims(in))
			in.Language = language
			got := objectiveFor(in, typ, rankClaims(in))
			if got == nil || got.Sentence.Text == "" || got.Caveat == "" || !reflect.DeepEqual(got.Sentence.Evidence, english.Sentence.Evidence) || got.Sentence.Nature != english.Sentence.Nature {
				t.Fatalf("%s %s lost objective facts", language, kind)
			}
			if language != "en" && (got.Sentence.Text == english.Sentence.Text || got.Caveat == english.Caveat) {
				t.Fatalf("%s %s retained English objective or caveat", language, kind)
			}
		}
		in := fullInput()
		in.Language = language
		in.PriorMeetings = []PriorMeetingIn{{ID: activityID, Subject: "Original meeting", StartsAt: instantOn(3)}}
		in.Commitments = append(in.Commitments, ClaimIn{Kind: kindObjection, Body: "Original objection", Status: statusOpen, SourceID: activityID})
		floor := DeterministicPlanFor(in)
		if !strings.Contains(floor.Objective.Sentence.Text, "send the security pack") || !strings.Contains(floor.Objective.Sentence.Text, "2026-08-08") || !strings.Contains(floor.Opening.Text, "Original meeting") || !strings.Contains(floor.Opening.Text, "2026-08-03") || !strings.Contains(floor.TopRisk.Response.Say, "Original objection") {
			t.Fatalf("%s lost recorded facts", language)
		}
		noModel, by := WritePlan(context.Background(), nil, in, floor, language)
		if by != crmcontracts.WrittenByDeterministic || !reflect.DeepEqual(noModel, floor) {
			t.Fatal("missing model changed the floor")
		}
		parsed, err := ParsePlan(`{"objective":{"text":"A grounded objective.","nature":"recommendation","evidence":[{"entity_type":"activity","entity_id":"`+activityID+`"}]}}`, in, floor)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Objective == nil || parsed.Objective.Caveat != floor.Objective.Caveat {
			t.Fatal("parsed objective lost the shared-language caveat")
		}
	}
}
