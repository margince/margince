// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func TestTierWireIsExhaustive(t *testing.T) {
	cases := map[mcp.RiskTier]crmcontracts.AgentToolTier{
		mcp.TierAutoExecute:          crmcontracts.AgentToolTierAutoExecute,
		mcp.TierConfirmationRequired: crmcontracts.AgentToolTierConfirmationRequired,
		mcp.TierDynamic:              crmcontracts.AgentToolTierDynamic,
	}
	for in, want := range cases {
		if got := tierWire(in); got != want {
			t.Fatalf("tierWire(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestAgentToolsMapPreservesRegistryOrderAndFields(t *testing.T) {
	specs := []mcp.ToolSpec{
		{
			Name: "b_tool", Title: "Send a mail", Description: "Put a mail on the wire.",
			OpenAPIOp: "send_email", RequiredScope: "send", Tier: mcp.TierConfirmationRequired, Egress: true,
		},
		{
			Name: "a_tool", Title: "Find records", Description: "Find records by name.",
			OpenAPIOp: "search_records", RequiredScope: "read", Tier: mcp.TierAutoExecute,
		},
	}
	got := agentToolsFromSpecs(specs)
	if len(got) != 2 || got[0].Name != "b_tool" || !got[0].Egress {
		t.Fatalf("mapping dropped fields or reordered: %+v", got)
	}
	// The two written fields, named one by one: a mapping that dropped either
	// would leave the console showing an inventory of verbs, which is what it
	// showed before there was anything written to show.
	if got[0].Title != "Send a mail" {
		t.Errorf("title = %q, want the spec's written display name", got[0].Title)
	}
	if !strings.HasPrefix(got[0].Description, "Put a mail on the wire.") {
		t.Errorf("description = %q, want it to open with the spec's written text", got[0].Description)
	}
	if !strings.Contains(got[0].Description, "a human approves") {
		t.Errorf("description = %q, want the governance clause an MCP client also gets", got[0].Description)
	}
}

// TestAgentToolsFromSpecsOmitsHumanOnlyTools: a HumanOnly spec requests no
// agent authority, so there is nothing here for an operator to review —
// AgentTool.tier is a required wire field and tierWire maps an unset Tier to
// confirmation_required, so listing it with fabricated governance was never
// an option; omitting it is the honest answer.
func TestAgentToolsFromSpecsOmitsHumanOnlyTools(t *testing.T) {
	specs := []mcp.ToolSpec{
		{Name: "normal_tool", Title: "t", Description: "d", Version: "1.0.0", Tier: mcp.TierAutoExecute, RequiredScope: "read"},
		{Name: "human_only_op", Title: "t2", Description: "d2", Version: "1.0.0", HumanOnly: true},
	}
	out := agentToolsFromSpecs(specs)
	if len(out) != 1 || out[0].Name != "normal_tool" {
		t.Fatalf("expected only normal_tool, got %+v", out)
	}
}
