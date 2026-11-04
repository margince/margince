// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// toolRoleFixtures are what a verb needs and its schema does not say. That is
// a field one record type requires, a record in a given state, or a value a
// rule outside the schema holds. They are laid over what the filler
// wrote, and a request_body is merged into the filled one.
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
		return map[string]any{"name": uniqueName(), "entity_type": "company", "list_type": "static"}
	},
	"catch_me_up_on":   recordNamed,
	"prep_for_meeting": recordNamed,
	"resolve_entities": func(*recordPool) map[string]any {
		return map[string]any{"candidates": []any{map[string]any{"kind": "company", "name": "Role Sweep"}}}
	},
	"query_workspace": func(*recordPool) map[string]any {
		return map[string]any{"plan": map[string]any{"version": "v1", "target": "deal", "where": []any{map[string]any{"field": "status", "op": "eq", "value": "open"}}}}
	},
	"merge_records": func(p *recordPool) map[string]any {
		return map[string]any{"record_type": "company", "source_id": p.take("company"), "target_id": p.take("company")}
	},
	"preview_import": func(*recordPool) map[string]any {
		return map[string]any{"object": "company", "csv": "display_name\nRole Sweep Import\n"}
	},
	"draft_email": func(p *recordPool) map[string]any {
		return map[string]any{"links": []any{map[string]any{"entity_type": "contact", "entity_id": p.take("contact")}}}
	},
	"send_company_email": func(p *recordPool) map[string]any {
		contact := p.take("consented_contact")
		return map[string]any{
			"links": []any{map[string]any{"entity_type": "deal", "entity_id": p.fact(contact, "deal")}},
			"to":    []any{p.fact(contact, "email")}, "consent_purpose": "transactional",
		}
	},
	"send_email": func(p *recordPool) map[string]any {
		contact := p.take("consented_contact")
		return map[string]any{"activity_id": p.fact(contact, "anchor"), "to": []any{p.fact(contact, "email")}, "consent_purpose": "transactional"}
	},
	"set_up_partner": func(*recordPool) map[string]any { return map[string]any{"partner_kinds": []any{"referral"}} },
	"link_company_contacts": func(p *recordPool) map[string]any {
		contact := p.take("linkable_contact")
		return map[string]any{"id": p.fact(contact, "company"), "request_body": map[string]any{"contact_ids": []any{contact}}}
	},
	"relink_activities": func(p *recordPool) map[string]any { return map[string]any{"activity_ids": []any{p.take("activity")}} },
	"draft_report": func(*recordPool) map[string]any {
		return map[string]any{"name": uniqueName(), "selection": map[string]any{
			"scope": map[string]any{"kind": "workspace"}, "period": "this_month", "target_basis": "month",
			"close_window": "all_open", "metrics": []any{"bookings_won"}, "blocks": []any{},
		}}
	},
	"run_analytics_query": func(*recordPool) map[string]any {
		return map[string]any{"entity": "pipeline-current", "measures": []any{map[string]any{"fn": "count", "as": "deals"}}}
	},
	"compose_analytics_report": func(*recordPool) map[string]any {
		return map[string]any{"blocks": []any{map[string]any{"kind": "title", "text": "Pipeline"}}}
	},
	"set_weekly_plan_contract": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"capacity_note": "Role sweep"}}
	},
	"edit_weekly_plan_commitment": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"label": "Role sweep edited"}}
	},
	"apply_tag":  func(p *recordPool) map[string]any { return map[string]any{"tag_id": p.take("tag")} },
	"remove_tag": func(p *recordPool) map[string]any { return map[string]any{"tag_id": p.take("tag")} },
	"get_mail_draft": func(p *recordPool) map[string]any {
		return map[string]any{"anchor_type": "company", "anchor_id": p.fact(p.take("mail_draft"), "anchor")}
	},
	"change_record_parts": func(p *recordPool) map[string]any {
		return map[string]any{"action": "add_company_fact", "company_id": p.take("company"), "category": "company", "field": "employee_range", "value": "11-50"}
	},
	"promote_lead": func(*recordPool) map[string]any { return map[string]any{"trigger": "human_qualify"} },
	"demote_lead":  func(p *recordPool) map[string]any { return map[string]any{"lead_id": p.take("promoted_lead")} },
	"enrich": func(p *recordPool) map[string]any {
		return map[string]any{"company_id": p.take("domain_company"), "depth": "site"}
	},
	"commit_import":      func(p *recordPool) map[string]any { return map[string]any{"run_id": p.take("import_run")} },
	"read_import_report": func(p *recordPool) map[string]any { return map[string]any{"run_id": p.take("import_run")} },
	"read_approval":      func(p *recordPool) map[string]any { return map[string]any{"staged_action_id": p.take("approval")} },
	"decide_approval": func(p *recordPool) map[string]any {
		return map[string]any{"staged_action_id": p.take("approval"), "decision": "approve"}
	},
	"decide_approval_bundle": func(p *recordPool) map[string]any {
		return map[string]any{"bundle_id": p.take("approval_bundle"), "decision": "approve"}
	},
	"decide_duplicate": func(p *recordPool) map[string]any { return map[string]any{"candidate_id": p.take("dedupe_candidate")} },
	"merge_members": func(p *recordPool) map[string]any {
		source := p.take("merge_source")
		return map[string]any{"source_id": source, "to_user_id": p.fact(source, "to_user_id"), "preview_token": p.fact(source, "token")}
	},
	"read_brief":     inBrief(map[string]any{}),
	"annotate_brief": inBrief(map[string]any{"narrative": "", "items": []any{}}),
	"get_report_run": func(p *recordPool) map[string]any { return map[string]any{"run_id": p.take("report_run")} },
	"search_report_evidence": func(p *recordPool) map[string]any {
		return map[string]any{"run_id": p.take("report_run"), "query": "role sweep"}
	},
	"data_coverage":         afterInputCheck,
	"forecast_input_checks": afterInputCheck,
	"forecast_movement": func(p *recordPool) map[string]any {
		return map[string]any{"from": p.take("forecast_snapshot"), "to": p.take("forecast_snapshot")}
	},
}

