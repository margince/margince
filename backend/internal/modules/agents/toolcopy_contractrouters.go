// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

var changeAttentionCopy = toolCopy{
	Purpose: "Act on what Margince puts in front of the user: accept, dismiss or restore a Deal Scout, " +
		"company or tag suggestion, mark notices read, pin or unpin Worklist rows, mark them done or " +
		"reopen them, snooze or dismiss a brief item, and write or update the weekly plan.",
	Limits: "Each action changes what the user sees next; only accepting a suggestion changes the record " +
		"it is about, because it runs what the suggestion proposes, as the app's Accept does.",
	Instead: "Use update_record or change_record_parts to change a record yourself, and " +
		"change_activities to log or plan work.",
}

var changeAccessCopy = toolCopy{
	Purpose: "Decide who works on what: share a record with a colleague or team and take the share back, " +
		"make a colleague responsible for a record, offer, request or claim ownership, and set up the " +
		"workspace — change a member's role, deactivate or remove a member, create " +
		"teams and add or remove their members.",
	Limits: "An agent never grants or removes the admin role, never changes its own user's role or " +
		"teams, never acts on a member who is an admin, never hands a colleague's work to its own user and " +
		"never changes a member's email; those are " +
		"refused. Role definitions are changed in the app.",
	Instead: "Use send_outbound invite_member to invite a member, which mails them a sign-in " +
		"link, update_record to change a record's owner field directly, and read_workspace topic " +
		"colleagues to find a member's id.",
}

var changeOffersCopy = toolCopy{
	Purpose: "Work the commercial paper around a deal: send, accept, reject, render or regenerate an " +
		"offer; record, change, renew, cancel or archive a contract; and run a deal room's documents, " +
		"access and expiry.",
	Limits: "Sending an offer fixes its version and its exchange rate from then on. Inviting a buyer " +
		"into a deal room sends mail, so it is send_outbound's deal_room_invite.",
	Instead: "Use change_record_parts to change an offer's line items, and change_state to move the " +
		"deal itself.",
}

var changeRelationshipsCopy = toolCopy{
	Purpose: "Work the relationships around a company: link its candidate contacts; check " +
		"its VAT number; ask for, decide, complete or cancel an introduction and record one that happened; " +
		"record, settle or take over a promise made in a conversation; save what research found.",
	Limits: "Each act runs as the app does it, on the records the user may open.",
	Instead: "Use run_research to ask about a company, refresh a brief, dossier, growth fit or scan, or research a contact, " +
		"read_details to read one that is already there, and " +
		"update_record to change a company's or contact's fields.",
}

var changeSettingsCopy = toolCopy{
	Purpose: "Change how the workspace is set up: lead sources, origins and disqualify reasons, loss " +
		"reasons and acquisition sources, lead, legal review, duplicate and follow-up settings, the " +
		"review template, the booking page, the workspace's own company and its legal " +
		"entities, and a partner company's terms.",
	Limits: "A setting changes what every colleague sees from then on; read the current value first, " +
		"through read_view.",
	Instead: "Use change_state to move one lead or deal, and update_record to change one record.",
}

var runResearchCopy = toolCopy{
	Purpose: "Run AI work for the user: ask about a company, refresh a company's brief, dossier, growth " +
		"fit or scan or a contact's brief, research a contact, or make today's morning brief.",
	Limits: "Each call spends AI budget, and research asks an outside provider, so the passport needs " +
		"the enrich scope. Read the panel first through read_details; run this only when it is out of date.",
	Instead: "Use read_details to read a panel, and enrich to fill a company's fields from its website.",
}

var readDetailsCopy = toolCopy{
	Purpose: "Read what the app shows beside a record or on the user's day: a company's dossier, " +
		"growth fit, scan or brief, a contact's brief and profile fields, a deal's products, win " +
		"evidence or legal reviews, a contract, a mail draft, scheduled sends and meeting proposals, " +
		"the Worklist's handled and hidden rows, the team board, weekly plans and reviews, notices, " +
		"deal and tag suggestions, introduction requests and record history.",
	Limits: "Each action reads one panel as the app draws it, for the records the user may open. Without the " +
		"enrich scope, a brief, dossier or growth fit not written yet answers its plain form from the records, " +
		"marked deterministic; run_research writes one.",
	Instead: "Use read_context for the story so far on a record, read_attention for what needs the " +
		"user now, and read_record for a record's own fields.",
}
