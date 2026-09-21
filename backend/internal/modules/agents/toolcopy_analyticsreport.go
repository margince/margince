// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for composing a report. See toolcopy.go for what each field
// answers.
//
// THE PURPOSE LEADS WITH THE JOB, not with the mechanism, and that ordering is
// load-bearing. Asked for "a short written pipeline section for the board pack",
// three measured runs read a catalogue of 75 tools, took the figures from
// forecast_readings and typed them into prose — the one thing this tool refuses
// to let a document do. It was served every time and chosen none of them. Its
// first sentence had been "Render a report whose every figure comes from a
// saved analytics run": true, and about handles, run ids and cells, while the
// words a caller scans for — write, document, section, summary — were absent or
// buried. A tool nobody picks for the job it does is a tool that is not there.
//
// AND IT SAYS WHAT IT IS NOT FOR, in the same breath, because the first attempt
// at this over-reached. Offering "a written-up answer with figures in it" took
// a model off run_report for a question that just wanted a number — asked for
// one, it went and composed. The boundary belongs beside the invitation: a
// Purpose broad enough to be found is broad enough to be found by the wrong
// caller, and Instead: alone was too far down to stop it.
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
	Purpose: "WRITE a DOCUMENT somebody keeps and reads — a board-pack section, a summary " +
		"for a meeting — whose every number comes from a saved analytics run instead of " +
		"being typed. Not for answering with a figure: a number in the reply is " +
		"run_analytics_query's or run_report's. The document carries the STRUCTURE and the " +
		"WORDS; each figure names a run id and a cell inside it, and the server resolves " +
		"those handles under the reader's own authority.",
	Limits: "It writes no number of its own and refuses any document that does. A block " +
		"carrying a literal figure is refused EVEN WHEN a valid handle sits beside it: the " +
		"literal is what renders, the two can disagree, and no reader could tell the page " +
		"shows a figure the database never computed. Save a run first — run an analytics " +
		"query with save, and cite the run id it answers with.",
	Instead: "Ask run_analytics_query for one number when a figure is what is wanted. This " +
		"composes a DOCUMENT of several, which is worth the round trip only when the answer " +
		"is a report somebody reads. describe_report_blocks holds the block kinds and their " +
		"fields for a caller that wants them before composing.",
	Retain: "Never put a number in a block — cite the cell that holds it. A block kind " +
		"outside the grammar is refused BY NAME with the whole set, so a first attempt costs " +
		"one refusal rather than a lookup.",
}