// The fixtures of route-backed verbs, whose arguments are their route's path
// and query parameters and a request_body.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
var routeVerbFixtures = map[string]func(p *recordPool) map[string]any{
	"get_ownership_offer":        onRecord("offered_company"),
	"withdraw_ownership_offer":   onRecord("offered_company"),
	"get_ownership_request":      onRecord("requested_company"),
	"withdraw_ownership_request": onRecord("requested_company"),
	"reactivate_user":            byID("deactivated_user"),
	"update_member": func(p *recordPool) map[string]any {
		user := p.take("user")
		return map[string]any{"id": user, "request_body": map[string]any{"email": p.fact(user, "email"), "expected_updated_at": p.fact(user, "updated_at")}}
	},
	"remove_workspace_member": func(p *recordPool) map[string]any {
		user := p.take("retiring_user")
		return map[string]any{"id": user, "request_body": map[string]any{"preview_token": p.fact(user, "token")}}
	},
	"get_attachment_extraction": byID("read_attachment"),
	"accept_attachment_extraction": func(p *recordPool) map[string]any {
		file := p.take("read_attachment")
		return map[string]any{"id": file, "request_body": map[string]any{"extraction_id": p.fact(file, "reading"), "field_keys": []any{"amount_minor", "currency"}}}
	},
	"read_attachment_for_fields":     byID("deal_attachment"),
	"read_transcript_for_next_steps": byID("transcript"),
	"confirm_meeting_ready":          readiness("meeting"),
	"undo_meeting_ready":             readiness("ready_meeting"),
	"decide_legacy_loss_reason": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"reason_key": p.take("loss_reason_key")}}
	},
	"discard_mail_draft":    byID("mail_draft"),
	"get_scheduled_send":    byID("scheduled_send"),
	"cancel_scheduled_send": byID("scheduled_send"),
	"reschedule_scheduled_send": func(p *recordPool) map[string]any {
		send := p.take("scheduled_send")
		version, _ := strconv.Atoi(p.fact(send, "version"))
		return map[string]any{"id": send, "if_version": version}
	},
	"save_mail_draft": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"anchor_type": "company", "anchor_id": p.take("company")}}
	},
	"set_activity_disposition": byID("inbound_email"),
	"set_thread_audience":      func(p *recordPool) map[string]any { return map[string]any{"thread_key": p.take("captured_thread")} },
	"hide_deal_document":       dealDocument,
	"unhide_deal_document":     dealDocument,
	"accept_deal_suggestion":   byID("deal_suggestion"),
	"dismiss_deal_suggestion":  byID("deal_suggestion"),
	"get_deal_suggestion":      byID("deal_suggestion"),
	"accept_tag_suggestion":    byID("tag_suggestion"),
	"dismiss_tag_suggestion":   byID("tag_suggestion"),
	"get_tag_suggestion":       byID("tag_suggestion"),
	"acknowledge_approval_failure": func(p *recordPool) map[string]any {
		approval := p.take("failed_approval")
		return map[string]any{"id": approval, "request_body": map[string]any{"failed_at": p.fact(approval, "failed_at")}}
	},
	"mark_brief_item_acted":     briefItem("brief_item"),
	"mark_brief_item_dismissed": briefItem("brief_item"),
	"unsnooze_brief_item":       briefItem("snoozed_brief_item"),
	"snooze_brief_item": func(p *recordPool) map[string]any {
		return map[string]any{"itemId": p.take("brief_item"), "request_body": map[string]any{"snoozed_until": p.sweepStart.Add(72 * time.Hour).Format(time.RFC3339)}}
	},
	"dismiss_company_suggestion": dismissedBy,
	"dismiss_deal_move":          dismissedBy,
	"restore_company_suggestion": func(*recordPool) map[string]any { return map[string]any{"fingerprint": sweepFingerprint} },
	"restore_deal_move":          func(*recordPool) map[string]any { return map[string]any{"fingerprint": sweepFingerprint} },
	"dismiss_relationship_nudge": func(*recordPool) map[string]any { return map[string]any{"request_body": map[string]any{"days": 7}} },
	"dismiss_relationship_nudges": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"contact_ids": []any{p.take("contact")}, "days": 7}}
	},
	"mark_notice_read": byID("notice"),
	"raise_notice": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"kind": "coach_reply_aging", "recipient_user_id": p.take("teammate")}}
	},
	"pin_worklist_row": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"source": "task", "row_id": p.take("activity")}}
	},
	"unpin_worklist_row": func(p *recordPool) map[string]any {
		return map[string]any{"source": "task", "row_id": p.take("activity")}
	},
	"reopen_worklist_rows": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"rows": []any{map[string]any{"source": "deal_at_risk", "row_id": p.take("deal")}}}}
	},
	"get_hidden_backlog_rows": func(*recordPool) map[string]any { return map[string]any{"rule": "set_aside"} },
	"accept_offer":            byID("sent_offer"),
	"reject_offer":            byID("sent_offer"),
	"regenerate_offer":        byID("sent_offer"),
	"send_offer":              byID("priced_offer"),
	"download_offer_pdf":      byID("rendered_offer"),
	"add_deal_room_document": func(p *recordPool) map[string]any {
		file := p.take("room_attachment")
		return map[string]any{"id": p.fact(file, "room"), "request_body": map[string]any{"attachment_id": file}}
	},
	"remove_deal_room_document":    inRoom("room_document", "documentId"),
	"update_deal_room_document":    inRoom("room_document", "documentId"),
	"revoke_deal_room_participant": inRoom("room_participant", "participantId"),
	"update_deal_room_participant": inRoom("room_participant", "participantId"),
	"resend_deal_room_invitation":  inRoom("room_participant", "participantId"),
	"resolve_deal_room_thread":     inRoom("room_thread", "threadId"),
	"resume_deal_room":             byID("paused_room"),
	"draft_intro_note": func(p *recordPool) map[string]any {
		contact := p.take("routed_contact")
		return map[string]any{"id": contact, "request_body": map[string]any{"via_user_id": p.fact(contact, "via")}}
	},
	"draft_intro_request": func(p *recordPool) map[string]any {
		contact := p.take("routed_contact")
		return map[string]any{"request_body": map[string]any{"via_user_id": p.fact(contact, "via"), "contact_id": contact}}
	},
	"record_introduction": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{
			"introduced_on": pastDay(p), "introducer_user_id": p.take("me"), "introduced_user_id": p.take("user"), "channel": "in_person",
		}}
	},
	"record_conversation_claim": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"source_quote": "Role sweep"}}
	},
	"settle_conversation_claim":    byID("claim"),
	"take_over_conversation_claim": byID("claim"),
	"put_anchor_company": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"display_name": p.take("anchor_name"), "offer_summary": "Role sweep", "icp": "Role sweep"}}
	},
	"update_activity_review_template": func(p *recordPool) map[string]any {
		template := p.take("review_template")
		version, _ := strconv.Atoi(p.fact(template, "version"))
		var questions []any
		if err := json.Unmarshal([]byte(p.fact(template, "questions")), &questions); err != nil {
			questions = nil
		}
		return map[string]any{"id": template, "request_body": map[string]any{"version": version, "questions": questions}}
	},
	"update_legal_review_settings": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"enabled": true}}
	},
	"accept_partner_lead":      byID("partner_lead"),
	"clear_lead_manual_signal": func(*recordPool) map[string]any { return map[string]any{"factor": "web_traffic"} },
	"set_lead_manual_signal": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"band": "high"}}
	},
	"correct_deal_close_day": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("lost_deal"), "request_body": map[string]any{"closed_on": pastDay(p)}}
	},
	"create_deal_outcome_review": func(p *recordPool) map[string]any {
		deal := p.take("lost_deal")
		return map[string]any{"id": deal, "request_body": map[string]any{
			"closing_occurrence_id": p.fact(deal, "closing_occurrence_id"), "submission_id": ids.NewV7().String(),
			"answers": map[string]any{"why_we_lost": "Role sweep"},
		}}
	},
	"decide_legal_review":      byID("colleague_legal_review"),
	"get_teammate_weekly_plan": func(p *recordPool) map[string]any { return map[string]any{"owner_id": p.take("teammate_plan")} },
	"answer_weekly_plan_commitment": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("teammate_commitment"), "request_body": map[string]any{"manager_response": "Role sweep"}}
	},
	"decide_intro_request": byID("intro_ask"),
	"complete_intro_request": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("accepted_intro_ask"), "request_body": map[string]any{"version": 2}}
	},
	"revert_stage_progression": func(p *recordPool) map[string]any {
		deal := p.take("auto_moved_deal")
		return map[string]any{"id": deal, "approvalId": p.fact(deal, "move")}
	},
	"accept_applied_deal_change": func(p *recordPool) map[string]any {
		deal := p.take("auto_moved_deal")
		version, _ := strconv.Atoi(p.fact(deal, "version"))
		return map[string]any{"id": deal, "changeId": p.fact(deal, "move"), "if_version": version}
	},
	"withdraw_legal_review": byID("legal_review"),
	"get_legal_review":      byID("legal_review"),
	"link_lead_company": func(p *recordPool) map[string]any {
		lead := p.take("matched_lead")
		return map[string]any{"id": lead, "request_body": map[string]any{"company_id": p.fact(lead, "company")}}
	},
	"progress_deal":            func(p *recordPool) map[string]any { return map[string]any{"to_stage_id": p.take("open_stage")} },
	"publish_captured_contact": byID("owner_contact"),
	"reopen_lead":              byID("disqualified_lead"),
	"restore_tag":              byID("archived_tag"),
	"download_import_problems": byID("import_run"),
	"get_claim_evidence": func(p *recordPool) map[string]any {
		field := p.take("profile_field")
		return map[string]any{"id": p.fact(field, "company"), "entityType": "profile_field", "entityId": field}
	},
	"export_forecast_share":    sharedReading,
	"open_forecast_share":      sharedReading,
	"download_corpus_document": byID("corpus_document"),
	"answer_reporting_question_card": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("question_report"), "card_id": "q1"}
	},
	"preview_reporting_report_as_role": byID("report"),
	"get_ai_task_flow":                 func(*recordPool) map[string]any { return map[string]any{"task": "account_scan"} },
	"list_automation_runs":             byID("automation"),
	"get_company_logo":                 byID("anchor_logo"),
	"get_voice_build": func(p *recordPool) map[string]any {
		build := p.take("voice_build")
		return map[string]any{"id": p.fact(build, "profile"), "buildId": build}
	},
	"get_anchor_company_site_read":     func(p *recordPool) map[string]any { return map[string]any{"readId": p.take("site_read")} },
	"message_anchor_company_site_read": func(p *recordPool) map[string]any { return map[string]any{"readId": p.take("site_read")} },
	"confirm_anchor_company_site_read": func(p *recordPool) map[string]any {
		read := p.take("finished_site_read")
		version, _ := strconv.Atoi(p.fact(read, "draft_version"))
		return map[string]any{"readId": read, "request_body": map[string]any{
			"draft_version": version, "proposal_hash": p.fact(read, "proposal_hash"), "selected_fact_keys": []any{},
			"profile": map[string]any{"display_name": p.take("anchor_name"), "offer_summary": p.fact(read, "offer_summary"), "icp": p.fact(read, "icp")},
		}}
	},
	"read_capture_trace_pipeline": byID("capture_trace"),
	"get_magic_line_records": func(p *recordPool) map[string]any {
		line := p.take("magic_line")
		return map[string]any{"id": line, "since": p.fact(line, "since")}
	},
	"get_latest_weekly_review": func(p *recordPool) map[string]any {
		p.take("weekly_review")
		return map[string]any{}
	},
	"get_team_weekly_review": func(p *recordPool) map[string]any { return map[string]any{"team": p.take("review_team")} },
	"change_meeting_invitation": func(p *recordPool) map[string]any {
		invitation := p.take("confirmed_invitation")
		version, _ := strconv.Atoi(p.fact(invitation, "version"))
		return map[string]any{"id": invitation, "request_body": map[string]any{
			"action": "reschedule", "version": version, "start": p.fact(invitation, "start"), "end": p.fact(invitation, "end"),
		}}
	},
	"get_meeting_availability": func(p *recordPool) map[string]any {
		from := p.sweepStart.Truncate(24 * time.Hour).Add(48 * time.Hour)
		return map[string]any{"id": p.take("confirmed_invitation"), "from": from.Format(time.RFC3339), "to": from.Add(7 * 24 * time.Hour).Format(time.RFC3339)}
	},
	"invite_meeting": func(p *recordPool) map[string]any {
		start, contact := bookableSlot(p), p.take("consented_contact")
		return map[string]any{
			"start": start.Format(time.RFC3339), "end": start.Add(30 * time.Minute).Format(time.RFC3339),
			"contact_id": contact, "attendee_email": p.fact(contact, "email"),
		}
	},
	"create_meeting_proposal": func(p *recordPool) map[string]any {
		contact := p.take("consented_contact")
		return map[string]any{"request_body": map[string]any{"contact_id": contact, "attendee_email": p.fact(contact, "email")}}
	},
	// The booking page the sweep's app set up, saved again as it stands. The
	// calls after it then still book against a page that takes bookings.
	"put_scheduling_profile": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{
			"provider": "gcal", "calendar_id": "ada@example.com", "enabled": true, "duration_minutes": 30, "notice_minutes": 0,
			"buffer_minutes": 0, "horizon_days": 30, "title": "Meeting", "host_name": "Ada", "location": "Video call",
		}}
	},
	"list_connector_containers": func(*recordPool) map[string]any { return map[string]any{"provider": "gmail"} },
	"mark_worklist_rows_done": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"rows": []any{map[string]any{"source": "meeting", "row_id": p.take("worklist_meeting")}}}}
	},
	"restore_contact_profile_field": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("vcard_contact"), "field": "title"}
	},
	"get_contact_enrichment_run": func(p *recordPool) map[string]any {
		run := p.take("enrichment_run")
		return map[string]any{"id": p.fact(run, "contact"), "run_id": run}
	},
	"send_message": func(p *recordPool) map[string]any {
		return map[string]any{"activity_id": p.take("channel_conversation"), "consent_purpose": "transactional"}
	},
	"request_company_vat_check": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.fact(p.take("profile_field"), "company")}
	},
	"get_company_logo_icon": byID("anchor_logo"),
	"get_legal_entity_logo": byID("entity_logo"),
}

