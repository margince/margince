// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for the record surface: the generic CRUD verbs, the deal and
// lead lifecycle transitions, and the configuration read the deal writes
// depend on. See toolcopy.go for what each field answers.

var searchRecordsCopy = toolCopy{
	Purpose: "Find records by the words they carry, best match first: contacts, companies, deals, " +
		"leads and projects, and products, offer templates or activity when you name them.",
	Limits: "It needs words in q. It searches the timeline (message bodies, call notes, meetings) " +
		"only when you name activity. When nothing matches, near_matches may list up to five " +
		"records whose names are spelt close to q; they are suggestions, not matches.",
	Instead: "Use list_records to list records without words or by a condition, read_record when " +
		"you already hold the record's id, and run_report when the question is a count, a total " +
		"or a breakdown rather than a set of records.",
	Retain: "Keep each result's record_type and id together: every other tool identifies a record " +
		"by both, and an id alone does not say which type it belongs to. A result's `owner` " +
		"says who holds it. " + sayWhoseItIs,
}

var listRecordsCopy = toolCopy{
	Purpose: "List the records of one record_type that meet exact conditions: every deal in one " +
		"pipeline, the leads one rep owns, a contact's timeline.",
	Limits: "It narrows only by the filters listed for that record_type, and answers ONE page: " +
		"the set continues past it.",
	Instead: "Use search_records to find a record by what it is called, and run_report for a count " +
		"or a total.",
	Retain: "Pass next_cursor back to read the next page; a call without it re-reads the first. " +
		"A result's `owner` says who holds it. " + sayWhoseItIs,
}

var readViewCopy = toolCopy{
	Purpose: "Read what the record tools do not answer: a record's derived views (company_360, " +
		"contact_graph, deal_stakeholders, company_facts), the workspace's reference lists (stages, " +
		"lead_sources, teams, record_roles) and reporting objects (reporting_targets, forecast_calls). " +
		"Each view answers exactly what the matching REST route answers.",
	Limits: "Name the view, then the id it reads and any of its filters. A view refuses an argument " +
		"it does not take and names the ones it does.",
}

var changeRecordPartsCopy = toolCopy{
	Purpose: "Change what is kept under a record: an offer's line items, a company's facts and " +
		"profile fields, a project's companies and stakeholders. Prices are in minor units; read a " +
		"product for its unit_price_minor rather than inventing one.",
	Limits: "An offer's totals are computed from its lines and cannot be supplied, and a sent or " +
		"accepted offer keeps its lines. Pass if_version from your read so a change made since is " +
		"refused. Each action answers as its route does.",
	Instead: "Use update_record for a record's own fields, and create_record with record_type offer " +
		"to make an offer with its first lines.",
}

var readRecordCopy = toolCopy{
	Purpose: "Read one record's own stored fields — the values a reader would see on its detail " +
		"page — when you already know which record you mean.",
	Limits: "It returns that record and nothing around it: no timeline, no related contacts, no " +
		"deals on the company.",
	Instead: "Use catch_me_up_on when the goal is what has been happening on the record rather " +
		"than what it currently says.",
	Retain: "Keep the version from the result and pass it back as if_version on a later update, " +
		"so a write is refused rather than silently overwriting a change made in between. Its " +
		"`owner` says who holds it. " + sayWhoseItIs,
}

var createRecordCopy = toolCopy{
	Purpose: "Create a contact, company, deal, lead, project, activity or relationship that " +
		"does not exist yet.",
	Limits: "Creating a deal requires a pipeline_id and a stage_id, and " + pipelinesSource + " is " +
		"what yields them for a deal that does not exist yet. Only the fields the chosen record_type " +
		"actually stores are accepted, and a field belonging to a neighbouring type is refused " +
		"rather than dropped. A CONTACT created here is visible to everyone in the workspace, " +
		"the same as one a human types in." + pastDayNeedsATime,
	Instead: "Search first when the record might already exist — a second copy of a contact or " +
		"company is a problem that then needs resolve_duplicates to undo. A record that shares only " +
		"a name, with a different email, phone or employer, is not proof of a duplicate. When the " +
		"search finds a contact with the same name at the same employer, ask the human whether " +
		"it is the same contact before you link anything to it; two colleagues can share a name. " +
		"Giving a contact its employer by creating an employment relationship files " +
		"any same-name contact at that employer for review: the result carries it as " +
		"duplicate_candidates, and you report the pair as filed, unresolved until the " +
		"reviewer decides. Several contacts, companies or leads at once — a pasted list, " +
		"a CSV — go through preview_import, which checks " +
		"every row and writes nothing; creating them one by one skips that check.",
	Retain: "The new record's id comes back in the result; keep it for anything that links to it. " +
		"When the user says where they met the contact, pass that word as offer_tag: it only offers " +
		"the tag, and change_tags still needs their yes.",
}

