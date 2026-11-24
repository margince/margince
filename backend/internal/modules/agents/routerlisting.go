// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"log/slog"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// listedActions are the router actions whose arguments tools/list names. Every
// other action of a router the action catalog describes keeps its enum line,
// and a model reads its arguments from read_workspace topic=actions. The set is
// what the use-case lane uses: actions a scenario requires or a recorded MCP
// run called. A test in compose/mcpe2e holds the two together.
var listedActions = map[string][]string{
	"change_access":      {"add_team_member", "share"},
	"change_activities":  {"attach", "check_send", "log", "relink", "relink_many", "save_draft"},
	"change_attention":   {"acknowledge_company_view", "create_signal"},
	"change_capture":     {"add_exclusion", "import_vcards"},
	"change_compliance":  {"open_data_subject_request", "place_legal_hold", "suppress_contact"},
	"change_my_settings": {"create_voice_profile", "save_locale", "save_timezone", "save_working_hours"},
	"change_offers":      {"record_contract", "set_contract_status"},
	"change_reports":     {"record_decision"},
	"change_settings":    {"create_stage", "reorder_stages", "update_automation", "upload_company_logo"},
	"change_state": {
		"demote", "disqualify", "move_deal", "move_deal_pipeline", "move_project", "promote", "qualify",
		"record_project_health",
	},
	"change_tags": {"apply", "merge", "remove", "update"},
	"read_details": {
		"company_brief", "company_contracts", "company_linkedin_reach", "company_logo", "contact_brief",
		"contact_consent", "contact_consent_guard", "contact_optout_records", "contact_overrides", "contact_stops",
		"deal_lines", "deal_margince_changes", "dossier", "field_history", "intro_requests", "job_health_history",
		"lead_promotion_preview", "magic", "mail_draft", "notice_case_review", "notice_source_evidence",
		"notice_source_preview", "notices", "scheduled_sends",
	},
	"run_research": {"draft_intro_request"},
}

// ListedActions is listedActions, router by router.
func ListedActions() map[string][]string {
	out := maps.Clone(listedActions)
	for router, actions := range out {
		out[router] = slices.Clone(actions)
	}
	return out
}

// rareActions reports which of a router's actions tools/list lists without
// their arguments, and nil when the router lists every action's arguments. Only
// a router the action catalog answers for may leave one out, since the catalog
// is where a model reads them instead.
func (r *Registry) rareActions(routerName, field string) func(choice string) bool {
	r.mu.RLock()
	described := len(r.routeActions[routerName]) > 0
	r.mu.RUnlock()
	if field != actionArg || !described {
		return nil
	}
	listed := listedActions[routerName]
	return func(choice string) bool { return !slices.Contains(listed, choice) }
}

// ForListing is spec as an MCP client's tools/list shows it: a router's input
// schema leaves out the arguments only its rare actions take. Every other
// reader keeps the registered schema with all of them. The in-app runner is one,
// and it has no action catalog to read them from.
func (r *Registry) ForListing(spec mcp.ToolSpec) mcp.ToolSpec {
	rare := r.rareActions(spec.Name, spec.ActionField)
	if rare == nil {
		return spec
	}
	schema, err := r.narrowedRouterSchema(spec.Name, spec.InputSchema, spec.Actions, nil, rare)
	if err != nil {
		slog.Warn("router schema could not be shortened, so it is listed whole", "tool", spec.Name, "err", err)
		return spec
	}
	spec.InputSchema = schema
	return spec
}

// routerChoices names every action of a router, and none for a tool that is
// not one.
func (r *Registry) routerChoices(routerName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, isRouter := r.tools[routerName].(router)
	if !isRouter {
		return nil
	}
	return slices.Sorted(maps.Keys(rt.routeTargets()))
}
