// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

var findRecordsCopy = toolCopy{
	Purpose: "Find records, choosing the engine by the question. search ranks contacts, companies, " +
		"deals, leads and projects by how well their text matches your words, and products, offer " +
		"templates or activity when named. list pages one record type by " +
		"its filters and sort, newest first, and reaches the timeline (activity), offers, " +
		"products and deal rooms. query runs a typed plan of fields, operators and one join, for " +
		"counts, sums and the records near a place. meaning finds passages whose meaning matches, " +
		"in notes, emails and transcripts. identify says which existing contact or company a " +
		"name, email or domain refers to.",
	Limits: "search needs words and reads a timeline only when activity is named; meaning reads " +
		"it and answers passages, not records. " +
		"list needs record_type, and read_workspace topic list_filters names the filters each " +
		"type takes. query names only fields read_workspace topic query lists. identify " +
		"answers unresolved for a record you may not see, as for one that does not exist.",
	Instead: "Use read_record for one record you already have the id of, and read_context to " +
		"learn what has been happening on it.",
	Retain: "Keep each record's id and version, and next_cursor to read on.",
}

var readContextCopy = toolCopy{
	Purpose: "Get the story so far. catch_up tells what has been happening on one contact, " +
		"company, deal or project. meeting prepares for a meeting: who is coming, the open " +
		"threads and what was promised. brief reads this morning's brief for the seat.",
	Limits: "It reads and writes nothing. catch_up summarises the timeline; a field's current " +
		"value is read_record's. brief answers the brief already made, never a fresh one.",
	Instead: "Use read_attention for what is at risk across the whole pipeline.",
	Retain:  "Keep the ids the answer cites to read or change those records next.",
}

var readAttentionCopy = toolCopy{
	Purpose: "See what needs attention. slipping lists deals at risk this week and why. at_risk " +
		"lists the open deals that rest on a single contact or whose contacts have gone quiet. " +
		"commitments lists promises made to customers that are still open.",
	Limits:  "Each view covers only what you may see, and says why each item is on it.",
	Instead: "Use read_context to dig into one item.",
	Retain:  "Keep each item's record id to act on it.",
}

var readReportsCopy = toolCopy{
	Purpose: "Read sales reports. standard reads the standard metrics, saved reports and their " +
		"frozen editions, and the open partner commission liability. run runs one of the named " +
		"reports, grouped and filtered. evidence searches the passages behind a saved run.",
	Limits: "Read only. Every figure names its coverage and when it was captured; quote both. " +
		"read_workspace topic report_vocabulary names what each run report takes.",
	Instead: "For a written section a reader keeps, such as a board pack, use read_analytics: " +
		"query with save, then compose. Use read_forecast for what a period is expected to close.",
	Retain: "Keep a run's id to search its evidence, and quote coverage with each number.",
}

var readAnalyticsCopy = toolCopy{
	Purpose: "Write a report document a reader keeps, such as a board-pack section, or ask the " +
		"data a question. For a document: query with save, then compose the saved results into " +
		"it. query alone groups and measures a population.",
	Limits: "read_workspace topic analytics names the populations, dimensions and measures you " +
		"may use. A figure in a composed document is cited from a saved query, never typed in.",
	Instead: "Use read_reports for the standard metrics and named reports.",
	Retain:  "Keep a saved query's id; compose cites it.",
}

var changeActivitiesCopy = toolCopy{
	Purpose: "Log, plan and file activities. log records an email, call, meeting, note or " +
		"message that already happened, on its records. task creates a task for a seat. attach " +
		"attaches a file to a record. relink moves one activity to another record; relink_many " +
		"moves a named set.",
	Limits: "relink and relink_many wait for a human when they file under a project, and run at " +
		"once anywhere else. attach checks the record first; read list_documents before " +
		"attaching the same file again.",
	Instead: "Use schedule_meeting to book a meeting still to come, and change_state with " +
		"move_deal when the same event moves a deal, so move and note are one change.",
	Retain: "Keep the activity id; a relink needs it. A staged answer's approval id is what " +
		"the retry carries.",
}

var scheduleMeetingCopy = toolCopy{
	Purpose: "Put a meeting on the calendar. book places it on the calendar of the seat this " +
		"passport acts for. invite sends the invitation to its attendees.",
	Limits: "book runs at once; invite reaches attendees outside the workspace and waits for a " +
		"human. Read check_availability before booking a time.",
	Instead: "Use change_activities with log to record a meeting that already happened.",
	Retain:  "Keep the meeting's id to send its invitation.",
}

var changeStateCopy = toolCopy{
	Purpose: "merge_member_accounts consolidates an unused member into a different surviving colleague after a confirmed retirement preview. Move a lead, deal or project on, or set up a company as a partner. qualify fills what a lead's own data already " +
		"implies, today the company name from its email domain, and names the fields still " +
		"empty. promote turns a lead who has genuinely engaged into a contact. disqualify " +
		"closes out a lead going nowhere. demote reverses a promotion. move_deal moves a deal " +
		"to a stage, with a note. move_project moves a project to a phase; closed closes it. set_up_partner starts a " +
		"partner program on a company, with company_id, partner_role and optionally " +
		"relationship_stage, next_step and next_step_due_at.",
	Limits: "promote needs a trigger naming the engagement; cold outreach nobody answered " +
		"never promotes. demote needs a reason and is refused once the contact owns a deal. " +
		"set_up_partner always waits for a human, takes no margin tier (a human sets that on the " +
		"company's Partner tab) and is refused for a company that already is a partner. " +
		"move_deal to a won or lost stage waits for a human; to an open stage it runs at once. " +
		"move_project to closed needs a reason and leaves the project's tasks open. When the user " +
		"asked for the close, report what is still open and close: an open task is news for " +
		"them, not a reason to hold back. " +
		"read_workspace topic pipelines gives the stage ids.",
	Instead: "Use update_record for a record's other fields, and enrich to learn about a " +
		"company from its website.",
	Retain: "A promotion answers with the new contact's id. A staged answer's approval id is " +
		"what the retry carries; do not report the change until the retry has answered.",
}

var readWorkspaceCopy = toolCopy{
	Purpose: "member_retirement previews a member removal, or a merge when mode is merge and to_user_id names the survivor. Read how this workspace is set up. colleagues: the seats that can receive work, " +
		"not the contacts stored as records; q narrows by name. pipelines: every pipeline with " +
		"its stages, each open, won or lost. channel_providers: the messaging transports. The " +
		"other topics name what a tool accepts: query (find_records engine query), analytics " +
		"(read_analytics), record_fields (create_record and update_record), list_filters " +
		"(find_records engine list), report_vocabulary (read_reports kind run), report_blocks " +
		"(read_analytics step compose).",
	Limits: "A q matching nobody answers with every colleague and a warning. A stage's semantic, " +
		"not its name, says whether it closes a deal. None of the name topics is a " +
		"prerequisite: each tool refuses an unknown name with the accepted list.",
	Instead: "Use find_records to find a customer contact; colleagues are seats here.",
	Retain: "user_id is what assignee_id and owner_id take; never assign to an is_agent seat. " +
		"Keep pipeline_id and stage_id for a deal. Take every name verbatim; a custom field is " +
		"spelled cf_<slug>.",
}
