// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"strings"
	"testing"
)

const coachedReply = `{"scenarios":[{"label":"They push on price","play":"Anchor on the agreed outcome, not the list.",
	"coach":"Watch whether the rep trades scope for price; ask them what budget the buyer named.",
	"evidence":[{"entity_type":"activity","entity_id":"` + activityID + `"}]}]}`

// The lead's paths are the coach's reading of each branch the model wrote, never
// the rep's own play again. A stock branch carries no reading and gives no path.
func TestTheCoachingPathsNeverRepeatTheRepsScenarios(t *testing.T) {
	in := fullInput()
	plan, _ := writtenPlan(t, coachedReply, in)
	paths := briefCopyFor(in.Language).coachingFor(plan).Paths

	if len(plan.Scenarios) < 2 {
		t.Fatalf("the plan has %d scenarios, want the model's and the floor's top-up", len(plan.Scenarios))
	}
	if len(paths) != 1 || paths[0].Label != "They push on price" || !strings.Contains(paths[0].Play, "budget the buyer named") {
		t.Fatalf("paths = %+v, want the one coached branch with the coach's words", paths)
	}
	for _, path := range paths {
		for _, scenario := range plan.Scenarios {
			if path.Label == scenario.Label && path.Play == scenario.Play {
				t.Errorf("the coaching path %+v repeats the rep's scenario row", path)
			}
		}
	}
}

// With no model the plan has only stock branches, so the lead sees no paths
// rather than the rep's list a second time.
func TestTheFloorPlanGivesNoCoachingPaths(t *testing.T) {
	in := fullInput()
	floor := floorFor(in)
	if len(floor.Scenarios) == 0 {
		t.Fatal("the floor has no scenarios; the case needs some")
	}
	if paths := briefCopyFor(in.Language).coachingFor(floor).Paths; len(paths) != 0 {
		t.Errorf("the floor plan gave the lead %+v, want no paths", paths)
	}
}

// A coach line that only restates the play says nothing a lead needs, and is dropped.
func TestACoachLineThatRepeatsThePlayIsDropped(t *testing.T) {
	in := fullInput()
	reply := strings.Replace(coachedReply,
		"Watch whether the rep trades scope for price; ask them what budget the buyer named.",
		"Anchor on the agreed outcome, not the list.", 1)
	plan, _ := writtenPlan(t, reply, in)
	if paths := briefCopyFor(in.Language).coachingFor(plan).Paths; len(paths) != 0 {
		t.Errorf("paths = %+v, want none for a coach line equal to its play", paths)
	}
}
