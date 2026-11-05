// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for the reporting writes. See toolcopy.go for what each field
// answers.
var draftReportCopy = toolCopy{
	Purpose: "PREPARE a saved report's charts for review: the metrics, graphs and " +
		"sections of a dashboard, saved as a draft of a report they edit, or as a new report only they see.",
	Limits: "Nothing a reader sees changes: a draft waits until it is published, " +
		"and a new report is private. A report that already has a draft is refused, since it may be " +
		"a colleague's unpublished work. Metrics and graphs come from read_reports kind standard mode " +
		"catalog, and anything else is refused. It never changes who sees a report, its name, or its schedule.",
	Instead: "Use read_reports kind standard mode catalog for the metric and graph IDs, and " +
		"read_analytics step query for a custom grouping, and change_reports publish_draft, update_report " +
		"or create_schedule to publish, share or schedule a report.",
	Retain: "Keep the returned report id and draft_revision; preview the draft with read_reports " +
		"kind standard mode report_evaluation at that revision.",
}

var changeReportsCopy = toolCopy{
	Purpose: "Work what the reporting and forecast pages write. draft saves a report's next charts as a " +
		"draft, or with no report_id creates a new report only its author sees. The others create, update, " +
		"publish, discard, restore or archive a report, set targets, the framework and schedules, freeze an " +
		"edition, retry a run, define and launch a sales initiative, make a forecast call, answer an input " +
		"check and share a forecast by link. record_decision records a decision from a sales review on its " +
		"deal, with an owner and a review date; record_decision_outcome closes it.",
	Limits: "A draft is refused on a report that already has one. publish_draft, discard_draft, " +
		"restore_revision, archive_report, publish_framework, launch_initiative and close_lost_input_check " +
		"change what colleagues see: ask the user first.",
	Instead: "Use read_reports to read reports and their metric catalog, run_research for the sales " +
		"review's assessment or a report set up from a description, and change_state move_deal to move a deal.",
	Retain: "Keep the report id and draft_revision to preview the draft with read_reports.",
}
