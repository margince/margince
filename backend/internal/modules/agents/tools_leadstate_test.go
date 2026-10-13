// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
)

func leadStateRegistry() *Registry {
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	r.Register(qualifyLead{})
	r.Register(promoteLead{})
	r.Register(disqualifyLead{})
	r.Register(demoteLead{})
	r.Register(changeLeadState{})
	return r
}

// Each action is refused by its own tool, which proves the call reached it with
// the router's arguments intact.
func TestChangeLeadStateRoutesEveryAction(t *testing.T) {
	r := leadStateRegistry()
	for action, target := range (changeLeadState{}).routeTargets() {
		served, err := r.InvokeServed(agentCtx(), "change_lead_state",
			json.RawMessage(`{"action":"`+action+`","lead_id":"not-a-uuid"}`))
		if _, bad := errors.AsType[*BadArgsError](err); !bad || served.Tool != target {
			t.Errorf("%s: served %q with %v, want %s's own refusal of the lead id", action, served.Tool, err, target)
		}
	}
}

func TestChangeLeadStatePassesArgsThrough(t *testing.T) {
	_, err := leadStateRegistry().InvokeServed(agentCtx(), "change_lead_state",
		json.RawMessage(`{"action":"promote","lead_id":"0192f5c4-6c1e-7a4a-9d36-1f1f0c3a2b11"}`))
	if err == nil || !strings.Contains(err.Error(), "trigger") {
		t.Fatalf("promote without a trigger answered %v, want promote_lead's refusal naming trigger", err)
	}
}
