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

func TestAdvanceAndScenariosKeepTheMeetingTheyCite(t *testing.T) {
	langcopytest.Census(t, advanceWords)
	kinds := []crmcontracts.MeetingPlanTypeValue{crmcontracts.MeetingPlanTypeValueMeetingPlanTypeUnknown, crmcontracts.MeetingPlanTypeValueMeetingPlanTypeFollowupDiscovery}
	for _, family := range subjectFamilies {
		kinds = append(kinds, family.value)
	}
	for _, kind := range kinds {
		in := fullInput()
		typ := MeetingType{Value: kind}
		english := advanceFor(in, typ)
		englishScenarios := scenariosFor(in, typ)
		for _, language := range []string{"en", "de", "vi"} {
			in.Language = language
			got := advanceFor(in, typ)
			for i, line := range []Sentence{got.Minimum, got.Best, got.Fallback} {
				before := []Sentence{english.Minimum, english.Best, english.Fallback}[i]
				if !reflect.DeepEqual(line.Evidence, before.Evidence) || line.Nature != natureRecommendation || line.Text == "" || (language != "en" && line.Text == before.Text) {
					t.Fatalf("%s %s changed recommendation or failed translation", language, kind)
				}
			}
			scenarios := scenariosFor(in, typ)
			if len(scenarios) != len(englishScenarios) || len(scenarios) < 2 {
				t.Fatal("language changed scenario branches")
			}
			for i, scenario := range scenarios {
				if !reflect.DeepEqual(scenario.Evidence, englishScenarios[i].Evidence) || (language != "en" && (scenario.Label == englishScenarios[i].Label || scenario.Play == englishScenarios[i].Play)) {
					t.Fatalf("%s scenario changed evidence or retained English", language)
				}
			}
			if kind == crmcontracts.MeetingPlanTypeValueMeetingPlanTypeRelationship && !strings.Contains(got.Best.Text, in.Attendees[0].FullName) {
				t.Fatal("working session lost attendee")
			}
		}
	}
}

func TestCoachingReusesThePlansScenariosInEveryLanguage(t *testing.T) {
	langcopytest.Census(t, coachingWords)
	for _, plan := range []Plan{{}, {Objective: &Objective{}}, {TopRisk: &Risk{}}, {LikelyAsks: make([]Ask, 2)}, {Type: MeetingType{Value: crmcontracts.MeetingPlanTypeValueMeetingPlanTypeUnknown}}, {Arc: []ArcSentence{{}}, Questions: []Question{{}}}} {
		plan.Scenarios = []Scenario{{Label: "Original branch", Play: "Original recommendation"}}
		english := briefCopyFor("en").coachingFor(plan)
		for _, language := range []string{"de", "vi"} {
			got := briefCopyFor(language).coachingFor(plan)
			if !reflect.DeepEqual(got.Paths, english.Paths) || got.Paths[0].Label != plan.Scenarios[0].Label || got.Paths[0].Play != plan.Scenarios[0].Play {
				t.Fatal("coaching invented a separate scenario")
			}
			actual := []string{got.Focus, got.FailureMode, got.ListenFor, got.WatchFor, got.InterveneIf}
			before := []string{english.Focus, english.FailureMode, english.ListenFor, english.WatchFor, english.InterveneIf}
			for i, line := range actual {
				if line == "" || line == before[i] {
					t.Fatalf("%s coaching field %d untranslated", language, i)
				}
			}
		}
	}
}
