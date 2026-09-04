// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for the report block grammar. See toolcopy.go for what each
// field answers.

var describeReportBlocksCopy = toolCopy{
	Purpose: "Answer what a compose_analytics_report document may CONTAIN: every block kind, " +
		"whether it renders figures or words, and the severities a callout may state.",
	Limits: "It describes the grammar; it composes nothing and returns no numbers. It is NOT " +
		"a prerequisite — an unknown block kind is refused by name with the whole set, so a " +
		"first attempt costs one refusal. The grammar is the same for every caller, because it " +
		"is the engine's and not a workspace's.",
	Instead: "Compose directly when the blocks needed are the obvious ones, and read the " +
		"refusal when a kind is wrong — it carries the accepted set. This tool answers the " +
		"same document as the margince://schema/report-blocks resource, for a caller that " +
		"reads tools rather than resources.",
	Retain: "A figure is never written into a block, only cited: every number names a saved " +
		"run and a cell inside it. A block carrying a literal number is refused even beside a " +
		"valid citation.",
}
