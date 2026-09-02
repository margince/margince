// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The tag verbs. Short on purpose: four tools ride every listing, and the tag
// vocabulary is the simplest thing on this surface.
var listTagsCopy = toolCopy{
	Purpose: "The workspace's words for grouping records, with the tag_id apply_tag takes.",
	Limits: "Archived words come only on request and cannot be applied. `truncated` means the " +
		"list was cut, so a word missing from it may still exist.",
}

var getTagCopy = toolCopy{
	Purpose: "Read one tag and how many people, companies and deals carry it.",
	Limits: "The counts cover those three record types only. They say how much retiring or " +
		"merging the word would touch; the records themselves come from list_records.",
}

var getRecordTagsCopy = toolCopy{
	Purpose: "Read the tags on one person, company or deal, with who applied each and when.",
	Limits: "Those three record types only. `withheld` true means the vocabulary is not visible " +
		"to this caller, so the list is empty for that reason — NOT because the record carries no " +
		"tags, and it must not be reported as none. An archived tag stays on whatever carries it.",
}

var applyTagCopy = toolCopy{
	Purpose: "Tag a person, company, deal, lead or project by tag_id, or by tag_name, which must " +
		"name a tag the workspace already has.",
	Limits: "This tool never creates a tag: an unknown name is refused, and only an admin or ops " +
		"seat can add a word to the vocabulary. A name matches case-insensitively; an archived " +
		"word is refused as archived rather than as unknown. Prefer a tag_id from list_tags. " +
		"The same tag twice is a conflict.",
}

var removeTagCopy = toolCopy{
	Purpose: "Take one tag off one record — by tag_id or tag_name — leaving the word itself.",
	Limits:  "Removing one that is not there succeeds. archive_record on a tag retires it for all.",
}