// fixtureOf is the fixture a verb has in either table.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func fixtureOf(verb string) (func(p *recordPool) map[string]any, bool) {
	if fixture, ok := toolRoleFixtures[verb]; ok {
		return fixture, true
	}
	fixture, ok := routeVerbFixtures[verb]
	return fixture, ok
}

// sweepFingerprint is a well-formed card fingerprint: a dismissal names one
// whether or not a card served it.
var sweepFingerprint = strings.Repeat("0", 64)

func uniqueName() string { return "Role Sweep " + ids.NewV7().String()[24:] }

func pastDay(p *recordPool) string { return p.sweepStart.Add(-48 * time.Hour).Format(time.DateOnly) }

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func byID(kind string) func(*recordPool) map[string]any {
	return func(p *recordPool) map[string]any { return map[string]any{"id": p.take(kind)} }
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func onRecord(kind string) func(*recordPool) map[string]any {
	return func(p *recordPool) map[string]any {
		return map[string]any{"record_type": "company", "id": p.take(kind)}
	}
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func readiness(kind string) func(*recordPool) map[string]any {
	return func(p *recordPool) map[string]any {
		meeting := p.take(kind)
		return map[string]any{"activity_id": meeting, "preparation_revision": p.fact(meeting, "revision")}
	}
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func briefItem(kind string) func(*recordPool) map[string]any {
	return func(p *recordPool) map[string]any { return map[string]any{"itemId": p.take(kind)} }
}

// inBrief reads or annotates the day's brief, which an item's seed assembles.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func inBrief(args map[string]any) func(*recordPool) map[string]any {
	return func(p *recordPool) map[string]any {
		p.take("brief_item")
		return args
	}
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func afterInputCheck(p *recordPool) map[string]any {
	p.take("input_check")
	return map[string]any{}
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func inRoom(kind, param string) func(*recordPool) map[string]any {
	return func(p *recordPool) map[string]any {
		child := p.take(kind)
		return map[string]any{"id": p.fact(child, "room"), param: child}
	}
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func dealDocument(p *recordPool) map[string]any {
	file := p.take("deal_attachment")
	return map[string]any{"id": p.fact(file, "deal"), "attachmentId": file}
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func dismissedBy(*recordPool) map[string]any {
	return map[string]any{"request_body": map[string]any{"fingerprint": sweepFingerprint}}
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func sharedReading(p *recordPool) map[string]any {
	return map[string]any{"token": p.take("forecast_share")}
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func recordNamed(p *recordPool) map[string]any {
	return map[string]any{"record_type": "company", "record_id": p.take("company")}
}
