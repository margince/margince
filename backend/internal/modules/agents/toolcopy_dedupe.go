// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

var decideDuplicateCopy = toolCopy{
	Purpose: "Settle a pair the review queue flagged: not_the_same says they are two different " +
		"contacts or companies, so the pair is not flagged again; reopen takes that back.",
	Limits: "Only for a pair create_record reported in duplicate_candidates, by its candidate_id; " +
		"neither record changes, and a merged pair cannot be re-opened.",
	Instead: "Use merge_records when they are the same one, never this.",
}
