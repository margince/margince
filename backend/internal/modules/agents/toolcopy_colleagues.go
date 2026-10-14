// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The distinction the surface was missing: a colleague is not a contact.
var listColleaguesCopy = toolCopy{
	Purpose: "List who works HERE — colleagues holding a seat, not the contacts stored " +
		"as contact records.",
	Limits: "Reads only, and lists seats that can actually receive work — archived, suspended " +
		"and locked-out ones are absent. `truncated` means there are more. A `q` matching nobody " +
		"answers with `all_colleagues` and a warning; that list is ABSENT if it could not be read " +
		"and partial if `all_colleagues_truncated`, so read the warning before concluding a " +
		"colleague has no seat.",
	Instead: "search_records/contact finds a CUSTOMER contact; this finds a colleague.",
	Retain:  "user_id is what assignee_id and owner_id take. Never assign to an is_agent seat.",
}

var readDirectoryCopy = toolCopy{
	Purpose: "Read how this workspace is set up. colleagues: who works HERE, the seats that can " +
		"receive work, not the contacts stored as records; `q` narrows by name. pipelines: every " +
		"pipeline with its live stages, each open, won or lost. channel_providers: the messaging " +
		"transports this installation composed.",
	Limits: "A `q` matching nobody answers with `all_colleagues` and a warning, which decides " +
		"whether someone truly has no seat. A deal cannot be created, or moved anywhere new, " +
		"without pipelines: a deal you read carries only its current stage, and a stage's " +
		"semantic, not its name, says whether it closes the deal. supplies_transport=false means " +
		"a reply on that transport will be refused.",
	Instead: "search_records finds a CUSTOMER contact; this finds a colleague.",
	Retain: "user_id is what assignee_id and owner_id take; never assign to an is_agent seat. " +
		"Keep the pipeline_id and stage_id: create_record for a deal needs both, and progress_deal " +
		"takes the stage_id as to_stage_id. Carry `provider` verbatim: log_activity takes it as " +
		"channel_provider when kind is \"message\".",
}
