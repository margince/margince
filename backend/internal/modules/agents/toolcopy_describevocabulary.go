// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

var describeVocabularyCopy = toolCopy{
	Purpose: "Answer the names another tool accepts, one topic at a time. query: the record " +
		"types, fields, operators and the one relationship hop a query_workspace plan may name. " +
		"analytics: the populations, group_by dimensions, measures and functions " +
		"run_analytics_query takes for this seat. record_fields: what a create_record or " +
		"update_record `fields` body accepts for each record_type. report_vocabulary: the " +
		"group_by, filters and aggregates each run_report report admits. report_blocks: the block " +
		"kinds a compose_analytics_report document may contain.",
	Limits: "It returns names, never records or numbers, and none of it is a prerequisite: each " +
		"of those tools refuses an unknown name together with the accepted list. query, analytics " +
		"and record_fields are narrowed to what you may already see.",
	Instead: "Call the tool that does the work directly when the names are already known, and " +
		"read its refusal when one is wrong.",
	Retain: "Take every name verbatim; a name outside the document is refused, not approximated. " +
		"In record_fields a key with no `?` is required, and a custom field is spelled " +
		"cf_<slug>. A report figure is cited from a saved run, never written into a block.",
}
