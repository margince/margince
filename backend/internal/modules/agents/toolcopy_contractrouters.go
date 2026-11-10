// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

var changeAttentionCopy = toolCopy{
	Purpose: "Act on what Margince puts in front of the user: accept, dismiss or restore a Deal Scout, " +
		"company or tag suggestion, mark notices read, pin or unpin Worklist rows, mark them done or " +
		"reopen them, snooze or dismiss a brief item, write or update the weekly plan, record, triage, resolve or " +
		"archive a signal, and mark a list, company or contact as seen, which spends the user's own " +
		"since-last-visit, so ask first.",
	Limits: "Each action changes what the user sees next; only accepting a suggestion changes the record " +
		"it is about, because it runs what the suggestion proposes, as the app's Accept does.",
	Instead: "Use update_record or change_record_parts to change a record yourself, and " +
		"change_activities to log or plan work.",
}

var changeAccessCopy = toolCopy{
	Purpose: "Decide who works on what: share a record with a colleague or team and take the share back, " +
		"make a colleague responsible for a record, offer, request or claim ownership, and set up the " +
		"workspace — change a member's role, deactivate or remove a member, create " +
		"teams and add or remove their members, and record a colleague who already left.",
	Limits: "An agent never grants or removes the admin role, never changes its own user's role or " +
		"teams, never acts on a member who is an admin, never hands a colleague's work to its own user and " +
		"never changes a member's email; those are " +
		"refused. Role definitions and declaring or retiring a service member are done in the app.",
	Instead: "Use send_outbound to invite a member or send their sign-in link again, update_record to change a record's owner field directly, and read_workspace topic " +
		"colleagues to find a member's id.",
}

var changeOffersCopy = toolCopy{
	Purpose: "Work the commercial paper around a deal: send, accept, reject or render an " +
		"offer; record, change, renew, cancel or archive a contract; and run a deal room's documents, " +
		"access and expiry.",
	Limits: "Sending an offer fixes its version and its exchange rate from then on. What a buyer reads " +
		"in a deal room reaches them at once, so inviting them or resending their invitation, writing to " +
		"them, adding, changing or removing a document, editing the room's words, setting its expiry and resuming it are send_outbound's.",
	Instead: "Use change_record_parts to change an offer's line items, change_state to move the " +
		"deal itself, and run_research to regenerate an offer.",
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
	Purpose: "Change how the workspace is set up: pipelines, stages, exit criteria and partner stages; custom " +
		"fields; automations and web forms; FX, tax and AI price sheets; product translations; lead sources, " +
		"origins, disqualify and loss reasons and acquisition sources; lead, legal review, duplicate and " +
		"follow-up settings; the review template and booking page; the workspace's own company, logos and " +
		"legal entities; a partner company's terms; the accounting and contact data sources; background " +
		"corrections; and the user's saved views.",
	Limits: "A workspace setting changes what every colleague sees from then on, while a saved view is the user's " +
		"alone. A request about the user's own preferences never reaches this tool. Read the current value first. Ask the user first where the action is " +
		"marked so: a custom field changes the database for everyone and retiring one cannot be undone, a tax rate " +
		"can never be removed, a rate reprices what colleagues see, and an enabled automation acts on its own. " +
		"Which model runs, the plan's budget, stage-move trust, what closes a deal, a stage's criteria and order while a " +
		"transition is trusted, a seeded automation, a rule's owner, a form's secret, origins and consent, retention and " +
		"permanent deletes are done in the app.",
	Instead: "Use change_my_settings for the user's own name, language, time zone, hours or voice, change_state " +
		"to move one lead or deal, and update_record to change one record.",
}

var changeMySettingsCopy = toolCopy{
	Purpose: "Change what belongs to the user alone: their display name, greeting name, language, time zone and " +
		"working hours, their onboarding step, and their Voice DNA: its preferences, the samples of their " +
		"writing it learns from, and which version of it is in use.",
	Limits: "Each action touches only this user. Colleagues see a new name and bookings follow new hours, so ask first, and " +
		"ask before applying, rejecting or rolling back a voice version, because drafts follow it. Set a time zone they ask " +
		"for with source agent, never chosen. What the product sends them and where, " +
		"what their drafts taught the voice, replacing or putting back a sample they filed or left out, and removing " +
		"samples for good are done in the app.",
	Instead: "Use change_settings for what every colleague shares, read_workspace topics my_working_hours and " +
		"my_brief_delivery to read their settings, and run_research to build the voice.",
}