var updateRecordCopy = toolCopy{
	Purpose: "Change stored field values on a record that already exists — a corrected title, an " +
		"amount, an expected close date.",
	Limits: "Only the fields you send change, and only the fields the record type stores (a " +
		"contact's email addresses are not among them). The whole patch applies in this call, " +
		"and the result names each change so a human can Undo it. It names the record by id; when a name matches " +
		"two records, a human picks. owner_id is NOT neutral — ownership decides visibility, so " +
		"reassigning moves the record onto someone else's book and can take it off the owner's. " +
		"An activity moved to another day keeps its time of day unless the user names one; " +
		`moving a date-only one, send occurred_precision "day" again, or it reads as a time.`,
	Instead: "Use progress_deal to move a deal between stages, and relink_activity " +
		"to change what an activity is about; neither is a field edit.",
	Retain: "Send if_version with the version you read. If the call comes back staged, keep its " +
		"approval id to retry the same change once a human has released it.",
}

// The Purpose names the records on purpose, and the ordering is the fix.
//
// Driven from claude.ai on 2026-08-25 the model logged a meeting with NO links
// and then relinked three times, staging three approvals — while in the same
// run create_task linked deal, contact and company in one call, four times
// over. The ids were all in hand: the meeting was created LAST, after the company,
// the contact and the deal. The difference was the copy. create_task's Purpose
// says what it is "on which records", so the links are part of what the caller
// is deciding; this one described a timeline and left linking to a subordinate
// clause, with the instruction itself down in the `links` field description —
// which is read after the call has already been shaped.
var logActivityCopy = toolCopy{
	Purpose: "Record something that happened — a call, a meeting, a note, a message — on the " +
		"records it was about: name every one of them in this call. A meeting is with a contact, " +
		"and also concerns their company and its deal.",
	Limits: "It writes history and can add a follow_up_task to a meeting, call or note in one " +
		"transaction; the task inherits source links and audience, and its assignee must be able to " +
		"read the source. A source-identity replay returns the original and adds no task. " +
		"No deal or field changes; nobody is notified. Unlinked, it is on no timeline; " +
		"relink_activity links it later (onto a project, after the user's yes)." +
		pastDayNeedsATime,
	Instead: "Use progress_deal when the event also moves a deal, so move and note are one " +
		"act; create_task for something still owed; attach_document for a file, whose text never " +
		"goes into a note.",
	Retain: "Keep the activity id — draft_outbound and send_outbound identify a conversation " +
		"by it.",
}

var createTaskCopy = toolCopy{
	Purpose: "Put a to-do on someone's list: what is owed, by whom, on which records.",
	Limits: "Creates the task only — no reminder, no deal move. A source activity supplies its " +
		"visible links when none are sent; without either, the task sits on no timeline.",
	Instead: "log_activity is for what already happened.",
}

var relinkActivityCopy = toolCopy{
	Purpose: "Fix what a recorded activity is about, when a captured mail or meeting landed on " +
		"the wrong record or on none.",
	Limits: "Changes only the association; content is untouched. By default the new link is " +
		"ADDED beside existing ones. Onto a project it waits for the user's yes; the answer says " +
		"whether you may relay it with decide_approvals.",
	Instead: "log_activity records an event not recorded yet; relink_activities moves a " +
		"picked set, such as a whole thread's activities.",
	Retain: "Set replace_existing_of_type to move rather than associate.",
}

var confirmMeetingReadyCopy = toolCopy{
	Purpose: "Say the user is ready for a meeting they attend, so their Worklist stops asking " +
		"them to prepare it.",
	Limits: "Only for a meeting the user attends, and only at the preparation revision " +
		"prep_for_meeting answered: a meeting that changed since answers a conflict. It holds " +
		"for the user alone, and a new start, agenda or guest list asks them again.",
	Instead: "prep_for_meeting reads the meeting and its current preparation revision; " +
		"undo_meeting_ready takes the confirmation back.",
	Retain: "Keep the revision you confirmed; the undo sends it.",
}

var undoMeetingReadyCopy = toolCopy{
	Purpose: "Take back the user's \"I'm ready\" for a meeting.",
	Limits: "Removes the confirmation only while it is the one given at the revision you send, " +
		"so a newer confirmation stays.",
	Instead: "confirm_meeting_ready records readiness.",
	Retain:  "The answer is the user's preparation after the undo.",
}

