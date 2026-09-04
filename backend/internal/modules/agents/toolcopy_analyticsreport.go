// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for composing a report. See toolcopy.go for what each field
// answers.
//
// The BLOCK VOCABULARY is deliberately not here. Fourteen block kinds with
// their fields would be run_report's 3.4KB mistake again — text every client
// holds for a session and every scheduled run re-sends on every step, to answer
// a question one call asks once. It lives in the margince://schema/report-blocks
// resource and describe_report_blocks, the same move #3950 made for the report
// vocabulary.
//
// This copy NAMES that document and never orders a read of it. The refusal path
// is what makes that safe: a kind outside the grammar is refused by name with
// the whole set, so a caller that guesses wrong pays one refusal — where an
// instruction to read first costs a turn on every goal, including the ones with
// nothing to look up.
var composeAnalyticsReportCopy = toolCopy{
	Purpose: "Render a report whose every figure comes from a saved analytics run. The " +
		"document carries the STRUCTURE and the WORDS; each number names a run id and a cell " +
		"inside it, and the server resolves those handles under the reader's own authority.",
	Limits: "It writes no number of its own and refuses any document that does. A block " +
		"carrying a literal figure is refused EVEN WHEN a valid handle sits beside it: the " +
		"literal is what renders, the two can disagree, and no reader could tell the page " +
		"shows a figure the database never computed. Save a run first — run an analytics " +
		"query with save, and cite the run id it answers with.",
	Instead: "Ask analytics_query for one number when a figure is what is wanted. This " +
		"composes a DOCUMENT of several, which is worth the round trip only when the answer " +
		"is a report somebody reads. describe_report_blocks holds the block kinds and their " +
		"fields for a caller that wants them before composing.",
	Retain: "Never put a number in a block — cite the cell that holds it. A block kind " +
		"outside the grammar is refused BY NAME with the whole set, so a first attempt costs " +
		"one refusal rather than a lookup.",
}
