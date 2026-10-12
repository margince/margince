// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "github.com/margince/margince/backend/internal/shared/ports/mcp"

// changeLeadState fronts the four lead transitions. Each stays its own tool
// because each is its own approval kind and decision right.
type changeLeadState struct{ routed }

func (changeLeadState) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "change_lead_state", Title: "Change a lead's state", Version: toolVersionV1,
		Description: changeLeadStateCopy.render(),
		Instead:     changeLeadStateCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"lead_id":{"description":"The lead"},
			"trigger":{"description":"promote only, required: the genuine engagement justifying it; cold outreach with no reply never promotes"},
			"evidence_note":{"description":"promote only"},
			"reason":{"description":"demote only, required: why the promotion is reversed, recorded in the audit trail"}}}`),
	}
}

func (changeLeadState) routeField() string { return "action" }

func (changeLeadState) routeTargets() map[string]string {
	return map[string]string{
		"qualify":    "qualify_lead",
		"promote":    "promote_lead",
		"disqualify": "disqualify_lead",
		"demote":     "demote_lead",
	}
}
