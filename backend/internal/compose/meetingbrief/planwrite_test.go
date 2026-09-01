// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"context"
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func floorFor(in Input) Plan {
	return DeterministicPlan(in, rankClaims(in))
}

func writtenPlan(t *testing.T, reply string, in Input) (Plan, crmcontracts.WrittenBy) {
	t.Helper()
	lane := &laneReturning{reply: reply}
	return WritePlan(context.Background(), lane, in, floorFor(in), "en")
}

// No lane is a deployment saying it runs no model, not an error.
func TestNoLaneKeepsTheFloorPlan(t *testing.T) {
	in := fullInput()
	got, by := WritePlan(context.Background(), nil, in, floorFor(in), "en")
	if by != crmcontracts.Deterministic {
		t.Errorf("writer = %q, want deterministic", by)
	}
	if got.Objective == nil {
		t.Error("the floor plan lost its objective")
	}
}

// A model that is over budget, unreachable or answering nonsense must not take
// the preparation down with it.
func TestAFailedLaneFallsToTheFloorPlan(t *testing.T) {
	in := fullInput()
	lane := &laneReturning{err: errors.New("over budget")}
	got, by := WritePlan(context.Background(), lane, in, floorFor(in), "en")
	if by != crmcontracts.Deterministic {
		t.Errorf("writer = %q, want deterministic after a lane error", by)
	}
	if got.Objective == nil {
		t.Error("a failed lane left the reader with no objective")
	}
}

// THE coverage rule. A model that answers one field well must not produce a
// shorter plan than a deployment with no model at all.
func TestASparseReplyKeepsTheFloorsCoveragePerField(t *testing.T) {
	in := fullInput()
	floor := floorFor(in)
	got, by := writtenPlan(t, `{"opening":{"text":"Open on the security pack.",
		"nature":"recommendation","evidence":[{"entity_type":"activity","entity_id":"`+activityID+`"}]}}`, in)
	if by != crmcontracts.Model {
		t.Errorf("writer = %q, want model — a written opening is a written plan", by)
	}
	if got.Opening == nil || !strings.Contains(got.Opening.Text, "security pack") {
		t.Error("the model's opening was not kept")
	}
	// Everything it did not answer is still there.
	if got.Objective == nil {
		t.Error("the objective was lost; the floor's should have been restored")
	}
	if len(got.Arc) != len(floor.Arc) {
		t.Errorf("arc = %d moments, want the floor's %d", len(got.Arc), len(floor.Arc))
	}
	if len(got.Unknowns) != len(floor.Unknowns) {
		t.Errorf("unknowns = %d, want the floor's %d", len(got.Unknowns), len(floor.Unknowns))
	}
	if got.Advance.Minimum.Text != floor.Advance.Minimum.Text {
		t.Error("the advance was rewritten; it is not a field the model is asked for")
	}
}

// A gap is a fact about the RECORD. A model must not be able to invent one.
func TestTheModelCannotWriteAnUnknown(t *testing.T) {
	in := fullInput()
	floor := floorFor(in)
	got, _ := writtenPlan(t, `{"opening":{"text":"Open plainly.","nature":"recommendation",
		"evidence":[{"entity_type":"activity","entity_id":"`+activityID+`"}]},
		"unknowns":[{"kind":"no_open_deal","question":"Invented."}]}`, in)
	if len(got.Unknowns) != len(floor.Unknowns) {
		t.Errorf("unknowns = %d, want the floor's %d — the reply's are not read at all",
			len(got.Unknowns), len(floor.Unknowns))
	}
	for _, unknown := range got.Unknowns {
		if unknown.Question == "Invented." {
			t.Error("a model-written gap reached the reader")
		}
	}
}

// The same allowlist the sections run. A citation the briefing never carried
// points somewhere the reader cannot go.
func TestAnUnsupportedLikelyAskIsDropped(t *testing.T) {
	in := fullInput()
	elsewhere := "0198f000-0000-7000-8000-0000000000ff"
	got, _ := writtenPlan(t, `{"likely_asks":[
		{"question":"Grounded?","basis":"They asked.","relevance":"high","prepare":"Answer it.",
		 "evidence":[{"entity_type":"activity","entity_id":"`+activityID+`"}]},
		{"question":"Invented?","basis":"From nowhere.","relevance":"high","prepare":"x",
		 "evidence":[{"entity_type":"activity","entity_id":"`+elsewhere+`"}]}]}`, in)
	for _, ask := range got.LikelyAsks {
		if ask.Question == "Invented?" {
			t.Error("an ask citing a record outside the briefing survived")
		}
	}
	if len(got.LikelyAsks) == 0 {
		t.Error("the grounded ask was dropped with the ungrounded one")
	}
}

