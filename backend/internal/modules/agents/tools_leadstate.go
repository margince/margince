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
			"action":{"type":"string","enum":["demote","disqualify","promote","qualify"]},
			"lead_id":{"type":"string","format":"uuid","description":"The lead"},
			"trigger":{"type":"string","enum":["inbound_reply","meeting_booked","meeting_held","human_qualify"],
				"description":"promote only, required: the genuine engagement justifying it"},
			"evidence_note":{"type":"string","description":"promote only"},
			"reason":{"type":"string","minLength":1,"description":"demote only, required: why the promotion is reversed"}}}`),
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
