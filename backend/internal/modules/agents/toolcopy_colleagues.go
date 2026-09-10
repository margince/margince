// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The distinction the surface was missing: a colleague is not a contact.
var listColleaguesCopy = toolCopy{
	Purpose: "List the people who work HERE — colleagues holding a seat, not the contacts stored " +
		"as person records.",
	Limits: "Reads only, and lists seats that can actually receive work — archived, suspended " +
		"and locked-out ones are absent. `truncated` means there are more. A `q` matching nobody " +
		"answers with `all_colleagues` and a warning; that list is ABSENT if it could not be read " +
		"and partial if `all_colleagues_truncated`, so read the warning before concluding a " +
		"person has no seat.",
	Instead: "search_records/person finds a CUSTOMER contact; this finds a colleague.",
	Retain:  "user_id is what assignee_id and owner_id take. Never assign to an is_agent seat.",
}
