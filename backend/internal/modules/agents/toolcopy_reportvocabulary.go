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
		"that. The names are the same for every caller: nothing here is narrowed by what you " +
		"may read, because a report's vocabulary is the engine's and not a workspace's.",
	Instead: "Call run_report once you know the names, with no plan first to see the report's " +
		"own default answer. This tool answers the same document as the " +
		"margince://schema/reports resource, for a caller that reads tools rather than resources.",
	Retain: "Take the names from a report's `group_by`, `filters` and `aggregates` verbatim — a " +
		"plan naming anything outside them is refused rather than approximated. `filters` is one " +
		"object holding both equality predicates and numeric thresholds, so a threshold key goes " +
		"there and not in a slot of its own.",
}
