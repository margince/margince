// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// citedFor is reply as a model answering req writes it: each record id the
// request showed spelled as its ref. The brief and the plan show one meeting
// in two payloads, so the request's own prompt says which one it read.
func citedFor(req model.Request, in Input, reply string) string {
	if strings.HasPrefix(req.System, planSystem) {
		return planRefs(in, floorFor(in)).Show(reply)
	}
	return shownRefs(in).Show(reply)
}

func parseCitedSections(reply string, in Input) ([]Section, error) {
	return ParseBriefSections(shownRefs(in).Show(reply), in)
}

func parseCitedPlan(reply string, in Input, floor Plan) (Plan, error) {
	return ParsePlan(planRefs(in, floor).Show(reply), in, floor)
}

// Both prompts show the meeting and its records by refs. A reply citing a ref
// reads back to the record's id; the id itself, and a ref the request never
// minted, ground nothing.
func TestTheMeetingPromptsShowRefsAndReadThemBackToRecords(t *testing.T) {
	in := fullInput()
	floor := floorFor(in)
	for name, req := range map[string]model.Request{"brief": BriefRequest(in, "en"), "plan": PlanRequest(in, floor, "en")} {
		if claims.SpellsRecordID(req.Messages[0].Content) {
			t.Errorf("the %s request shows a record id:\n%s", name, req.Messages[0].Content)
		}
	}
	meeting := in.ActivityID
	ref, shown := shownRefs(in).ID("activity", "a1")
	if !shown || ref != meeting {
		t.Fatalf("the brief shows the meeting as %q, want a1", ref)
	}
	for cite, want := range map[string]int{"a1": 1, meeting: 0, "a99": 0} {
		sections, err := ParseBriefSections(`{"sections":[{"kind":"goal","sentences":[{"text":"Get them to name a date.","nature":"recommendation","evidence":[{"entity_type":"activity","entity_id":"`+cite+`"}]}]}]}`, in)
		if err != nil {
			t.Fatal(err)
		}
		if len(sections) != want || (want == 1 && sections[0].Sentences[0].Evidence[0].EntityID != meeting) {
			t.Errorf("the brief citing %q kept %+v, want %d section(s) citing the meeting's id", cite, sections, want)
		}
	}
	planMeeting := planRefs(in, floor).Show(meeting)
	for cite, want := range map[string]bool{planMeeting: true, meeting: false, "a99": false} {
		plan, err := ParsePlan(`{"opening":{"text":"Open on the security pack.","nature":"recommendation","evidence":[{"entity_type":"activity","entity_id":"`+cite+`"}]}}`, in, floor)
		if (err == nil) != want || (want && plan.Opening.Evidence[0].EntityID != meeting) {
			t.Errorf("the plan citing %q gave %+v, %v", cite, plan.Opening, err)
		}
	}
}
