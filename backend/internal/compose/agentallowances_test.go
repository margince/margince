// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
)

func TestTheAllowanceAnswerNamesAWaitingChangeAndItsStart(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	plan := agentvolume.WindowPlan{Length: 24 * time.Hour}.Reschedule(now, 12*time.Hour)

	out := toAgentAllowances(agentvolume.Policy{Window: plan}, now)

	if out.AgentWindowHours != 24 || out.AgentWindowNextHours == nil || *out.AgentWindowNextHours != 12 {
		t.Fatalf("answered %d h now and %v next, want 24 now and 12 next", out.AgentWindowHours, out.AgentWindowNextHours)
	}
	midnight := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	if !out.AgentWindowResetsAt.Equal(midnight) || !out.AgentWindowNextStartsAt.Equal(midnight) {
		t.Errorf("resets at %v, change starts at %v; want both %v", out.AgentWindowResetsAt, out.AgentWindowNextStartsAt, midnight)
	}
}

func TestAChangeThatTookOverReadsAsTheLengthInForce(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	plan := agentvolume.WindowPlan{Length: 24 * time.Hour}.Reschedule(now, 12*time.Hour)
	later := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)

	out := toAgentAllowances(agentvolume.Policy{Window: plan}, later)

	if out.AgentWindowHours != 12 || out.AgentWindowNextHours != nil || out.AgentWindowNextStartsAt != nil {
		t.Fatalf("answered %+v, want 12 h in force and nothing waiting", out)
	}
	if want := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC); !out.AgentWindowResetsAt.Equal(want) {
		t.Errorf("resets at %v, want %v", out.AgentWindowResetsAt, want)
	}
}