// A judgement filed where the plan states facts is dropped rather than
// relabelled: relabelling keeps a claim the writer meant differently.
func TestAJudgementIsRefusedWhereThePlanRecommends(t *testing.T) {
	in := fullInput()
	got, _ := writtenPlan(t, `{"objective":{"text":"They seem unhappy.","nature":"assessment",
		"evidence":[{"entity_type":"activity","entity_id":"`+activityID+`"}]},
		"opening":{"text":"Open plainly.","nature":"recommendation",
		"evidence":[{"entity_type":"activity","entity_id":"`+activityID+`"}]}}`, in)
	if got.Objective != nil && got.Objective.Sentence.Text == "They seem unhappy." {
		t.Error("an assessment was accepted where the objective must recommend")
	}
}

// A risk with two thirds of a response is a warning with no plan, which is
// what the reader already had.
func TestARiskWithoutItsWholeResponseIsRefused(t *testing.T) {
	in := fullInput()
	got, _ := writtenPlan(t, `{"top_risk":{"text":"The pack is late.",
		"evidence":[{"entity_type":"activity","entity_id":"`+activityID+`"}],
		"say":"Own it.","show":""}}`, in)
	if got.TopRisk != nil && got.TopRisk.Text.Text == "The pack is late." {
		t.Error("a risk with no `show` and no `avoid` was accepted")
	}
}

// A reply whose every claim was dropped is the same as no answer.
func TestAReplyThatSurvivesNothingFallsToTheFloor(t *testing.T) {
	in := fullInput()
	elsewhere := "0198f000-0000-7000-8000-0000000000ff"
	got, by := writtenPlan(t, `{"objective":{"text":"Invented.","nature":"recommendation",
		"evidence":[{"entity_type":"activity","entity_id":"`+elsewhere+`"}]}}`, in)
	if by != crmcontracts.Deterministic {
		t.Errorf("writer = %q, want deterministic — nothing the model said survived", by)
	}
	if got.Objective == nil {
		t.Error("the floor's objective was not restored")
	}
}

// The prompt carries what was SAID, not just what it was called. Without the
// excerpts a model has dates and subjects, and writes a plan that fits any
// account with dates and subjects.
func TestThePlanPromptCarriesWhatTheConversationsSaid(t *testing.T) {
	in := fullInput()
	in.History = []HistoryIn{mail(activityID, 3, "CRM requirements", "inbound")}
	in.Excerpts = []ExcerptIn{{
		ActivityID: activityID, Subject: "CRM requirements", Direction: "inbound",
		At: at(3), Text: "We need issue tracking and quote tracking.",
	}}
	prompt := planPromptOf(in, floorFor(in))
	encoded := encodePlanPrompt(prompt)
	if !strings.Contains(encoded, "issue tracking") {
		t.Error("the prompt does not carry what the conversation said; the plan can only be generic")
	}
	if !strings.Contains(encoded, activityID) {
		t.Error("the prompt does not name the id a citation must use")
	}
}

// Two hundred history rows must not ride in the prompt: the arc condensed them
// for a reason, and the budget is spent on what was said in the few that matter.
func TestThePlanPromptDoesNotCarryTheWholeHistory(t *testing.T) {
	in := fullInput()
	for i := range 50 {
		in.History = append(in.History, mail(
			padID(i), 3+i, "Chatter", "outbound"))
	}
	encoded := encodePlanPrompt(planPromptOf(in, floorFor(in)))
	// No excerpts were read, so no message body should appear.
	if strings.Contains(encoded, `"messages":[{`) {
		t.Error("the prompt carries messages nobody read an excerpt for")
	}
}

func padID(i int) string {
	const base = "0198f000-0000-7000-8000-0000000"
	return base + string(rune('0'+i/10)) + string(rune('0'+i%10)) + "000"
}
