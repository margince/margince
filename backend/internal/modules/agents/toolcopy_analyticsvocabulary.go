// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for the analytics vocabulary. See toolcopy.go for what each
// field answers.

var describeAnalyticsVocabularyCopy = toolCopy{
	Purpose: "Answer what an analytics query may SAY for THIS seat: the populations that can " +
		"be measured, the group_by dimensions and measures each carries, and the aggregate " +
		"functions and filter operators the grammar takes. It is the vocabulary " +
		"run_analytics_query refuses against, so it holds the spelling of a population or " +
		"field a query got wrong.",
	Limits: "It describes the vocabulary; it computes nothing — run_analytics_query does " +
		"that. The document is derived per caller and narrowed to what this seat may already " +
		"see, so a withheld field is simply absent rather than marked. It answers the same " +
		"document as the margince://schema/analytics resource, for a caller that reads tools " +
		"rather than resources.",
	Instead: "Call run_analytics_query directly when the names are already known — an " +
		"unknown population is refused with the allowed set, so a near-miss costs one round " +
		"trip rather than a lookup.",
	Retain: "Take population, dimension and measure names verbatim — a name outside the " +
		"document is refused rather than approximated. The version line is the " +
		"schema_version a saved run answers with.",
}
