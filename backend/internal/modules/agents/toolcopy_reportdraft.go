// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for the reporting writes. See toolcopy.go for what each field
// answers.
var draftReportCopy = toolCopy{
	Purpose: "PREPARE a saved report's charts for a human to review: the metrics, graphs and " +
		"sections of a dashboard, saved as a draft of a report they edit, or as a new report only they see.",
	Limits: "Nothing a reader sees changes: a draft waits for a human to publish it in Analytics, " +
		"and a new report is private. A report that already has a draft is refused, since it may be " +
		"a human's unpublished work. Metrics and graphs come from read_reports kind standard mode " +
		"catalog, and anything else is refused. It never changes who sees a report, its name, or its schedule.",
	Instead: "Use read_reports kind standard mode catalog for the metric and graph IDs, and " +
		"read_analytics step query for a custom grouping. Publish, share and schedule a report in Analytics.",
	Retain: "Keep the returned report id and draft_revision; preview the draft with read_reports " +
		"kind standard mode report_evaluation at that revision.",
}

var changeReportsCopy = toolCopy{
	Purpose: "Change saved reports for a human to review. draft saves a report's next charts as a " +
		"draft of a report they edit, or with no report_id creates a new report only they see.",
	Limits: "A draft waits for a human to publish it in Analytics, and is refused on a report that " +
		"already has one. Publishing, discarding, sharing, scheduling and archiving a report stay a human's.",
	Instead: "Use read_reports to read reports and their metric catalog.",
	Retain:  "Keep the report id and draft_revision to preview the draft with read_reports.",
}
