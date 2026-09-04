// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// Both writers here ask through ai.Ask, so a reply their own parse refuses goes
// back to the model rather than dropping the reader to the floor.
//
// The assertion is on ReAsking.Refused, and only a lane that can re-ask makes
// it: laneReturning never calls the validator, so a site handing the lane a
// looser check than its own parse passes every other test in this package.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai/aitest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestTheBriefWriterReAsksThroughItsOwnParse(t *testing.T) {
	in := fullInput()
	lane := &aitest.ReAsking{
		// Valid JSON, refused by ParseBriefSections alone: sections is an object
		// where the site reads a list.
		First: `{"sections":{"kind":"goal"}}`,
		Second: `{"sections":[{"kind":"goal","sentences":[
			{"text":"Get them to name a date.","nature":"recommendation",
			 "evidence":[{"entity_type":"activity","entity_id":"` + in.ActivityID + `"}]}]}]}`,
	}
	sections, err := writeWithModel(t.Context(), lane, in, string(textlang.English))
	if err != nil {
		t.Fatalf("writing: %v", err)
	}
	if lane.Refused == nil {
		t.Fatal("the site accepted a reply ParseBriefSections refuses, so its validator is looser than its own read")
	}
	if lane.Bare != 0 {
		t.Errorf("the bare lane was taken %d time(s) on a lane that can re-ask", lane.Bare)
	}
	if len(sections) == 0 {
		t.Fatal("the re-asked brief carried no section, so the second reply never landed")
	}
}

func TestThePlanWriterReAsksThroughItsOwnParse(t *testing.T) {
	in := fullInput()
	lane := &aitest.ReAsking{
		// Valid JSON, refused by ParsePlan alone: opening is a string where the
		// site reads an evidenced sentence.
		First: `{"opening":"Open on the security pack."}`,
		Second: `{"opening":{"text":"Open on the security pack.","nature":"recommendation",
			"evidence":[{"entity_type":"activity","entity_id":"` + activityID + `"}]}}`,
	}
	plan, err := writePlanWithModel(t.Context(), lane, in, floorFor(in), string(textlang.English))
	if err != nil {
		t.Fatalf("writing the plan: %v", err)
	}
	if lane.Refused == nil {
		t.Fatal("the site accepted a reply ParsePlan refuses, so its validator is looser than its own read")
	}
	if plan.Opening == nil || !strings.Contains(plan.Opening.Text, "security pack") {
		t.Fatal("the re-asked plan did not carry the second reply's opening")
	}
}
