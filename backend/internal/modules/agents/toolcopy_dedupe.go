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

var resolveDuplicatesCopy = toolCopy{
	Purpose: "Settle two records that may be one. With candidate_id and decision: a pair " +
		"create_record flagged in duplicate_candidates — not_the_same says they are two, so the " +
		"pair is not flagged again, and reopen takes that back; neither record changes. With " +
		"record_type, source_id and target_id: merge two contacts or two companies into one, " +
		"moving the source's activities, deals and links onto the survivor. With record_type contact, " +
		"contact_id, member_ids, survivor_id, owner_id and revision: merge a contact case read through " +
		"read_view in one step; preview true answers who keeps access and writes nothing.",
	Limits: "A merge archives the source and redirects it to the target; it cannot be reversed by " +
		"merging the other way round, and by default nothing holds it. A merged pair cannot be " +
		"reopened.",
	Instead: "Use archive_record when the extra record has nothing worth keeping.",
	Retain: "target_id is the record that survives and source_id the one merged away — read both " +
		"records before choosing.",
}