var relinkThreadCopy = toolCopy{
	Purpose: "Move one whole conversation (by thread_key) onto a record, in one transaction.",
	Limits: "Refused for an assistant, at every destination and however it is called: a thread " +
		"key cannot be confirmed because the conversation may grow before the retry. List the " +
		"thread's activities and call relink_activities with exactly those ids.",
	Instead: "relink_activities moves a named set; relink_activity moves one message.",
}

var relinkActivitiesCopy = toolCopy{
	Purpose: "Move up to 500 named activities onto one record, all or nothing.",
	Limits: "Each id must be visible and writable to you. A project destination, or any move the " +
		"installation's policy raises, is staged for confirmation and the retry moves exactly " +
		"these ids; once the user says yes, relay it with decide_approvals. A filing under a " +
		"project can be undone by a member from the activity.",
	Instead: "relink_activity moves one message.",
	Retain:  "The answer is the count moved.",
}

var bulkUpdateRecordsCopy = toolCopy{
	Purpose: "Hand up to 500 contacts, companies, deals or leads to one owner, archive them, add " +
		"them to or take them off a Shortlist, put a tag on or take it off, or file one task under " +
		"each, in one change; or mark up to 500 of the user's Worklist tasks done " +
		"(record_type worklist_item, verb complete) — or undo such a change. A Worklist " +
		"commitment is not yours to mark done: it is skipped as commitment_needs_the_user.",
	Limits: "Call mode preview first and show the user what it says: how many records change, " +
		"which are left alone and why, and the sample rows. Execute only after they agree. Each " +
		"record is changed only if it still has the version you sent and you may change it. To " +
		"undo, pass the batch_id to undo_preview, show the user the answer, and call undo after " +
		"they agree; a change is undone once, and records changed since are left alone.",
	Instead: "update_record and archive_record change one record.",
	Retain: "Above 10 records, execute and undo need the confirm_token their preview answered; " +
		"it is good once. Keep batch_id from the answer: undo names the change by it.",
}

var archiveRecordCopy = toolCopy{
	Purpose: "Retire a record that should no longer be worked — a duplicate, a dead company, a " +
		"project that ended.",
	Limits: "Archiving hides the record from day-to-day work; it does not delete it and does not " +
		"move anything attached to it, so an archived duplicate still holds the activities and " +
		"deals that were logged against it.",
	Instead: "Use resolve_duplicates when a duplicate's history should end up on the record that " +
		"survives, and disqualify_lead to close out a lead going nowhere — a lead's own transition " +
		"records the reason where archiving would not.",
	Retain: "By default the record is archived when this call answers; where an installation " +
		"has raised this verb to confirm first, the answer is a staged approval and you must not " +
		"report the record as archived " +
		"until the retry that carries their approval has answered.",
}

var mergeRecordsCopy = toolCopy{
	Purpose: "Collapse two records for the same real contact or company into one, moving the " +
		"source's activities, deals and links onto the record that survives.",
	Limits: "Contacts merge with contacts and companies with companies; the source is " +
		"archived and redirected to the target, and the direction is not reversible by calling " +
		"this again the other way round.",
	Instead: "Use archive_record when the extra record has nothing worth keeping, rather than " +
		"merging to make it disappear.",
	Retain: "target_id is the record that survives and source_id the one merged away — read both " +
		"records before choosing: the fold cannot be called back, and by default nothing holds it.",
}

var advanceDealCopy = toolCopy{
	Purpose: "Move a deal to a different stage of its pipeline.",
	Limits: "The stage is named by id from " + pipelinesSource + " — call it first; a deal you read " +
		"carries only its current stage. Moving onto or off a won/lost stage is a human's " +
		"decision: staged for approval, with a lost_reason_code from the loss reasons view, a lost_reason note, or both, for a losing stage. Read the target " +
		"stage's semantic rather than guessing from its name.",
	Instead: "Use progress_deal when the move should also leave a note explaining it, which is " +
		"almost always what a human means by moving a deal on.",
	Retain: "Send if_version with the version you read of the deal, and keep the staged approval " +
		"id when a closing move comes back for approval.",
}

var progressDealCopy = toolCopy{
	Purpose: "Move a deal to a different stage of its pipeline, optionally leaving a note on its " +
		"timeline saying why, in one call.",
	Limits: "The stage is named by id from " + pipelinesSource + " — call it first; a deal you read " +
		"carries only its current stage. Moving onto or off a won or lost stage is a human's " +
		"decision: staged for approval, with a lost_reason_code from the loss reasons view, a lost_reason note, or both, for a losing stage. Read the target " +
		"stage's semantic rather than guessing from its name. The move commits first and the " +
		"note follows it, so a note that fails to write does not put the deal back — the answer " +
		"says so, and the note is then log_activity's to retry.",
	Instead: "Use log_activity when something happened but the deal did not move.",
	Retain: "Send if_version with the version you read of the deal; keep the staged approval id " +
		"if a closing move is sent for approval.",
}

