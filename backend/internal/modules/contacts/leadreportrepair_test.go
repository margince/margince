// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"
	"time"
)

func TestStepDownFollowsTheRemainingTouchesAndOnlyForASystemPlacedStatus(t *testing.T) {
	t.Parallel()
	system, human := "system", "human"
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	outbound := leadResponseTouch{direction: "outbound", capturedBy: "human:rep", occurredAt: at}
	reply := leadResponseTouch{direction: "inbound", occurredAt: at}

	for _, c := range []struct {
		name    string
		current LeadStatus
		setBy   *string
		touches []leadResponseTouch
		want    LeadStatus
		down    bool
		floor   LeadStatus
	}{
		{"no touch left", LeadStatusEngaged, &system, nil, LeadStatusNew, true, ""},
		{"only our outbound left", LeadStatusEngaged, &system, []leadResponseTouch{outbound}, LeadStatusContacted, true, ""},
		{"a real reply remains", LeadStatusEngaged, &system, []leadResponseTouch{outbound, reply}, LeadStatusEngaged, false, ""},
		{"a human placed it", LeadStatusEngaged, &human, nil, "", false, ""},
		{"no recorded placer", LeadStatusEngaged, nil, nil, "", false, ""},
		{"already at the earned step", LeadStatusContacted, &system, []leadResponseTouch{outbound}, LeadStatusContacted, false, ""},
		{"terminal", LeadStatusPromoted, &system, nil, "", false, ""},
		{"a human placed contacted before the bounce", LeadStatusEngaged, &system, nil, LeadStatusContacted, true, LeadStatusContacted},
		{"the human step is already the current one", LeadStatusContacted, &system, nil, LeadStatusContacted, false, LeadStatusContacted},
	} {
		got, down := stepDownFor(c.current, c.setBy, c.touches, c.floor)
		if got != c.want || down != c.down {
			t.Errorf("%s: stepDownFor = (%q, %v), want (%q, %v)", c.name, got, down, c.want, c.down)
		}
	}
}

func TestRestampKeepsAStampSomeRemainingTouchExplains(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	first := created.Add(2 * time.Hour)
	later := created.Add(5 * time.Hour)
	human := leadResponseTouch{direction: "outbound", capturedBy: "human:rep", occurredAt: later}
	agentOnly := leadResponseTouch{direction: "outbound", capturedBy: "agent:x", occurredAt: first}

	if moved, _ := restampFirstResponse(&first, created, []leadResponseTouch{agentOnly, human}); !moved {
		t.Error("a stamp only an agent outbound with nothing to answer explains was kept")
	}
	if _, to := restampFirstResponse(&first, created, []leadResponseTouch{agentOnly, human}); to == nil || !to.Equal(later) {
		t.Errorf("restamped to %v, want the human's later touch", to)
	}
	if moved, to := restampFirstResponse(&first, created, []leadResponseTouch{agentOnly}); !moved || to != nil {
		t.Errorf("with no accepted touch left: moved=%v to=%v, want the stamp cleared", moved, to)
	}
	if moved, _ := restampFirstResponse(&first, created, []leadResponseTouch{{direction: "outbound", capturedBy: "human:rep", occurredAt: first}}); moved {
		t.Error("a stamp an accepted touch explains was moved")
	}
	if moved, _ := restampFirstResponse(&first, created, nil); moved {
		t.Error("a stamp no touch explains (a disposition wrote it) was moved")
	}
}
