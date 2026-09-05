// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for the typed analytics query. See toolcopy.go for what each
// field answers.
//
// The VOCABULARY is deliberately not here: the populations and their fields
// are derived per caller — a masked field is indistinguishable from one that
// does not exist — so the tool names margince://schema/analytics instead of
// carrying a list that would be wrong for somebody. The refusal path makes
// that safe: an unknown name is refused by name with the allowed set.
var runAnalyticsQueryCopy = toolCopy{
	Purpose: "Compute a grouped aggregate — counts, sums, averages, medians — over a " +
		"governed population, in the database. The answer carries its columns, rows and " +
		"schema version; groups too small to disclose are withheld, never estimated.",
	Limits: "Populations, dimensions and measures come from margince://schema/analytics, " +
		"derived for this seat; a name outside it is refused with what would work. Money " +
		"measures are minor units. An omitted scope is this seat's own default population, " +
		"never the workspace.",
	Instead: "run_report answers a prebuilt report by key; query_workspace lists exact " +
		"records; the forecast tools answer forecast readings and movement. This one is for " +
		"a novel aggregate no prebuilt report shapes.",
	Retain: "Set save to get a run_id whose cells compose_analytics_report can cite; " +
		"without it the answer is served once and not stored.",
}
