// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for the evidence search over a saved run. See toolcopy.go for
// what each field answers.

var searchReportEvidenceCopy = toolCopy{
	Purpose: "Check a claim against the records behind a saved analytics run — or one cell of " +
		"it — by searching their text: records that carry the words are citations, records that " +
		"do not are counterexamples, and records with no text to judge are abstentions.",
	Limits: "It searches only the run's own records that this seat can read today, and every " +
		"figure counts those — never records hidden from this seat. It states a prevalence only " +
		"when coverage is complete_exact; otherwise prevalence is null and the notes say why.",
	Instead: "search_context sweeps the whole workspace by meaning; run_analytics_query " +
		"counts. This one answers how much of a counted set supports a claim.",
	Retain: "Cite records by id. Quote a share only from prevalence, never by dividing the lists.",
}
