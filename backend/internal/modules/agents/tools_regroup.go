// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "github.com/margince/margince/backend/internal/shared/ports/mcp"

// The routers below each front one family of verbs under one listed name. A
// call runs as the verb its action names, at that verb's tier and scope.

type findRecords struct{ routed }

func (findRecords) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "find_records", Title: "Find records", Version: toolVersionV1,
		Description: findRecordsCopy.render(), Instead: findRecordsCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"engine":{"description":"search: records ranked by how well their text matches your words; list: one record type by its filters, newest first; query: a typed plan of fields, operators and one join; meaning: passages whose meaning matches; identify: which existing contact or company a name, email or domain is"},
			"q":{"description":"list: text to match alongside any filters; search: the words to rank by, required"},
			"record_types":{"description":"search: these types in place of record_type, omitted for contact, company, deal, lead and project; meaning: these types, omitted for all five"}}}`),
	}
}

func (findRecords) routeField() string { return "engine" }

func (findRecords) routeTargets() map[string]string {
	return map[string]string{
		"search": "search_records", "list": "list_records", topicQuery: "query_workspace",
		"meaning": "search_context", "identify": "resolve_entities",
	}
}

type readContext struct{ routed }

func (readContext) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_context", Title: "Get the story so far", Version: toolVersionV1,
		Description: readContextCopy.render(), Instead: readContextCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"about":{"description":"catch_up: what has been happening on one record; meeting: what to know before a meeting; brief: this morning's brief"}}}`),
	}
}

func (readContext) routeField() string { return "about" }

func (readContext) routeTargets() map[string]string {
	return map[string]string{"catch_up": "catch_me_up_on", "meeting": "prep_for_meeting", "brief": "read_brief"}
}

type readAttention struct{ routed }

func (readAttention) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_attention", Title: "See what needs attention", Version: toolVersionV1,
		Description: readAttentionCopy.render(), Instead: readAttentionCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"view":{"description":"slipping: deals at risk this week; at_risk: relationships going cold; commitments: promises still open"}}}`),
	}
}

func (readAttention) routeField() string { return viewArg }

func (readAttention) routeTargets() map[string]string {
	return map[string]string{
		"slipping": "whats_slipping_this_week", "at_risk": "at_risk_relationships", "commitments": "review_commitments",
	}
}

// kindArg is the argument read_reports picks its report by; the two relink
// verbs are named where their approvals and their router name them.
const (
	kindArg              = "kind"
	relinkActivityTool   = "relink_activity"
	relinkActivitiesTool = "relink_activities"
)

type readReports struct{ routed }

func (readReports) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_reports", Title: "Read sales reports", Version: toolVersionV1,
		Description: readReportsCopy.render(), Instead: readReportsCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"kind":{"description":"standard: the standard sales metrics, saved reports and frozen editions; run: one of the named reports, grouped and filtered; evidence: the passages behind a saved run"}}}`),
	}
}

func (readReports) routeField() string { return kindArg }

func (readReports) routeTargets() map[string]string {
	return map[string]string{"standard": "read_reporting", "run": "run_report", "evidence": "search_report_evidence"}
}

// changeReports fronts the reporting writes an agent may make. Each stays its
// own tool, so a later report write joins as one more action, not one more
// listed tool.
type changeReports struct{ routed }

func (changeReports) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "change_reports", Title: "Prepare saved reports", Version: toolVersionV1,
		Description: changeReportsCopy.render(), Instead: changeReportsCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"action":{"description":"draft: save a report's next charts as a draft for a human to publish, or create a new report only they see"},
			"selection":{"description":"The report's metrics, charts and sections; a pipeline_id comes from ` + pipelinesSource + `"}}}`),
	}
}

func (changeReports) routeField() string { return actionArg }

func (changeReports) routeTargets() map[string]string {
	return map[string]string{"draft": draftReportTool}
}

type readAnalytics struct{ routed }

func (readAnalytics) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_analytics", Title: "Write a report or query data", Version: toolVersionV1,
		Description: readAnalyticsCopy.render(), Instead: readAnalyticsCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"step":{"description":"query: ask a question of the data; compose: build a report document from saved queries"}}}`),
	}
}

func (readAnalytics) routeField() string { return "step" }

func (readAnalytics) routeTargets() map[string]string {
	return map[string]string{topicQuery: "run_analytics_query", "compose": "compose_analytics_report"}
}

type changeActivities struct{ routed }

func (changeActivities) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "change_activities", Title: "Log, plan and file activities", Version: toolVersionV1,
		Description: changeActivitiesCopy.render(), Instead: changeActivitiesCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"action":{"description":"log: record an email, call, meeting, note or message; task: create a task; attach: attach a file; relink: move one activity to another record; relink_many: move a named set; ready: say the user is ready for a meeting; unready: take that back"}}}`),
	}
}

func (changeActivities) routeField() string { return actionArg }

func (changeActivities) routeTargets() map[string]string {
	return map[string]string{
		"log": "log_activity", resultTypeTask: "create_task", "attach": "attach_document",
		"relink": relinkActivityTool, "relink_many": relinkActivitiesTool,
		"ready": confirmMeetingReadyTool, "unready": undoMeetingReadyTool,
	}
}

type scheduleMeeting struct{ routed }

func (scheduleMeeting) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "schedule_meeting", Title: "Book or invite to a meeting", Version: toolVersionV1,
		Description: scheduleMeetingCopy.render(), Instead: scheduleMeetingCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"action":{"description":"book: put a meeting on the calendar; invite: send the invitation to the attendees"}}}`),
	}
}

func (scheduleMeeting) routeField() string { return actionArg }

func (scheduleMeeting) routeTargets() map[string]string {
	return map[string]string{"book": "book_meeting", "invite": "invite_meeting"}
}
