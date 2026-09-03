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

// The vocabulary verbs. Every one of them is refused without the grant the
// seeded roles give Admin and Ops alone, so the copy says what the tool does
// rather than warning about an authority the store already enforces.
var createTagCopy = toolCopy{
	Purpose: "Coin a new word in the workspace vocabulary, so records can be grouped by it.",
	Limits: "list_tags FIRST: a workspace with \"Key Account\" does not want \"key accounts\" " +
		"beside it, and the two then split the records that belong together. A name already " +
		"taken is a conflict, matched case-insensitively — including a RETIRED word holding " +
		"it, which a person restores in Settings; no tool does. Needs the tag.create grant, " +
		"which an ordinary seat does not hold.",
}

// merge_tags is the one tag verb a human releases. The copy says so plainly
// and says what cannot be walked back, because a model that reads "merge" as
// the reversible record merge will propose one casually.
var mergeTagsCopy = toolCopy{
	Purpose: "Fold a duplicate word into the one the workspace keeps, moving every record that " +
		"carries it.",
	Limits: "A HUMAN APPROVES THIS, and it is NOT UNDOABLE: the source is retired, its name is " +
		"released, and no pointer home is kept — unlike a person or company merge. The " +
		"TARGET is the word that survives. Needs the tag.update grant.",
}

var updateTagCopy = toolCopy{
	Purpose: "Rename, recolour or describe a word that already exists. Fields left out are " +
		"unchanged, so a recolour need not restate the name.",
	Limits: "The word keeps every record carrying it — this changes what it is CALLED, not what " +
		"it is on. LAST WRITE WINS: this tool sends no version, so an edit made between your " +
		"read and your write is overwritten without a conflict. Read with get_tag immediately " +
		"before editing. A name another word already holds is a conflict.",
}
