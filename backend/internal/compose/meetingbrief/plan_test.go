// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/claims"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func planOf(in Input) crmcontracts.MeetingPlan {
	return wirePlan(DeterministicPlan(in, rankClaims(in)), in)
}

// The plan aims the meeting at the same promise the sections name. Two
// rankings of one claim set would drift, and the drift shows as a brief whose
// goal says one thing and whose objective aims at another.
func TestThePlanAndTheSectionsAgreeOnWhatMatters(t *testing.T) {
	in := fullInput()
	plan := planOf(in)
	if plan.Objective == nil {
		t.Fatal("no objective")
	}
	if !strings.Contains(plan.Objective.Sentence.Text, "security pack") {
		t.Errorf("objective = %q, want the open promise the goal section also names",
			plan.Objective.Sentence.Text)
	}
	sections := Deterministic(in)
	goal := sectionOf(t, sections, crmcontracts.MeetingBriefSectionKindGoal)
	if !strings.Contains(goal.Sentences[0].Text, "security pack") {
		t.Errorf("goal = %q, want the same promise", goal.Sentences[0].Text)
	}
}

// An unknown is a fact about the RECORD. It must never be produced because a
// writer left a field out, and it must disappear when the record answers.
func TestUnknownsComeOnlyFromAbsence(t *testing.T) {
	full := planOf(fullInput())
	for _, unknown := range full.Unknowns {
		if unknown.Kind == crmcontracts.MeetingPlanUnknownNoOpenDeal {
			t.Error("a meeting WITH a deal reported no_open_deal")
		}
		if unknown.Kind == crmcontracts.MeetingPlanUnknownNoCommitmentsCaptured {
			t.Error("a meeting WITH commitments reported none captured")
		}
	}

	bare := Input{ActivityID: meetingID, Subject: "Sync", Now: at(10), StartsAt: at(12)}
	got := map[crmcontracts.MeetingPlanUnknownKind]bool{}
	for _, unknown := range planOf(bare).Unknowns {
		got[unknown.Kind] = true
	}
	for _, want := range []crmcontracts.MeetingPlanUnknownKind{
		crmcontracts.MeetingPlanUnknownIntentNotCaptured,
		crmcontracts.MeetingPlanUnknownNoOpenDeal,
		crmcontracts.MeetingPlanUnknownNoPriorMeeting,
		crmcontracts.MeetingPlanUnknownNoCommitmentsCaptured,
		crmcontracts.MeetingPlanUnknownNoHistory,
	} {
		if !got[want] {
			t.Errorf("a record answering nothing did not report %q", want)
		}
	}
}

// Every unknown carries the question that closes it, or it is a complaint
// rather than preparation.
func TestEveryUnknownCarriesItsQuestion(t *testing.T) {
	bare := Input{ActivityID: meetingID, Now: at(10), StartsAt: at(12)}
	for _, unknown := range planOf(bare).Unknowns {
		if strings.TrimSpace(unknown.Question) == "" {
			t.Errorf("the %q gap carries no question to close it", unknown.Kind)
		}
	}
}

// A question that would read the same on any other prospect is a
// questionnaire. Every one printed must name something only this account
// produced.
func TestAQuestionNamesSomethingOnlyThisAccountProduced(t *testing.T) {
	in := fullInput()
	plan := planOf(in)
	if len(plan.Questions) == 0 {
		t.Fatal("no questions")
	}
	for _, question := range plan.Questions {
		specific := strings.Contains(question.Ask, "security pack") ||
			strings.Contains(question.Why, in.Company) ||
			strings.Contains(question.Why, "Ana Roth")
		if !specific {
			t.Errorf("question %q / %q names nothing from this account",
				question.Ask, question.Why)
		}
	}
}

// With no company and no attendee there is nothing to anchor a templated
// question on, and printing one anyway is the failure above.
func TestNoAnchorMeansNoTemplatedQuestion(t *testing.T) {
	bare := Input{ActivityID: meetingID, Now: at(10), StartsAt: at(12)}
	if got := planOf(bare).Questions; len(got) != 0 {
		t.Errorf("questions = %d, want 0 — nothing in the record to make one specific", len(got))
	}
}

// Readiness decides whether a client leads with the plan. It must be read off
// what SURVIVED grounding, not off what was built.
func TestReadinessReportsWhatSurvived(t *testing.T) {
	if got := planOf(fullInput()).Readiness; got != crmcontracts.MeetingPlanReadinessOutline {
		// fullInput has one claim: a risk, two asks and three questions cannot
		// all come from it, so this is an outline and says so.
		t.Errorf("readiness = %q, want outline for a one-claim meeting", got)
	}
	bare := Input{ActivityID: meetingID, Now: at(10), StartsAt: at(12)}
	if got := planOf(bare).Readiness; got != crmcontracts.MeetingPlanReadinessOutline {
		t.Errorf("readiness = %q, want outline for an empty record", got)
	}
}

// The contract requires all three advances, so a leg that fails grounding
// falls back rather than leaving a reader two ways to close and no third.
func TestEveryAdvanceLegSurvives(t *testing.T) {
	in := fullInput()
	plan := planOf(in)
	for name, leg := range map[string]crmcontracts.OrganizationBriefSentence{
		"minimum": plan.Advance.Minimum, "best": plan.Advance.Best, "fallback": plan.Advance.Fallback,
	} {
		if strings.TrimSpace(leg.Text) == "" {
			t.Errorf("the %s advance is empty", name)
		}
	}
}

// A citation the reader cannot open is dropped whole, and an id must never
// reach the prose.
func TestAnUngroundedPlanClaimIsDropped(t *testing.T) {
	in := fullInput()
	plan := DeterministicPlan(in, rankClaims(in))
	plan.Opening = &Sentence{
		Text:     "Open on the thing nobody recorded.",
		Nature:   natureRecommendation,
		Evidence: []Evidence{{EntityType: citeActivity, EntityID: "not-an-id"}},
	}
	wired := wirePlan(plan, in)
	if wired.Opening != nil {
		t.Errorf("an opening citing an unresolvable record survived: %q", wired.Opening.Text)
	}
	// The rest of the plan is unharmed: one bad claim drops itself, not the page.
	if wired.Objective == nil {
		t.Error("a dropped opening took the objective with it")
	}
}

func TestThePlanNeverSpellsARecordIDInItsProse(t *testing.T) {
	in := fullInput()
	in.History = []HistoryIn{mail(activityID, 3, "Security review", "inbound")}
	plan := planOf(in)
	var prose []string
	if plan.Objective != nil {
		prose = append(prose, plan.Objective.Sentence.Text, plan.Objective.Caveat)
	}
	if plan.Opening != nil {
		prose = append(prose, plan.Opening.Text)
	}
	for _, moment := range plan.AccountArc {
		prose = append(prose, moment.Title, moment.Summary.Text)
	}
	for _, question := range plan.Questions {
		prose = append(prose, question.Ask, question.Why, question.ListenFor)
	}
	for _, line := range prose {
		if claims.SpellsRecordID(line) {
			t.Errorf("a record id reached the reader in %q", line)
		}
	}
}
