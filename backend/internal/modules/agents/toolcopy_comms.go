// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for the tools that compose a message, put one on the wire, take
// time in someone's day, or read something outside the workspace. See
// toolcopy.go for what each field answers.

var draftEmailCopy = toolCopy{
	Purpose: "Compose an email: a reply to a recorded thread (activity_id), or a FIRST message " +
		"to a record (links).",
	Limits: "It writes the message and stops: nothing is sent. With no drafting model configured " +
		"the text is a short deterministic note rather than a composed one.",
	Instead: "draft_follow_ups_for drafts across a set of slipping deals at once; send_email " +
		"sends a reply, send_company_email a first message.",
	Retain: "Keep what comes back — subject, body, and the activity_id or links echoed with it; " +
		"the send takes them. Re-writing the text in between means a human approves one " +
		"message and another goes out.",
}

var draftFollowUpsForCopy = toolCopy{
	Purpose: "Draft a follow-up for each deal in a segment at once — today only the slipping " +
		"deals — and leave each draft on its own deal's timeline.",
	Limits: "It writes drafts and sends none of them, and it drafts only for deals whose risk is " +
		"evidenced, so it covers the same set whats_slipping_this_week reports. One call writes " +
		"to many records, up to a server-side ceiling of 25.",
	Instead: "Use draft_email for one specific conversation; this tool answers \"chase everything " +
		"that is slipping\", not \"reply to this\".",
	Retain: "Each draft comes back with its deal_id and draft_activity_id — those are how a " +
		"human finds the drafts to review.",
}

var sendEmailCopy = toolCopy{
	Purpose: "Put a mail on the wire to a real recipient, from this workspace, and record it on " +
		"the thread it belongs to.",
	Limits: "It sends EXACTLY the subject and body it is given and composes nothing, so it is " +
		"not the tool to reach for when the message does not exist yet. Every recipient must " +
		"have granted the consent purpose the call names. A message leaving the workspace cannot " +
		"be recalled, and by default nothing holds it: where an installation has raised this verb " +
		"to confirm first, the answer is a staged approval instead of a send.",
	Instead: "Use draft_email first to produce the message and let it be read, and send_message " +
		"when the conversation is on a chat channel rather than mail.",
	Retain: "Send the same activity_id, subject and body the draft produced, and keep the staged " +
		"approval id: the approval is bound to that exact message, so changed text needs a new " +
		"approval.",
}

var sendCompanyEmailCopy = toolCopy{
	Purpose: "Put a mail on the wire to a real recipient, from this workspace, starting a new " +
		"conversation rather than answering one, and file it on the records it is about.",
	Limits: "Sends EXACTLY the subject and body given; composes nothing. Needs at least one link " +
		"naming the records it belongs to. Every recipient must have granted the named consent " +
		"purpose. A sent mail cannot be recalled, and by default nothing holds it: where an " +
		"installation has raised this verb to confirm first, the answer is a staged approval " +
		"instead of a send.",
	Instead: "Use send_email to answer a conversation already recorded here; this starts a " +
		"separate thread beside it.",
	Retain: "Keep the staged approval id and re-send the identical text and links: the approval " +
		"is bound to that exact message. The activity_id that comes back is the new conversation.",
}

var sendMessageCopy = toolCopy{
	Purpose: "Reply on a captured chat conversation — the channels this workspace has connected " +
		"— on the thread it was captured from.",
	Limits: "It replies to an existing conversation named by activity_id; it cannot start one, " +
		"and it cannot choose a channel. The recipient must have granted the consent purpose the " +
		"call names. By default the message leaves when this call answers; where an installation " +
		"has raised this verb to confirm first, the answer is a staged approval instead.",
	Instead: "Use send_email when the thread is a mail thread, and log_activity when the point is " +
		"to record that something was said rather than to say it.",
	Retain: "Keep the activity_id of the conversation and the staged approval id; the approval " +
		"binds the exact text, so changed text needs a new approval.",
}

var checkAvailabilityCopy = toolCopy{
	Purpose: "Find candidate times for a host without booking or sending anything.",
	Limits:  "Set reliable=true to check the acting host's selected Google or Microsoft calendars and pending reservations. Otherwise only meetings recorded in this CRM are considered. calendar_backing=calendar is the evidence that live occupancy was checked. This never checks invitees' availability.",
	Instead: "Use invite_meeting to request an approved calendar invitation after a time is agreed. book_meeting only records a meeting. prep_for_meeting prepares for an existing meeting.",
	Retain:  "Keep the exact start and end, the calendar_backing value, and the truncated flag. Availability is checked again when reserving the time.",
}

var bookMeetingCopy = toolCopy{
	Purpose: "Record a meeting against linked CRM records without sending an invitation.",
	Limits:  "Reserves the recorded interval locally. It does not create a calendar event or notify attendees. Needs at least one record link.",
	Instead: "Use invite_meeting for an explicit provider-backed invitation, or log_activity for a past meeting.",
	Retain:  "Keep the recorded activity id. Do not describe a record-only booking as an invitation sent.",
}

var enrichCopy = toolCopy{
	Purpose: "Learn about a company by reading its public website, and propose what was " +
		"found for a human to accept onto the record.",
	Limits: "It reaches OUTSIDE the workspace, and what it returns is a PROPOSAL — nothing " +
		"lands on the record until someone accepts it, which is the review that guards this, not " +
		"an approval on the call. " +
		"Reading one page answers immediately; reading a whole site is queued and answers with a " +
		"read id rather than the content. What it finds is captured text from a third party, not " +
		"a fact this workspace has verified.",
	Instead: "Use qualify_lead when the missing values are already derivable from the record " +
		"itself, which costs no external read and needs no approval.",
	Retain: "Keep the company_id you enriched, and the read id when a whole-site read was " +
		"queued — the result is collected against it later.",
}
