// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for the record write vocabulary. See toolcopy.go for what each
// field answers.

var describeRecordFieldsCopy = toolCopy{
	Purpose: "Answer what a create_record or update_record `fields` body may SAY: for each " +
		"record_type, the fields that write accepts, which of them are REQUIRED, the shape each " +
		"takes, and the things a field list cannot show — where a deal's pipeline ids come from, " +
		"which endpoints a relationship kind needs, which types carry no custom fields. It is the " +
		"vocabulary the two write tools refuse against, so it holds the spelling of a field a " +
		"write got wrong.",
	Limits: "It describes the writes; it creates and changes nothing — create_record and " +
		"update_record do that. It is NOT a prerequisite: an unknown field is refused BY NAME " +
		"with that record_type's whole accepted list, so a first attempt costs one refusal rather " +
		"than a lookup. Create and update are separate sections because they disagree: a field " +
		"one accepts the other may not.",
	Instead: "Call create_record or update_record directly when the names are already known, and " +
		"read the refusal when one is wrong. This tool answers the same document as the " +
		"margince://schema/record-fields resource, for a caller that reads tools rather than " +
		"resources.",
	Retain: "Take the field names verbatim — a name outside the document is refused rather than " +
		"approximated — and mind the notation: a key with no `?` is REQUIRED. An " +
		"extra key must be spelled cf_<slug> or it is not a custom field at all.",
}
