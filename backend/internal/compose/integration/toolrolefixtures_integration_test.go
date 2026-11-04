// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// toolRoleFixtures are the arguments a verb needs that its schema does not
// say, such as a field one record type requires or a list that must not be
// empty. They are laid over what the filler wrote. A request_body is merged
// into the filled one.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
var toolRoleFixtures = map[string]func(p *recordPool) map[string]any{
	"create_record": func(*recordPool) map[string]any {
		return map[string]any{"fields": map[string]any{"display_name": "Role Sweep", "source": "manual"}}
	},
	"update_record": func(*recordPool) map[string]any {
		return map[string]any{"fields": map[string]any{"display_name": "Role Sweep Renamed"}}
	},
	"bulk_update_records": func(p *recordPool) map[string]any {
		return map[string]any{"record_type": "company", "verb": "archive", "items": []any{map[string]any{"id": p.take("company"), "version": 1}}}
	},
	"change_lists": func(*recordPool) map[string]any {
		return map[string]any{"name": "Role Sweep", "entity_type": "company", "list_type": "static"}
	},
	"catch_me_up_on":   recordNamed,
	"prep_for_meeting": recordNamed,
	"resolve_entities": func(*recordPool) map[string]any {
		return map[string]any{"candidates": []any{map[string]any{"kind": "company", "name": "Role Sweep"}}}
	},
	"query_workspace": func(*recordPool) map[string]any { return map[string]any{"plan": map[string]any{"version": "v1"}} },
	"merge_records": func(p *recordPool) map[string]any {
		return map[string]any{"record_type": "company", "source_id": p.take("company"), "target_id": p.take("company")}
	},
	"preview_import": func(*recordPool) map[string]any {
		return map[string]any{"object": "company", "csv": "display_name\nRole Sweep Import\n"}
	},
	"draft_email": func(p *recordPool) map[string]any {
		return map[string]any{"links": []any{map[string]any{"entity_type": "contact", "entity_id": p.take("contact")}}}
	},
	"send_company_email": func(p *recordPool) map[string]any { return withRecipient(linksToACompany(p)) },
	"send_email":         func(*recordPool) map[string]any { return withRecipient(map[string]any{}) },
	"set_up_partner":     func(*recordPool) map[string]any { return map[string]any{"partner_kinds": []any{"referral"}} },
	"link_company_contacts": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"contact_ids": []any{p.take("contact")}}}
	},
	"relink_activities":   func(p *recordPool) map[string]any { return map[string]any{"activity_ids": []any{p.take("activity")}} },
	"draft_report":        func(*recordPool) map[string]any { return map[string]any{"name": "Role Sweep"} },
	"run_analytics_query": func(*recordPool) map[string]any { return map[string]any{"entity": "deals-by-stage"} },
	"set_weekly_plan_contract": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"capacity_note": "Role sweep"}}
	},
	"edit_weekly_plan_commitment": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"label": "Role sweep edited"}}
	},
	// Query parameters the route requires and its verb's schema does not mark.
	"get_field_history": func(p *recordPool) map[string]any {
		return map[string]any{"entity_type": "company", "entity_id": p.take("company")}
	},
	"get_mail_draft": func(p *recordPool) map[string]any {
		return map[string]any{"anchor_type": "company", "anchor_id": p.take("company")}
	},
	"list_meeting_proposals":  func(p *recordPool) map[string]any { return map[string]any{"contact_id": p.take("contact")} },
	"get_company_for_address": func(*recordPool) map[string]any { return map[string]any{"email": "buyer@role-sweep.test"} },
	"apply_tag":               func(p *recordPool) map[string]any { return map[string]any{"tag_id": p.take("tag")} },
	"remove_tag":              func(p *recordPool) map[string]any { return map[string]any{"tag_id": p.take("tag")} },
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func recordNamed(p *recordPool) map[string]any {
	return map[string]any{"record_type": "company", "record_id": p.take("company")}
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func linksToACompany(p *recordPool) map[string]any {
	return map[string]any{"links": []any{map[string]any{"entity_type": "company", "entity_id": p.take("company")}}}
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func withRecipient(args map[string]any) map[string]any {
	args["to"] = []any{"buyer@role-sweep.test"}
	return args
}
