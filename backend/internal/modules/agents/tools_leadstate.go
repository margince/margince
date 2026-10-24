// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "github.com/margince/margince/backend/internal/shared/ports/mcp"

// changeState fronts the lead transitions, a deal move and a project phase move.
// Each stays its own tool because each is its own approval kind and decision
// right, and a deal move decides its tier per call.
type changeState struct{ routed }

func (changeState) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "change_state", Title: "Move a lead, deal, project or partner", Version: toolVersionV1,
		Description: changeStateCopy.render(),
		Instead:     changeStateCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"lead_id":{"description":"The lead"},
			"trigger":{"description":"promote only, required: the genuine engagement justifying it; cold outreach with no reply never promotes"},
			"evidence_note":{"description":"promote only"},
			"company_id":{"description":"set_up_partner: the company"},
			"partner_role":{"description":"set_up_partner, required: hosting, consulting or strategic"},
			"reason":{"description":"demote, required: why the promotion is reversed. move_project: required when to_phase is closed. Recorded in the audit trail"}}}`),
	}
}

func (changeState) routeField() string { return actionArg }

func (changeState) routeTargets() map[string]string {
	return map[string]string{
		"qualify":        "qualify_lead",
		"promote":        "promote_lead",
		"disqualify":     "disqualify_lead",
		"demote":         "demote_lead",
		"move_deal":      "progress_deal",
		"move_project":   "advance_project_phase",
		"set_up_partner": "set_up_partner",
	}
}