var changeComplianceCopy = toolCopy{
	Purpose: "Do the user's privacy work on a contact or record: record consent, record that a contact " +
		"asked to stop, allow a refused category, record an exchange with a lead or contact, " +
		"answer a refused send, open or update a data subject request, assign a disclosure duty, confirm a " +
		"source, accept or decline what a subject proposed, place a legal hold and pin an activity to the " +
		"retention floor.",
	Limits: "record_consent, allow_contact, revoke_override, lift_suppression, record_qualifying_event, " +
		"retract_qualifying_event, record_lead_qualifying_event, retract_lead_qualifying_event, retract_acquisition_request and " +
		"record_communication_context have legal weight: tell the user what you will record and call only " +
		"after they say yes. The proof names you as its author. Record only what the user or the contact " +
		"said, never your reading of their tone. Fulfilling an erasure request, lifting a legal hold, " +
		"releasing a restricted record, restoring a change and retention policy are done in the app.",
	Instead: "Use send_outbound double_opt_in, privacy_notice or details_confirmation to mail the contact, " +
		"read_details contact_consent, contact_consent_guard or contact_stops to check a contact before you record, read_workspace topics data_subject_requests, notice_cases, audit_log, legal_holds and " +
		"restricted_records to read the queues, and read_workspace topic record_history for one record's trail.",
}

var changeCaptureCopy = toolCopy{
	Purpose: "Decide what comes into the CRM: keep an address or domain out, answer a sender or domain " +
		"question, keep one party's mail private, declare the user's own other addresses, set a mailbox's " +
		"posture, signature pass, history baseline and filing word, cancel a backfill, disconnect a mailbox " +
		"or channel, define the knowledge corpora and upload a text document to one, import the user's LinkedIn " +
		"connections export, and carry on, test, cancel or undo an import. import_vcards takes a vCard file or " +
		"its text, and fills a contact that already matches rather than doubling it.",
	Limits: "Keep a domain out of the user's own capture with add_exclusion, scope user. Scope workspace " +
		"stops capture for every colleague, so ask the user first. set_blocked_domain " +
		"is a different act: it refuses the parent domain for the whole workspace, so a request about the " +
		"user's own mail never reaches it. " +
		"Answers change what is captured from then on. A sender's keep_out destroys mail, so the " +
		"app does that one. Connecting a mailbox needs the provider's consent in a browser, and deleting " +
		"a document for good stays in the app. Ask the user first where the action is marked so: a vCard " +
		"writes contacts at once, and says the contact handed the card over.",
	Instead: "Use read_workspace topics capture_senders, held_threads, counterparty_holds and corpora to read " +
		"first, run_research start_backfill to bring in old mail, run_research ask_corpus to ask a corpus, " +
		"preview_import and commit_import to start an import, and change_tags to tag a record.",
}

var runResearchCopy = toolCopy{
	Purpose: "Run AI work for the user: ask about a company, refresh a company's brief, dossier, growth " +
		"fit or scan or a contact's brief, research a contact, make today's morning brief, ask a knowledge " +
		"corpus, write the sales review's assessment, propose a report or filter from a description, " +
		"draft an introduction, regenerate an offer, propose a deal's roles, talk through a site read, " +
		"read a photo of business cards into proposals, build the user's voice, preview the Home repair, " +
		"buy a contact's details from the data provider, rebuild the search index, or bring in old mail.",
	Limits: "Each call spends AI budget, and research asks an outside provider, so the passport needs " +
		"the enrich scope. Read the panel first through read_details; run this only when it is out of date. " +
		"Ask the user first before buying a contact's details, and before rebuilding the search index for the whole " +
		"workspace. Buying needs a daily limit and a healthy connection, skips a contact already looked up, " +
		"and buys a priced detail only when you name its category.",
	Instead: "Use read_details to read a panel, and enrich to fill a company's fields from its website.",
}

var readDetailsCopy = toolCopy{
	Purpose: "Read what the app shows beside a record or on the user's day: a company's dossier, " +
		"growth fit, scan or brief, a contact's brief, profile fields, consent and proof, whether you may " +
		"write to them and the privacy mail they would get, a deal's products, win " +
		"evidence or legal reviews, a contract, a mail draft, scheduled sends and meeting proposals, " +
		"the Worklist's handled and hidden rows, the team board, weekly plans and reviews, notices, " +
		"deal and tag suggestions, introduction requests and record history, a disclosure duty, why a " +
		"send was refused, what a filter, a draft automation, a voice sample, an AI setting or a repair would " +
		"do, and what a mail backfill would bring in; and a product's translations, the jurisdiction's documents " +
		"and a site read's logo.",
	Limits: "Each action reads one panel as the app draws it, for the records the user may open. Without the " +
		"enrich scope, a brief, dossier or growth fit not written yet answers its plain form from the records, " +
		"marked deterministic; run_research writes one.",
	Instead: "Use read_context for the story so far on a record, read_attention for what needs the " +
		"user now, and read_record for a record's own fields.",
}
