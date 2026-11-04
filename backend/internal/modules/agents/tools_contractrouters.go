// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "github.com/margince/margince/backend/internal/shared/ports/mcp"

// contractRouters are the listed routers whose every action is a REST route the
// contract declares. Each fronts one topic, so a client lists one tool for it.
var contractRouters = []mcp.ToolSpec{
	{
		Name: "change_attention", Title: "Act on proposals, notices and the weekly plan",
		Description: changeAttentionCopy.render(), Instead: changeAttentionCopy.Instead,
	},
	{
		Name: "change_access", Title: "Share records and set up members and teams",
		Description: changeAccessCopy.render(), Instead: changeAccessCopy.Instead,
	},
	{
		Name: "change_offers", Title: "Work offers, contracts and deal rooms",
		Description: changeOffersCopy.render(), Instead: changeOffersCopy.Instead,
	},
	{
		Name: "change_relationships", Title: "Work a company's relationships and briefs",
		Description: changeRelationshipsCopy.render(), Instead: changeRelationshipsCopy.Instead,
	},
	{
		Name: "run_research", Title: "Run AI work for your user",
		Description: runResearchCopy.render(), Instead: runResearchCopy.Instead,
	},
	{
		Name: "read_details", Title: "Read a record's panels and the day's lists",
		Description: readDetailsCopy.render(), Instead: readDetailsCopy.Instead,
	},
	{
		Name: "change_settings", Title: "Change how the workspace is set up",
		Description: changeSettingsCopy.render(), Instead: changeSettingsCopy.Instead,
	},
}

// registerContractRouters lists each contract router the contract gave actions.
func registerContractRouters(r *Registry) {
	for _, spec := range contractRouters {
		r.mu.RLock()
		served := len(r.routeActions[spec.Name]) > 0
		r.mu.RUnlock()
		if !served {
			continue
		}
		spec.Version = toolVersionV1
		spec.InputSchema = schema(`{"type":"object","properties":{"action":{"description":"Which act to run; ` +
			`read_workspace topic actions lists each action's body"}}}`)
		r.Register(contractRouter{spec: spec})
	}
}
