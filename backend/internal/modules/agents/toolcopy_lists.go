// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

var readListsCopy = toolCopy{
	Purpose: "Find the team's Live Lists (saved filters whose members join and leave on their own) " +
		"and Shortlists (records chosen by hand), read one, page through its members, say why a " +
		"record is or is not on it, read what changed on it — including which records a Live List " +
		"was seen to gain and lose, and how many since the user last opened it — or preview what a " +
		"filter would select before a Live List is saved.",
	Limits: "Every count, member and reason is what the user you act for may see: a list shared " +
		"with them never shows a member record they cannot read, so two users may see different " +
		"counts for one list. Live Lists are checked every 15 minutes, longest-unchecked first, so " +
		"with very many lists one can wait longer (last_check says when); a record that joined and " +
		"left between two checks is not recorded. A preview is logged as a read of those records. " +
		"A list with health retired_field still works but filters on a retired custom field, named " +
		"in retired_fields; its steward should replace that clause. A list with health retired_tag " +
		"filters on an archived or merged-away tag, named in retired_tags, and that tag no longer matches any " +
		"record, so its steward should name the live tag.",
	Instead: "search_records finds records by name; tags are applied with apply_tag, not lists.",
	Retain:  "Keep list_id, the version for a later change, and next_cursor to read the next page.",
}

var changeListsCopy = toolCopy{
	Purpose: "Make a Live List from a filter or a Shortlist of chosen records, change its name, " +
		"purpose, filter, sharing or steward, archive or restore it, and add or remove one " +
		"Shortlist member with a note on why.",
	Limits: "Only the steward of a list or a list admin may change it. A change must carry the " +
		"version you read; a list changed since is refused. An archived list is read-only. A Live " +
		"List's members follow its filter and cannot be added or removed by hand. Preview a filter " +
		"with read_lists before saving it.",
	Instead: "bulk_update_records adds or removes many records at once, with a confirmation.",
	Retain:  "Keep the list id and its new version from the answer.",
}
