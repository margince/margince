// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for the forecast reads. See toolcopy.go for what each field
// answers.
//
// These five were the last tools on the surface carrying a hand-rolled
// Description literal instead of a toolCopy, and the cost was not tidiness.
// The struct has an Instead field; a literal has nowhere to put one. So the
// forecast family named no neighbour and no neighbour named it — a complete
// isolate in a catalogue of 75 tools, and every measured wrong-tool choice in
// this area had one leg in it:
//
//   - asked for a board-pack section, a model took figures from
//     forecast_readings and typed them into prose, which is the one thing
//     compose_analytics_report refuses to let a document do;
//   - asked whether the numbers could be trusted, a model read
//     forecast_input_checks and never data_coverage, so it reported a clean
//     set of findings over sources nobody had opened.
//
// Both are the same defect: three tools answer "can I trust this number?" and
// none of them said which question it is holding. The boundaries below are the
// point of the conversion.

var forecastReadingsCopy = toolCopy{
	// EVERY FIGURE IS NAMED rather than counted. "Four readings" is the
	// contract's own phrasing and the payload carries five money fields, so a
	// caller told a number has to guess which one is not a reading — and
	// `weighted` is the one that goes unreported when it guesses.
	Purpose: "Answer what a period is expected to close — `won`, `evidence`, `best_case` and " +
		"`open`, plus `weighted` — under the installation's own fiscal calendar and base " +
		"currency.",
	Limits: "`won` counts deals by the day they ACTUALLY closed, not the day they were " +
		"expected to. `evidence` is committed pipeline whose close date somebody confirmed; a " +
		"provisional date stays in `open` and out of `evidence`. `coverage_note` says what the " +
		"totals do not cover and is absent only when they cover every eligible deal, so " +
		"quoting a total without it reports a partial pipeline as a complete one.",
	Instead: "run_report's forecast report and a hand-summed query_workspace also produce a " +
		"number, and NEITHER is the forecast: only this applies the fiscal calendar, the base " +
		"currency conversion and the weighting. These figures also cannot be cited in a " +
		"composed document — for a board-pack section or anything a reader keeps, " +
		"run_analytics_query with save and compose_analytics_report from the run id. Ask " +
		"forecast_input_checks whether the inputs behind these numbers were read.",
	Retain: "Quote `as_of`, `timezone` and `base_currency` with the number — a total placed " +
		"in the reader's own zone is a different total — and `eligible_count`, `priced_count` " +
		"and `fx_missing_count` are the counts `coverage_note` is written from.",
}

var forecastMovementCopy = toolCopy{
	Purpose: "Explain why a forecast changed between two points, as named causes that account " +
		"for the whole difference.",
	Limits: "Opening plus every bucket equals closing, exactly, so the buckets are a complete " +
		"account of the change and not a selection from it. A deal appears in exactly ONE " +
		"bucket: one that both slipped and was repriced has moved for one reason as far as a " +
		"reader is concerned, which is that it left. Two buckets are about the machinery " +
		"rather than the business, and quoting them as sales movement is the mistake this " +
		"classification exists to prevent: `definition` means the two snapshots were computed " +
		"under different rules, and then the WHOLE difference is in that bucket; `model` means " +
		"a probability the product re-scored.",
	Instead: "IT NEEDS TWO SNAPSHOT IDS, and nothing on this surface hands one out — no tool " +
		"lists snapshots and no resource publishes them, so a caller that has not been given " +
		"ids from elsewhere cannot call this. Reading forecast_readings twice and subtracting " +
		"is NOT the same answer and must not be reported as one: the difference between two " +
		"reads is a number with no account of where it went.",
	Retain: "`reopened_or_archived` carries a deal that left the population entirely — " +
		"archived, or no longer visible to this caller — with its whole prior contribution, so " +
		"no money disappears without a row that says where it went.",
}

var forecastInputChecksCopy = toolCopy{
	Purpose: "Answer whether the forecast's inputs are sound enough to quote — a verdict, and " +
		"how much of the pipeline last night's check reached.",
	Limits: "A forecast is only as good as its inputs, and the failures are mundane: a close " +
		"date that went by, an amount that disagrees with the offer that was sent, a deal " +
		"nobody has heard from in ninety days. `checks_incomplete` is NOT a worse " +
		"`needs_review` — one says the pipeline has problems, the other says we could not " +
		"look, and reporting the first when the second is true tells somebody their pipeline " +
		"is sound when nobody read the mailbox.",
	Instead: "This is the VERDICT. list_input_checks is the findings themselves, one row per " +
		"problem, when the question is what to go and fix. data_coverage is the third " +
		"question and the one this cannot answer: whether the CONNECTORS were readable at " +
		"all, which is what makes a clean verdict trustworthy rather than merely clean.",
	Retain: "Read `readiness` before quoting any forecast figure. `sources` says why: each " +
		"carries the state the run reached, and only a `checked` source has a date — an absent " +
		"or unread source means the run could not confirm anything from it, which is different " +
		"from finding nothing there. `eligible_deals` is how much there was to check.",
}

var listInputChecksCopy = toolCopy{
	Purpose: "List the open input problems behind the forecast, most material first, so they " +
		"can be fixed.",
	Limits: "A close date that went by, or an amount that disagrees with the offer that was " +
		"sent, makes a total wrong without making the arithmetic wrong. Scoped to what this " +
		"caller can open, with no count of what was withheld — a count of what somebody may " +
		"not read is itself a statement about how much there is.",
	Instead: "forecast_input_checks answers the VERDICT — whether the numbers are quotable at " +
		"all — which is the question before this one and the cheaper call. data_coverage " +
		"answers whether the sources were readable, which an empty list here cannot " +
		"distinguish from a clean pipeline.",
	Retain: "`affected_minor` absent means the money at stake cannot be said, not that " +
		"nothing is at stake.",
}

var dataCoverageCopy = toolCopy{
	Purpose: "Answer how much of what is going on this workspace can actually SEE — which " +
		"connectors the nightly check could read, and how far back each reaches.",
	Limits: "Needs the data_coverage grant, which operators hold and sellers do not — a " +
		"refusal here is a seat boundary, not a missing run. Only a `checked` source carries a " +
		"date; on any other state nothing was read, and a quiet week is indistinguishable from " +
		"a broken connector until somebody looks.",
	Instead: "forecast_input_checks and list_input_checks answer what the check FOUND, and " +
		"both are silent on whether it could look — a clean set of findings over sources " +
		"nobody opened reads as good news and is not. Ask this one when the question is " +
		"whether to trust the other two.",
}
