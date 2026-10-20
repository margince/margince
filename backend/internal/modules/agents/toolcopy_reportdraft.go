// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for drafting a saved report. See toolcopy.go for what each
// field answers.
var draftReportCopy = toolCopy{
	Purpose: "PREPARE a saved report's charts for a human to review: the metrics and graphs " +
		"of a dashboard, saved as a draft of a report they edit, or as a new report only they see.",
	Limits: "Nothing a reader sees changes: a draft waits for a human to publish it in Analytics, " +
		"and a new report is private. Metrics and graphs come from read_reporting's catalog, and " +
		"anything else is refused. It never changes who sees a report, its name, or its schedule.",
	Instead: "Use read_reporting with mode catalog for the metric and graph IDs, and run_analytics_query " +
		"for a custom grouping. Publish, share and schedule a report in Analytics.",
	Retain: "Keep the returned report id and draft_revision; preview the draft with read_reporting " +
		"mode report_evaluation at that revision.",
}