var qualifyLeadCopy = toolCopy{
	Purpose: "Fill in what a lead's own data already implies — today the company name, from the " +
		"domain of its email address — and report which qualification fields are still empty.",
	Limits: "It fills only a field that is currently EMPTY and derivable from the lead itself. It " +
		"never overwrites a value, never invents one, and reaches nothing outside the record, so " +
		"a lead with nothing to derive from comes back unchanged with its gaps named.",
	Instead: "Use enrich to learn about a company from its website, and promote_lead once a real " +
		"engagement means the lead should become a contact.",
	Retain: "The gaps in the result are what a human still has to supply; they are the honest " +
		"answer to \"is this lead ready\", not a failure of the call.",
}

var promoteLeadCopy = toolCopy{
	Purpose: "Turn a lead who has genuinely engaged into a contact record, carrying their history " +
		"across.",
	Limits: "It requires a trigger naming the engagement that justifies it — a reply, a booked " +
		"or held meeting, or a human's decision. Cold outreach that nobody answered is not a " +
		"promotion, and there is no trigger for it.",
	Instead: "Use qualify_lead when the lead is merely incomplete rather than ready, and " +
		"disqualify_lead when the engagement says the opposite.",
	Retain: "By default the lead is promoted when this call answers and the promoted contact's " +
		"id comes back with it. Where an installation has raised this verb to confirm first, the " +
		"answer is a staged approval instead and the id arrives only from the retry that " +
		"carries it.",
}

var disqualifyLeadCopy = toolCopy{
	Purpose: "Close out a lead that is not going anywhere, so it stops appearing as live work.",
	Limits: "It is the lead's own terminal state and keeps the record and its history; it is not " +
		"a deletion and not an archive.",
	Instead: "Use promote_lead when engagement says the opposite, and qualify_lead when the lead " +
		"is only missing information.",
	Retain: "By default the lead is disqualified when this call answers; where an installation " +
		"has raised this verb to confirm first, do not report the lead as disqualified " +
		"until the retry carrying their approval has answered.",
}

var demoteLeadCopy = toolCopy{
	Purpose: "Reverse a promotion that should not have happened, putting the lead back on the " +
		"open ladder.",
	Limits: "It blocks rather than orphans: a promotion whose contact now owns a deal is refused, " +
		"and activities captured since the promotion stay on the contact's timeline — they are real " +
		"history. A promotion that merged into an existing contact leaves that contact untouched and " +
		"only clears the lineage.",
	Instead: "Use disqualify_lead when the lead is real but going nowhere; demotion says the " +
		"promotion itself was wrong.",
	Retain: "The lead is demoted when this call answers. Where an installation has raised this " +
		"verb to confirm first, the answer is a staged approval instead — keep its id, and do not " +
		"report the demotion until the retry carrying it has answered.",
}

var advanceProjectPhaseCopy = toolCopy{
	Purpose: "Move a project to another phase — initiative, pursuing, delivering, closed.",
	Limits: "The four names are fixed but the order is not enforced: a project may go back a " +
		"phase, and a closed one may be reopened. Closing requires a reason, which is recorded on " +
		"the phase history either way. Closing does not complete, cancel or hide anything open " +
		"on the project: its tasks and commitments stay open on their own records. When the " +
		"user has asked for the close, report what is still open and close — an open " +
		"commitment is something to tell them, not a reason to hold back a close they asked for.",
	Instead: "Use progress_deal for a deal's pipeline stages; a project's phases are a different " +
		"vocabulary on a different record.",
	Retain: "Send if_version with the version you read. By default the phase moves when this " +
		"call answers. Where an installation has raised this verb to confirm first, the answer " +
		"is a staged approval and the phase has NOT moved — keep its id and do not report the " +
		"project as advanced until the retry that carries the approval has answered.",
}

var listPipelinesCopy = toolCopy{
	Purpose: "List every pipeline this workspace has with its live stages — the configuration " +
		"the deal-shaped writes are named against.",
	Limits: "It is where the id of a stage a deal could move TO comes from, so a deal cannot be " +
		"created, or moved anywhere new, without calling this first — a deal you have already " +
		"read carries only the stage it is in. Each stage carries a semantic — open, won or lost " +
		"— and that, not its name, is what decides whether moving onto it needs a human's " +
		"approval; a stage called \"Closed\" may be either.",
	Retain: "Keep the pipeline_id and the stage_id of the stage you mean: create_record for a " +
		"deal requires both, and progress_deal takes that stage_id as its " +
		"to_stage_id.",
}
