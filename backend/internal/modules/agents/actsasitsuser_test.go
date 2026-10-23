// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// The listing tells a caller the tier the gate admits it on. An agent on a
// colleague's passport is offered a confirm-first tool as one that runs. A tool
// that waits even for its user stays waiting, and automatic work sees the
// declared tier.
func TestTheListingOffersAPassportAgentTheTierItIsAdmittedOn(t *testing.T) {
	r := NewRegistry(&recordingApprovals{}, auth.NewGate(fullSeatAuthority{}))
	for _, spec := range []mcp.ToolSpec{
		{
			Name: "merge_tags", Title: "Merge tags", Version: "1", Description: "merges",
			RequiredScope: principal.ScopeWrite, Tier: mcp.TierConfirmationRequired, InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		{
			Name: "enrich", Title: "Enrich", Version: "1", Description: "reads the web",
			RequiredScope: principal.ScopeWrite, Tier: mcp.TierConfirmationRequired, WaitsEvenForItsUser: true,
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	} {
		r.Register(&servingTool{spec: spec})
	}
	for _, tc := range []struct {
		who  string
		ctx  context.Context
		want map[string]mcp.RiskTier
	}{
		{"a colleague's agent", passportCtx(), map[string]mcp.RiskTier{
			"merge_tags": mcp.TierAutoExecute, "enrich": mcp.TierConfirmationRequired,
		}},
		{"automatic work", agentCtx(), map[string]mcp.RiskTier{
			"merge_tags": mcp.TierConfirmationRequired, "enrich": mcp.TierConfirmationRequired,
		}},
	} {
		offered := map[string]mcp.RiskTier{}
		for _, spec := range r.Offered(tc.ctx) {
			offered[spec.Name] = spec.Tier
		}
		for name, want := range tc.want {
			if got, listed := offered[name]; !listed || got != want {
				t.Errorf("%s is offered %s at tier %v (listed %v), want %v", tc.who, name, got, listed, want)
			}
		}
	}
}
