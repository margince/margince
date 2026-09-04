// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for the report vocabulary. See toolcopy.go for what each field
// answers.

var describeReportVocabularyCopy = toolCopy{
	Purpose: "Answer what a run_report plan may SAY: for each prebuilt report, the names its " +
		"group_by, filters and aggregates admit, what it answers with no plan at all, and what " +
		"a name means when the name alone does not say. It is the vocabulary run_report refuses " +
		"against, so it holds the spelling of a name a plan got wrong.",
	Limits: "It describes the reports; it runs none and returns no numbers — run_report does " +
		"that. It is NOT a prerequisite: run_report with `report` alone answers that report's " +
		"default question and needs nothing from here, so reach for this only when a plan has to " +
		"name a grouping, a filter or a measure. The names are the same for every caller, because " +
		"a report's vocabulary is the engine's and not a workspace's.",
	Instead: "Call run_report directly when the report's default answer is the answer wanted, and " +
		"read its refusal when a name is wrong — it carries that argument's accepted list. This " +
		"tool answers the same document as the margince://schema/reports resource, for a caller " +
		"that reads tools rather than resources.",
	Retain: "Take the names from a report's `group_by`, `filters` and `aggregates` verbatim — a " +
		"plan naming anything outside them is refused rather than approximated. `filters` is one " +
		"object holding both equality predicates and numeric thresholds, so a threshold key goes " +
		"there and not in a slot of its own.",
}
