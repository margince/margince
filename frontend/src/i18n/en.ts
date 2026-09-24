// English catalog. Keys are the contract; every other catalog mirrors them
// exactly (compile-time via satisfies, runtime via i18n.test.ts). Placeholders
// use {name} and are filled by t(key, params).
// Every value is written to docs/reference/ui-copy-style.md.
export const en = {
  "aiAdmin.allowance": "Monthly AI allowance",
  "aiAdmin.pool":
    "Shared company pool. Not an individual quota or a dollar spending cap.",
  "aiAdmin.consumption": "{spent} of {total} tokens used · {pct}%",
  "aiAdmin.remaining": "{tokens} tokens remaining",
  "aiAdmin.reset": "Resets {date} UTC",
  "aiAdmin.fixed":
    "Fixed company allowance overrides the per-user calculation.",
  "aiAdmin.formula":
    "{users} active full users × {tokens} tokens per user per month.",
  "aiAdmin.floor": "With no eligible users, the allowance counts 1 user.",
  "aiAdmin.normal": "Within the normal allowance band",
  "aiAdmin.degraded": "80% threshold reached: reduced-tier routing is active",
  "aiAdmin.queued": "Allowance reached: background AI is deferred",
  "aiAdmin.policy":
    "At 80%, routing moves to lower tiers; the model may stay the same. At 100%, background AI work waits and interactive AI uses the lowest tier. Search indexing continues and counts toward usage.",
  "aiAdmin.saved": "Allowance saved",
  "aiAdmin.recovery":
    "Eligible website reads, company scans and voice builds become runnable on the next reconciliation pass, normally within a minute. Completion depends on worker capacity, current permissions and provider availability. Other background passes keep their normal schedule.",
  "aiAdmin.edit": "Edit allowance",
  "aiAdmin.perUser": "Tokens per full user per month",
  "aiAdmin.range": "Whole tokens, from 1 to 1,000,000,000,000.",
  "aiAdmin.company": "Fixed company total (optional)",
  "aiAdmin.overrideHint":
    "Leave blank to use the per-user calculation. The per-user value is retained.",
  "aiAdmin.routingStale": "Model bindings changed while you were editing",
  "aiAdmin.stale": "This allowance changed while you were editing",
  "aiAdmin.staleHelp":
    "Cancel and reopen the editor to use the latest settings.",
  "aiAdmin.failed": "Change not applied",
  "aiAdmin.preview": "Preview effects",
  "aiAdmin.previewHint":
    "Preview of current conditions. Saving checks the latest settings and usage again; this does not reserve capacity or call a model.",
  "aiAdmin.features": "AI by activity",
  "aiAdmin.featuresWithheld":
    "Only a user with both AI diagnostics read and AI allowance read can see which features are live now.",
  "aiAdmin.save": "Save allowance",
  "aiAdmin.cancel": "Cancel",
  "aiAdmin.prospective":
    "Model selected by current policy. Actual calls can use a fallback or fail. It does not show provider status or which model was actually used.",
  "aiAdmin.calls": "Inspect actual model calls",
  "aiAdmin.website": "Website reads",
  "aiAdmin.scans": "Company scans",
  "aiAdmin.voice": "Voice builds",
  "aiAdmin.waiting": "Recorded work waiting on the allowance",
  "aiAdmin.coverage":
    "Counts cover durable website reads, company scans and voice builds only. They do not count every scheduled AI pass or guarantee that a request is still eligible to run.",
  "aiAdmin.unavailable": "Unavailable",
  "aiAdmin.impact.blocked": "Waiting on allowance",
  "aiAdmin.impact.model": "Different model selected",
  "aiAdmin.impact.fallback": "Fallback chain changed",
  "aiAdmin.impact.unconfigured": "No model configured",
  "aiAdmin.impact.exempt": "Continues beyond allowance",
  "aiAdmin.impact.same": "Same model selection",
  "aiAdmin.activity": "Activity",
  "aiAdmin.model": "Model selected by policy",
  "aiAdmin.cloud": "Cloud provider",
  "aiAdmin.endpoint": "Configured endpoint; location not verified",
  "aiAdmin.editBinding": "Edit shared binding",
  "aiAdmin.effect": "Effect",
  "aiAdmin.advanced": "Advanced: shared model bindings",
  "aiAdmin.unused": "Not used by current shipped activities: {tiers}",
  "aiAdmin.inputRate": "Input {input} per 1M tokens",
  "aiAdmin.rates": "Input {input} · Output {output} per 1M tokens",

  "brief.weekly.tasksCompleted": "Tasks completed",
  "teamweekly.noPriority": "No priority indicated by the recorded metrics",
  "teamweekly.basis":
    "Member summaries for the recorded week. Current plans and Worklists show today’s responsibilities.",
  "brief.weekly.noMeetings": "None recorded",
  "brief.weekly.noLeads": "None received",
  "brief.weekly.noCommitments": "None due",
  "brief.weekly.basis":
    "Recorded CRM work for this closed week. Missing records do not establish inactivity.",
  "home.receipt.date": "Close date: {before} → {after}",
  "home.receipt.undated": "No date",
  "home.receipt.confidence": "Updated forecast confidence",
  "home.receipt.forecast": "Forecast category changed",
  "home.task.yours": "Your task: {action}",
  "home.change.unknown": "Change author unknown.",
  "home.change.stageUnknown": "Unknown stage",
  "home.change.unidentified": "Unknown",
  "home.change.by": "Changed by: {actor}",
  "brief.focus.inQueue": "In the Worklist",
  "brief.focus.position": "{at} of {count}",
  "worklist.bandCount_one": "{count} item",
  "worklist.bandCount_other": "{count} items",
  "brief.focus.context": "View details",
  "brief.focus.back": "Back to Focus",
  "brief.queue.back": "Back to Worklist",
  "brief.queue.title": "Worklist",
  "brief.queue.show": "Show Worklist",
  "brief.queue.hide": "Hide Worklist",
  "brief.focus.urgentRemaining_one": "{count} more urgent item in the Worklist",
  "brief.focus.urgentRemaining_other":
    "{count} more urgent items in the Worklist",
  "brief.focus.remaining": "{count} more priorities in the Worklist",
  "brief.team.planUnavailable": "No current plan visible to you for {name}.",
  "brief.team.noCommitments": "{name} has no commitments in this plan.",
  "brief.team.outcomes":
    "{won} won · {lost} lost · {moved} deals moved · {leads} leads assigned",
  "brief.schedule.unavailable": "Calendar did not load.",
  "brief.schedule.more":
    "Load more agenda items to see the remaining meetings.",
  "brief.readings.riskPartial": "Known value only · not all checked",
  "brief.readings.unpricedCount_one": "1 deal not priced · excluded",
  "brief.readings.unpricedCount_other": "{count} deals not priced · excluded",
  "brief.coverage.source.generic": "Other work",
  "brief.coverage.source.weekly_commitment": "Weekly commitments",
  "brief.coverage.source.batch": "Grouped work",
  "brief.coverage.source.introduction_request": "Introduction requests",
  "brief.coverage.source.automation_run": "Automation failures",
  "brief.coverage.source.undelivered": "Unsent emails",
  "brief.coverage.source.bounce": "Undeliverable emails",
  "brief.coverage.source.ai_work_health": "Automation checks",
  "brief.coverage.source.capture_health": "Mailbox connections",
  "brief.coverage.source.failed_approval": "Failed approved actions",
  "brief.coverage.source.relationship_decay": "Quiet relationships",
  "brief.coverage.source.meeting_outcome": "Meeting follow-up",
  "brief.coverage.source.meeting": "Upcoming meetings",
  "brief.coverage.source.deal_at_risk": "Flagged deals",
  "brief.coverage.source.lead_response": "Assigned leads",
  "brief.coverage.source.customer_waiting": "Unanswered messages",
  "brief.coverage.source.conversation_claim": "Customer commitments",
  "brief.coverage.source.brief_item": "Deal updates",
  "brief.coverage.source.dedupe_candidate": "Duplicate candidates",
  "brief.team.commitmentRate":
    "{done} of {total} due commitments were completed.",
  "brief.team.meetingRate":
    "{done} of {total} meetings had a recorded next step.",
  "brief.digest.messages": "Emails synced",
  "brief.team.week": "Week starting",
  "brief.week.lostLabel": "Lost",
  "brief.feed.refreshFailed":
    "The Morning brief did not refresh. Showing the last loaded work.",
  "brief.team.saveResponse": "Save response",
  "brief.team.response": "Response to help request",
  "brief.team.planFor": "Current plan · {name}",
  "brief.team.plan": "Review current plan",
  "brief.approval.approve": "Approve",
  "brief.approval.email": "Approve email",
  "brief.coverage.source.approval": "Proposals",
  "brief.coverage.source.task": "Tasks",
  "brief.coverage.source.dsr": "Privacy requests",
  "brief.coverage.source.notice_case": "Privacy notices",
  "brief.coverage.source.notice": "Notices",
  "brief.weekly.learnings.caveat":
    "These observations describe associations in recorded work; they do not establish what caused the outcome.",
  "brief.plan.open": "Open weekly plan",
  "worklist.source.weekly_commitment": "Weekly commitment",
  "worklist.untitled.weekly_commitment": "Weekly commitment",
  "brief.plan.select": "Find a deal, lead, contact, company or project",
  "brief.plan.period": "Current plan · week of {date}",
  "brief.forecast.period":
    "Forecast period: {start} to {end}. Saved with this weekly review.",
  "brief.team.none": "No teams available. Add a team in Settings.",
  "brief.week.supporting": "Metrics, forecast and observations",
  "brief.coverage.retry": "Refresh brief",
  "brief.reply.owed": "Customer awaiting your reply",
  "brief.createdAt": "Brief created {when}",
  "brief.updatedAt": "Agenda updated {when}",
  "brief.changes.superseded":
    "This deal changed again. Open the deal to review its current state.",
  "brief.changes.title": "Changes made for you",
  "brief.changes.accept": "Accept",
  "brief.changes.accepted": "Accepted",
  "brief.changes.undone": "Undone",
  "brief.changes.empty": "No changes made on your behalf in the last 24 hours.",
  "brief.updates.title": "Updates",
  "brief.task.undated": "No due date",
  "brief.readings.summary": "Work summary",
  "brief.readings.unpriced": "Not priced",
  "brief.readings.noDealWork": "None",
  "brief.readings.noDealWorkWhy": "No deal flagged today",
  "brief.readings.unavailable": "Not counted",
  "brief.readings.unavailable.urgent": "Sources unavailable",
  "brief.readings.unavailable.meetings": "Calendar unavailable",
  "brief.readings.unavailable.leads": "Lead source unavailable",
  "brief.readings.unavailable.decisions": "Source unavailable",
  "brief.feed.incomplete": "No items loaded. Some work could not be checked.",
  "brief.feed.fullWorklist": "Open full Worklist",
  "brief.week.workRecorded": "Work completed this week.",
  "brief.week.leads": "Leads assigned: {count}.",
  "brief.week.responses": "Leads answered within target: {count}.",
  "brief.week.lost": "Deals lost: {count}.",
  "brief.row.details": "Details",
  "brief.glance.introTeam": "Your team’s day at a glance.",
  "teamweekly.focus.deals_at_risk": "Deal recovery",
  "teamweekly.headline.partial":
    "Partial coverage. These figures cover the members counted below.",
  "teamweekly.headline.unmeasured":
    "No member snapshots are available for this week. Performance is not measured.",
  "worklist.lead.noTarget": "no response target set",
  "worklist.lead.lastTouch": "last activity {date}",
  "worklist.deal.omitted": "omitted from forecast",
  "worklist.deal.provisional": "forecast close {date} (unconfirmed)",
  "brief.readings.riskBasis": "Expected value · unpriced excluded",
  "brief.readings.risk": "Revenue at risk",
  "brief.glance.introTeamWeekly": "Review your team’s recorded week.",
  "brief.feed.teamTitle": "Team priorities",
  "theme.toDark": "Dark theme",
  "theme.toLight": "Light theme",
  // The three appearance choices, as the options of a chooser — the account menu
  // offers all of them at once, so each is named by what it IS rather than by
  // what picking it does. The two labels above stay the names of the ICON-ONLY
  // control on sign-in and onboarding, where one button stands for the theme it
  // switches to. "System" names the machine whose preference it follows, not a
  // third appearance: it resolves to one of the other two and keeps resolving.
  "theme.light": "Light",
  "theme.dark": "Dark",
  "theme.system": "System",

  "trust.accept": "Accept",
  "trust.edit": "Edit",
  "trust.dismiss": "Dismiss",
  "trust.save": "Save",
  "trust.typedByYou": "Typed by you",
  "trust.typedByHuman": "Typed by a person",
  "trust.typedByBuyer": "Typed by a buyer",
  "trust.typedByPrefix": "Typed by",
  // An imported row names who wrote it in the system it came from, which is
  // neither the reader nor the administrator who ran the import. The first
  // form names that system too, because "logged in HubSpot" tells a reader why
  // a colleague who left years ago is on a row this installation holds.
  "trust.loggedInByVia": "Logged in {via} by {name}",
  "trust.loggedInBy": "Logged by {name}",
  "trust.sourceUnknown": "Source not recorded",
  "trust.agentTag": "Automated by {agent}",
  // A passport call stamps an opaque id and nothing on this side resolves it to
  // a name, so the tag says the kind and stops rather than printing an
  // identifier at a reader who can do nothing with it.
  "trust.agentUnnamed": "Automated by an agent",
  // A job the installation ran itself — a scheduled sweep, a backfill, a public
  // endpoint. Named apart from an agent because "a model decided this" and "the
  // system did its housekeeping" are different answers to "who do I ask".
  "trust.systemTag": "System task {job}",
  "trust.systemUnnamed": "System task",
  "trust.connectorTag": "Via {connector}",
  "trust.dismissed": "Suggestion dismissed",
  "trust.stagedProposal": "Proposed value",
  "trust.resolvedValue": "Resolved value",
  "trust.editValue": "Edit {description}",
  "trust.evidenceFrom": "Evidence from {source}",
  "trust.evidenceLine_one": "line {lines}",
  "trust.evidenceLine_other": "lines {lines}",

  "history.created": "(created)",
  "history.oldValue": "Previous value",
  "history.newValue": "New value",
  "history.cleared": "(cleared)",
  "history.passport": "Agent passport",
  "history.empty": "No changes recorded",
  "history.fieldEmpty":
    "Set at creation and never changed. The audit log records no edits.",
  "history.filterEmpty": "No changes match this filter.",
  "history.clearFilter": "Clear filters",
  "history.allFields": "All fields",
  "history.actorAll": "All",
  "history.actorHuman": "Human",
  "history.actorAgent": "Agent",
  "history.tabChanges": "By change",
  "history.tabFields": "By field",
  "history.undo.action": "Undo",
  "history.undo.redo": "Redo",
  "history.undo.busy": "Undoing change…",
  "history.undo.confirmTitle": "Undo this change?",
  "history.undo.confirmEdgeBody":
    "This changes the link with {other}. Both records stay; only the link between them changes.",
  "history.undo.confirmBody":
    "Fields reverting to their values before this change: {count}",
  "history.undo.versionSkew":
    "The record changed while open. The history was reloaded; review the change again before undoing it.",
  "history.undo.noBeforeImage":
    "This change did not record the previous values, so it cannot be undone.",
  "history.undo.notReplayable": "This kind of change cannot be undone.",
  "history.undo.unsupportedRecordType":
    "Changes to this record type cannot be undone.",
  "history.undo.superseded":
    "These fields have changed since. Undoing this change also undoes the later edits.",
  "history.undo.behindErasureBoundary":
    "This change predates an erasure, and its values were permanently deleted.",
  "history.undo.alreadyUndone": "This change was already undone.",
  "history.undo.notRestorableByThisPath":
    "These fields cannot be restored by undo.",
  "history.undo.recordArchived":
    "The record is archived. Restore the record before undoing a change.",
  "history.undo.nullUnwritable":
    "Undoing this change would clear a field that cannot be empty, so it cannot be undone.",
  "history.undo.notWritableByCaller":
    "You do not have permission to write these fields.",
  "history.undo.edgeRelinkUnsupported":
    "A removed link cannot be restored by undo. Add the link again on this record.",
  "history.reversal.collapsed": "{actor}’s change, undone by {undoer}",
  "history.reversal.collapsedSelf": "{actor} undid their own change",
  "history.reversal.partly": "{actor}’s change, partly undone by {undoer}",
  "history.reversal.partlySelf": "{actor} partly undid their own change",
  "history.reversal.net": "Net: unchanged",
  "history.reversal.stillChanged": "Still changed",
  "history.reversal.expand": "Show both changes",
  "history.reversal.collapse": "Hide",
  "history.reversal.undoneBy": "Undone by {undoer}",
  "history.reversal.unpaired": "Undoes an earlier change",
  "history.edge.marker": "Link",
  "history.field.address": "Address",
  "history.field.admission": "Domain intake check",
  "history.field.admission_reason": "Intake check reason",
  "history.field.admission_source": "Intake check source",
  "history.field.amount_minor": "Value",
  "history.field.bounce": "Delivery bounce",
  "history.field.capture_question": "Capture question",
  "history.field.channel_identity": "Channel account",
  "history.field.channel_username": "Channel handle",
  "history.field.cohort_linked": "Messages linked",
  "history.field.cohort_promoted": "Messages attributed",
  "history.field.corrected": "Result corrected",
  "history.field.disposition": "Disposition",
  "history.field.domain": "Domain",
  "history.field.expected_arr_minor": "Expected ARR",
  "history.field.assignee_id": "Assignee",
  "history.field.body": "Notes",
  "history.field.emails": "Email addresses",
  "history.field.nudge_dismissal": "Nudge dismissed",
  "history.field.phones": "Phone numbers",
  "history.field.meeting_status": "Meeting outcome",
  "history.field.candidate_company_key": "Matched company",
  "history.field.communication_basis": "Legal basis",
  "history.field.company_name": "Company name",
  "history.field.confirm_submission": "Confirmation kind",
  "history.field.currency": "Currency",
  "history.field.decided_by_level": "Decided by",
  "history.field.description": "Description",
  "history.field.display_name": "Name",
  "history.field.domains": "Domains",
  "history.field.due_at": "Due",
  "history.field.email": "Email",
  "history.field.ended_at": "Ended",
  "history.field.expected_close_date": "Expected close",
  "history.field.first_name": "First name",
  "history.field.forecast_category": "Forecast category",
  "history.field.full_name": "Name",
  "history.field.fx_rate_date": "FX rate date",
  "history.field.fx_rate_to_base": "FX rate",
  "history.field.industry": "Industry",
  "history.field.is_done": "Done",
  "history.field.last_name": "Last name",
  "history.field.legal_name": "Legal name",
  "history.field.lifecycle": "Lifecycle",
  "history.field.lifted_by": "Lifted by",
  "history.field.lifted_by_level": "Lifted at level",
  "history.field.lifted_suppression": "Lifted suppression",
  "history.field.linkedin_url": "LinkedIn URL",
  "history.field.lost_reason": "Lost reason",
  "history.field.name": "Name",
  "history.field.note": "Note",
  "history.field.occurred_at": "Occurred",
  "history.field.company_id": "Company",
  "history.field.owner_id": "Owner",
  "history.field.provider_claims_received": "Provider claims received",
  "history.field.reachability": "Reachable",
  "history.field.reply_verdict": "Reply result",
  "history.field.reply_verdict_by": "Reply result by",
  "history.field.research_claims_accepted": "Research claims accepted",
  "history.field.scope": "Scope",
  "history.field.stopped": "Stopped",
  "history.field.stops_carried": "Contact blocks copied",
  "history.field.submission_decision": "Submission decision",
  "history.field.vat_checked_at": "VAT checked",
  "history.field.vat_consultation_number": "VAT consultation number",
  "history.field.vat_number": "VAT number",
  "history.field.vat_registered_address": "VAT registered address",
  "history.field.vat_registered_name": "VAT registered name",
  "history.field.vat_requested": "VAT check requested",
  "history.field.vat_status": "VAT status",
  "history.field.visibility": "Visibility",
  "history.field.parent_company_id": "Parent company",
  "history.field.partner_attribution": "Partner attribution",
  "history.field.partner_company_id": "Partner",
  "history.field.project_id": "Project",
  "history.field.qualifying_event": "Qualifying event",
  "history.field.reason": "Reason",
  "history.field.recorded_at_level": "Recorded at level",
  "history.field.relationship_types": "Relationship types",
  "history.field.remind_at": "Reminder",
  "history.field.resolved_category": "Message category",
  "history.field.score": "Score",
  "history.field.score_override_reason": "Score override reason",
  "history.field.size_band": "Size",
  "history.field.social": "Social profiles",
  "history.field.source": "Source",
  "history.field.started_at": "Started",
  "history.field.status": "Status",
  "history.field.subject": "Subject",
  "history.field.submission_id": "Submission",
  "history.field.suppression_kind": "Suppression kind",
  "history.field.target_end_date": "Target end",
  "history.field.title": "Job title",
  "history.field.wait_until": "Waiting until",
  "history.field.commercial_motion": "Commercial motion",
  "history.field.priority": "Priority",
  "history.field.acquisition_source": "Acquisition source",
  // What an empty stored jsonb array means to a reader of a change row —
  // never a blank, which reads as a value the row failed to show.
  "history.emptyList": "None set",

  "confidence.high": "high",
  "confidence.med": "medium",
  "confidence.low": "low",

  "autonomy.auto": "automatic",
  "autonomy.confirm": "approval first",

  "nav.brief": "Home",
  "nav.contacts": "Contacts",
  "nav.companies": "Companies",
  "nav.leads": "Leads",
  "nav.deals": "Deals",
  "nav.analytics": "Analytics",
  "nav.settings": "Settings",
  "nav.automations": "Automations",
  "nav.group.records": "Records",
  "nav.group.work": "Work",
  "nav.group.intelligence": "Intelligence",
  "nav.offers": "Offer",
  "nav.share": "Sharing",
  "nav.search": "Search results",
  "nav.tags": "Tag",

  "shell.railAria": "Primary navigation",
  "shell.skipToContent": "Skip to content",
  "shell.logoAria": "Margince",
  "shell.companyLogoAria": "{company} home, powered by Margince",
  "shell.poweredBy": "Powered by Margince",
  "shell.poweredByPrefix": "Powered by",
  "shell.beta": "Beta",
  "shell.searchEverything": "Search or ask Margince",
  "shell.breadcrumbAria": "Breadcrumb",
  "shell.license.none": "No license",
  "shell.license.refused": "License refused",
  "shell.signOutAria": "Sign out",
  "shell.signOutErr": "Sign-out failed",
  "shell.collapse": "Collapse sidebar",
  "shell.expand": "Expand sidebar",
  "shell.accountAria": "Account",
  "shell.theme": "Theme",
  "shell.more": "More",
  "shell.unknownPage": "Not found",
  "shell.closeMenu": "Close",
  // The agent panel's one promise, as opposed to its readings: what the agent
  // can REACH. Row scope bounds it on the server, and a guarantee nobody is
  // told about is one nobody can rely on — "what can this thing see" is the
  // question a contact most reasonably has about an agent working over their
  // data. Held by AC-shell-8.
  "shell.agent.scope": "Margince reads only what you can see.",
  "shell.capture.importing": "Importing mailbox history",
  "shell.capture.share": "{percent} · {scanned} of {total} messages",
  "shell.capture.count": "{scanned} messages so far",
  "shell.capture.open": "Open import",
  // The sidebar's second level. Out of the section the control names the whole
  // product it returns to; deeper it READS one word at every depth while its
  // accessible name says which list it leads back to.
  "shell.navBackApp": "Back to app",
  "shell.navBack": "Back",
  "shell.navBackTo": "Back to {name}",
  // At phone width a section's entries are reached from the page head. The
  // control READS the entry it is on; the name says what pressing it does and
  // keeps that word inside itself (WCAG 2.5.3).
  "shell.sectionSwitch": "{name}: change section",
  "attention.selected": "{n} selected",
  "locale.name.en": "English",
  "locale.name.de": "Deutsch",
  "locale.name.vi": "Tiếng Việt",
  "locale.switchLabel": "Language",

  "screen.pending": "This screen is not available yet.",

  // The composed extension tier (ADR-0120): #/ext/<unit>. The registry is
  // generated per installation, so these two strings are the only part of a
  // unit surface the core catalogs own.
  "ext.notFound":
    "No extension named “{name}” is enabled on this installation.",

  // The reference extension's own screen (#/ext/notes) carries no keys here:
  // a unit that ships a screen ships its copy with it, under
  // extensions/<unit>/frontend/i18n/, namespaced `ext<Unit>.` and merged into
  // the catalogue by gen-composition (see i18n/index.tsx).

  "search.placeholder":
    "Search contacts, companies, deals, projects, products, activities, leads…",
  "search.prompt": "Enter a search term.",
  "search.empty": "No matches for “{q}”.",
  "search.group.contact": "Contacts",
  "search.group.company": "Companies",
  "search.group.deal": "Deals",
  "search.group.project": "Projects",
  "search.group.product": "Products",
  "search.group.offerTemplate": "Offer templates",
  "search.group.activity": "Activities",
  "search.group.lead": "Leads",
  "search.group.tag": "Tags",
  "search.kind.contact": "Contact",
  "search.kind.company": "Company",
  "search.kind.deal": "Deal",
  "search.kind.project": "Project",
  "search.kind.product": "Product",
  "search.kind.offerTemplate": "Offer template",
  "search.kind.activity": "Activity",
  "search.kind.lead": "Lead",
  "search.kind.tag": "Tag",
  "search.filter.label": "Show only",
  "search.filter.all": "All",
  "search.pending": "Searching…",
  "search.tag.carriedBy": "Tagged records: {count}",
  "search.tier.mirrored": "From a connected system",
  "search.tier.unverified": "Unverified",

  "context.recentTouches": "Recent activity",
  "context.openTasks": "Open tasks",
  "context.relatedContacts": "Related contacts",
  "context.relatedCompanies": "Related companies",
  "context.relatedProjects": "Related projects",
  "context.whoKnows": "Colleague connections",
  "context.relatedDeals": "Related deals",
  "context.title": "Related records",
  "context.empty": "No related records.",

  "palette.aria": "Command palette",
  "palette.placeholder": "Search or ask Margince",
  "palette.empty": "No matches.",
  "palette.typeScreen": "Screen",
  "palette.typeAction": "Action",
  "palette.typeRecord": "Record",
  "palette.seeAll": "See all results for “{query}”",
  "palette.searching": "Searching records…",
  "palette.searchFailedTitle": "Search failed",
  "palette.searchFailed": "The commands above still work.",
  "action.newDeal": "New deal",
  "action.readCompany": "Read a company",
  "action.booking": "Booking page",

  "common.undo": "Undo",
  "common.close": "Close",
  // The one sentence every copy failure opens with. It is a fact about the
  // BROWSER rather than about the link, the secret or the agenda, so the
  // screens differ only in the way out they offer after it.
  "clipboard.copyFailedTitle": "Clipboard access denied",

  "explain.open": "Explain this number",
  "explain.mayHaveMoved":
    "This link does not record when the number was calculated, so these figures were recalculated now. If an exchange rate changed since, they may not match the number you clicked.",
  "explain.title": "How this number is built",
  "explain.rate": "rate {rate} on {date}",

  "board.count": "{count} deals",
  "board.weighted": "weighted {value}",
  "board.mixedCurrencies": "several currencies, no single total",
  "dealfiles.hidden": "Hidden from this deal",
  "dealfiles.unhidden": "Shown on this deal again",
  "deal.stalled": "stalled",
  "deal.stalledBadge": "Stalled",
  "deal.singleThreaded": "Single-threaded",
  "deal.staged": "Staged",
  "deal.archived": "Archived",
  "deal.closes": "closes {date}",
  "deal.undated": "no close date",
  "deal.lastMail": "Last email",
  "deal.mail.title": "Previous emails",
  "deal.mail.sent": "Sent {ago}",
  "deal.mail.received": "Received {ago}",
  "deal.mail.none": "No email on this deal yet",
  "deal.mail.viewAll": "View all activity",
  "deal.closesProvisional": "provisional close date, not confirmed by a human",
  "record.notShown": "Not shown",
  "reading.restricted": "Restricted",
  "reading.loading": "Loading",
  "format.notForecast": "Not forecast",
  "record.timelineLoading": "Loading history…",
  "record.chronologyLoading": "Loading change history…",
  // The same word the tab strip uses (`tab.timeline`): the heading over the
  // slot and the tab that opens it name one thing, and two words for it read
  // as two things.
  "record.timeline": "History",
  "record.edit": "Edit",

  "record.fieldRequired": "This field is required.",
  "record.registration": "Registration",
  "record.leadProfileReadOnly": "LinkedIn cannot be changed on a lead.",
  "record.leadStatusAction": "Change the status with the lead status controls.",
  "record.leadScoreAction":
    "Change the score with the score override and a reason.",
  "record.openProfile": "Open profile",
  "record.fieldsFailed": "Custom fields could not be loaded.",
  "record.fieldsLoading": "Loading custom fields…",
  "record.fieldsRetry": "Retry",

  "record.companyRoutingKey": "Company routing key",
  "record.finishFieldEdit":
    "Save or cancel the current edit before closing details.",
  "record.save": "Save",
  "record.saveDone": "“{name}” saved",
  "record.archiveDone": "“{name}” archived",
  "record.archive": "Archive",
  "record.disqualify": "Disqualify",
  "record.archiveConfirm": "Archive this record? There is no undo.",
  "record.archived": "Archived",
  "record.archivedReadOnly":
    "This company is archived. Restore it to make changes.",
  "record.notYoursToChange":
    "You cannot edit this company. Ask its owner to share it, or an administrator for edit rights.",
  "record.logActivityRefused":
    "You do not have permission to log activities on this record.",
  "record.share": "Share",
  "record.moreActions": "More actions",
  "record.fullHistory": "Full history",

  "share.title": "Share this record",
  "share.ceiling.pre": "A grant changes who can see ",
  "share.ceiling.recordEmphasis": "exactly this one record",
  "share.ceiling.mid":
    ". Nothing else about a person’s scope moves. A share is capped at your own access, ",
  "share.ceiling.noWider": "no wider",
  "share.ceiling.post": ".",
  "share.unknownRecord": "This record cannot be shared.",
  "share.grantAccess": "Grant access",
  "share.subject": "Person or team",
  "share.holdsRead": "Has read",
  "share.holdsWrite": "Has write",
  "share.kindPerson": "Person",
  "share.kindTeam": "Team",
  "share.access": "Access level",
  "share.access.read": "Read",
  "share.access.write": "Write",
  "share.access.readNote":
    "Can open and read this record, but not edit or send.",
  "share.access.writeNote":
    "Can open, edit and add to this record, but not change ownership or sharing.",
  "share.expiry": "Expiry",
  "share.expiry.none": "No expiry (until revoked)",
  "share.expiry.day": "Expires in 24 hours",
  "share.expiry.week": "Expires in 7 days",
  "share.expiry.month": "Expires in 30 days",
  "share.expiryConsequence_one":
    "Access is revoked automatically in {days} day. You can revoke it sooner.",
  "share.expiryConsequence_other":
    "Access is revoked automatically in {days} days. You can revoke it sooner.",
  "share.expiryConsequenceNone": "Access lasts until you revoke it.",
  "share.reason": "Reason",
  "share.grant": "Grant access",
  "share.update": "Update access",
  "share.unchanged":
    "Nothing changed. {name} already had {access} access to this record.",
  "share.downgradeTitle": "Reduce access?",
  "share.downgradeBody":
    "{name} has {from} access to this record and will keep only {to} access. The change is recorded in the audit trail.",
  "share.downgradeConfirm": "Reduce to {to}",
  "share.seatCeiling":
    "A read-only seat cannot hold write access. Upgrade the seat first, or grant read access.",
  "share.whoHasAccess": "Shared with",
  "share.grantedBy": "granted by",
  "share.revoke": "Revoke",
  "share.revokeConfirm":
    "Revoke this access? It ends at the next request and cannot be undone.",
  "share.approvalRequired":
    "This share takes effect only after approval. It is not applied yet.",
  "share.teamMembers_one": "Team · {count} member",
  "share.teamMembers_other": "Team · {count} members",
  "share.rosterLoading": "Loading people and teams…",
  "share.rosterErrorUsers": "People did not load. Teams are shown below.",
  "share.rosterErrorTeams": "Teams did not load. People are shown below.",
  "share.rosterErrorBoth": "People and teams did not load.",
  "share.rosterEmpty": "No people or teams to share with.",

  "edit.versionSkew":
    "This record changed since it was opened. Reload and retry.",

  "merge.contact": "Merge contact",
  "merge.company": "Merge company",
  "merge.searchPlaceholder": "Search…",
  "merge.pickTarget": "Select record to keep",
  "merge.confirm": "Merge {source} into {target}? This archives {source}.",
  "merge.submit": "Merge",

  "tab.overview": "Overview",
  "tab.relationships": "People and companies",
  "tab.partner": "Partner",
  "tab.rollup": "Roll-up",
  "tab.history": "History",

  "rollup.weightedPipeline": "Weighted deal value",
  "rollup.closedWon": "Won this quarter",
  "rollup.activity30d": "Activity, 30 days",
  "rollup.accounts": "Companies included",
  "rollup.excluded": "Hidden companies excluded: {count}",
  "rollup.fxUnavailable":
    "An exchange rate is missing, so the total cannot be calculated.",
  "rollup.computedAt": "Computed at {when}",

  "nav.partners": "Partners",
  "deal.partnerSourced": "via",
  "deal.partnerInfluenced": "helped by",
  "deal.partnerAttribution": "Partner attribution",
  "deal.attributionUnset": "Not set (counted as sourced)",
  "deal.attributionSourced": "Sourced the deal (earns commission)",
  "deal.attributionInfluenced": "Influenced an existing deal (no commission)",
  "partnerDeals.panelTitle": "Partner deals",
  "partnerDeals.none": "No partner deals yet",
  "partnerDeals.column.deal": "Deal",
  "partnerDeals.column.customer": "Customer",
  "partnerDeals.column.attribution": "Attribution",
  "partnerDeals.column.amount": "Deal value",
  "partnerDeals.column.status": "Status",
  "commission.panelTitle": "Commission",
  "commission.none": "No commission yet",
  "commission.column.deal": "Deal",
  "commission.column.amount": "Earned",
  "commission.column.rate": "Rate",
  "commission.column.basis": "Deal value",
  "commission.column.status": "Status",
  "commission.status.accrued": "Accrued",
  "commission.status.approved": "Approved",
  "commission.status.paid": "Paid",
  "commission.status.void": "Reversed",
  "commission.outstanding": "Outstanding",
  "commission.outstandingDetail_one": "1 entry · accrued or approved",
  "commission.outstandingDetail_other": "{count} entries · accrued or approved",
  "commission.column.actions": "Actions",
  "commission.decide.withheld": "No permission to decide",
  "commission.decide.approve": "Approve",
  "commission.decide.pay": "Mark as paid",
  "commission.decide.void": "Reverse",
  "commission.decide.approveConfirm":
    "Approving records that this commission is agreed. It pays nothing: settle the payment in your finance system, then mark it paid here.",
  "commission.decide.payConfirm":
    "Mark as paid only after your finance system has paid it. Margince records the payment and moves no money.",
  "commission.decide.voidConfirm":
    "Reversing adds an offsetting entry beside this one. Nothing is deleted, and the original stays readable.",
  "commission.decide.reasonLabel": "Reversal reason",
  "commission.decide.reasonRequired":
    "Enter a reason. It explains the reversal to the partner later.",
  "commission.decide.approved": "Commission approved",
  "commission.decide.paid": "Commission marked as paid",
  "commission.decide.voided": "Commission reversed",
  "commission.decide.settledElsewhere":
    "Payment happens in your finance system. Margince records the result.",
  "partner.setup": "Make this a partner",
  "partner.edit": "Edit partner",
  "partner.none": "Not a partner yet",
  "partner.company": "Company",
  "partner.role": "Partner role",
  "partner.roleAll": "All roles",
  "partner.certStatus": "Certification status",
  "partner.certStatusAll": "All statuses",
  "partner.marginTier": "Margin tier",
  "partner.stage": "Relationship stage",
  "partner.nextStep": "Next step",
  "partner.nextStepDue": "Next step due",
  "partner.servedSegments": "Served segments",
  "partner.servedSegmentsHint": "comma-separated",
  "partner.role.hosting": "Hosting",
  "partner.role.consulting": "Consulting",
  "partner.role.strategic": "Strategic",
  "partner.cert.applied": "Applied",
  "partner.cert.certified": "Certified",
  "partner.cert.suspended": "Suspended",
  "partner.marginTier.tier1": "Intro (15%)",
  "partner.marginTier.tier2": "Active collaboration (20%)",
  "partner.marginTier.tier3": "Partner closed (25%)",
  "partner.stage.research": "Research",
  "partner.stage.identified": "Identified",
  "partner.stage.contacted": "Contacted",
  "partner.stage.inConversation": "In conversation",
  "partner.stage.fitConfirmed": "Fit confirmed",
  "partner.stage.agreementPending": "Contract pending",
  "partner.stage.active": "Active",
  "partner.stage.activeReferring": "Active, referring",
  "partner.stage.dormant": "Dormant",
  "partner.stage.noFit": "No fit",

  "rel.add": "Add relationship",
  "rel.seatOnDeal": "Add to a deal",
  "rel.addStakeholder": "Add stakeholder",
  "rel.dealStakeholders": "Stakeholders",
  "rel.dealStakeholdersEmpty": "No stakeholders on this deal",
  "rel.kind": "Kind",
  "rel.saveDone": "Relationship saved",
  "rel.role": "Role",
  "rel.startedAt": "Started",
  "rel.endedAt": "Ended",
  "rel.current": "Current",
  "rel.endedOn": "Until {when}",
  "rel.remove": "Remove",
  "rel.removeConfirm": "Remove this relationship? There is no undo.",
  "rel.empty": "No relationships yet",
  "rel.counterparty": "Linked to",
  "rel.dates": "Dates",
  "rel.addConfirm": "Add a {kind} link to {target}.",
  "rel.kind.employment": "Employment",
  "rel.kind.dealStakeholder": "Deal stakeholder",
  "rel.kind.projectStakeholder": "Project stakeholder",
  "rel.kind.projectCompany": "Company on project",
  "rel.kind.partnerOf": "Partner of",
  "rel.kind.referredBy": "Referred by",
  "rel.kind.coSellWith": "Co-sell with",
  "rel.kind.worksWith": "Works with",
  "rel.kind.billingContact": "Billing contact",

  "common.error": "Could not load this view. Reload the page.",
  // What a failure that carries no server problem is allowed to say. A rejected
  // fetch and a bug in our own code both report in wording nobody authored for
  // a reader, so the screen states the fact it can stand behind and stops.
  "common.errorNoCause": "The request failed. No cause reported.",
  "common.assistantUnavailable":
    "The assistant did not respond, so no draft was created. Enter the details manually, or ask an administrator to check the model in Settings under AI.",
  "common.gatewayUnavailable":
    "The server did not finish the request in time and may still be processing it. Wait before retrying, or the work can run twice.",
  // Every 403 the server codes `permission_denied`, which is two refusals with
  // one name: a role that does not admit the action on this kind of record, and
  // a record the reader holds read-only through a share. Nothing on the wire
  // tells them apart, so the copy names neither and offers both ways out — an
  // admin widens a role, the colleague who shared a record widens the share. It
  // does not say the record may be missing: a row the reader may not see at all
  // comes back 404, so by the time this is read the record is one they may know
  // about. Two sentences, no dash (VOICE-RULE-5).
  "common.permissionDenied":
    "You do not have permission for this action. Ask an administrator, or the user who shared this record, to extend your access.",
  // The licensing ceiling, decided before any role is consulted: a read seat is
  // refused every mutating request, so a reader whose role admits the action is
  // refused anyway. Names the SEAT rather than the reader, because the same code
  // answers a read seat's own request, an agent passport acting for one, and a
  // grant that would give a read seat write access. Two sentences, no dash
  // (VOICE-RULE-5).
  "common.seatReadOnly":
    "This seat is read-only, so the request was refused. Ask an administrator to upgrade the seat.",
  "common.retry": "Retry",
  "common.empty": "Nothing here yet.",
  "common.saving": "Saving…",
  "common.loading": "Loading…",
  // A reference whose name READ failed, which is not the same as a record that
  // has no name: the id stays reachable through the title, and the reference
  // never becomes a link, because a name that did not load cannot be trusted as
  // a destination.
  "ref.nameLoadFailed": "Name did not load",
  "ref.notInRoster": "Currently assigned (no longer in the user list)",

  // A record search that ANSWERED and found nothing. Distinct from a search
  // that has not run: both drew the same empty space before, and a reader
  // could not tell "nobody here" from a field still thinking.
  "picker.noMatch": "No match",

  // The app-level boundary's fallback. It says what happened and what to do
  // next, and nothing about the error itself: a render throw carries our own
  // internals, which the reader can neither read nor act on. Two sentences,
  // no dash (VOICE-RULE-5).
  "app.errorTitle": "This view stopped working",
  "app.errorBody": "Retry. If it fails again, reload the page.",
  "app.errorRetry": "Retry",

  // The card-level render boundary (design-system/cardboundary.tsx). It says
  // less than the app-level one because it has taken less: the page and its
  // navigation are still there, and only this card is gone.
  "card.errorTitle": "This card stopped working",
  "card.errorRetry": "Retry",

  // The nine-state honesty vocabulary (design-system/surfacestate.tsx). These
  // words belong to the STATE and to no particular surface, which is why they
  // are keyed `state.*` rather than under any one screen — the same sentence
  // has to read correctly under a deal list and under a retention card. What
  // there is none OF stays the caller's word, passed as `emptyLabel`.
  "state.withheld": "Hidden for your role",
  "state.unavailable":
    "Some data did not load, so this section may be incomplete.",
  "state.failed": "This section did not load.",
  "state.loading": "Loading this section…",
  "state.retry": "Retry",
  "state.stale": "Last known values, not refreshed",
  "state.staleAsOf": "Last known values, as of {when}",
  "state.partial": "Showing part of the list",
  "state.partialCount": "{count} more not shown",
  // The opened file. `FilePreview` is a design-system control every surface
  // that draws a file card can open, so its words belong to the control rather
  // than to any one screen â the same three verbs stand over a contract on an
  // account and over a scan that arrived on a message.
  "filePreview.download": "Download",
  "filePreview.print": "Print",
  "filePreview.loading": "Loading file…",
  "filePreview.failedTitle": "File cannot be previewed",
  "filePreview.failed": "Download the file to open it in another application.",

  "list.headActions": "More actions",
  "list.search": "Search",
  "list.showArchived": "Show archived",
  "list.loadMore": "Load more",
  "list.viewAll": "All",
  "list.viewHot": "Hot",

  // The list surface (design-system/listtable.tsx). The count says "loaded"
  // rather than a total on purpose: paging is a keyset cursor, so the number
  // of rows in hand is the only figure the client can state honestly.
  "table.range": "{first} to {last} of {count} {unit}",
  "table.pagination": "Pages",
  "table.page": "Page {number}",
  "table.prev": "Previous",
  "table.next": "Next",
  "table.rowsPerPage": "Rows per page",
  "table.perPage": "{count} per page",
  "table.sortedBy": "sorted by {column}",
  "table.shownColumns": "Shown columns",
  "table.display": "Display",
  "table.density": "Density",
  "table.compact": "Compact",
  "table.sort": "Sort",
  "table.sortNamed": "Sort: {column}",
  "table.sortMenu": "Sort by",
  "table.sortDefault": "Default order",
  "table.sortAscending": "ascending",
  "table.sortDescending": "descending",
  "table.sortBy": "Sort by {column}",
  "table.noMatches": "No {unit} match these filters.",
  "table.clearFilters": "Clear filters",
  "table.none": "No {unit} yet.",
  "table.actions": "Actions",
  "table.rangeLoaded": "{first} to {last} of {count} {unit} loaded",
  "unit.contacts": "contacts",
  "unit.companies": "companies",
  "unit.deals": "deals",
  "unit.leads": "leads",
  "unit.partners": "partners",
  "unit.products": "products",
  "unit.offerTemplates": "offer templates",
  "table.filter": "Filter",
  "table.filterSearch": "Search attributes",
  "table.addFilter": "Add filter",
  "table.filterIs": "is",
  "table.filterCondition": "Condition",
  "table.filterMore": "More actions for the {filter} filter",
  "table.deleteFilter": "Delete filter",
  "table.filterValueSearch": "Search {filter} values",
  "table.filterTypeToSearch": "Type to search",
  "table.filterSearching": "Searching…",
  "table.filterSearchFailed": "Search failed. Retry.",
  "table.filterNoMatches": "No matches.",
  "table.filterBack": "Back to all filters",

  "contacts.name": "Name",
  "contacts.email": "Email",
  "list.owner": "Owner",
  "list.unowned": "Unassigned",
  "list.created": "Created",
  "list.lastActivity": "Last activity",
  "list.filterOwnerMe": "Owned by you",
  "list.filterOwnerAll": "Any owner",
  "list.filterOwnerUnassigned": "Unassigned",
  "views.save": "Save view",
  "views.saveConfirm": "Save",
  "views.saveTitle": "Save this view",
  "views.name": "Name",
  // Names the saved-view rail where the rail itself cannot: a failure notice
  // lands beside the list's own tools, and "this section did not load" under a
  // toolbar says nothing about WHICH section.
  "views.rail": "Saved views",
  "list.viewMine": "Mine",
  "list.viewCustomers": "Customers",
  "list.viewProspects": "Prospects",
  "company.filterLifecycleAll": "Any stage",
  "company.filterRelTypeAll": "Any type",
  "company.filterSizeBandAll": "Any size",
  "consent.confirmRecipient": "Send to {address}",
  "consent.guardFailed": "Communication permissions could not be loaded.",
  "consent.permissionScope":
    "Permissions apply to the named purpose. Account and service notices do not authorize sales or marketing; each message is checked before sending.",
  "consent.manage": "Manage consent and proof history",
  "contact.consent": "Consent",
  "consent.grant": "Record consent",
  // Submitted as the proof row's wording when an operator records a grant
  // here. It does NOT quote a screen the subject read — this door has none —
  // so it says what actually happened: a named operator attested to a consent
  // obtained away from the product. A canned subject-facing sentence would be
  // the fabricated proof this rule exists to remove.
  "consent.operatorWording":
    "Recorded in the CRM by a member of staff, who attested that this contact gave their consent for {label} outside the product. No wording was shown to them here.",
  "consent.withdraw": "Withdraw",
  "consent.doiBySubject":
    "Only this contact can confirm this purpose through a link sent to their recorded address. Use “Ask them to confirm their details” in Communication permissions.",
  "consent.askToConfirm": "Ask them to confirm their details",
  "consent.askToConfirmWhat":
    "Mails this contact a private link to see what you hold about them, correct it, and say whether they want to hear from you. It goes to their own recorded address; you cannot send it anywhere else.",
  "consent.askQueued": "Link queued for {address}.",
  "consent.askNotDelivered":
    "The link was created for {address}, but this installation sends no mail, so it was not sent.",
  "consent.askExpires": "The link works until",
  "consent.noRecord": "no record",
  "consent.noPurposes": "This company tracks no consent purposes yet.",
  "consent.defaultDeny":
    "This history records consent for each purpose. Communication permission may also depend on other recorded grounds; the message is checked again before sending.",
  "consent.basis": "Basis: {basis}",
  "consent.proofLog": "Proof log",
  "consent.proofEmpty":
    "No consent decision recorded for this purpose. An empty log means no consent event was recorded, not that one is missing.",
  "consent.sourceUnknown": "source not recorded",
  "consent.actorHuman": "Human",
  "consent.actorAgent": "Agent",
  "consent.actorSystem": "System",
  "consent.actorConnector": "Connector",
  "consent.actorUnknown": "actor not recorded",
  "consent.purposesUnavailable":
    "The consent purpose catalog did not load, so double opt-in requirements cannot be shown.",

  "company.reject": "Not a company",
  "company.rejectConfirm":
    "This archives “{name}” and blocks {domain} as a company, so later messages from that domain do not create it again. An administrator can unblock the domain in Settings under Capture.",
  "company.rejectReasonLabel": "Reason this is not a company",
  "company.rejectReasonHint":
    "A reason a reviewer of blocked domains can act on. The block outlasts the record.",
  "company.rejectDone": "“{name}” archived and {domain} blocked as a company",
  "company.name": "Company",
  "company.brief.title": "Company brief",
  "company.description": "Description",
  "company.website": "Website",
  "company.contactCount": "Contacts",
  "company.openDealCount": "Open deals",
  // Offered only where there is no partner programme yet: the tab that holds
  // the form appears once one exists, so this is how the first one is made.
  // Where the account stands with us, and what it is to us — the two
  // questions the retired classification answered with one value.
  "company.lifecycle": "Lifecycle",
  "company.relationshipTypes": "Relationship type",
  "company.sizeBand": "Company size",
  "company.lifecycle.unknown": "Not assessed",
  "company.lifecycle.target": "Target",
  "company.lifecycle.prospect": "Prospect",
  "company.lifecycle.opportunity": "Opportunity",
  "company.lifecycle.customer": "Customer",
  "company.lifecycle.former_customer": "Former customer",
  "company.lifecycle.disqualified": "Disqualified",
  "company.relType.customer": "Customer",
  "company.relType.partner": "Partner",
  "company.relType.supplier": "Supplier",
  "company.relType.investor": "Investor",
  "company.relType.portfolio_company": "Portfolio company",
  "company.relType.competitor": "Competitor",
  "company.relType.other": "Other",
  // Why a stored fact contradicts its own field. The fact is still shown
  // with its evidence — a reader can tell, and hiding it would be worse.
  "co.factSuspect.phoneShapedLocation": "Looks like a phone number",
  "co.factSuspect.notAPhone": "Does not look like a phone number",
  "co.factSuspect.notAYear": "Does not look like a year",
  "co.factSuspect.notAnEmail": "Does not look like an email address",
  "co.factSuspect.notASize": "Does not look like a headcount",
  // The three readings the overview leads with, and what performing a
  // suggestion means. "Whose move" is the question the 0-100 score was
  // mistaken for.
  "co.strip.title": "Company status",
  "co.strip.convertedAsOf": "{count} converted, rates {date}",
  "co.strip.noOpenDeals": "None",
  "co.strip.pipeline": "Open deals",
  "co.description.label": "Description",

  "co.strip.netInvoiced": "Revenue · 12 mo",
  "co.strip.notAssessed": "Not assessed",
  "co.strip.lifetimeOf": "{amount} lifetime",
  "stat.evidence": "Evidence",
  "stat.evidence.rests": "What this rests on",
  "stat.open": "Open",
  "co.strip.fin.neverInvoiced": "Not invoiced",
  "co.strip.fin.noConnection": "Accounting not connected",
  "co.strip.fin.unmapped": "Customer not matched",
  "co.strip.fin.syncing": "Syncing",
  "co.strip.fin.staleFigure": "Sync outdated",
  "co.strip.fin.notCurrent": "Not current",
  "co.strip.fin.errorFigure": "Last sync failed",
  "co.strip.fin.error": "Unavailable",
  "co.strip.fin.errorWhy": "Load failed",
  "co.strip.fin.noFigure": "No figure",
  "co.strip.fin.loading": "Loading…",
  "co.strip.unpriced": "No amount",
  "co.strip.pricedPartly": "{priced} of {total} priced",
  // The same word as the health receipt's Relationship dimension beside it,
  // and deliberately so: both read the correspondence — who wrote, how
  // recently, which side started it. Two nouns for that one reading is what
  // once let the tile say "One-sided" while the receipt said "Strong".
  "co.strip.health": "Relationship",
  "co.strip.healthOneSided": "One-sided",
  "co.strip.healthBalanced": "Balanced",
  "co.strip.replyShare": "{percent}% inbound",
  "co.strip.healthActive": "Active",
  "co.strip.lastTouch": "Last contact",
  "co.strip.lastTouch.today": "Today",
  "co.strip.lastTouch.ago": "{count} d",
  "co.strip.lastTouch.theirs": "Inbound",
  "co.strip.lastTouch.ours": "Outbound",
  "co.strip.lastTouch.never": "None",
  // Named for what the card READS. "Next" over a meeting date, on a card whose
  // door opened the task list, let a company with a due task and no meeting
  // booked read as a contradiction with itself.
  "co.strip.nextMeeting": "Next meeting",
  "co.strip.next.none": "None scheduled",
  "co.360.thread": "Activity",
  "co.360.threadCount": "Activity · {count}",
  "co.360.fullHistory": "Full history",
  "co.strip.healthQuiet": "Quiet",
  "co.strip.noInboundEver": "No inbound messages",
  "co.strip.unanswered": "Unanswered",
  "co.strip.unansweredDetail": "No reply · {days} d",
  "co.strip.engagement.never_contacted": "Never contacted",
  "co.strip.engagement.active": "Active",
  "co.strip.engagement.waiting_on_them": "Waiting on them",
  "co.strip.engagement.waiting_on_us": "Waiting on your team",
  "co.strip.engagement.dormant": "Quiet",
  "co.strip.openDeals": "{count} open",
  "co.strip.stalled": "{count} stalled",
  "co.suggest.act.draftReply": "Create draft",
  "co.suggest.act.openDeal": "Open deal",
  "co.suggest.act.addTask": "Add next step",
  // A conversation shown as one event says what it IS before what it
  // says: the reader is scanning for an event, not a sentence.
  "timeline.group.thread_other": "{count} messages",
  "timeline.group.thread_one": "{count} message",
  "timeline.group.bulk_other": "sent to {count} people",
  "timeline.group.bulk_one": "sent to {count} person",
  "timeline.group.expand": "Open",
  "timeline.group.collapse": "Close",
  "timeline.group.openThread": "View full thread",
  "timeline.group.mayContinue": "Earlier messages may exist",
  // A thread on the chronology is one card: the kind, the count and who was
  // on it over the subject, then each message with who wrote it and when.
  "timeline.group.kind": "Thread",
  "timeline.group.earlier_other": "Show {count} earlier messages",
  "timeline.group.earlier_one": "Show {count} earlier message",
  "timeline.group.hideEarlier": "Hide earlier messages",
  // Who a message in a thread is from, as an actor and a verb: "Ida Keller
  // wrote", "We sent to Ida Keller". "We" rather than a name, because a
  // captured mail names the other side and not which seat sent it.
  "timeline.thread.wrote": "wrote",
  "timeline.thread.you": "You",
  "timeline.thread.we": "Your team",
  "timeline.thread.them": "Them",
  "timeline.thread.sentTo": "sent to {who}",
  "timeline.thread.sent": "sent",
  "timeline.filters.kind": "Activity kind",
  "timeline.filters.kind.all": "All kinds",
  "timeline.filters.kind.email": "Email",
  "timeline.filters.kind.message": "Messages",
  "timeline.filters.kind.call": "Calls",
  "timeline.filters.kind.meeting": "Meetings",
  "timeline.filters.kind.note": "Notes",
  "timeline.filters.kind.task": "Tasks",
  "timeline.filters.search": "Search this timeline",
  "timeline.filters.from": "From",
  "timeline.filters.to": "To",
  "timeline.filters.searchOmitsLimited":
    "Search excludes threads whose content you cannot open.",
  "tab.contacts": "Contacts",
  "tab.deals": "Deals",
  "tab.dealRoom": "Deal Room",
  // Shared by the account's own tab and the lead's: both hold a deal AND the
  // projects beside it, unlike `tab.deals`, which the contact page's own
  // Deals tab uses for a list with no projects in it.
  "tab.dealsProjects": "Deals and projects",
  "tab.tasks": "Tasks",
  "tab.timeline": "History",
  "tab.finance": "Finance",
  "tab.network": "Network",
  "tab.documents": "Documents",
  "tab.profile": "Profile",
  "tab.meetings": "Meetings",
  "tab.research": "Data and tools",
  // The brief under the questions it answers, and what kind of claim each
  // sentence makes — a judgment must not read as a stored fact.
  "co.brief.nature.fact": "Fact",
  "co.brief.nature.assessment": "Assessment",
  "co.brief.nature.recommendation": "Suggested",
  // The rail's own details grid: the account's own fields, at a glance above
  // the collapsible sections.
  "co.details.title": "Details",
  "co.health.dim.relationship": "Relationship",
  "co.health.dim.commercial": "Commercial",
  "co.health.dim.payment": "Payment",
  // What each dimension weighs. Three words on a card cannot say what
  // "Commercial · Good" was read from, and a rating a reader cannot interpret
  // is one they have to take on trust.
  "co.health.means.relationship":
    "Whether contacts at this company are still in touch: who wrote, how recently and which side started.",
  "co.health.means.commercial":
    "Whether open deals are moving: their stages and how long each has been idle.",
  "co.health.means.payment":
    "Whether invoices are paid on time: what is overdue now and how late this company usually pays.",
  "co.health.rating.atRisk": "At risk",
  "co.health.rating.good": "Good",
  "co.health.rating.strong": "Strong",
  "co.health.payment.overdue": "Payment is overdue.",
  "co.health.payment.late": "Typically pays {days} days after due.",
  "co.health.payment.onTime": "Pays on time.",
  "company.partnerSetUp": "Set up partner program",
  "signal.kind.stalled_deal": "Deal stalled",
  "signal.kind.champion_left": "Champion left",
  "signal.kind.reengagement": "Re-engagement",
  "signal.kind.buying_intent": "Buying intent",
  "signal.kind.risk": "Risk",
  "signal.kind.other": "Other",
  "signal.kind.contract_ended": "Contract ending",
  "signal.kind.new_opportunity": "New opportunity",
  "signal.kind.commitment_made": "Commitment made",
  "signal.kind.ghosted_thread": "No reply",
  "signal.kind.project_gone_quiet": "Project gone quiet",
  "signal.kind.funding": "Funding",
  "signal.kind.leadership_change": "Leadership change",
  "signal.kind.expansion": "Expansion",
  "signal.kind.product_launch": "Product launch",
  "co.routeIn.band.strong": "in regular contact",
  "co.routeIn.band.some": "some contact",
  "co.routeIn.band.faint": "barely in contact",
  "co.routeIn.bandBadge.strong": "In regular contact",
  "co.routeIn.bandBadge.some": "Some contact",
  "co.routeIn.bandBadge.faint": "Barely in contact",
  "co.routeIn.bandBadge.unknown": "Contact on file, no pattern yet",
  "record.profile": "Profile",
  "record.context": "Context",
  "record.restsOn": "Sources",
  "record.restsOn.source_one": "source",
  "record.restsOn.source_other": "sources",
  "record.tabs": "Record tabs",
  "record.panel.showDetails": "Show details",
  "record.panel.hideDetails": "Hide details",
  // The Deal Room aside on a deal. `room.` rather than `dealroom.` for the
  // reason the other abbreviated namespaces give: the surface is named once at
  // the top of the panel and every key under it is read in that context.
  "room.editorial": "Documents and comments appear to buyers immediately.",
  "room.readOnly": "You can read this room but not change it.",
  "room.finished":
    "This room has ended. Its shared content is kept as a record.",
  "room.card.title": "Deal Room",
  "room.card.contacts": "{invited} invited · {active} signed in",
  "room.card.lastSeen": "Last seen by a buyer: {when}",
  "room.create.sub":
    "A page where the buyer opens shared documents by link and discusses them.",
  "room.create.open": "Open a Deal Room",
  "room.create.confirm": "Open",
  "room.create.titleLabel": "Room title",
  "room.create.titleHint":
    "The heading the buyer sees. You can change it later.",
  "room.create.defaultTitle": "{deal}",
  "roompage.none":
    "This deal has no Deal Room yet. Open one from the deal page.",
  "roompage.backToDeal": "← Back to deal",
  "roompage.accessMenu": "Room access",
  "roompage.pause": "Pause",
  "roompage.pauseHint":
    "Buyers keep their links but see a paused page until you resume.",
  "roompage.resume": "Resume",
  "roompage.close": "Close room",
  "roompage.closeHint": "Buyers can still read. Nothing new can be added.",
  "roompage.setExpiry": "Set end date",
  "roompage.setExpiryHint": "Access stops on that day.",
  "roompage.closeTitle": "Close this Deal Room?",
  "roompage.closeBody":
    "Buyers can still read the room. No document, comment or decision is accepted afterward. You can still revoke people and issue links.",
  "roompage.expiryLabel": "Access ends on",
  "roompage.expiryHint": "Leave empty for no end date.",
  "roompage.banner.paused":
    "Paused. Buyers see a paused page until you resume.",
  "roompage.banner.closed":
    "Closed. Buyers can still read the room; nothing more is accepted.",
  "roompage.banner.expired": "Expired. Buyer links no longer work.",
  "roompage.banner.archived": "Archived. No one can enter this room.",
  "roompage.banner.liveUntil": "Live. Access ends on {when}.",
  "roompage.text.title": "Title and welcome",
  "roompage.text.titleLabel": "Room title",
  "roompage.text.welcomeLabel": "Welcome message",
  "roompage.viewAsBuyer": "View as buyer",
  "roompage.previewArchived": "An archived room has nothing to preview.",
  // Deliberately does NOT say "only the owner". The preview also opens for a
  // manager, an admin and anybody holding a write grant on the deal, so naming
  // the owner would send a reader who already has the right access to ask the
  // wrong contact for it.
  "roompage.previewNotYours":
    "Your access to this deal does not include the buyer preview.",
  "access.title": "Access",
  "access.invite": "Invite",
  "access.empty": "No one has been invited yet.",
  "access.cap.view": "Read-only",
  "access.cap.viewHint": "Can read the documents and comments.",
  "access.cap.comment": "Read and comment",
  "access.cap.commentHint": "Can also ask questions and reply.",
  "access.state.invited": "invited",
  "access.state.active": "signed in",
  "access.state.revoked": "revoked",
  "access.state.revokedBadge": "Revoked",
  "access.lastSeen": "last seen {when}",
  "access.downloads": "Documents downloaded: {count}",
  "access.linkRequested":
    "Asked for a new link {when}. Issue one and send it yourself.",
  "access.rowActions": "Actions for {name}",
  "access.issueLink": "Issue new link",
  "access.changeCapability": "Change permissions",
  "access.revoke": "Revoke access",
  "access.inviteTitle": "Invite to Deal Room",
  "access.inviteConfirm": "Invite",
  "access.done": "Done",
  "access.save": "Save",
  "access.nameLabel": "Name",
  "access.emailLabel": "Email",
  "access.capabilityLegend": "Permissions",
  "access.inviteNote":
    "The link is shown for you to copy. If a mail relay is configured, it is also emailed, but delivery is not guaranteed.",
  "access.issued.title": "Link for {name}",
  "access.issued.mailed": "Sent to {email}. You can also copy it below.",
  "access.issued.notMailed":
    "The link was not mailed. Copy it and send it yourself.",
  "access.issued.mailedTitle": "Invitation email queued",
  "access.issued.notMailedTitle": "Invitation not emailed",
  "access.issued.linkLabel": "Invitation link",
  "access.issued.copy": "Copy link",
  "access.issued.copied": "Copied",
  "access.issued.copyFailed": "Select the link and copy it manually.",
  "access.issued.oneTime":
    "Personal, one-time link. It works once, on one device. Each person needs their own invitation.",
  "access.issueLinkTitle": "Issue a new link for {name}",
  "access.issueLinkBody":
    "Their current link stops working. The new link is shown for you to copy.",
  "access.revokeTitle": "Revoke access for {name}?",
  "access.neverSignedIn": "never signed in",
  "access.revokeBody":
    "Their session ends and their link stops working. Their comments stay visible and attributed. Requesting a new link does not restore access.",
  "access.changeCapabilityTitle": "Permissions for {name}",
  "contactdealrooms.title": "Deal Rooms",
  "contactdealrooms.open": "Open",
  "contactdealrooms.seatGone":
    "This address no longer holds a seat in that room.",
  "contactdealrooms.cut":
    "Only the first rooms are shown. This contact is in more rooms.",
  "contactdealrooms.revokeTitle": "Revoke access to {room}?",
  "room.state.draft": "Draft",
  "room.state.building": "Building",
  "room.state.ready": "Ready",
  "room.state.publishing": "Publishing",
  "room.state.live": "Live",
  "room.state.paused": "Paused",
  "room.state.closed": "Closed",
  "room.state.expired": "Expired",
  "room.state.archived": "Archived",
  // The later of the two directions \u2014 which side wrote last moved to the
  // daily brief's own detail line, so the header states only that the
  // relationship is or is not live.
  "co.pulse.owner": "Owner",
  "co.pulse.sizeBand": "{band} employees",
  "co.pulse.strongestLead": "Best route",
  "co.pulse.strengthTail_one": ", the only contact here",
  "co.pulse.strengthTail_other": ", of {count} contacts here",
  "co.pulse.unowned": "Unassigned",
  "co.since.first": "You have not opened this company before.",
  "co.partial":
    "Some sections could not be loaded, so this page may be incomplete.",
  "evidence.explain": "Where “{value}” came from",
  "evidence.fullHistory": "Full history",
  "co.section.unavailable":
    "Some data did not load, so this section may be incomplete.",
  "billing.title": "Billing contacts",
  "billing.contactTitle": "Billing roles",
  "billing.none":
    "No billing contacts yet. Add the contact that invoices are addressed to.",
  "billing.noEmail": "No email recorded",
  "billing.role.recipient": "Invoice recipient",
  "billing.role.approver": "Approver",
  "billing.role.accountsPayable": "Accounts payable",
  "billing.add": "Add contact",
  "billing.change": "Change",
  "billing.remove": "Remove",
  "billing.changeOne": "Change {who}’s role",
  "billing.removeOne": "Remove {who} from this company’s invoices",
  "billing.addTitle": "Add billing contact",
  "billing.changeTitle": "Change billing role",
  "billing.who": "Contact",
  "billing.findContact": "Find a contact",
  "billing.role": "Role",
  "billing.roleNote": "A contact can hold each role once for this company.",
  "billing.saveAdd": "Add contact",
  "billing.saveChange": "Save change",
  "billing.versionUnresolved":
    "The billing contact could not be read back, so the change was not sent. Reload and retry.",
  "finance.title": "Finance",
  "finance.titleHistorical": "Finance · historical",
  "finance.none": "Nothing recorded.",
  "finance.loading": "Loading invoices…",
  "finance.syncing":
    "Syncing with the accounting system. Figures appear after the first sync.",
  "finance.noConnection":
    "No accounting system connected. Connect one to see invoices and payment behavior for this customer.",
  "finance.unmapped":
    "Connected, but this company is not yet matched to a customer in the accounting system.",
  "finance.netInvoiced": "Net invoiced · 12 months",
  "finance.coveragePeriod": "Invoices issued {from} to {to}",
  "finance.overdueRelationshipEnded":
    "Hidden because the relationship has ended.",
  "finance.overdue": "Overdue",
  "finance.behaviour": "Payment behavior",
  "finance.behaviourShape": "Days late per settled invoice, oldest first",
  "finance.shareOfOpen": "{percent}% of open balance",
  "finance.overdueShareLabel": "Overdue share of the open balance",
  "finance.legendOverdue": "Overdue {amount}",
  "finance.legendOpen": "Open {amount}",
  "finance.medianAfterDue": "Typically {days} days after due",
  "finance.medianEarly": "Typically {days} days early",
  "finance.col.invoice": "Invoice",
  "finance.col.dates": "Issued → due",
  "finance.recentInvoices": "Recent invoices",
  "finance.paidOn": "paid {when}",
  "finance.paidDaysLate_one": "Paid 1 day late",
  "finance.paidDaysLate_other": "Paid {days} days late",
  "finance.overdueDays_one": "{days} day overdue",
  "finance.overdueDays_other": "{days} days overdue",
  "finance.col.amount": "Amount",
  "finance.col.status": "Status",
  "finance.unnumbered": "No number",
  "finance.moreInvoices": "More invoices in the accounting system",
  "finance.connect": "Connect finance",
  "finance.syncedFrom": "From {provider} · synced {when}",
  "finance.fromNeverSynced": "From {provider} · not yet synced",
  "finance.status.draft": "Draft",
  "finance.status.open": "Open",
  "finance.status.partiallyPaid": "Partially paid",
  "finance.status.paid": "Paid",
  "finance.status.overdue": "Overdue",
  "finance.status.disputed": "Disputed",
  "finance.status.credited": "Credited",
  "finance.status.void": "Void",
  "commercial.closes": "closes {when}",
  "contracts.title": "Contracts",
  "contracts.empty": "No contracts yet",
  "contracts.noneActive": "No contract active today",
  "contracts.filter.all": "All",
  "contracts.filter.active": "Active",
  "contracts.status.draft": "Draft",
  "contracts.status.active": "Active",
  "contracts.status.expired": "Expired",
  "contracts.status.cancelled": "Canceled",
  "contracts.status.superseded": "Superseded",
  "contracts.endsOn": "Ends {when}",
  "contracts.renewsOn": "Renews {when}",
  "contracts.endedPendingStatus": "Term ended, status pending",
  "contracts.form.title": "Record contract",
  "contracts.form.name": "Title",
  "contracts.form.number": "Contract number",
  "contracts.form.value": "Value",
  "contracts.form.basis": "This value is",
  "contracts.form.arr": "Annual recurring revenue",
  "contracts.form.arrMonthly": "Monthly equivalent:",
  // The two agreement terms a READ surface states. "Net 30" and "Due on
  // receipt" are what the paper says; the form's own labels end in "(days)"
  // because they name an input, and a reading that said so would be odd.
  "contracts.terms.net": "Net {days}",
  "contracts.terms.onReceipt": "Due on receipt",
  "contracts.terms.arr": "{amount}/year",
  "contracts.terms.monthly": "({amount}/month)",
  "contracts.basis.total": "the total for the whole term",
  "contracts.basis.annual": "12 months of an open-ended contract",
  "contracts.form.startsOn": "Starts",
  "contracts.form.endsOn": "Ends",
  "contracts.form.endsOnHint": "Leave empty for an open-ended contract.",
  "contracts.form.renewalOn": "Renews",
  "contracts.form.noticeDays": "Notice period (days)",
  "contracts.form.noticeDaysHint":
    "Notice period for a cancellation. The renewal warning fires before this deadline, not the renewal date.",
  "contracts.form.paymentTerms": "Payment terms (days)",
  "contracts.form.paymentTermsHint":
    "Days the customer has to pay. 0 means due on receipt; leave empty if no terms are agreed.",
  "contracts.form.signedOn": "Signed",
  "contracts.form.signedOnHint":
    "Only when someone confirms the signature. Never taken from the deal’s close date.",
  "contracts.form.save": "Record contract",
  "contracts.form.errNoName": "Enter a contract title.",
  "contracts.form.errTermOrder": "A term cannot end before it starts.",
  "contracts.add": "Add contract",
  "contracts.rowMenu": "Contract actions",
  "contracts.renew.title": "Renew contract",
  "contracts.renew.hint":
    "Creates a new contract with its own terms and marks this one superseded. Only the counterparty is kept.",
  "contracts.renew.deal": "Deal",
  "contracts.renew.dealHint":
    "The deal that won this term, if any. Never the previous contract’s deal.",
  "contracts.renew.dealNone": "No deal",
  "contracts.renew.dealWithheldCompany":
    "You cannot open this contract’s company, so its deals cannot be listed. The renewal keeps the same counterparty and records no deal.",
  "contracts.renew.dealWithheldTitle": "No deal available",
  "contracts.renew.submit": "Renew",
  "contracts.deal": "Deal",
  "contracts.statusChange.title": "Change status",
  "contracts.statusChange.label": "New status",
  "contracts.statusChange.submit": "Change status",
  "contracts.statusChange.errSame": "The contract already has this status.",
  "contracts.cancel.title": "Record cancellation",
  "contracts.cancel.hint":
    "The customer stays under contract until the effective date. This records notice and does not change the status.",
  "contracts.cancel.noticeOn": "Notice given",
  "contracts.cancel.effectiveOn": "Takes effect",
  "contracts.cancel.effectiveOnHint":
    "Not after the term ends, and not before the notice date.",
  "contracts.cancel.submit": "Record cancellation",
  "contracts.cancel.menuLabel": "Cancel contract",
  "contracts.cancel.errIncomplete": "Enter both dates.",
  "contracts.cancel.errOrder":
    "Cancellation cannot take effect before notice was given.",
  "contracts.cancel.errTermEnd":
    "Cancellation cannot take effect after the term ends.",
  "contracts.value.perYear": "per year",
  "contracts.value.total": "for the whole term",
  "contracts.files": "Files",
  "contracts.noTerm": "No dates recorded",
  "contracts.openStart": "Open start",
  "contracts.openEnd": "Open-ended",
  "contracts.edit": "Edit",
  "contracts.archive": "Archive",
  "contracts.archive.title": "Archive this contract?",
  "contracts.archive.body":
    "“{title}” leaves the lists and the company totals. The record and its history are kept; nothing is deleted.",
  "contracts.archive.confirm": "Archive",
  "contracts.form.editTitle": "Edit contract",
  "contracts.form.saveEdit": "Save changes",
  "contracts.form.file": "Signed document",
  "contracts.form.fileHint":
    "Drop the signed PDF here, or click to choose it. It is filed on this contract and appears in the company’s documents.",
  "contracts.form.fileEmpty": "Drop a file here or click to choose",
  "contracts.form.fileAdd": "Drop another file here or click to choose",
  "contracts.perYear": "{amount}/year",
  "contracts.state.title": "Under contract · {count} active",
  "contracts.state.none": "No contract on record",
  "contracts.state.renewsOn": "Renews {when}",
  "contracts.state.endsOn": "Canceled, ends {when}",
  "contracts.state.partial": "{priced} of {total} priced",
  "commercial.lastOffer": "Last offer · {deal}",
  "commercial.offerUnnumbered": "Offer",
  "commercial.validUntil": "valid until {when}",
  "commercial.offer.draft": "Draft",
  "commercial.offer.sent": "Sent",
  "commercial.offer.accepted": "Accepted",
  "commercial.offer.rejected": "Rejected",
  "commercial.offer.expired": "Expired",
  "commercial.offer.superseded": "Superseded",
  "co.section.restricted": "Hidden for your role",
  // The same word the tab strip uses (`tab.tasks`): a tab called Tasks
  // opening a card called Next steps reads as a wrong turn.
  "co.next.title": "Tasks",
  "co.next.empty": "No open tasks for this company.",
  "co.next.overdue": "Overdue",
  "co.next.due": "Due {when}",
  "co.next.undated": "No due date",
  "co.facts.pipeline": "Open deals",
  "co.facts.inFlight": "In flight",
  "co.facts.reading": "Loading…",
  "co.facts.noDeals": "No open deals",
  "co.facts.unpriced": "Not priced yet",
  "co.facts.nothing": "Nothing",
  "co.facts.deals_one": "1 deal",
  "co.facts.deals_other": "{count} deals",
  "co.facts.projects_one": "1 project",
  "co.facts.projects_other": "{count} projects",
  "co.facts.atLeast": "or more",
  "co.work.noDeals": "No open deals.",
  "co.work.closes": "closes {date}",
  "co.brief.by.model": "Written by Margince",
  "co.brief.by.deterministic": "Compiled from CRM records",
  "co.brief.generatedAt": "as of {when}",
  "co.growthFit.title": "Growth fit",
  "co.growthFit.unavailable":
    "Assessment could not be loaded. The company record is unchanged.",
  "co.growthFit.assembling":
    "Assessing this company. The first assessment reads the whole record and can take a minute.",
  "co.growthFit.reassess": "Assess again",
  "co.growthFit.reassessing": "Assessing…",
  "co.growthFit.band.strong": "Strong fit",
  "co.growthFit.dim.industryFit": "Industry fit",
  "co.growthFit.dim.companySize": "Company size",
  "co.growthFit.dim.transformationNeed": "Transformation need",
  "co.growthFit.dim.access": "Access",
  "co.growthFit.band.moderate": "Moderate fit",
  "co.growthFit.band.weak": "Weak fit",
  "co.growthFit.band.unknown": "Not enough to judge",
  "co.growthFit.completeness": "{present} of {expected} inputs recorded",
  "co.growthFit.missing": "Still missing",
  "co.growthFit.capped": "Held back: {reason}.",
  "co.growthFit.nextStep": "Next: {step}.",
  "co.growthFit.positive": "Supporting factors",
  "co.growthFit.negative": "Limiting factors",
  "co.growthFit.whitespace": "Sales potential",
  "co.growthFit.objections": "Likely objections",
  "co.growthFit.angle": "Suggested approach",
  "co.dossier.title": "Company overview",
  "co.dossier.unavailable":
    "Overview could not be loaded. The company record is unchanged.",
  "co.dossier.empty":
    "Nothing recorded about this company yet. Read its website, or complete the profile below.",
  "co.dossier.stale": "Last read over a month ago",
  "co.dossier.rewrite": "Regenerate",
  "co.dossier.rewriting": "Regenerating…",
  "co.dossier.section.summary": "Summary",
  "co.dossier.section.products_services": "Products and services",
  "co.dossier.section.markets": "Markets",
  "co.dossier.section.buying_center": "Buying center",
  "co.dossier.section.differentiation": "Differentiation",
  "co.dossier.section.firmographics": "Size, age and registration",
  "co.evidence.unavailable":
    "Source could not be loaded. The record is unchanged.",
  "co.evidence.producedBy": "recorded by {who}",
  "co.evidence.retrievedAt": "Read {when}",
  "co.evidence.verifiedAt": "Confirmed by a person {when}",
  "co.evidence.confidence": "Model confidence {percent}%",
  "co.evidence.gaps": "Not recorded: {fields}.",
  "co.evidence.kind.site_read": "Read from company website",
  "co.evidence.kind.connector": "From a connected system",
  "co.evidence.kind.human": "Entered by a person",
  "co.evidence.kind.migration": "Imported",
  "co.evidence.kind.rule": "Derived",
  "co.brief.cite.deal": "deal",
  "co.brief.cite.activity": "activity",
  "co.brief.cite.contact": "contact",
  "co.brief.cite.company": "company",
  "co.brief.cite.fact": "fact",
  "co.brief.cite.profile_field": "profile field",
  // Several sources of one kind that have no screen to open collapse into one
  // counted chip, rather than a run of identical labels.
  "co.brief.cite.deal.many": "{count} deals",
  "co.brief.cite.activity.many": "{count} activities",
  "co.brief.cite.contact.many": "{count} contacts",
  "co.brief.cite.company.many": "{count} companies",
  "co.brief.cite.fact.many": "{count} facts",
  "co.brief.cite.profile_field.many": "{count} profile fields",
  "approval.kind.advance_deal": "Advance deal",
  "approval.kind.close_date_correction": "Correct close date",
  "approval.kind.deal_follow_up": "Add deal follow-up",
  "approval.kind.promote_lead": "Qualify lead",
  "approval.kind.archive_record": "Archive record",
  "approval.kind.merge_records": "Merge records",
  "approval.kind.merge_tags": "Merge tags",
  "approval.kind.update_record": "Update record",
  "approval.kind.create_record": "Create record",
  "approval.kind.send_email": "Send email",
  "approval.kind.communication_review": "Review refused email",
  "approval.kind.held_draft": "Review drafted email",
  "approval.kind.book_meeting": "Book meeting",
  "approval.kind.volume_release": "Allow agent to continue",
  "approval.kind.coldstart": "Fill in new company",
  "approval.kind.enrich": "Enrich from web",
  "approval.kind.deepread": "Read company website",
  "approval.kind.linkedin_match": "LinkedIn match",
  "approval.kind.site_lead": "Add contact from website",
  "approval.kind.capture_counterparty": "Add contact from mail",
  "approval.kind.company_name_promotion": "Rename company",
  "approval.kind.vcard_create": "Create contact from card",
  "approval.kind.lifecycle_change": "Lifecycle stage",
  "approval.kind.stage_progression": "Advance deal stage",
  "approval.field.because": "Reason",
  "approval.field.from_stage": "From",
  "approval.field.to_stage": "To",
  "approval.kind.transcript_proposal": "Add next step from transcript",
  "approval.kind.fx_rate_proposal": "Update exchange rates",
  "approval.kind.ai_model_rate_proposal": "Update model prices",
  "approval.kind.disqualify_lead": "Disqualify lead",
  "approval.kind.demote_lead": "Reverse lead qualification",
  "approval.kind.advance_project_phase": "Advance project phase",
  "approval.kind.assign_owner": "Assign owner",
  "approval.kind.commit_import": "Commit import",
  "approval.kind.emit_flow_event": "Record automation step",
  "approval.kind.relink_activity": "Refile activity",
  "approval.kind.relink_thread": "Refile thread",
  "approval.kind.relink_activities": "Refile activities",
  "approval.kind.scheduled_send_held": "Release held message",
  "approval.kind.send_company_email": "Email company",
  "approval.kind.send_message": "Send message",
  // What a staged proposal's own fields are CALLED. Without these a card falls
  // back to the payload's JSON keys, and a business question reads as a
  // database row.
  "approval.field.basis": "Reason",
  "approval.field.step": "Step",
  "approval.field.intent": "Draft reason",
  "approval.field.evidence_snippet": "Page excerpt",
  "approval.field.previous_close_date": "Current close date",
  "approval.field.expected_close_date": "Proposed date",
  "approval.field.due_date": "Due",
  "approval.field.scheduled_at": "Scheduled for",
  "approval.field.flags": "Issues",
  "approval.field.closeDateFlag.overdue": "date has passed",
  "approval.field.closeDateFlag.missing": "no date set",
  "approval.field.closeDateFlag.unrealistic_soon": "unrealistically soon",
  "approval.field.closeDateFlag.unrealistic_stale": "no recent progress",
  "approval.field.name": "Name",
  "approval.field.role": "Role",
  "approval.field.email": "Email",
  "approval.field.domain": "Domain",
  "approval.field.company": "Company",
  "approval.field.title": "Job title",
  "approval.field.leadNameNow": "Current name",
  "approval.field.capturedName": "Name in the message",
  "approval.field.leadCompanyNow": "Current company",
  "approval.field.capturedCompany": "Company in the message",
  "approval.field.leadTitleNow": "Current job title",
  "approval.field.capturedTitle": "Job title in the message",
  "approval.field.phone": "Phone",
  "approval.field.url": "Website",
  "approval.field.address": "Address",
  "approval.field.published_email": "Email on page",
  "approval.field.connection_name": "Name on LinkedIn",
  "approval.field.connection_company": "Company on LinkedIn",
  "approval.field.contact_name": "Contact in CRM",
  "approval.field.owner": "Owner",
  "approval.field.to": "To",
  "approval.field.currency": "Currency",
  "approval.field.rate": "New rate",
  "approval.field.prior_rate": "Current rate",
  "approval.field.provider": "Provider",
  "approval.field.model": "Model",
  "approval.field.input_per_mtok": "Input, per million tokens",
  "approval.field.output_per_mtok": "Output, per million tokens",
  "approval.field.tool": "Tool",
  "approval.field.observed": "Used",
  "approval.field.limit": "Limit",
  "approval.field.allowance": "Requested",
  "co.assistant.title": "Ask about this company",
  "co.assistant.aiTag": "AI-assisted",
  "co.decisions.open": "Review {count} pending",
  "co.decisions.title": "Pending approvals",
  "co.decisions.group": "{count} × {kind}",
  "co.decisions.empty": "No pending approvals for this company.",
  "co.ask.title": "Ask Margince",
  "co.ask.q.whats_open": "What is open here?",
  "co.ask.q.meeting_prep": "Prepare for a meeting",
  "co.ask.q.whats_changed": "What changed recently?",
  "co.ask.nothing": "No records you can access answer that question.",
  "co.ask.failed": "The question could not be answered. Retry.",
  "co.suggest.title": "Margince suggests",
  "co.suggest.kind.no_reply": "No reply",
  "co.suggest.kind.stalled_deal": "Stalled deal",
  "co.suggest.kind.no_next_step": "No next step",
  "co.suggest.kind.lifecycle_conflict": "Lifecycle conflict",
  "co.suggest.kind.commitment_unmet": "Commitment unmet",
  "co.suggest.kind.question_unanswered": "Question unanswered",
  "co.suggest.kind.risk_raised": "Risk raised",
  "co.suggest.kind.need_raised": "Need raised",
  "co.suggest.more": "{count} more not shown.",
  "co.suggest.basedOn": "Based on",
  "co.cite.open": "Open record",
  "co.suggest.dismiss": "Not now",
  "co.suggest.byline": "Margince suggests",
  "co.suggest.dismissFailed": "Suggestion was not dismissed. Retry.",
  // Said plainly, because the reader's next move depends on it: a step they
  // believe was written is a step nobody goes looking for again.
  "co.suggest.addTaskFailed": "Next step was not saved. Retry.",
  "co.suggest.viewTasks": "View tasks",
  "co.suggest.commitment.overdueCount": "{count} overdue",
  "co.suggest.commitment.openCount": "{count} open",
  "co.suggest.commitment.overdueAtLeast": "{count}+ overdue",
  "co.suggest.commitment.openAtLeast": "{count}+ open",
  "co.deals.title": "Deals",
  "co.deals.empty": "No open deals for this company.",
  "co.deals.wonLifetime": "Won to date",
  "co.deals.lostCount": "{count} lost",
  "co.deals.noStage": "No stage",
  "co.rail.all": "All {count}",
  "co.rail.add": "Add",
  "co.rail.allUncounted": "All",
  "co.rail.deals.title": "Active deals",
  "co.rail.deals.empty": "No deals for this company yet.",
  "co.rail.deals.emptyClosedOnly": "No open deals, only closed ones.",
  "co.rail.deals.noCloseDate": "no close date",
  "co.rail.deals.attentionOverdue": "Overdue",
  "co.rail.deals.attentionCommitment": "Buyer commitment due",
  "co.rail.contacts.title": "Key contacts",
  "co.rail.contacts.empty": "No contacts yet.",
  "co.rail.contacts.add": "Add contact",
  "co.rail.contacts.inTouch": "Already in touch",
  "co.rail.projects.title": "Projects",
  "co.rail.projects.empty": "No projects yet.",

  "co.commercial.title": "Commercial",
  "co.commercial.lostFigure": "Lost deals",
  "co.commercial.allDeals": "All deals",
  "co.commercial.truncated":
    "More open deals than fit here. Open All deals to see the rest.",
  "linkedinImport.title": "LinkedIn connections",
  "linkedinImport.sub":
    "Import your LinkedIn export to see who your team already knows.",
  "linkedinImport.profileLabel": "Your LinkedIn profile URL",
  "linkedinImport.profilePlaceholder": "https://www.linkedin.com/in/…",
  "linkedinImport.saveProfile": "Save profile",
  "linkedinImport.saveFailed": "Profile URL was not saved. Retry.",
  "linkedinImport.profileReadFailed": "Your profile URL could not be loaded.",
  "linkedinImport.importFailed": "The export was not imported. Retry.",
  "linkedinImport.editProfile": "Edit",
  "linkedinImport.editProfileTitle": "Your LinkedIn profile",
  "linkedinImport.profileNotSet": "Not recorded yet",
  "linkedinImport.connectedNote":
    "Connected. Imported connections are attributed to this profile, so the CRM shows which colleague knows someone.",
  "linkedinImport.notConnectedNote":
    "Saving your profile URL attributes imported connections to you by name.",
  "linkedinImport.whichFile":
    "LinkedIn provides Connections.csv under “Settings”, then “Data privacy”, then “Get a copy of your data”. Upload that file from the archive. Uploaded connections never become contacts: they stay out of search, lists and contact pages, and no one can write to or email them.",
  "linkedinImport.choose": "Choose Connections.csv",
  "linkedinImport.importLabel": "Connections export",
  "linkedinImport.noMatchesYet":
    "No matches yet, which is normal for a new company. Connections are matched against contacts as mail is read, and matching reruns every hour.",
  "linkedinImport.working": "Reading export…",
  "linkedinImport.imported": "Connections imported",
  "linkedinImport.confirmed": "Matched to a contact",
  "linkedinImport.suggested": "Awaiting confirmation",

  // The review queue and the reach table (ADR-0078 §2.1b).
  "linkedinReach.title": "Network reach",
  "linkedinReach.sub":
    "Companies on file where you know someone, most connections first.",
  "linkedinReach.readFailed": "Network reach could not be loaded.",
  "linkedinReach.empty":
    "None of your connections work at a company on file yet.",
  "linkedinReach.allUnresolved":
    "All {unresolved} of your connections work at companies not on file yet.",
  "linkedinReach.accountsLabel": "Companies you reach",
  "linkedinReach.account": "Company",
  "linkedinReach.connections": "Connections",
  "linkedinReach.onFile": "Already on file",
  "linkedinReach.onFileOf": "{onFile} of {total}",
  "linkedinReach.footnote":
    "Showing {shown} of {total} companies. {unresolved} connections work at companies not on file yet.",
  "linkedinImport.skipped": "Rows skipped (no usable name)",
  "co.signals.title": "Signals",
  "co.signals.emptyDetail":
    "Margince reads meetings, mail and invoices for commitments, blockers and risks. It needs at least one of these sources.",
  "co.signals.empty": "No open signals for this company.",
  "co.signals.openProject": "Open project",
  "co.signals.openSource": "Read announcement",
  "chronology.label": "Timeline filter",
  "chronology.activities": "Activities",
  "chronology.changes": "Changes",
  "filter.label": "Filter",
  "chronology.all": "All",
  "chronology.conversations": "Threads",
  "chronology.conversationsEmpty": "No threads yet.",
  "convo.yourMove": "Needs reply",
  "convo.waitingOnThem": "Awaiting their reply",
  "chronology.changesEmpty":
    "No field has changed since this record was created.",
  "chronology.allEmpty": "No activity on this record yet.",
  "chronology.truncated":
    "Older entries are not shown because there are too many to order together. Select Activities or Changes to see further back.",
  "chronology.truncatedActivities":
    "Only the most recent activities are shown.",
  "timeline.sentTo": "Sent to {who}",
  "timeline.receivedFrom": "From {who}",
  "timeline.withWhom": "With {who}",
  "timeline.fieldUpdated": "field updated",
  "timeline.sent": "Sent",
  "timeline.received": "Received",
  "timeline.kind.email": "Email",
  "timeline.kind.meeting": "Meeting",
  "timeline.kind.note": "Note",
  "timeline.kind.call": "Call",
  "timeline.kind.task": "Task",
  "timeline.kind.message": "Message",
  "timeline.kind.change": "Record",
  "timeline.withheld": "Content for participants only",
  "compose.deadRecipients":
    "Mail to {addresses} is bouncing: the last delivery was refused and none has succeeded since. Send anyway, or use another address.",
  "compose.threadShare": "Share with the company",
  "compose.threadMakePrivate": "Make private",
  "compose.threadScope": "Applies to the whole thread.",
  "compose.threadStillHeld":
    "Other seats that have not shared this thread: {count}",
  "compose.reason.posture": "Held by your setting",
  "compose.reason.workspaceFloor": "Held by the company",
  "compose.reason.noRecord": "Held: no record",
  "compose.reason.pendingVerdict": "Held until classified",
  "compose.reason.manual": "Kept private",
  "compose.reason.verdict": "Held by classification",
  "compose.reason.counterparty": "Held: counterparty mail",
  "compose.reason.explicitlyConfidential": "Marked confidential",
  "compose.reason.noCounterparty": "Held: no counterparty",
  "compose.audience": "Change visibility",
  "compose.audienceTitle": "Who may read this message?",
  "compose.audienceLegend": "Message visibility",
  // The canonical email row and its detail. "Team" never means the whole
  // workspace: who may discover the linked record still decides who sees the
  // row at all, so the word is about the audience and not the population.
  "email.aMessage": "A message",
  "email.noSubject": "No subject",
  "email.withheldSubject": "Not shared with you",
  "email.receivedFrom": "Received from {who}",
  "email.received": "Received",
  "email.sentTo": "Sent to {who}",
  "email.sent": "Sent",
  "email.outgoingTo": "Outgoing to {who}",
  "email.outgoing": "Outgoing",
  "email.sendingTo": "Sending to {who}",
  "email.sending": "Sending",
  "email.notSentTo": "Not sent to {who}",
  "email.notSent": "Not sent",
  "email.bouncedFrom": "Did not reach {who}",
  "email.bounced": "Did not arrive",
  "email.access.sentence.workspace": "Everyone in the company can read this.",
  "email.access.sentence.team":
    "Everyone who can open the records this is filed against can read it.",
  "email.access.sentence.participants":
    "Only the people on this message can read it.",
  "email.access.sentence.selected":
    "Only the people named below can read this.",
  // The one mark for who may read a thing, drawn on mail rows, in the drawer,
  // on a contact and on a limited note. "Team" never means the whole
  // workspace (see above); "Only you" is a captured contact's owner-only
  // state, which a message never has.
  "visibility.team": "Team",
  // A shared company or contact, where the audience IS the whole workspace —
  // no linked record narrows it the way one narrows a message. "Shared" pairs
  // with the verb beside it ("Make private"), which is how a reader tells the
  // two apart without reading the tooltip.
  "visibility.workspace": "Shared",
  "visibility.participants": "Participants",
  "visibility.selected": "Selected",
  "visibility.private": "Only you",
  "visibility.withheld": "Withheld",
  "email.access.unnamedMember": "Former user",
  "email.move.needsReply": "Needs reply",
  "email.move.waitingForThem": "Awaiting their reply",
  "email.detail.loading": "Loading message…",
  "email.detail.none": "This message",
  "email.detail.attachments_one": "{count} attachment",
  "email.detail.attachments_other": "{count} attachments",
  "email.detail.showQuoted": "Show quoted history",
  "email.detail.withheldReason": "This message is not shared with you",
  "email.detail.from": "From",
  "email.detail.to": "To",
  "email.detail.cc": "Cc",
  "email.detail.filedUnder": "Filed under",
  "email.detail.when": "Sent",
  "email.detail.bccWithheld":
    "Some recipients were blind-copied and are not shown.",
  "compose.audienceWorkspace": "Everyone in the company",
  "compose.audienceWorkspaceHint":
    "Anyone who can open the records this is filed against can read this message.",
  "compose.audienceParticipants": "Participants only",
  "compose.audienceParticipantsHint":
    "Only the people on this message can read its subject and body. Others see only that a message was exchanged that day.",
  "compose.audienceSelected": "Named people",
  "compose.audienceSelectedHint":
    "Only the people and teams you name, plus anyone already on the message.",
  "compose.audienceMembersLegend": "Who may read it",
  "compose.audienceMembersLoading": "Loading people…",
  "compose.audienceConfirm": "Save visibility",
  "compose.audienceNote":
    "Applies to this message only, not to the thread or the contact.",
  "timeline.textMore": "Show more",
  "timeline.textLess": "Show less",
  "timeline.tailMore": "Show signature and quoted text",
  "timeline.tailLess": "Hide signature and quoted text",
  "co.profileField.display_name": "Company name",
  "co.profileField.offer_summary": "Offering",
  "co.profileField.icp": "Ideal customer profile",
  "co.profileField.buying_center": "Buying center",
  "co.profileField.value_proposition": "Value proposition",
  "co.profileField.usp": "Differentiation",
  "co.profileField.customer_pains": "Customer pains",
  "co.profileField.desired_outcomes": "Desired outcomes",
  "co.profileField.buying_intents": "Purchase triggers",
  "co.profileField.common_objections": "Common objections",
  "co.profileField.sales_motion": "Sales motion",
  "co.profileField.legal_name": "Registered legal name",
  "co.profileField.registered_address": "Registered address",
  "co.profileField.register_vat": "Register / VAT ID",
  "co.profileField.legal_form": "Legal form",
  "co.profileField.register_court": "Register court",
  "co.profileField.register_number": "Register number",
  "co.profileField.industry": "Industry",
  "co.profileField.history": "History",
  "co.narrative.title": "Description",
  "co.narrative.add": "Add",
  "co.contacts.engagement": "Engagement",
  "co.contacts.lastInteraction": "Last exchange",
  "co.contacts.strength": "Relationship",
  "co.contacts.neverInTouch": "No exchange yet",
  "co.contacts.theyWrote": "They wrote",
  "co.contacts.weWrote": "Your team wrote",
  "co.contacts.filter.status": "Engagement",
  "co.contacts.filter.statusAll": "Any engagement",
  "co.contacts.band.wayIn": "Best route",
  "co.contacts.band.noWayIn": "None",
  "co.contacts.band.noWayInWhy": "Contacted · no replies",
  "co.contacts.band.committee": "Buying committee",
  "co.contacts.band.missing": "No {role}",
  "co.contacts.band.committeeComplete": "Complete",
  "co.contacts.band.committeeUnread": "Restricted",
  "co.contacts.band.committeeUnreadWhy": "Deals not visible",
  "co.contacts.band.seatsHeld": "{count} on the team",
  "co.contacts.band.someHidden": "{count} hidden",
  "co.contacts.band.coverage": "Coverage",
  "co.contacts.band.reachable": "{count} of {total}",
  "co.contacts.band.untried":
    "{count} not contacted · {waiting} awaiting your reply",
  "co.contacts.board.nobodyHolds": "No one holds this role",
  "co.contacts.band.noOpenDeal": "No open deal",
  "co.contacts.band.noOpenDealWhy": "Roles are set per deal",
  "co.contacts.band.committeePartial": "Partly hidden",
  "co.contacts.board.otherRoles": "Other roles",
  "co.contacts.band.unavailable": "Unavailable",
  "co.contacts.band.unavailableWhy": "Load failed · list unaffected",
  "co.contacts.view": "Buying committee view",
  "co.contacts.view.board": "Board",
  "co.contacts.view.map": "Map",
  "co.contacts.map.region": "Contact map for this company",
  "co.contacts.map.bestRoute": "Best route",
  "co.contacts.map.alternatives": "Alternatives",
  "co.contacts.map.noRoute": "No route recorded",
  "co.contacts.map.more": "Show {count} more",
  "co.contacts.map.clear": "Clear selection",
  "co.contacts.map.emptyTitle": "No route recorded yet",
  "co.contacts.map.emptyBody":
    "Assign buying roles, or import this company’s existing interactions.",
  "co.contacts.map.nothingSelected":
    "Select a contact to see the best route to them.",
  "co.contacts.map.ourSide": "Your team",
  "co.contacts.map.account": "Company",
  "co.contacts.map.missing": "{role} missing",
  "co.contacts.map.awaiting": "awaiting reply",
  "co.contacts.map.owed": "reply owed",
  "co.contacts.map.replied": "they replied",
  "co.contacts.map.never": "never contacted",
  "co.contacts.map.onDeal": "on the deal",
  "co.contacts.map.routesWithheld":
    "Routes to this contact are hidden from you",
  "co.contacts.map.assignHint": "No one owns this deal",
  "co.contacts.map.scope":
    "{count} on the buying committee · selected deal only.",
  "co.contacts.map.scopePartial":
    "{count} on the buying committee · {hidden} more you cannot see.",
  "co.contacts.board.readFromMessages": "Read from their messages",
  "co.intro.title": "Request introduction",
  "co.intro.who": "Asking {colleague} to introduce you to {contact}.",
  "co.intro.write": "Draft message",
  "co.intro.writing": "Writing…",
  "co.intro.fromTemplate":
    "Written from a template. This installation has no model configured.",
  "co.intro.subject": "Subject",
  "co.intro.body": "Message",
  "co.intro.basedOn": "Based on",
  "co.intro.copy": "Copy",
  "co.intro.copyFailed":
    "Copy failed. Select the message and copy it manually.",
  "co.intro.copied": "Copied",
  "co.intro.openMail": "Open in mail app",
  "co.map.askIntro": "Request introduction",
  "co.contacts.board.suggest": "Suggest roles",
  "co.contacts.board.suggesting": "Reading messages…",
  "co.contacts.board.suggestNoDeal":
    "Roles are set per deal, and this company has no open deal.",
  "co.contacts.board.suggestWrote":
    "Roles assigned from their messages: {count}",
  "co.contacts.board.suggestUnavailable":
    "Role suggestions need a model, and this installation has none configured.",
  "co.contacts.board.suggestNothing": "Their messages do not show who buys.",
  "co.contacts.board.suggestRefused":
    "Nothing was clear enough to record. Suggestions dropped for weak evidence: {count}.",
  "co.contacts.board.confirm": "Confirm",
  "co.contacts.board.confirming": "Confirming…",
  "co.contacts.board.change": "Change role",
  "co.reach.waiting": "Needs reply",
  "co.reach.answered": "Answered",
  "co.reach.silent": "No reply",
  "co.reach.lapsed": "Went quiet",
  "co.reach.untried": "Not contacted",
  "co.role.champion": "champion",
  "co.role.economic_buyer": "economic buyer",
  "co.role.blocker": "blocker",
  "co.role.influencer": "influencer",
  "co.role.user": "end user",
  "co.roleLabel.champion": "Champion",
  "co.roleLabel.economic_buyer": "Economic buyer",
  "co.roleLabel.blocker": "Blocker",
  "co.roleLabel.influencer": "Influencer",
  "co.roleLabel.user": "End user",
  "co.evidence.extractedUnconfirmed": "Extracted by AI · not confirmed",
  "co.evidence.previous": "Previous claim",
  "co.evidence.next": "Next claim",
  "co.evidence.title": "Source",
  "co.prep.withheld":
    "Parts of this company are hidden from you, so this summary is incomplete.",
  "co.read.newActivity_one": "1 new item since your last visit.",
  "co.read.newActivity_other": "{count} new items since your last visit.",
  "co.factField.founded_year": "Founded",
  "co.factField.employee_range": "Employees",
  "co.factField.phone": "Phone",
  "co.factField.contact_email": "Contact email",
  "co.factField.location": "Location",
  "co.factField.service": "Service",
  "co.factField.product": "Product",
  "co.factField.capability": "Capability",
  "co.factField.served_industry": "Serves",
  "co.factField.company_size": "Size",
  "co.factField.geography": "Geography",
  "co.factField.language": "Language",
  "co.factField.certification": "Certification",
  "co.factField.partner": "Partner",
  "co.factField.named_customer": "Customer",
  "co.factField.technology": "Technology",
  "co.factField.mail_provider": "Mail system",
  "co.factField.email_security": "Mail authentication",
  "co.factField.hosting_provider": "Hosting",
  "co.factField.operated_service": "Operated service",
  "co.vat.markVerdict": "VAT ID: {verdict}",
  "co.vat.markUnchecked": "VAT ID: not yet checked with the register",
  "co.vat.markUnreadable":
    "VAT ID: check result could not be loaded. Select to retry.",
  "co.vat.numberMoved":
    "The number on this record changed after this check. Check the new number with the register.",
  "co.vat.verdict": "Register result",
  "co.vat.number": "Number consulted",
  "co.vat.registeredName": "Registered to",
  "co.vat.registeredAddress": "Registered address",
  "co.vat.checkedAt": "Consulted on",
  "co.vat.receipt": "Consultation number",
  "co.vat.status.valid": "Valid",
  "co.vat.status.invalid": "Not valid",
  "co.vat.noReceipt":
    "None issued. The register issues a consultation number only for checks made under your own VAT ID. Add it in Settings so later checks include proof a tax authority accepts.",
  "co.vat.never":
    "This company’s VAT ID has not been checked. It is checked automatically when read from the company’s imprint, or you can check it now.",
  "co.vat.askNow": "Check with the register",
  "co.vat.askAgain": "Check again",
  "co.vat.askingBusy": "Checking with register…",
  "co.vat.asking":
    "Checking with the register. The result appears here when it replies.",
  "co.tech.title": "Technology",
  "co.tech.sub":
    "What this company publicly runs, read from its DNS records, certificates and homepage.",
  "co.tech.mail": "Mail",
  "co.tech.web": "Website technology",
  "co.tech.services": "Services",
  "co.tech.hosting": "Hosting",
  "co.tech.empty":
    "No technical data yet. Filled in and refreshed automatically when the company’s website is read.",
  "co.tech.laneFailed":
    "{lane} did not respond. The previous result is unchanged.",
  "co.tech.laneRefused": "The site blocked the read.",
  "co.tech.lane.dns": "DNS",
  "co.tech.lane.certlog": "Certificates",
  "co.tech.lane.homepage": "Homepage",
  "signal.kind.technical_change": "Technology change",
  "co.factField.quantified_outcome": "Result",
  "co.facts.title": "Company facts",
  "co.facts.empty": "No facts yet. Read the website, or add what you know.",
  "co.facts.add": "Add fact",
  "co.facts.addField": "Fact type",
  "co.facts.addValue": "Value",
  "co.facts.addSave": "Save fact",
  "co.facts.addCancel": "Cancel",
  "co.facts.addIncomplete": "Select a fact type and enter a value.",
  "co.facts.remove": "Remove {value}",
  "co.facts.removeTitle": "Remove this fact?",
  "co.facts.removeConfirm": "Remove",
  "co.facts.removeAsk":
    "{field} is recorded as “{value}”. Removing it marks this as not a fact about the company. A later read of the website may add it again.",
  "co.facts.showAll": "Show all {count}",
  "co.facts.showLess": "Show fewer",
  "co.project.new": "New project",
  "co.deal.new": "New deal",
  "co.recent.title": "Recent activity",
  "co.recent.emptyDetail":
    "Sent emails, logged calls and meetings appear here, with who did what on each side.",
  "co.recent.empty": "No activity logged yet.",
  "co.recent.kind.email": "Email",
  "co.recent.kind.call": "Call",
  "co.recent.kind.meeting": "Meeting",
  "co.recent.kind.note": "Note",
  "co.recent.kind.task": "Task",
  "co.recent.kind.message": "Message",
  "co.recent.dir.theyWrote": "they wrote",
  "co.recent.dir.weSent": "your team sent",
  "co.recent.dir.theyCalled": "they called",
  "co.recent.dir.weCalled": "your team called",
  "co.recent.dir.both": "both sides",
  "co.recent.minutes": "{count} min",
  "co.recent.re": "on a deal",
  "co.recent.reNamed": "on {name}",
  "tagAdmin.title": "Tags",
  "tagAdmin.sub":
    "Tags this company files records under. Anyone can apply a tag; only administrators and operations users can add, rename or retire tags.",
  "tagAdmin.listLabel": "Tag list",
  "tagAdmin.empty": "No tags yet. Add the first tag.",
  "import.contextTag": "Tag for this import",
  "import.contextTagChosen":
    "Records this import creates will be filed under {name}.",
  "import.contextTagChosenUnnamed":
    "Records this import creates will be filed under the tag chosen for this run.",
  "import.contextTagHint":
    "Applied to created records so the batch stays findable. Updated records keep their tags.",
  "import.contextTagNone": "No tag",
  "tagAdmin.add": "Add tag",
  "tagAdmin.addTitle": "Add tag",
  "tagAdmin.editTitle": "Edit tag",
  "tagAdmin.nameLabel": "Name",
  "tagAdmin.colorLabel": "Color",
  "tagAdmin.colorNone": "No color",
  "tagAdmin.color.teal": "Teal",
  "tagAdmin.color.amber": "Amber",
  "tagAdmin.color.rose": "Rose",
  "tagAdmin.color.slate": "Slate",
  "tagAdmin.color.sky": "Sky",
  "tagAdmin.color.violet": "Violet",
  "tagAdmin.color.lime": "Lime",
  "tagAdmin.color.orange": "Orange",
  "tagAdmin.create": "Add",
  "tagAdmin.save": "Save",
  "tagAdmin.edit": "Edit",
  "tagAdmin.merge": "Merge",
  "tagAdmin.archive": "Retire",
  "tagAdmin.restore": "Restore",
  "tagAdmin.usage": "{count} records",
  "tagAdmin.usagePending": "Counting…",
  "tagAdmin.nearMatchTitle": "Similar tag exists",
  "tagAdmin.nearMatch":
    "{names}. Apply the existing tag unless this one is different.",
  "tagAdmin.mergeTitle": "Merge {name} into another tag",
  "tagAdmin.mergeIntoLabel": "Keep this tag",
  "tagAdmin.mergeIntoNone": "Select tag",
  "tagAdmin.mergeConfirm": "Merge",
  "tagAdmin.mergeWarningTitle": "This cannot be undone",
  "tagAdmin.mergeWarning":
    "Records tagged {name} get the other tag instead, and the name becomes available again.",
  "tagAdmin.mergedTitle": "Merged",
  "tagAdmin.mergedBody":
    "Records moved to the kept tag: {moved}. Duplicates removed, where a record already had both: {collapsed}.",
  "tagAdmin.countUsage": "Count records",
  "tagAdmin.noVersion":
    "This tag loaded without a version and cannot be saved. Reload the page and retry.",
  "tagAdmin.withheld": "You do not have access to this company’s tags.",
  "tagAdmin.truncatedTitle": "List shortened",
  "tagAdmin.truncated":
    "Tags past the limit are not shown and cannot be edited or merged into.",
  "tagAdmin.usageFailed": "Count unavailable",
  "tagAdmin.changeFailed": "The tag was not changed. Retry.",
  "tagAdmin.done": "Done",
  "tags.archived": "archived",
  "tags.columnHeader": "Tags",
  "tags.filterAll": "Any tag",
  "tags.moreUncounted": "more",
  "tags.moreUncountedTip":
    "Including {names}. Open the record for the full set.",
  "tags.columnHeaderPartial": "Tags (partial list)",
  "tags.loading": "Loading tags…",
  "tags.panelTitle": "Tags",
  "tags.add": "Add tag",
  "tags.more": "+{count} more",
  "tags.showLess": "Show less",
  "tags.removeTag": "Remove {name}",
  "tags.removeTitle": "Remove {name} from this record?",
  "tags.addedBy": "Added by {who} · {when}",
  "tags.addedByUndated": "Added by {who}",
  "tags.addedOn": "Added {when}",
  "tags.visibleWorkspaceWide": "Tag names are visible across the company.",
  "tags.removeFromRecord": "Remove from this record",
  "tags.withheld": "Hidden for your role",
  "tags.emptyTitle": "No tags yet",
  "tags.emptyBody":
    "Add lasting context such as an event, a relationship or a cohort.",
  "tags.pickerLabel": "Search tags",
  "tags.alreadyAdded": "Already added",
  "tags.catalogTruncatedTitle": "List shortened",
  "tags.catalogTruncated":
    "A tag may be missing. Search by name before requesting a new one.",
  "tags.noMatch":
    "No tag with that name. An administrator or operations user can add one.",
  "tagResult.gone":
    "This tag no longer exists. It may have been merged into another.",
  "tagResult.totalVisible": "{count} visible assignments",
  "tagResult.contacts": "Contacts",
  "tagResult.companies": "Companies",
  "tagResult.deals": "Deals",
  "tagResult.viewAll": "View all {count} {kind}",
  "tagResult.resultsTitle": "Records with this tag",
  "tagResult.nothingCarries":
    "No records have this tag yet. Apply it from any contact, company or deal.",
  "tagResult.loadingRows": "Loading {kind}…",
  "tagResult.noneLeft": "No records have this tag anymore",
  "tagResult.unnamed": "Unnamed",
  "co.timeline.empty": "No activity logged for this company yet.",
  "company.domains": "Domains",
  "company.factCategory.company": "Company",
  "company.factCategory.offering": "Offering",
  "company.factCategory.market": "Market",
  "company.factCategory.signal": "Signals",

  "lead.score": "Score",
  "lead.status": "Status",
  "lead.nextTask": "Next step",
  "lead.openTaskCount": "{count} open tasks",
  "lead.noNextTask": "No next step",
  "lead.scoreNoSignals": "No signals",
  "lead.source": "Source",
  "lead.project": "Project",

  "lead.filterSource": "Source",
  "lead.filterSourceAll": "All sources",
  "lead.source.manual": "Created manually",
  "lead.source.inbound": "Inbound",
  "lead.source.webform": "Web form",
  "lead.source.referral": "Referral",
  "lead.source.import": "Import",
  "lead.source.crawl": "Web research",
  "lead.source.unknown": "Unknown source",
  "lead.sourceFromConnector":
    "Written by a connector, which keeps its own source.",
  "leadSources.title": "Lead sources",
  "leadSources.sub":
    "Where leads come from. Used in the New lead form, in filters and in scoring.",
  "leadSources.readOnly":
    "Only an administrator or operations user can change this list.",
  "leadSources.readOnlyTitle":
    "Only an administrator or operations user can change this list",
  "leadSources.notSaved": "Change was not saved",
  "leadSources.notAdded": "Source was not added",
  "leadSources.labelFor": "Label of source {key}",
  "leadSources.intentFor": "Intent of {label}",
  "leadSources.intent": "Intent",
  "leadSources.intent.high": "High intent",
  "leadSources.intent.neutral": "Neutral",
  "leadSources.intent.low": "Low intent",
  "leadSources.intentHint":
    "High adds points to the score and Low subtracts them. Changes apply at each lead’s next rescore.",
  "leadSources.leadCount": "{count} leads",
  "leadSources.builtIn": "Built-in",
  "leadSources.builtInKept":
    "Built-in sources can be renamed or deactivated, not removed.",
  "leadSources.inUse":
    "Leads using this source: {count}. Deactivate it instead.",
  "leadSources.deactivateInstead": "deactivate instead",
  "leadSources.activeFor": "{label} is active",
  "leadSources.remove": "Remove",
  "leadSources.removeTitle": "Remove this source?",
  "leadSources.removeBody":
    "“{label}” is not used by any lead and is removed from the list.",
  "leadSources.newLabel": "New source",
  "leadSources.labelField": "Label",
  "leadSources.addOpen": "New source",
  "leadSources.listLabel": "Sources in the list",
  "leadSources.discovered": "Discovered values",
  "leadSources.newPlaceholder": "Trade show",
  "leadSources.add": "Add source",
  "leadSources.discoveredSub":
    "Values on leads from connectors and imports that are not in the list yet. Add one to give it a label and weight.",
  "leadSources.adopt": "Add to list",
  "leadReasons.title": "Disqualification reasons",
  "leadReasons.sub":
    "What a rep chooses when disqualifying a lead. The reason shows on the lead and can be filtered.",
  "leadReasons.labelFor": "Label of reason {label}",
  "leadReasons.leadCount": "{count} leads",
  "leadReasons.inUse":
    "Leads with this reason: {count}. Deactivate it instead.",
  "leadReasons.newLabel": "New reason",
  "leadReasons.listLabel": "Reasons in the list",
  "leadReasons.add": "Add reason",
  "leadReasons.removeTitle": "Remove this reason?",
  "leadReasons.removeBody":
    "“{label}” is not used by any lead and is removed from the list.",
  "leadHandling.title": "Lead handling",
  "leadHandling.sub": "How new leads are handled.",
  "leadHandling.firstResponse": "First-response target",
  "leadHandling.firstResponseHint":
    "Off by default. When on, every open lead gets a reply deadline, the list gains an Overdue view, and overdue leads sort first.",
  "leadHandling.targetMinutes": "Target (minutes)",
  "leadHandling.targetOutOfRange":
    "Enter a whole number of minutes from 15 to 10,080 (7 days).",
  "leadHandling.targetHint":
    "Maximum wait for a first reply after routing or creation, 15 minutes to 7 days.",
  "lead.boardCount": "{count} leads",
  "lead.duplicateFound":
    "A lead with this email or LinkedIn profile already exists.",
  "lead.promote": "Qualify",
  "lead.promoteIneligible": "Requires an email address and an open status.",
  "lead.filterStatus": "Status",
  "lead.filterStatusAll": "All statuses",
  "lead.filterScore": "Score",
  "lead.filterScoreAll": "Any score",
  "lead.bulkSelected": "{count} selected",
  "lead.bulkOwner": "New owner",
  "lead.bulkOwnerPick": "Choose owner",
  "lead.bulkAssign": "Assign",
  "lead.bulkDisqualify": "Disqualify",
  "lead.bulkDisqualifyTitle_one": "Disqualify this lead?",
  "lead.bulkDisqualifyTitle_other": "Disqualify {count} leads?",
  "lead.bulkDisqualifyBody":
    "Closed with the reason “{reason}”. Each lead keeps its record, and they cannot be reopened in one step.",
  "lead.bulkFailed": "{count} not applied:",
  "lead.bulkFailedRow": "not saved",
  "lead.bulkOutcomeConflict": "changed by someone else in the meantime",
  "lead.bulkOutcomeForbidden": "no permission to reassign",
  "lead.bulkOutcomeNotFound": "no longer in your list",
  "lead.bulkSelectRow": "Select {name}",
  "lead.unnamed": "Unnamed lead",
  "lead.timeline.empty": "No activity logged on this lead yet.",
  "lead.sla.breached": "Overdue",
  "lead.sla.atRisk": "Due soon",
  "lead.sla.withinTarget": "On time",
  "lead.sla.answeredAt": "{at}",
  "lead.sla.dueBy": "Due {at}",
  "lead.sla.overdueSince": "Was due {at}",
  "lead.filterSla": "Response",
  "lead.filterSlaAll": "Any",
  "list.viewOverdue": "Overdue",
  "lead.filterScoreHot": "80 and up",
  "lead.filterScoreWarm": "60 and up",
  "lead.filterScoreCool": "40 and up",
  "lead.details": "Details",
  "lead.rail.deal.title": "Deal",
  "lead.rail.deal.empty": "No deal yet. Qualifying this lead can open one.",
  "lead.rail.project.title": "Project",
  "lead.rail.project.empty": "No project yet.",
  "lead.rail.project.attach": "Attach project",
  "lead.rail.project.change": "Change project",
  "lead.terminalReadOnly": "This lead is closed and cannot be changed.",
  "lead.notYoursToChange":
    "You cannot change this lead. Ask its owner to share it, or an administrator for edit access.",
  "lead.boardCountsUnavailable":
    "Qualified and Disqualified counts did not load.",
  "lead.boardTerminalRowsUnavailable":
    "These leads did not load. The count above is still correct.",
  "lead.boardTerminalOnly":
    "No open leads here. They are counted under Qualified and Disqualified.",

  "lead.mergedTitle": "Merged into another lead",
  "lead.mergedBody":
    "This lead is the same prospect as another lead. Its timeline, consent and score moved there; this record is kept as history.",
  "lead.promotedTitle": "Qualified as contact",
  "lead.promotedMerged":
    "This lead merged into an existing contact. No duplicate was created.",
  "lead.promotedCreated": "This lead became a new contact.",
  "lead.promotedAt": "Qualified",
  "lead.promotedTrigger": "Trigger:",
  "lead.promotedEvidence": "Evidence:",
  "lead.previewPending": "Checking for an existing contact…",
  "lead.previewCreate": "Qualifying creates a new contact.",
  "lead.previewMerge": "Qualifying merges into the existing contact",
  "lead.previewMergeWithheld":
    "Qualifying merges into an existing contact you cannot see.",
  "lead.demote": "Reverse qualification",
  "lead.demoteDialog": "Reverse qualification?",
  "lead.demoteExplain":
    "The lead returns to “Working”. A contact created by the qualification is archived; a contact it merged into is unchanged. A qualification cannot be reversed while the contact is on a live deal.",
  "lead.demoteReason": "Reason (recorded in the audit trail)",
  "lead.demoteReasonRequired": "Enter a reason first.",
  "lead.demoteConfirm": "Reverse",
  "lead.reopen": "Reopen",
  "lead.writeRefused": "Change was not saved",
  "lead.reopenDialog": "Reopen this lead?",
  "lead.reopenExplain":
    "The lead returns to its status before disqualification, and the reason is cleared. History and score are kept.",
  "lead.reopenConfirm": "Reopen lead",
  "lead.promotedOutcomePending": "Loading qualification result…",
  "lead.promotedOutcomeUnavailable":
    "The qualification result cannot be shown.",
  "lead.terminalPromoted": "Qualified. This lead is read-only.",
  "lead.statusNew": "New",
  "lead.statusContacted": "Contacted",
  "lead.statusEngaged": "Engaged",
  "lead.statusPromoted": "Qualified",
  "lead.statusDisqualified": "Disqualified",
  "lead.disqualified": "Disqualified",
  "lead.merged": "Merged",
  "lead.status.new": "New",
  "lead.status.contacted": "Contacted",
  "lead.status.engaged": "Engaged",
  "lead.explainScore": "Explain this score",
  "lead.scoreOverridden": "Human override: {reason}",
  "lead.machineScore": "Model score: {score}",
  "lead.overrideScore": "Override score",
  "lead.clearOverride": "Clear override",
  "lead.overrideReason": "Reason",
  "lead.shortfall.lead": "Score inputs:",
  "lead.shortfall.engagementMoves":
    "A reply or a meeting raises the score most.",
  "lead.shortfall.noSource": "No source on record.",
  "lead.shortfall.sourcePenalised": "Source “{source}” lowers the score.",
  "lead.shortfall.noTitle": "No job title on record.",
  "lead.shortfall.titleNotSenior":
    "“{title}” is not a senior title the model recognizes.",
  "lead.shortfall.sourceNoIntent":
    "Source “{source}” does not indicate buying intent on its own.",
  "lead.scoreNotStoredYet":
    "The breakdown for this score is not stored yet. The next update shows it.",
  "lead.scoreLoading": "Loading score factors…",
  "lead.scoreNoFactors": "No factors counted toward this score yet.",
  "lead.scoreFactorsFailed": "Score factors did not load.",
  "lead.scoreFactorsExplainMachine":
    "You set this score manually. The factors below explain the model score: {score}.",
  "lead.scoreDecayed": "{base}, halved every 14 days",
  "lead.scoreSources": "{count} activities",
  "lead.scoreReconciles": "Sum {raw}, rounded to {rounded}, score {score}",
  "lead.factor.decision_maker_title": "Decision-maker title",
  "lead.factor.high_intent_source": "High-intent source",
  "lead.factor.low_intent_source": "Low-intent source",
  "lead.factor.reply": "Lead replied",
  "lead.factor.meeting_held": "Meeting held",
  "lead.factor.meeting_booked": "Meeting booked",
  "lead.signalsTitle": "Lead signals",
  "lead.signalUnset": "Not entered",
  "lead.signalClear": "Withdraw",
  "lead.signalBandPick": "Choose value",
  "lead.signalMore": "More",
  "lead.signalProvenanceHint":
    "If left unchanged, the answer is stored as an estimate with no stated confidence.",
  "lead.signalEvidenceQuality": "Evidence quality",
  "lead.signalConfidence": "Confidence",
  "lead.signalConfidenceUnstated": "Not stated",
  "lead.signalConfidenceValue": "{value}% confidence",
  "lead.signalRecordedAt": "Recorded {at}",
  "lead.signalSuperseded": "Previously {value}, replaced by {source}",
  "lead.signalAutomaticSource": "an automatic source",
  "lead.signalReason": "Evidence",
  "lead.signalReasonHint": "Optional. Stored with the score.",
  "lead.signalReasonUnstated": "No evidence given. Entered manually.",
  "lead.signalSave": "Add to score",
  "lead.signal.web_traffic": "Web traffic",
  "lead.signal.employees": "Employees",
  "lead.signal.budget_hint": "Budget",
  "lead.signal.ask.web_traffic": "Website traffic?",
  "lead.signal.ask.employees": "Company size?",
  "lead.signal.ask.budget_hint": "Budget?",
  // The breakdown's own spelling of the three kinds. NOT lead.signal.*,
  // which is the entry form's: that picker asks a rep how THEY know it
  // ("My assessment"), and the breakdown tells a reader whose judgement a
  // factor was, which may be somebody else's.
  "lead.factorKind.fact": "Verified",
  "lead.factorKind.assumption": "Estimated",
  "lead.factorKind.judgement": "Assessment",
  "lead.signal.fact": "Verified",
  "lead.signal.assumption": "Estimated",
  "lead.signal.judgement": "Assessment",
  "lead.signal.web_traffic.low": "Low",
  "lead.signal.web_traffic.medium": "Medium",
  "lead.signal.web_traffic.high": "High",
  "lead.signal.employees.1-10": "1 to 10",
  "lead.signal.employees.11-50": "11 to 50",
  "lead.signal.employees.51-200": "51 to 200",
  "lead.signal.employees.201+": "201+",
  "lead.signal.budget_hint.none": "No budget",
  "lead.signal.budget_hint.unknown": "Unknown",
  "lead.signal.budget_hint.some": "Some budget",
  "lead.signal.budget_hint.confirmed": "Budget confirmed",
  "lead.factor.manual:web_traffic": "Web traffic (manual)",
  "lead.factor.manual:employees": "Employees (manual)",
  "lead.factor.manual:budget_hint": "Budget (manual)",
  "lead.ownerLabel": "Owner",
  "lead.ownerYou": "You",
  "lead.overriddenBadge": "overridden",
  "lead.unassigned": "Unassigned",
  "lead.terminalDisqualified": "Disqualified. This lead is read-only.",
  "lead.marker": "Lead",
  "lead.assign": "Assign",
  "lead.assignToMe": "Assign to me",
  "lead.assignTo": "Assign this lead to",
  "lead.assignChoose": "Choose a colleague",
  "lead.assignNobodyElse": "No other user to assign this lead to.",
  "lead.saveOverride": "Save override",
  "lead.overrideScoreValue": "Score",
  "lead.trigger.inboundReply": "Inbound reply",
  "lead.trigger.meetingBooked": "Meeting booked",
  "lead.trigger.meetingHeld": "Meeting held",
  "lead.trigger.humanQualify": "Qualified manually",
  "lead.evidenceNote": "Evidence note (optional)",
  "lead.segregationTitle": "Leads are kept separate from contacts",
  "lead.segregation": "A lead becomes a contact only when you qualify it.",
  "lead.segregationDismiss": "Dismiss this notice",
  "list.emptyMine": "No {unit} owned by you.",
  "list.showAll": "Show all",
  "lead.assignedAway": "{names} assigned to {owner}. No longer in Mine.",
  "lead.viewNew": "New",
  "lead.viewNewUnassigned": "New and unassigned",
  "lead.viewUnassigned": "Unassigned",
  "lead.viewNeedsFollowUp": "Needs follow-up",
  "lead.viewEngaged": "Engaged",
  "lead.ladder": "Lead status",
  "lead.ladder.new": "New: no outreach yet.",
  "lead.ladder.automatic": "{label} · set automatically from captured activity",
  "lead.ladder.automaticWith": "{label} · set automatically: {what} on {at}",
  "lead.ladder.byHand": "{label} · set manually",
  "lead.ladder.theyReplied": "the lead replied",
  "lead.ladder.meetingBooked": "a meeting was booked",
  "lead.ladder.meetingHeld": "a meeting was held",
  "lead.ladder.qualified": "Qualified: this lead is now a contact.",
  "lead.ladder.qualifiedOn": "Qualified on {at}: this lead is now a contact.",
  "lead.ladder.disqualified": "Disqualified.",
  "lead.ladder.disqualifiedWithReason": "Disqualified: {reason}",
  "lead.qualify.title": "Qualify {name}",
  "lead.qualify.contact": "Contact",
  "lead.qualify.alsoDeal": "Also open a deal",
  "lead.qualify.pipeline": "Pipeline",
  "lead.qualify.stage": "Stage",
  "lead.qualify.dealName": "Deal name",
  "lead.qualify.amount": "Amount ({currency})",
  "lead.qualify.amountHint":
    "Optional. Whole units in the installation’s base currency.",
  "lead.qualify.amountInvalid": "Enter a number, or leave it empty.",
  "lead.qualify.amountNoCurrency":
    "Base currency not loaded yet. Retry shortly, or leave the amount empty.",
  "lead.qualify.why": "Why",
  "lead.qualify.reasonReplied": "Reason: the lead replied on {at}.",
  "lead.qualify.reasonMeetingBooked": "Reason: a meeting was booked for {at}.",
  "lead.qualify.reasonMeetingHeld": "Reason: a meeting was held on {at}.",
  "lead.qualify.reasonHuman": "Reason: qualified by you.",
  "lead.qualify.confirm": "Qualify",
  "lead.qualify.confirmWithDeal": "Qualify and open deal",
  "lead.qualify.done": "{name} is now a contact:",
  "lead.disqualify.title": "Disqualify {name}?",
  "lead.disqualify.reason": "Reason",
  "lead.disqualify.pickReason": "Choose reason",
  "lead.disqualify.reasonRequired": "Choose a reason first.",
  "lead.disqualify.note": "Note (optional)",
  "lead.disqualify.confirm": "Disqualify",

  "deals.viewBoard": "Board",
  "deals.viewTable": "Table",
  "deals.amount": "Value",
  "deals.lastSignal": "Last signal",
  "deals.lastSignalNone": "no signal yet",
  "deals.lastMailNone": "no email yet",
  "deals.stage": "Stage",
  "deals.close": "Expected close",
  "deals.confirmAdvance": "Move to {stage}?",
  "deals.confirmTerminal":
    "This closes the deal as {status}. Nothing changes until you confirm.",
  "deals.lostReason": "Lost reason",
  "deals.winNoEvidence":
    "No signed contract is attached. Record how the deal was won; the answer is kept on the deal and counted in reports.",
  "deals.winReason": "How was it won?",
  "deals.winReasonPick": "Pick how it was won",
  "deals.winReasonImported": "Imported from another system",
  "deals.winReasonPurchaseOrder": "On a purchase order",
  "deals.winReasonVerbal": "Verbally, in person or by phone",
  "deals.winReasonRenewalByEmail": "Renewed by email",
  "deals.winReasonOther": "Other",
  "deals.winReasonDetail": "Details",
  "deals.confirm": "Confirm",
  "deals.cancel": "Cancel",
  "deals.advanced": "Moved to {stage}",
  "deal.pendingApprovals": "Awaiting approval",

  "deal.ownerKeep": "Keep current owner",
  "deal.ownerMe": "Assign to me",
  "deal.ownerUnassign": "Unassign",
  "deal.partnerCompany": "via partner",
  // A reference the reader may not read, on a surface with no room for the
  // mask glyph: a Kanban card's company line, and the one entry a withheld
  // picker offers. Both have to say WHICH thing is withheld, because a card
  // and a form field carry no header to say it for them.
  "deal.companyWithheld": "Company withheld",
  "deal.partnerWithheld": "Partner withheld",
  "deal.forecastCategory": "Forecast category",
  "deal.strip.title": "Deal status",
  "deal.seats.ours": "Colleagues on this deal: {count}",
  "deal.committee.title": "Buying committee",
  "deal.committee.legendEngaged": "Engaged",
  "deal.committee.legendQuiet": "Not engaged",
  "deal.committee.legendGap": "Coverage gap",
  "deal.committee.threads": "{engaged} of {total} committee members engaged.",
  "deal.committee.engagement": "Engagement",
  "deal.strip.close": "Close date",
  "deal.strip.close.none": "No date",
  "deal.strip.close.inDays_one": "in {days} day",
  "deal.strip.close.inDays_other": "in {days} days",
  "deal.strip.close.overdue_one": "{days} day overdue",
  "deal.strip.close.overdue_other": "{days} days overdue",
  "deal.strip.close.provisional": "provisional, not confirmed by a human",
  "deal.strip.close.waiting": "buyer asked to wait until {date}",
  "deal.forecast.commit": "commit",
  "deal.forecast.bestCase": "best case",
  "deal.forecast.pipeline": "pipeline",
  "deal.forecast.omitted": "omitted from forecast",
  "deal.pulse.yourMove": "Your reply is due.",
  "deal.pulse.nothingFlagged": "No reply due.",
  "deal.pulse.nothingFlaggedWhy":
    "No inbound message on this deal is flagged for an answer.",
  "deal.pulse.wroteOn": "They last wrote on {date}, {days} days ago.",
  "deal.pulse.wroteUnknown": "They wrote and no one has replied.",
  "deal.timeline.empty": "No activity on this deal yet.",
  "acqSources.title": "Acquisition sources",
  "acqSources.sub":
    "Business channels a deal can be attributed to. Lead sources, which record how a record entered Margince, are separate.",
  "acqSources.listLabel": "Sources",
  "acqSources.loading": "Loading sources…",
  "acqSources.addOpen": "New source",
  "acqSources.addTitle": "New acquisition source",
  "acqSources.addLabel": "Label",
  "acqSources.addHint":
    "The key is derived from the label and cannot be changed later.",
  "acqSources.addConfirm": "Add source",
  "acqSources.builtIn": "Built-in",
  "acqSources.readOnly": "These sources are read-only for your role.",
  "acqSources.labelFor": "Label for {key}",
  "acqSources.activeFor": "{label} can be chosen on a deal",
  "settings.page.reviewtemplates.sub":
    "Questions a rep answers when a deal is won or lost.",
  "settings.tab.reviewtemplates": "Outcome reviews",
  "reviewTemplates.editHint":
    "Edits apply to future reviews. Existing reviews retain their original questions and answers.",
  "reviewTemplates.question": "Question",
  "reviewTemplates.answerType": "Answer type",
  "reviewTemplates.options": "Choices (one per line)",
  "reviewTemplates.requiredChoice": "Answer required",
  "reviewTemplates.removeQuestion": "Remove question",
  "reviewTemplates.addQuestion": "Add question",
  "reviewTemplates.save": "Save template",
  "reviewTemplates.edit": "Edit questions",
  "reviewTemplates.title": "Outcome review questions",
  "reviewTemplates.empty": "No review questions are set up",
  "reviewTemplates.retired": "Retired",
  "reviewTemplates.required": "(required)",
  "outcomeReview.title": "Outcome review",
  "outcomeReview.add": "Add review",
  "outcomeReview.save": "Save review",
  "outcomeReview.empty": "No review written yet",
  "outcomeReview.emptyDetail":
    "Record the reasons for this outcome while they are recent.",
  "outcomeReview.earlier": "From an earlier close",
  "outcomeReview.earlierMark": "earlier close",
  "outcomeReview.outcomeWon": "Won",
  "outcomeReview.outcomeLost": "Lost",
  "outcomeReview.noAnswer": "Not answered",
  "outcomeReview.notes": "Other notes",
  "deal.commercialContext": "Commercial context",

  "deal.arrFromOffer":
    "Expected ARR comes from the accepted offer and is not edited here.",
  "deal.brief": "Deal brief",
  "deal.briefHint": "Customer need, scope and intended outcome.",
  "deal.briefMore": "Read more",
  "deal.briefLess": "Show less",
  "deal.briefEdit": "Edit",
  "deal.briefAdd": "Write brief",

  "deal.briefEmpty": "No brief written yet",
  "deal.briefEmptyDetail":
    "Record the customer’s need and what a win looks like, so the next owner has the context.",

  "deal.motion": "Commercial motion",
  "deal.motionUnset": "Not set",
  "deal.motionNewBusiness": "New business",
  "deal.motionRenewal": "Renewal",
  "deal.motionUpsell": "Upsell",
  "deal.motionCrossSell": "Cross-sell",
  "deal.motionExpansion": "Expansion",
  "deal.motionExistingBusiness": "Existing business, unspecified",
  "deal.priority": "Priority",
  "deal.priorityUnset": "Not set",
  "deal.priorityHigh": "High",
  "deal.priorityMedium": "Medium",
  "deal.priorityLow": "Low",
  "deal.acquisitionSource": "Acquisition source",
  "deal.expectedArr": "Expected ARR",

  "deal.monthlyApproximate": "approx.",
  "assignments.title": "Responsible",
  "assignments.noAccessNote":
    "Records who is accountable. It does not grant access to this record.",
  "assignments.empty": "No one is assigned yet",
  "assignments.emptyDetail":
    "Assign a colleague or a team to record who is accountable for this work.",
  "assignments.roleRetired": "(retired role)",
  "assignments.teamSuffix": "(team)",
  "assignments.subjectInactive": "(inactive)",
  "assignments.add": "Assign",
  "assignments.change": "Change",
  "assignments.changeOne": "Change assignee: {who}",
  "assignments.removeOne": "Remove assignee: {who}",
  "assignments.addTitle": "Assign responsibility",
  "assignments.changeTitle": "Change assignee",
  "assignments.subjectKind": "Colleague or team",
  "assignments.kindUser": "Colleague",
  "assignments.kindTeam": "Team",
  "assignments.who": "Who",
  "assignments.findColleague": "Find a colleague",
  "assignments.findTeam": "Find a team",
  "assignments.role": "Role",
  "assignments.rolePlaceholder": "Pick a role",
  "assignments.saveAdd": "Assign",
  "assignments.saveChange": "Save change",
  "recordRoles.title": "Responsibility roles",
  "recordRoles.sub":
    "What a colleague or team can be accountable for on a company, deal or project. A role grants no access to the record.",
  "recordRoles.listLabel": "Roles",
  "recordRoles.loading": "Loading roles…",
  "recordRoles.readOnly": "Only an administrator can change these roles.",
  "recordRoles.builtIn": "Built in",
  "recordRoles.addOpen": "Add role",
  "recordRoles.addTitle": "Add responsibility role",
  "recordRoles.recordTypes": "Applies to",
  "recordRoles.assigneeKinds": "Can be assigned to",
  "recordRoles.addLabel": "Name",
  "recordRoles.addHint":
    "What the responsible party is accountable for, in plain words.",
  "recordRoles.addConfirm": "Add role",
  "recordRoles.labelFor": "Name for {key}",
  "recordRoles.activeFor": "{label} can be newly assigned",
  "deal.acquisitionUnset": "Not set",
  "deal.acquisitionRetired": "(retired)",
  "deal.waitUntil": "Wait until",
  "deal.fxBase": "Base {value} · rate {rate} as of {date}",
  "deal.archive": "Archive deal",
  "deal.archiveConfirm":
    "Archiving removes this deal from open deals. This cannot be undone here.",
  "deal.archivedReadOnly": "This deal is archived and takes no changes.",
  "deal.notYoursToChange":
    "You cannot change this deal. Ask its owner to share it, or an administrator for edit rights.",
  "deal.closedTakesNoStage":
    "This deal is closed. Reopen it to move it to another stage.",
  "deal.reopen": "Reopen",
  "deal.reopenPick": "Move this deal back to an open stage",
  "deal.reopenConfirm": "Reopen",
  "deal.fcCommit": "Commit",
  "deal.fcBestCase": "Best case",
  "deal.fcPipeline": "Pipeline",
  "deal.fcOmitted": "Omitted",
  "deal.fcSlipped": "Slipped",
  "deal.fcUncategorised": "No category",

  "deals.pipeline": "Pipeline",
  "deals.filterStalled": "Stalled only",
  "deals.filterOwnerMe": "My deals",
  // Both reasons say "loaded only" rather than naming the sum alone: with no
  // server aggregate the column's figure is the cards LOADED, and the board
  // pages on demand, so that number grows as the reader presses Load more.
  // Naming only the total would leave the count reading as final.
  "deals.totalsNeedOwnerFilter":
    "Loaded deals only. Filter to My deals for the total.",
  "deals.totalsNoTagFilter":
    "Loaded deals only. No total while a tag filter is on.",
  "deals.filterPartner": "Partner",
  "deals.filterPartnerAnyOne": "Any partner",
  "deals.filterMotion": "Motion",
  "deals.filterMotionAll": "Any motion",
  "deals.filterPriority": "Priority",
  "deals.filterPriorityAll": "Any priority",
  "deals.filterAcquisition": "Source",
  "deals.filterAcquisitionAll": "Any source",
  "deals.filterForecast": "Forecast",
  "deals.filterForecastAll": "Any forecast category",
  "deals.filterPartnerSourced": "Partner-sourced",
  "deals.filterStageAll": "All stages",
  "deals.filterCompanyAll": "All companies",
  "deals.filterStalledAll": "All deals",
  "deals.filterOwnerAll": "All owners",
  "deals.filterPartnerAll": "All sources",
  "deals.sortNewest": "Newest",
  "deals.unit": "deals",
  "deals.bulkSelected": "{count} selected",
  "deals.bulkSelectRow": "Select {name}",
  "deals.bulkOwner": "New owner",
  "deals.bulkOwnerPick": "Pick an owner",
  "deals.bulkAssign": "Assign",
  "deals.bulkStage": "Move to stage",
  "deals.bulkStagePick": "Pick a stage",
  "deals.bulkMove": "Move",
  "deals.bulkArchive": "Archive",
  "deals.bulkArchiveConfirmTitle_one": "Archive this deal?",
  "deals.bulkArchiveConfirmTitle_other": "Archive {count} deals?",
  "deals.bulkArchiveConfirmBody":
    "Archived deals leave every list and report and cannot be restored here.",
  "deals.bulkFailed": "{count} not applied:",
  "deals.bulkFailedRow": "could not be saved",

  "deal.offers": "Offers",
  "deal.newOffer": "New offer",
  "deal.offerNeedsCurrency":
    "Set the deal value first. An offer uses the deal’s currency.",
  "deal.offerNumber": "Offer #",
  "deal.offerRevision": "Revision",
  "deal.offersEmpty": "No offers yet",

  "offer.backToDeal": "Back to deal",
  "offer.totals": "Totals",
  "offer.net": "Net",
  "offer.tax": "Tax",
  "offer.gross": "Gross",
  "offer.arr": "Annual recurring",
  "offer.committedNet": "Committed net",
  "offer.edit": "Edit header",
  "offer.currency": "Currency",
  "offer.buyerCompany": "Buyer company",
  "offer.buyerCompanyConfirm": "Buyer company: {name}",
  "offer.template": "Template",
  "offer.validUntil": "Valid until",
  "offer.introText": "Intro text",
  "offer.termsText": "Terms text",
  "offer.lines": "Line items",
  "offer.addLine": "Add line",
  "offer.position": "Position",
  "offer.description": "Description",
  "offer.unit": "Unit",
  "offer.quantity": "Quantity",
  "offer.unitPrice": "Unit price",
  "offer.discountPct": "Discount %",
  "offer.taxRate": "Tax %",
  "offer.committedPeriods": "Periods",
  "offer.lineTotal": "Line total",
  "offer.unpriced": "unpriced, excluded from total",
  "offer.removeLine": "Remove",
  "offer.pickProduct": "Pick product",
  "offer.pickProductConfirm": "Product: {name}",
  "offer.send": "Send",
  "offer.sendConfirm": "Send this offer to the buyer?",
  "offer.sendBody": "The offer becomes read-only until the buyer responds.",
  "offer.accept": "Accept",
  "offer.acceptConfirm": "Mark this offer as accepted?",
  "offer.acceptBody":
    "The deal value and currency are updated to match this offer.",
  "offer.reject": "Reject",
  "offer.rejectConfirm": "Mark this offer as rejected?",
  "offer.rejectReason": "Reason (optional)",
  "offer.regenerate": "Regenerate revision",
  "offer.aiDisclosureTitle": "AI-assisted offer",
  "offer.diffAdded_one": "{count} line added",
  "offer.diffAdded_other": "{count} lines added",
  "offer.diffRemoved_one": "{count} line removed",
  "offer.diffRemoved_other": "{count} lines removed",
  "offer.diffChanged_one": "{count} line changed",
  "offer.diffChanged_other": "{count} lines changed",
  "offer.renderPdf": "Render PDF",
  "offer.viewPdf": "View PDF",
  "offer.pdfUnavailable":
    "PDF rendering is not available on this installation.",

  // The queue of staged actions a colleague has to decide. It is a DECISION here
  // and an `approval` on the wire, and those two are the only names it has: it
  // was also being called an inbox, a drafts queue and a staged list, and a
  // reader told four names for one surface has been told none. Copy that has to
  // point at it says what the reader does there ("waiting on you", "wait for
  // your decision") rather than inventing a fifth noun for the place.
  "decision.approveEdited": "Approve edited",
  "decision.reject": "Reject",
  "decision.draftSubject": "Subject",
  "decision.draftBody": "Message",
  "decision.dismiss": "Dismiss",
  "decision.versionSkew":
    "This record changed after it was staged. Stage it again before deciding.",
  "decision.reRead": "Reload",
  "decision.alreadyDecided": "Already decided. Nothing left to do.",
  "decision.expired": "Expired",
  "decision.expiresIn": "expires in {countdown}",
  "decision.detail": "Approval detail",
  "decision.detailLoading": "Loading approval…",
  "decision.detailTechnical": "Technical details",
  "decision.detailAsked": "Asked",
  "decision.detailDecided": "Decided",
  // The confirmation after an approval, and the way back to the change it
  // made. The undo lives on the RECORD — the history panel reverses the
  // update this approval wrote — so the verb says where it is going.
  "decision.applied": "Applied",
  "decision.undoOnRecord": "Undo on record",
  "decision.status.approved": "Approved",
  "decision.status.rejected": "Rejected",
  "decision.status.expired": "Expired",

  // The morning brief's own narrative. The "no pass" line is the honest degrade:
  // a run nobody annotated and a night with nothing in it read identically as
  // silence, so the screen says which one this is.
  // The week just gone. No nav entry of its own: Today is the single door to
  // the work that waits on a contact, and this is a view of that same work.
  "brief.panel.weekly": "Weekly review",
  // What the week TAUGHT, as against what it was. Every learning shows what it
  // rests on, because a claim about cause is one the reader cannot check
  // against anything else on the page.
  "brief.weekly.learnings.title": "Observations to review",
  "brief.weekly.learnings.worked": "Positive outcome",
  "brief.weekly.learnings.didNotWork": "Unsuccessful outcome",
  "brief.weekly.learnings.pattern": "Pattern",
  "brief.weekly.learnings.experiment": "Experiment",
  // Two empty states, never one: a week nobody read and a week that held no
  // lesson are different facts, and only one of them is about the week.
  "brief.weekly.learnings.notRun": "This week has not been analyzed yet.",
  "brief.weekly.learnings.insufficient":
    "Not enough recorded evidence for useful observations.",
  // How WELL the week went, beside what happened in it. Each block is drawn only
  // when the server sent it: a rep who carried no leads did not score zero on
  // the funnel, and an empty row would read as failure at something nobody
  // asked of them.
  "brief.weekly.scorecard.title": "Weekly scorecard",
  "brief.weekly.scorecard.leadBlock": "Leads and meetings",
  "brief.weekly.scorecard.dealBlock": "Deals",
  "brief.weekly.scorecard.advanced": "Lead advances",
  "brief.weekly.scorecard.advancedBasis": "Per step · not per lead",
  "brief.weekly.scorecard.answeredInTarget": "Target breaches",
  "brief.weekly.scorecard.answeredDetail": "{count} answered in target",
  "brief.weekly.scorecard.allInTarget": "All answered in target",
  "brief.weekly.scorecard.meetingsHeld": "Meetings held",
  "brief.weekly.scorecard.meetingsBasis": "{booked} booked · {noShow} no-show",
  "brief.weekly.scorecard.partialHistory": "Meetings without history",
  "brief.weekly.scorecard.partialHistoryBasis":
    "Before history began · counts are minimums",
  "brief.weekly.scorecard.advances": "Stage advances",
  "brief.weekly.scorecard.regressionsDetail": "{count} back a stage",
  "brief.weekly.scorecard.medianDaysInStage": "Days in stage",
  "brief.weekly.scorecard.medianBasis": "Median · stages left this week",
  "brief.weekly.scorecard.withNextStep": "Next step set",
  "brief.weekly.scorecard.ofOpen": "of {total} open deals",
  "brief.weekly.scorecard.noOpen": "No open deals",
  "brief.weekly.scorecard.multiThreaded": "Multiple contacts",
  "brief.weekly.scorecard.multiThreadedBasis":
    "of {total} open deals · last 30 days",
  "brief.weekly.scorecard.closeDateSound": "Firm close date",
  "brief.weekly.scorecard.forecastMoves": "Forecast upgrades",
  "brief.weekly.scorecard.forecastMovesBasis": "{down} downgraded",
  "brief.weekly.scorecard.unreconstructible": "Deals not rebuilt",
  "brief.weekly.scorecard.unreconstructibleBasis":
    "Affected by an erasure · counts are minimums",
  // The week ahead. The frozen review says what happened; this is the only part
  // of that page anybody can still change.
  "plan.title": "This week’s commitments",
  // The Brief's two dials. Which brief, and whose.
  "brief.view.label": "Home view",
  "brief.view.morning": "Morning",
  "brief.view.weekly": "Weekly",
  "brief.scope.label": "Whose brief",
  "brief.scope.mine": "Mine",
  "brief.scope.team": "Team",
  // The Brief's opening sentence, composed from the rows the page is showing —
  // never model-written, so it cannot say what the rows contradict.
  "brief.sentence.clear": "No immediate priorities found.",
  "brief.sentence.one": "First: {lead}",
  "brief.sentence.many": "First: {lead}. Then {rest}.",
  "brief.sentence.rest": "{count} more",

  // The weekly Brief's opening sentence, composed from the counts the week was
  // frozen with. Result first, then what carried — the outcome before the debt.
  "brief.week.won": "Deals won: {count}.",
  "brief.week.moved": "Deals moved forward: {count}.",
  "brief.week.met": "Meetings held: {count}.",
  "brief.week.carryPromises": "Commitments carried over: {count}.",
  "brief.week.carryTasks": "Tasks carried over: {count}.",
  "brief.week.andCarry": "{result} {carry}",
  "brief.week.quiet": "No completed work or deal movement recorded.",

  "brief.feed.title": "Focus",
  // What is on screen out of what the day holds, on the panel's own line.
  "brief.feed.changedBadge_one": "1 changed",
  "brief.feed.changedBadge_other": "{count} changed",
  "brief.feed.loading": "Loading Morning brief…",
  "brief.feed.clear":
    "No immediate priorities. The full Worklist is available.",
  "teamweekly.title": "Team week",
  "teamweekly.weekOf": "{team} · week of {day}",
  "teamweekly.frozen": "Frozen",
  "teamweekly.loading": "Loading team week…",
  "teamweekly.empty": "Nothing to show for this week.",
  "teamweekly.forbidden":
    "Team reviews need team access. Your access covers your own records only.",
  "teamweekly.noSnapshot":
    "No saved review is available for this team and week. Choose another week or review the team’s current work.",
  "teamweekly.pickTeam": "Choose a team",
  "teamweekly.chooseTeam": "Choose a team to review its work.",
  "teamweekly.repsUnread":
    "Snapshots are missing for {count} team members. These figures cover {counted} members.",
  "teamweekly.ofTotal": "{part} of {whole}",
  "teamweekly.headline.plain": "No meetings or due commitments were recorded.",
  "teamweekly.card.firstResponse": "Leads answered",
  "teamweekly.card.firstResponseBasis": "{breached} past target",
  "teamweekly.card.noLeads": "None received",
  "teamweekly.card.meetings": "Meetings with follow-up",
  "teamweekly.card.meetingsBasis": "Task linked before week end",
  "teamweekly.card.noMeetings": "None recorded",
  "teamweekly.card.commitments": "Commitments kept",
  "teamweekly.card.commitmentsBasis": "Due in the plan",
  "teamweekly.card.noCommitments": "None due",
  "teamweekly.card.won": "Won",
  "teamweekly.card.wonBasis": "{lost} lost · value unavailable",
  "teamweekly.card.wonBasisValue": "{value} · {lost} lost",
  "teamweekly.card.reps": "Members counted",
  "teamweekly.card.repsBasis": "Full week recorded",
  "teamweekly.movement.title": "Week activity",
  "teamweekly.movement.won": "Won",
  "teamweekly.movement.lost": "Lost",
  "teamweekly.movement.moved": "Advanced",
  "teamweekly.movement.meetings": "Meetings held",
  "teamweekly.movement.leads": "Leads routed",
  "teamweekly.agenda.title": "Monday agenda",
  "teamweekly.agenda.empty":
    "No member’s week could be loaded for this team, so there is no agenda.",
  "teamweekly.agenda.summary":
    "Items for Monday: {count}, starting with {first}.",
  "teamweekly.agenda.copy": "Copy agenda",
  "teamweekly.agenda.copied": "Copied",
  "teamweekly.agenda.copyFailed":
    "Agenda was not copied. Select the list and copy it manually.",
  "teamweekly.focus.help_requested": "Help requested",
  "teamweekly.focus.leads_breached": "Response targets missed",
  "teamweekly.focus.commitments_missed": "Plan commitments missed",
  "teamweekly.focus.meetings_without_next_step": "Follow-up evidence missing",
  "teamweekly.focus.strong_week": "Strong week",
  "teamweekly.focus.quiet_week": "No priority identified",

  "plan.loading": "Loading plan…",
  "plan.empty": "Nothing on the plan yet.",
  "plan.none": "No plan for this week yet.",
  "plan.start": "Plan week",
  "plan.readOnly":
    "Read-only view. You cannot plan this week or settle commitments here.",
  "plan.add": "Add commitment",
  "plan.refusedTitle": "Some changes not saved",
  "plan.saveRefused_one":
    "1 commitment was not saved. It is still checked. Retry.",
  "plan.saveRefused_other":
    "{count} commitments were not saved. They are still checked. Retry.",
  "plan.save_one": "Save {count} change",
  "plan.save_other": "Save {count} changes",
  "plan.due": "due {day}",
  "plan.state.open": "Open",
  "plan.state.done": "Done",
  "plan.state.missed": "Missed",
  "plan.state.dropped": "Dropped",
  "plan.help.label": "What do you need from your lead?",
  // What the week is up against, beside what it is for. The two prose fields
  // draw three states, not two: unwritten, "nothing to name", and text. A lead
  // who cannot tell the first two apart reads an unconsidered week as a safe one.
  "plan.contract.title": "Constraints this week",
  "plan.contract.risks": "Risks",
  "plan.contract.risksHint":
    "What you expect could go wrong, in your own words",
  "plan.contract.capacityNote": "Available capacity",
  "plan.contract.capacityNoteHint":
    "Anything the calendar does not show, such as leave, travel or a launch",
  "plan.contract.unwritten": "Not written yet",
  "plan.contract.nothingToName": "Nothing to name",
  "plan.contract.edit": "Edit",
  "plan.contract.save": "Save",
  "plan.contract.cancel": "Cancel",
  "plan.contract.crowded": "Week already full",
  "plan.contract.crowdedBody":
    "{committed} items are already booked and {commitments} commitments are planned. Something will not fit.",
  "plan.help.ask": "Ask for help",
  "plan.help.edit": "Edit request",
  "plan.help.send": "Send",
  "plan.help.cancel": "Cancel",
  "plan.help.asked": "Request: {text}",
  "plan.help.waiting": "Help requested · awaiting a response",
  "plan.new.label": "Commitment",
  "plan.new.due": "Due date",
  "plan.new.save": "Add",
  "plan.new.cancel": "Cancel",

  "brief.weekly.outlook": "Forecast outlook",
  "brief.weekly.outlook.week": "This week",
  "brief.weekly.outlook.month": "This month",
  "brief.weekly.outlook.quarter": "This quarter",
  "brief.weekly.outlook.none":
    "No forecast snapshot is available for this week.",
  "brief.weekly.outlook.won": "Won value",
  "brief.weekly.outlook.commit": "Commit remaining",
  "brief.weekly.outlook.bestCase": "Best case",
  "brief.weekly.outlook.bestCaseDetail": "Includes commit",
  "brief.weekly.outlook.weighted": "Weighted deal value",
  "brief.weekly.outlook.landing": "Projected landing",
  "brief.weekly.outlook.measure.commit_evidence": "From commit evidence",
  "brief.weekly.outlook.measure.weighted": "From weighted deals",
  "brief.weekly.outlook.measure.manager_call": "From the manager call",
  "brief.weekly.bridge": "Weekly change",
  "brief.weekly.bridge.opening": "Monday",
  "brief.weekly.bridge.closing": "Friday",
  "brief.weekly.bridge.noOpening":
    "No Monday snapshot for this week, so there is no opening figure.",
  "brief.weekly.bridge.reconcile":
    "The bars do not sum to the closing figure. Compare the 2 totals instead.",
  "brief.weekly.bar.created": "Created",
  "brief.weekly.bar.advanced": "Advanced",
  "brief.weekly.bar.slipped": "Slipped",
  "brief.weekly.bar.won": "Won",
  "brief.weekly.bar.lost": "Lost",
  "brief.weekly.bar.other": "Rates and definitions",
  "brief.weekly.frozen": "Frozen",
  "brief.weekly.written": "created {at}",
  "brief.weekly.pickWeek": "Open another week",
  "brief.weekly.none":
    "No weekly review yet. The first one is created on the Monday after your first full week.",
  "brief.weekly.tasksDelivered": "Tasks delivered",
  "brief.weekly.ofDue": "{done} of {due}",
  "brief.weekly.dealsWon": "Won",
  "brief.weekly.dealsLost": "Lost",
  "brief.weekly.dealsMoved": "Moved",
  "brief.weekly.decided": "Proposals decided",
  "brief.weekly.acceptedRejected": "{accepted} accepted · {rejected} rejected",
  "brief.weekly.queueWorked": "Morning brief items",
  "brief.weekly.actedDismissed": "{acted} acted · {dismissed} dismissed",
  "brief.weekly.sincePrior": "{delta} compared with {week}",
  "brief.weekly.wonVsPrior": "{value} · {delta} compared with {week}",
  "brief.weekly.leadsAnswered": "Leads answered",
  "brief.weekly.ofRouted": "{answered} of {routed}",
  "brief.weekly.planCommitmentsKept": "Commitments kept",
  "brief.weekly.meetingsHeld": "Meetings with follow-up",
  "brief.weekly.ofMeetings": "{withStep} of {held}",
  "brief.weekly.carriedOver": "Carried over",
  "brief.weekly.outcome.moved": "moved",
  "brief.weekly.outcome.won": "won",
  "brief.weekly.outcome.lost": "lost",
  "brief.act": "Done",
  "brief.dismiss": "Dismiss",

  "brief.digestSynced": "Sync details",
  "brief.digestContacts": "Contacts created",
  "brief.digestCompanies": "Companies created",
  "brief.digestDedupe": "Duplicates to review",
  "brief.digestClassify":
    "Commitments: {commitments} · Meetings: {meetings} · Messages without a sales action: {noise}",
  "brief.digestProjects": "Projects",
  "brief.digestPhaseChanges": "Phase changes",
  "brief.digestNewCommitments": "New commitments",
  "brief.digestGoneQuiet": "Gone quiet",
  "brief.digestPhaseChange": "{from} → {to}",
  "brief.digestCommitmentCount": "{count} new open commitments",
  "brief.digestQuietDays": "quiet for {days} days",
  "brief.glance.morning": "Good morning, {name}.",
  "brief.glance.morningAnon": "Good morning.",
  "brief.glance.afternoon": "Good afternoon, {name}.",
  "brief.glance.afternoonAnon": "Good afternoon.",
  "brief.glance.evening": "Good evening, {name}.",
  "brief.glance.eveningAnon": "Good evening.",
  "brief.glance.night": "Working late, {name}.",
  "brief.glance.nightAnon": "Working late.",
  "brief.glance.introWeekly": "Review results and plan your next steps.",
  "brief.glance.intro": "Your day at a glance.",
  "brief.panel.decisions": "Approvals",
  "brief.panel.overnight": "Overnight",
  // The rail's own panel titles, after the rename that made each claim exactly
  // what its rows are. "Promises & tasks" named a thing the product does not
  // have and stood over a standing line of apology for it.
  "brief.rail.quietSchedule": "Nothing booked",
  "brief.panel.schedule": "Upcoming meetings",
  "brief.overnight.connectorsUnhealthy": "Connectors need attention",
  "brief.overnight.fixConnector": "Fix connector",
  "brief.readings.label": "Today’s figures",
  // Why a figure on this plate wears a `+`. It rides the whole cell, because a
  // mark that explains itself only to a pointer resting on three characters is
  // one most readers never read.
  "brief.readings.floorTip":
    "A source reached its limit, so this figure is a minimum.",
  "brief.readings.urgent": "Urgent",
  "brief.readings.urgentBasis": "Contact waiting · commitment due",
  "brief.readings.decisions": "Reviews",
  "brief.readings.decisionsBasis": "Proposals · record checks",
  // Only drawn where a decision on the page carries the server's own
  // `work_blocked` consequence — a held SEND, not a duplicate pair. The basis
  // above it claims nothing about who is waiting, because most decisions are
  // contact hygiene and nobody is held up by one.
  "brief.readings.decisionsBlocking_one": "1 holding up customer work",
  "brief.readings.decisionsBlocking_other": "{count} holding up customer work",
  // The same reading where the page cannot name a quarter: the read has not
  // landed, or its period start is not a month this calendar has.
  // The cell's hover line. The title has room for a quarter and nothing more,
  // so the full range lives here — and where the figure is the whole
  // company's rather than this reader's, whose deals they are.
  "brief.snooze.done": "Snoozed until {at}",
  "brief.snooze.undo": "Undo",
  "brief.readings.meetings": "Upcoming meetings",
  "brief.readings.meetingsBasis": "Remaining on today’s calendar",
  "brief.readings.needsPrep_one": "1 needs prep",
  "brief.readings.needsPrep_other": "{count} need prep",
  "brief.readings.prepUnknown": "Prep not checked",
  "brief.readings.prepared": "All prepared",
  "brief.readings.leads": "Prospecting",
  "brief.readings.leadsBasis": "Assigned · no first contact",
  "brief.readings.leadsDue": "Next due {value}",
  "brief.rail": "Context",
  "brief.deck.later": "Later",
  "brief.deck.showMore": "Show full message",
  "brief.deck.showLess": "Show less",
  "brief.deck.view": "View",
  // The two words the LINE-PER-DECISION list needs. Brief opens with the
  // decisions and goes on to the day's own work, so a reader is passing
  // through rather than working a queue to its end — which is what earns the
  // dense line. It hides the proposal behind one control and folds the rarer
  // verdicts into another, and neither is reachable unnamed.
  "brief.deck.rowDetail": "Proposal",
  "brief.deck.rowMore": "More options",
  // The way to the rest, where the list draws a prefix of the queue.
  "brief.deck.rest_one": "1 more decision in the Worklist",
  "brief.deck.rest_other": "{count} more decisions in the Worklist",
  "brief.deck.viewDeck": "Deck",
  "brief.deck.viewList": "List",
  "brief.deck.keys":
    "Arrow keys stage a decision: → accept · ← reject · ↑ edit · ↓ later · U undo · Enter sends staged decisions",
  "brief.deck.behind_one": "1 more behind",
  "brief.deck.behind_other": "{count} more behind",
  "brief.deck.staged_one": "1 decision staged",
  "brief.deck.staged_other": "{count} decisions staged",
  "brief.deck.commit": "Send staged decisions",
  // A TRAY HOLDS TWO KINDS OF THING. `staged` counts what the commit will
  // SEND; a skip is held and sent nowhere, so counting it beside a Send button
  // told a reader their skip was about to go somewhere and then dropped it.
  "brief.deck.skipped_one": "1 skipped",
  "brief.deck.skipped_other": "{count} skipped",
  // The same control where the tray holds only skips: pressing it clears them
  // out of the deck and sends nothing, so it does not say "Send".
  "brief.deck.edited_one": "1 being edited",
  "brief.deck.edited_other": "{count} being edited",
  // The same control where NOTHING in the tray will be sent. It still commits —
  // that is what moves the skips out of the deck and opens an edit on its own
  // form — so it says what pressing it does rather than offering a send.
  "brief.deck.commitNothingToSend": "Clear skipped",
  "brief.deck.unstage": "Undo last",
  "brief.deck.clearedTitle": "Deck clear",
  "brief.deck.cleared_one": "1 decision sent",
  "brief.deck.cleared_other": "{count} decisions sent",
  "brief.deck.clearedTime": "at {at}",
  "brief.deck.empty": "Nothing is waiting on you.",
  "brief.deck.bundleSummary": "1 decision · {count} items",
  "brief.deck.bundleMembers": "Show {count} items",
  // A deal the rep dismissed, come back. The suppression rule holds a dismissed
  // deal out until a linked activity arrives after the mark, so the sentence
  // states that rule rather than guessing: it can only ever name an activity.
  "brief.snooze": "Snooze",

  "enrich.toInbox": "Open Worklist",

  "deepread.title": "Research this company",
  // Once a read exists the offer has been answered, so the panel names
  // what it now holds rather than pitching a capability already used.
  "deepread.titleRead": "Website research",
  "deepread.sub":
    "Reads the company’s website for domain, industry, size, locations and likely decision-makers, then suggests a first step. Findings stay staged until you accept.",
  "deepread.cta": "Start company research",
  "deepread.ctaAgain": "Read website again",
  "deepread.starting": "Starting…",
  "deepread.unavailable": "Website research is not configured on this server.",
  "deepread.statusQueued": "Queued",
  "deepread.statusDeferred": "Waiting for AI budget",
  "deepread.statusRunning": "Reading website…",
  "deepread.statusDone": "Done",
  "deepread.statusPartial": "Stopped early",
  // A read that spent the budget it was given did what it was configured to
  // do, so it is named for that rather than for the rest of the site it was
  // never going to reach. One line per ceiling: a byte cap did not reach a
  // page limit, and saying so would be wrong about which budget ran out.
  "deepread.statusPageCapped": "Read up to the page limit",
  "deepread.statusByteCapped": "Read up to the size limit",
  "deepread.statusTimeCapped": "Read up to the time limit",
  "deepread.statusFailed": "Failed",
  "deepread.statusCancelled": "Canceled",
  "deepread.resumesAt": "Resumes automatically {when}.",
  "deepread.pagesSoFar_one": "{count} page read so far",
  "deepread.pagesSoFar_other": "{count} pages read so far",
  "deepread.stoppedEarly": "Stopped early: {reason}",
  "deepread.stage.crawling": "Reading website",
  "deepread.stage.extracting": "Extracting facts",
  "deepread.step.done": "done",
  "deepread.step.running": "running",
  "deepread.step.queued": "waiting",
  "deepread.stopBudget": "model budget",
  "deepread.factCount_one": "{count} fact with evidence staged",
  "deepread.factCount_other": "{count} facts with evidence staged",
  "deepread.proposals_other": "{count} proposals awaiting review",
  "deepread.proposals_one": "{count} proposal awaiting review",
  "deepread.kindHome": "Home",
  "deepread.kindImpressum": "Impressum",
  "deepread.kindAbout": "About",
  "deepread.kindTeam": "Team",
  "deepread.kindServices": "Services",
  "deepread.kindProducts": "Products",
  "deepread.kindContact": "Contact",
  "deepread.kindOther": "Other",

  "transcriptread.title": "Transcript reading",
  "transcriptread.cta": "Read transcript",
  "transcriptread.starting": "Starting…",
  "transcriptread.unavailable":
    "Transcript reading is not configured on this server.",
  "transcriptread.statusQueued": "Queued",
  "transcriptread.statusRunning": "Reading…",
  "transcriptread.statusDone": "Done",
  "transcriptread.statusFailed": "Failed",
  "transcriptread.lineCount_one": "{count} line read",
  "transcriptread.lineCount_other": "{count} lines read",
  "transcriptread.proposals_other": "{count} next steps awaiting review",
  "transcriptread.proposals_one": "{count} next step awaiting review",
  "transcriptread.staged_one": "{count} next step suggested",
  "transcriptread.staged_other": "{count} next steps suggested",
  "transcriptread.decided_one": "{count} suggestion reviewed",
  "transcriptread.decided_other": "{count} suggestions reviewed",
  "transcriptread.decidedDetail": "{accepted} accepted, {rejected} declined",
  "transcriptread.expired_one": "{count} expired without review",
  "transcriptread.expired_other": "{count} expired without review",
  "transcriptread.effectFailed_one":
    "{count} accepted suggestion did not produce its task.",
  "transcriptread.effectFailed_other":
    "{count} accepted suggestions did not produce their tasks.",
  "transcriptread.statusUnknown_one":
    "Status unavailable for {count} suggestion",
  "transcriptread.statusUnknown_other":
    "Status unavailable for {count} suggestions",
  "transcriptread.nothingStated":
    "Transcript read in full. No next steps found.",
  "transcriptread.failedTitle": "Transcript could not be read",
  "transcriptread.failedFallback": "Nothing was staged.",
  "transcriptread.effectFailedTitle": "Accepted, but no task was created",

  "create.cancel": "Cancel",
  "create.save": "Create",
  "create.saving": "Creating…",
  "create.contact": "New contact",
  // The fast path beside it: reading a profile in another window and typing
  // what it says. The label names the ACT, not the source, because the same
  // form takes a conference badge and a business card.
  "vcardImport.action": "Import vCards",
  "vcardImport.title": "Import vCards",
  "vcardImport.fileLabel": "vCard file",
  "vcardImport.whichFile":
    "A .vcf file, the contact export format of phones and mail clients. A card comes from its owner, so imported cards skip approval.",
  "vcardImport.choose": "Select .vcf file",
  "vcardImport.working": "Reading cards…",
  "vcardImport.done": "Close",
  "vcardImport.noCards": "The file contains no cards.",
  "vcardImport.failed": "The cards were not imported. Retry.",
  "vcardImport.outcome.created": "Added",
  "vcardImport.outcome.updated": "Missing fields added",
  "vcardImport.outcome.needsReview": "Possible duplicate",
  "vcardImport.outcome.skipped": "Skipped",
  "create.quickCapture": "Quick capture",
  "create.quickCaptureSaved": "{name} saved",
  "create.company": "New company",
  "create.lead": "New lead",
  "create.deal": "New deal",
  "create.fullName": "Full name",
  "create.firstName": "First name",
  "create.lastName": "Last name",
  "create.contactTitle": "Title",
  "create.email": "Email",
  "create.phone": "Phone",
  "create.linkedin": "LinkedIn",
  "create.linkedinUrl": "LinkedIn URL",
  "create.displayName": "Company name",
  "create.legalName": "Legal name",
  "create.industry": "Industry",
  "create.sizeBand": "Company size",
  "co.address.summary": "Address",

  "create.addressLine1": "Street and number",
  "create.addressLine2": "Address line 2",
  "create.city": "City",
  "create.region": "State or region",
  "create.postalCode": "Postal code",
  "create.country": "Country code (ISO 3166)",
  "create.companyName": "Company",
  "create.dealName": "Deal name",
  "create.amount": "Value",
  "create.currency": "Currency",
  "create.stage": "Stage",
  "create.relatedCompany": "Company",
  "create.expectedClose": "Expected close",

  "field.unset": "Not set",
  "field.addEmail": "Add email",
  "field.addPhone": "Add phone",
  "field.addDomain": "Add domain",

  "field.addRegisterVat": "Add VAT ID",
  "field.addRegisteredAddress": "Add registered address",

  "field.addTitle": "Add title",

  "field.domain": "Domain",

  "field.emailType": "Type",
  "field.emailWork": "Work",
  "field.emailPersonal": "Personal",
  "field.emailOther": "Other",
  "field.phoneType": "Type",
  "field.phoneWork": "Work",
  "field.phoneMobile": "Mobile",
  "field.phoneHome": "Home",
  "field.phoneOther": "Other",
  "field.primary": "Primary",
  "field.removeRow": "Remove",
  "field.removeRowLabel": "Remove row {n}",
  "field.moveRowUp": "Move row {n} up",
  "field.moveRowDown": "Move row {n} down",
  "field.rowMoved": "Moved to position {n}",
  "field.yes": "Yes",
  "field.no": "No",

  "dedupe.viewExisting": "View existing record",

  "co.spine.earlierMore": "More threads before this",
  "co.spine.failed": "The thread could not be loaded.",
  "co.spine.exchangeCount": "{count} messages",
  "co.spine.kind.email": "Email",
  "co.spine.kind.call": "Call",
  "co.spine.kind.meeting": "Meeting",
  "co.spine.kind.note": "Note",
  "co.spine.kind.message": "Message",
  "co.spine.andOthers": "{names} and {count} others",
  "co.spine.said.to": "{what} to {who}",
  "co.spine.said.from": "{what} from {who}",
  "co.spine.said.with": "{what} with {who}",
  "co.spine.today": "Today",
  "co.spine.said.met": "{host} met {who}",
  "co.spine.said.held": "Meeting held by {host}",
  "co.spine.lastSpoke": "Last contact",
  "co.spine.days_one": "{count} day",
  "co.spine.days_other": "{count} days",
  "co.spine.quietSince": "No contact since",
  "co.spine.neverReplied": "Never replied",
  "co.spine.singleThreaded": "One contact, no reply",
  "co.spine.overdue": "Overdue",
  "co.spine.expectedClose": "Expected close",
  "co.360.subject": "{name} · 360",
  "co.360.subjectUnnamed": "This company · 360",
  "today.title": "Needs attention",
  "co.spine.earlier_other": "{count} earlier threads",
  "co.spine.earlier_one": "{count} earlier thread",
  "today.failed":
    "This section did not load. The rest of the page is unaffected.",
  "today.quiet": "Nothing needs attention right now.",
  "task.untitled": "Untitled task",
  "today.withheld": "Not included: {sections}. You do not have access to them.",
  "today.source.moments": "Margince findings",
  "today.source.nextSteps": "open tasks",
  "today.source.nextMeeting": "calendar",
  "today.source.deals": "deals",
  "today.meeting.prepare": "Prepare meeting",
  "today.source.contacts": "contacts",
  "today.source.standing": "company status",
  "today.source.activities": "activity",
  "today.silence.days": "no answer in {count} days",
  "today.draft.new": "New email",
  "today.draft.act": "Draft",
  "today.moment.act.openTask": "Open task",
  "today.moment.act.followUp": "Follow up",
  "today.moment.act.writeToThem": "Write email",
  "today.workQueue": "Worklist",

  "evidence.mark": "read",
  "evidence.confirm": "Confirm",
  "evidence.correct": "Correct",
  "evidence.save": "Save",
  "evidence.saving": "Saving…",
  "evidence.cancel": "Cancel",
  "evidence.correctedValue": "Corrected value",
  "evidence.confirmedAt": "Confirmed by a person {when}",
  "evidence.humanSet": "Set by a person",
  "acctCoverage.open": "Compare coverage",
  "acctCoverage.title": "Company coverage",
  "acctCoverage.contact": "Contact",
  "acctCoverage.findContact": "Find a contact",
  "acctCoverage.untried": "Untried",
  "acctCoverage.noMatch": "No matches.",
  "acctCoverage.columnCap":
    "Showing {cap} colleagues. Deselect one to add another.",
  "acctCoverage.partial":
    "Built from a partial read, so a blank cell may mean the read stopped early, not that no one tried.",
  "acctCoverage.noneButPartial":
    "No connections returned, but the read was capped. The company may still have coverage.",
  "acctCoverage.noneAtAll":
    "No colleague has exchanged messages with this company yet.",
  "docs.title": "Documents",
  "docs.empty": "No documents for this company yet.",
  "docs.noneInCategory": "No documents in this category.",
  "docs.allOnAgreements":
    "Every document here is filed under a contract above.",
  "docs.allSuperseded":
    "Only superseded documents remain. Show them to see the history.",
  "docs.superseded.show": "Show superseded",
  "docs.superseded.hide": "Hide superseded",
  "docs.superseded.hidden_one": "1 superseded document is hidden.",
  "docs.superseded.hidden_other": "{count} superseded documents are hidden.",
  "docs.superseded.shown_one": "1 superseded document is listed below.",
  "docs.superseded.shown_other":
    "{count} superseded documents are listed below.",
  "docs.reading.show": "Show extracted fields",
  "docs.reading.hide": "Hide extracted fields",

  // Adding one. The "About" wording is doing real work: it decides whether the
  // file becomes evidence in a deal or a paper about the account, and only the
  // first can be read for deal fields — so the hint says so rather than leaving
  // the reader to discover it from a panel that never appears.
  "docs.add.action": "Add document",
  "docs.add.title": "Add document",
  "docs.add.about": "About",
  "docs.add.aboutHint":
    "Documents on a deal can be read for deal fields; documents on the company cannot.",
  "docs.add.thisCompany": "This company",
  "docs.add.aDeal": "A deal",
  "docs.add.dealSearch": "Search company deals",
  "docs.add.dealSearchReach":
    "Search covers this company’s {deals} newest deals and shows the first {matches} matches. Older deals cannot be selected here.",
  "docs.add.category": "Category",
  "docs.add.name": "Title",
  "docs.add.nameHint": "Optional. Defaults to the filename.",
  "docs.add.file": "File",
  "docs.add.fileHint": "Up to {size}.",
  "docs.add.fileEmpty": "Drop a file here, or click to choose one",
  "docs.add.cancel": "Cancel",
  "docs.add.submit": "Upload",
  "docs.add.uploading": "Uploading…",
  "docs.add.errNoFile": "Choose a file to upload.",
  "docs.add.errNoDeal": "Select the deal to file this under.",
  "docs.add.errRefused":
    "You do not have permission to add documents to this record.",
  "docs.add.errTooLarge":
    "The file exceeds {size}, the limit on this installation. Choose a smaller file.",
  "docs.add.failedTitle": "Upload failed",
  "docs.add.failed": "Nothing was stored. Retry, or choose another file.",
  "docs.add.partialTitle": "Uploaded, but not filed",
  "docs.add.partial":
    "The file is stored and listed below, but its category and title were not saved, so it is filed under Other.",

  // The staged-document-reading panel (RD-AC-N-2/-3). Three states that must
  // stay apart in the words as well as in the data: not answered yet, answered
  // and states none of them, could not be read at all.
  "extraction.neverRead": "This file has not been read for deal fields yet.",
  "extraction.readIt": "Read this file",
  "extraction.readAgain": "Read again",
  "extraction.starting": "Starting…",
  "extraction.startFailed":
    "The file was not sent for reading. Nothing changed.",
  "extraction.loading": "Checking read status…",
  "extraction.reading": "Reading this file…",
  "extraction.stalled":
    "Reading is taking unusually long and may have stopped.",
  "extraction.failed": "This file could not be read.",
  "extraction.groundedNothing":
    "AI read this file and found none of the deal fields.",
  "extraction.heading_one":
    "AI read this file: {count} field with evidence, staged for review. Accept to save.",
  "extraction.heading_other":
    "AI read this file: {count} fields with evidence, staged for review. Accept to save.",
  "extraction.accept_one": "Accept {count} field",
  "extraction.accept_other": "Accept {count} fields",
  "extraction.dismiss": "Dismiss",
  "extraction.dismissed": "Nothing was saved. The file stays attached.",
  "extraction.acceptedLabel": "Accepted fields",
  "extraction.acceptedHeading_one":
    "{count} field accepted to the deal. Original snippets kept.",
  "extraction.acceptedHeading_other":
    "{count} fields accepted to the deal. Original snippets kept.",
  "extraction.acceptFailed":
    "The fields were not saved. The deal is unchanged.",
  "extraction.edit": "Edit",
  "extraction.editValue": "Edit {field}",
  "extraction.omitted.notStated": "omitted (not stated in this file)",
  "extraction.omitted.notConfident":
    "omitted (stated, but not clearly enough to accept)",
  "extraction.field.name": "Deal name",
  "extraction.field.amount": "Amount",
  "extraction.field.currency": "Currency",
  "extraction.field.closeDate": "Expected close date",
  "docs.filterLabel": "Document category",
  "docs.category.all": "All",
  "docs.category.contract": "Contract",
  "docs.category.offer": "Offer",
  "docs.category.legal": "Legal",
  "docs.category.email": "Email attachment",
  "docs.category.message": "Message attachment",
  "docs.category.other": "Other",
  "files.title": "Files",
  "files.empty":
    "No files on this deal yet. Upload a file, or link an email with an attachment.",
  "files.origin": "Attachment of a message from {who}, {when}",
  "files.originUnknown": "an unknown sender",
  "files.uploaded": "Uploaded {when}",
  "files.hiddenBadge": "Hidden",
  "files.rowActions": "Actions for {name}",
  "files.hide": "Hide from this deal",
  "files.unhide": "Show on this deal again",
  "files.delete": "Delete",
  "files.hideTitle": "Hide {name} from this deal?",
  "files.hideBody":
    "The message and its attachment stay on the activity and in the company library. Only this deal stops listing it.",
  "files.deleteTitle": "Delete {name}?",
  "files.deleteBody":
    "The file is removed from this deal, and from any Deal Room sharing it.",
  "files.showHidden": "Show hidden files",
  "files.hideHidden": "Hide hidden files",
  "docs.state.draft": "Draft",
  "docs.state.current": "Current",
  "docs.state.final": "Final",
  "docs.state.superseded": "Superseded",
  "log.title": "Log activity",
  "log.addTask": "Add task",
  "log.kind": "Type",
  "log.kindNote": "Note",
  "log.kindTask": "Task",
  "log.kindMeeting": "Meeting",
  "log.kindCall": "Call",
  "log.attendee": "Attendees",
  "log.subject": "Subject",
  "log.body": "Details",
  "log.transcriptLabel": "Transcript",
  "log.transcriptHint":
    "Paste from your meeting tool, such as Teams, Zoom or Meet. Speaker labels, if present, are kept.",
  "log.asTranscript": "This text is a transcript",
  "log.transcriptUpload": "Or upload a file",
  "log.transcriptUploadRejected": "Only a .txt file is accepted.",
  "log.transcriptUploadFailed":
    "File could not be read. Paste the text instead.",
  "log.dueAt": "Due date",
  "log.date": "Date",
  "log.assignee": "Assignee",
  "log.unassigned": "Unassigned",
  "log.save": "Log",
  "log.saving": "Logging…",

  // Who may READ a record, on its own header. The company half says "account"
  // where the contact half says "contact", because that is the word the rest
  // of the company page uses for itself.
  "recordAccess.contact.title": "Who can see this contact",
  "recordAccess.contact.privateToYou":
    "Private to its owner. No one else in the company can see this contact, including team members and administrators.",
  "recordAccess.contact.shared":
    "All users in the company can see this contact.",
  "recordAccess.contact.privateTip":
    "Only you can see this contact. Share it to make it visible to all users.",
  "recordAccess.contact.share": "Share with all users",
  "recordAccess.contact.published": "This contact is now visible to all users.",
  "recordAccess.contact.makePrivate": "Make private",
  "recordAccess.contact.madePrivate":
    "This contact is now private to its owner. Users it was shared with directly keep access.",
  "recordAccess.company.title": "Who can see this company",
  "recordAccess.company.privateToYou":
    "Private to its owner. No one else in the company can see this record, including team members and administrators.",
  "recordAccess.company.shared":
    "All users in the company can see this record.",
  "recordAccess.company.privateTip":
    "Only you can see this company. Share it to make it visible to all users.",
  "recordAccess.company.share": "Share with all users",
  "recordAccess.company.published": "This company is now visible to all users.",
  "recordAccess.company.makePrivate": "Make private",
  "recordAccess.company.madePrivate":
    "This company is now private to its owner. Deals, contacts and mail filed against it keep their own visibility.",
  "compose.reply": "Reply",
  "compose.writeEmail": "Write email",
  "compose.relink": "Relink",
  "compose.previewEmail": "Preview",
  "compose.readEmail": "Read full email",
  "compose.replyIntent": "Purpose of the reply",
  "compose.newIntent": "Purpose of the email",
  "compose.draftReply": "Draft reply with AI",
  "compose.newEmail": "New email",
  "compose.replyingTo": "Replying to “{subject}” · {when}",
  "compose.followingUp": "Following up on your email “{subject}” · {when}",
  "compose.draftContextHint":
    "Describe the purpose. Margince uses the record context.",
  "compose.draftWithAi": "Draft with AI",
  "compose.drafting": "Drafting…",
  "compose.discardDraft": "Discard draft",
  "compose.discardDraftHint":
    "Marks this draft as a miss for your Voice DNA. The generated text is never kept.",
  "compose.saveDraft": "Save as draft",
  "compose.savedDraftSaved": "Draft saved",
  "compose.savedDraftDelete": "Delete",
  "compose.savedDraftDeleted": "Saved draft deleted",
  "compose.savedDraftRestored": "Saved draft restored",
  "compose.savedDraftRemove": "Delete saved draft",
  "compose.savedDraftChangedTitle": "Draft changed in another window",
  "compose.savedDraftChangedBody":
    "Saving keeps the text on screen. Load the saved version to continue from it instead.",
  "compose.savedDraftGoneBody":
    "It was sent or deleted there. Saving keeps the text on screen as a new draft.",
  "compose.savedDraftLoad": "Load saved version",
  "compose.aiDisclosureTitle": "AI-assisted draft",
  "compose.aiDisclosureFallback":
    "This draft was written by AI. Review and edit it before sending.",
  "compose.voiceVersion": "Built from your writing samples · v{n}",
  "compose.voiceDegraded":
    "Your voice profile could not be loaded, so this draft does not use your voice. Draft again, or edit before sending.",
  "compose.voiceDegradedTitle": "This draft is not in your voice",
  "compose.provisional": "Provisional voice",
  "compose.provisionalHint":
    "Your Voice DNA is still being built. It shapes this draft the same way a finished one would.",
  "compose.to": "To",
  "compose.cc": "Cc",
  "compose.subject": "Subject",
  "compose.noGroundableRecipient":
    "No contacts at this company yet. Write the message manually, or add a contact first.",
  "compose.draftTo": "Draft to",
  "compose.draftToUnset": "Choose a contact",
  "compose.relatedTo": "Related to",
  "compose.relatedToNone": "Whole company",
  "compose.project": "Project",
  "compose.projectNone": "No project",
  "compose.scopedToCounted":
    "Scoped to {key} · {inScope} of {total} activities",
  "compose.scopedTo": "Scoped to {key}",
  // A channel reply has no picker: its send carries no filing field, so the
  // conversation's own project is inherited. This is the disclosure that
  // replaces the choice.
  "compose.channelFiling":
    "Sending files it under {project}, with the thread it answers.",
  "compose.basedOn": "Based on: {inputs}",
  "compose.whyThisDraft": "Why this draft?",
  "compose.body": "Body",
  "compose.bodyHint": "Click the text to edit it.",
  "compose.transport": "Send via",
  "compose.transportEmail": "Email",
  "compose.recipientHint": "Name or address",
  "compose.subjectHint": "Topic of the email",
  "compose.bodyPlaceholder": "Message text",
  "compose.bcc": "Bcc",
  "compose.bccHint":
    "Other recipients cannot see these addresses or that anyone else was copied.",
  "compose.threadGone":
    "That thread can no longer be answered, so a new email was opened instead. Check the recipient before sending.",
  "compose.colleagueMailbox_one":
    "This message was delivered to the mailbox of {names}. Your reply is sent from your own mailbox, under your name.",
  "compose.colleagueMailbox_other":
    "This message was delivered to the mailboxes of {names}. Your reply is sent from your own mailbox, under your name.",
  "compose.colleagueUnnamed": "a colleague",
  "compose.threadGoneTitle": "Thread cannot be answered",
  "compose.colleagueMailboxTitle": "Delivered to a colleague",
  "compose.deadRecipientsTitle": "Mail to these addresses bounces",
  "compose.attach": "Attach",
  "compose.filesOnRecord": "On this record",
  "compose.filesLoading": "Loading record files…",
  "compose.filesNone": "No files on this record yet.",
  "compose.filesFull":
    "A message can include at most {most} files. Send the rest in a second message.",
  "compose.fileRemove": "Remove {filename}",
  "compose.fileUpload": "Upload file",
  "compose.fileUploadHint":
    "The file is stored on this record first, so the history keeps every attachment sent.",
  "compose.fileUploadEmpty": "Drop a file here, or choose one",
  "compose.fileUploading": "Uploading…",
  "compose.fileStoredUnnamed":
    "The file is on the record, but it could not be identified. Attach it from the list above.",
  "compose.carriageTitle": "Cannot send on {channel}",
  "compose.carriageCarries":
    "{channel} does not support files, so this message and its attachments ({count}) cannot be sent there. Send the text on {channel} and the files another way.",
  "compose.carriageCount":
    "{channel} allows at most {limit} files per message, and this one has {named}. Send the rest in a second message.",
  "compose.carriagePerFile":
    "{filename} exceeds the {limit} per-file limit on {channel}. Send a smaller version, or share it another way.",
  "compose.carriageAggregate":
    "These {count} files total {total}, and {channel} allows at most {limit} per message. Split them across several messages.",
  "compose.carriageCaption":
    "{channel} sends the text of a message with files as a caption of at most {limit} characters; this one has {length}. Shorten it, or send the files separately.",
  "calendar.previousMonth": "Previous month",
  "calendar.nextMonth": "Next month",
  "compose.schedulePick": "Select date and time",
  "compose.scheduleDate": "Date",
  "compose.scheduleTime": "Time",
  "compose.scheduleGoesOut": "Sends {when}",
  "compose.willGoOut": "Sends {when}",
  "compose.scheduleAfternoon": "Tomorrow afternoon",
  "compose.rewrite": "Rewrite",
  "compose.rewriteShorter": "Shorter",
  "compose.rewriteShorterAsk": "Keep the meaning and use fewer words.",
  "compose.rewriteWarmer": "Warmer",
  "compose.rewriteWarmerAsk": "Warmer in tone, without getting familiar.",
  "compose.rewriteFormal": "More formal",
  "compose.rewriteFormalAsk": "More formal in tone.",
  "compose.rewriteDeadline": "Add a deadline",
  "compose.rewriteDeadlineAsk": "Ask for an answer by a named date.",
  "compose.sendOptions": "Other ways to send",
  "compose.scheduleSend": "Schedule send",
  "compose.scheduleTomorrow": "Tomorrow morning",
  "compose.scheduleMonday": "Monday morning",
  "compose.scheduleNow": "Send now",
  "compose.why": "Reason for contact",
  "compose.whyHint":
    "The record determines what is allowed; the reason lets the send be checked against it.",
  "compose.why.requestedFollowup": "Follow-up they requested",
  "compose.why.activeDeal": "Active deal",
  "compose.why.quote": "Quote or proposal they requested",
  "compose.why.service": "Support for a purchase",
  "compose.why.invoice": "Invoice or payment",
  "compose.why.contract": "Their contract",
  "compose.why.account": "Their customer relationship",
  "compose.why.marketing": "Marketing",
  "sendPermission.refused": "Message cannot be sent",
  "sendPermission.sayWhy": "Record reason for contact",
  "sendPermission.unproven": "No recorded reason to contact this recipient",
  "sendPermission.unprovenHint":
    "If a reason exists, for example a request, a meeting or a customer relationship, record it under your name.",
  "sendPermission.unprovenRefuses":
    "Sending is refused until a reason is recorded.",
  "sendPermission.ready": "Ready to send",
  "sendPermission.markName": "Communication status for this message",
  "sendPermission.checking": "Checking whether this message can be sent…",
  "sendPermission.unanswered":
    "The send check did not complete. Sending runs the check again.",
  "sendPermission.reason.objected":
    "The recipient objected to marketing. No one here can lift this, including an administrator.",
  "sendPermission.reason.withdrawn":
    "The recipient withdrew consent. No one here can lift this, including an administrator.",
  "sendPermission.reason.restricted":
    "The recipient’s data is under a processing restriction. No one here can lift this, including an administrator.",
  "sendPermission.reason.askedUsToStop":
    "The recipient asked not to be contacted. No one here can lift this, including an administrator.",
  "sendPermission.reason.bounced":
    "This address does not accept mail. Correct the address; an override does not apply.",
  "sendPermission.reason.tooMany":
    "The recipient has reached the marketing message limit for now. This clears automatically.",
  "sendPermission.reason.ambiguous":
    "Several records share this address, so the recipient cannot be identified. Merge the records to resolve this.",
  "sendPermission.reason.unconfirmed":
    "The recipient has not confirmed they want to receive messages. Only the recipient can confirm.",
  "sendPermission.reason.other":
    "This message cannot be sent, and no seat here can override that.",
  "compose.derivedReply":
    "This replies to the recipient’s own message, so no reason is needed.",
  "compose.send": "Send",
  "compose.sendConfirmTitle": "Send email",
  "compose.threadHeading": "This thread",
  "compose.continueHeading": "Continue thread?",
  "compose.threadLeave": "New email",
  "compose.messageCount_one": "{count} message",
  "compose.messageCount_other": "{count} messages",
  "compose.threadContinuing": "Last exchange in this thread",
  "compose.draftKept": "Your edits were kept. Draft again when ready.",
  "compose.threadFailed":
    "The message could not be loaded. Select Retry in the thread.",
  // The message a reply anchors on is gone — archived, or no longer this
  // reader's to open. It names the state and what the reader can still do,
  // because the server's own answer here is a bare "not found", which tells
  // them neither.
  "compose.anchorGone":
    "That message is no longer available to reply to. Write a new message instead.",
  "compose.threadPending": "Loading thread…",
  "compose.sendBody": "Review and edit the draft. Sending cannot be undone.",
  // A moment picked in the field above turns this dialog into a different
  // promise, so it says a different thing. The three sentences it replaces all
  // claim the send is happening NOW and is irreversible; a scheduled message is
  // neither, and it can be moved or withdrawn until it goes.
  "compose.schedule": "Schedule send",
  "compose.scheduleConfirmTitle": "Schedule email",
  // The composer computed that it had scheduled a send and said nothing —
  // it closed the way a SENT message closes it. The confirm dialog above
  // promises a place to move or withdraw the message from; these two are how a
  // rep gets there.
  "compose.scheduledQueued": "Email scheduled. It has not been sent yet.",
  "compose.scheduledOpenQueue": "Scheduled messages",
  "compose.scheduleBody":
    "The email is sent at the selected time, and the consent and mailbox checks run again then. Until then it can be moved or canceled from Scheduled messages.",
  "compose.sendMessageConfirmTitle": "Send message",
  "compose.sendMessageBody":
    "The message is sent immediately. This cannot be undone.",
  "compose.consentBlockedTitle": "Send blocked: no consent",
  "compose.consentBlocked":
    "A recipient has not granted consent for this purpose, so the send was blocked (denied by default).",
  "directSend.open": "Send with a recorded exception",
  "directSend.opening": "Opening…",
  "directSend.alreadySettled":
    "This has already been decided. Open the review for the outcome.",
  "directSend.couldNotOpen": "The review could not be opened. Retry.",
  "directSend.title": "Send on your own authority",
  "directSend.confirm": "Record the exception and send",
  "directSend.failed":
    "The message was not sent. Your decision may already be recorded; open the review before deciding again.",
  "directSend.noWarningServed":
    "This installation has not published the acknowledgement text, so the exception cannot be recorded here.",
  "directSend.needsAcknowledgement": "Select the acknowledgement to continue.",
  "directSend.needsReason": "Enter a reason for sending.",
  "directSend.reasonCodeLabel": "Basis",
  "directSend.explanationLabel": "Explanation",
  "directSend.acknowledge":
    "I have read the above and take responsibility for sending this message.",
  "directSend.reason.customer_requested_outside_crm":
    "Requested outside the CRM",
  "directSend.reason.contractual_necessity": "Contractual obligation",
  "directSend.reason.legal_obligation": "Legal obligation",
  "directSend.reason.other": "Other",
  "compose.reviewReference": "Review",
  "compose.reviewRequest": "Request review",
  "compose.reviewRequesting": "Requesting…",
  "compose.reviewRequested":
    "Review requested. A user allowed to send this decides, and the message waits until then.",
  "compose.reviewRequestFailed":
    "The review request failed. Retry, or open the review from the refusal above.",
  "compose.consentGoto": "Review consent",
  "compose.draftUnavailable":
    "AI drafting is unavailable because no model is configured. The email can still be written manually.",
  "compose.draftUnsupportedHere":
    "AI drafting is not available on this page. The email can still be written manually.",
  "compose.sendUnavailable":
    "Sending is unavailable because no mailer is configured.",
  "compose.mailboxNotSendCapable":
    "Your mailbox is connected for capture only, without permission to send. Reconnect it and approve sending; a mailbox connected before sending existed cannot be upgraded in place.",
  "compose.mailboxNotSendCapableGoto": "Reconnect your mailbox",
  "compose.sharedUnsubscribeToken":
    "A message with an unsubscribe link goes to one recipient at a time, because the link is that recipient’s consent record. Send it once per recipient, with no Cc.",
  "compose.multiRecipientWarning":
    "This purpose adds an unsubscribe link, so a send to more than one recipient is refused. Send it once per recipient, with no Cc.",
  "compose.relinkTitle": "Relink this activity",
  "compose.relinkTarget":
    "Search contacts, companies, deals, leads or projects",
  "compose.relinkNoVersion":
    "This activity was loaded without a version, so the relink cannot be applied. Reopen it and retry.",
  "compose.relinkReplace": "Replace existing link",
  "compose.relinkReplaceHint":
    "Replaces the existing link of the same type instead of adding one.",
  "compose.relinkConfirm": "Relink",
  "compose.relinkThread": "Move rest of thread",
  "compose.relinkThreadHint":
    "Every message in this thread that you can edit moves in one step.",
  "compose.emptyRecipients": "Add at least one recipient.",
  "compose.missingSubject": "Enter a subject.",
  "compose.missingBody": "Enter a message before sending.",
  "compose.missingWhy": "Select the reason for contact.",
  "compose.actionFailed": "The request failed. Retry.",

  "tasks.complete": "Done",
  "tasks.snooze": "Snooze 1 day",
  "tasks.moveTo": "Move to",
  "tasks.detail": "Task",
  "tasks.source": "Source meeting",
  "tasks.sourceEmail": "Source email",
  "tasks.openSource": "Open original",
  "tasks.detailLoading": "Loading task…",
  "tasks.isDone": "Completed",
  "tasks.logged": "Logged",

  "analytics.sub":
    "Open deals only, converted to {currency}, unweighted and weighted",
  "analytics.currency": "Currency",
  "analytics.count": "Open deals",
  "analytics.unweighted": "Unweighted",
  "analytics.weighted": "Weighted",
  "analytics.priced": "{priced} of {total} priced",
  "analytics.planNote":
    "The executed plan and the rows this number reconciles to",
  "analytics.reportDeals": "Open deals by stage",
  "analytics.sections": "Analytics sections",
  "analytics.sectionForecast": "Forecast",
  "analytics.sectionPipeline": "Deals",
  "analytics.sectionPerformance": "Performance",
  "analytics.noClosedDeals": "No deals have closed yet.",
  "analytics.sectionOutcomes": "My outcomes",
  "analytics.sectionCoverage": "Data coverage",
  "analytics.sectionDelivery": "Delivery",
  "analytics.reportProjectsByPhase": "Projects by phase",
  "analytics.reportProjectCommitments": "Project commitments",
  "analytics.reportProjectsGoneQuiet": "Projects gone quiet",
  "analytics.projects": "Projects",
  "analytics.project": "Project",
  "analytics.openDealValue": "Open deal value ({currency})",
  "analytics.wonDealValue": "Won deal value ({currency})",
  "analytics.openCommitments": "Open",
  "analytics.overdueCommitments": "Overdue",
  "analytics.quietSince": "Quiet since",
  "analytics.nothingQuiet": "No project in delivery has gone quiet.",
  "analytics.noProjectsYet": "No projects yet. A won deal creates one.",
  "analytics.coverageSub":
    "Sources the nightly check could read, and how far. A read source with no activity counts as checked; an unread source shows why.",
  "analytics.covSource": "Source",
  "analytics.covState": "State",
  "analytics.covThrough": "Checked through",
  "analytics.covChecked": "Checked",
  "analytics.covStale": "Stale, nothing read recently",
  "analytics.covUnavailable": "Unavailable, the check could not read it",
  "analytics.covPermissionLimited": "Access needs renewal",
  "analytics.covNotConnected": "Not connected",
  "analytics.coverageInputsElsewhere":
    "Record-level input problems are listed and resolved in the Forecast input review.",
  "analytics.myPipeline": "My open deals",
  "analytics.myMeetings": "My meetings",
  "analytics.meetingsAsTheyStand":
    "Meetings you host, by current status. A held meeting no longer counts as booked.",
  "analytics.meetingsBooked": "Booked",
  "analytics.meetingsHeld": "Held",
  "analytics.meetingsNoShow": "No-show",
  "analytics.meetingsCanceled": "Canceled",
  "analytics.outcomesOwnLensOnly":
    "This section measures one user’s records. Wider sections cover the rest of the lens.",
  "analytics.openOutcomeDeals": "Open the {outcome} deals",
  "analytics.reportWinLoss": "Won and lost",
  "analytics.reportStageAge": "Time in stage",
  "analytics.outcome": "Outcome",
  "analytics.won": "Won",
  "analytics.lost": "Lost",
  "analytics.baseValue": "Deal value · {currency}",
  "analytics.baseValueUnnamed": "Deal value",
  "analytics.noBaseCurrency": "No amount",
  "analytics.noBaseCurrencyWhy": "Currency not set",
  "analytics.forecastNoFigure": "No deals",
  "analytics.forecastDeals_one": "1 deal",
  "analytics.forecastDeals_other": "{count} deals",
  "analytics.forecastNoAmount": "No amount",
  "analytics.forecastWeighted": "{amount} weighted",
  "analytics.forecastPriced": "{priced} of {count} priced",
  "analytics.readingNone": "None",
  "analytics.readingLoading": "Loading",
  "analytics.readingUnavailable": "Unavailable",
  "analytics.medianDaysToClose": "Median days to close",
  "analytics.p75DaysToClose": "P75 days to close",
  "analytics.medianDaysInStage": "Median days in stage",
  "analytics.p75DaysInStage": "P75 days in stage",
  "analytics.tooFewForMedian": "Too few deals",
  "analytics.days": "{days} days",
  "analytics.unknownStage": "Former stage",
  "analytics.share.open": "Share view",
  "analytics.share.title": "Share this view",
  "analytics.share.kindLegend": "What the link shows",
  "analytics.share.liveLabel": "Live view",
  "analytics.share.liveHelp":
    "Recalculated on each open, limited to what the reader may see. Figures change as deals change.",
  "analytics.share.snapshotLabel": "Snapshot",
  "analytics.share.snapshotHelp":
    "Figures as they stood when the snapshot was taken. They do not change, and the link names the time it was taken.",
  "analytics.share.snapshotUnavailable":
    "No snapshot exists for this period yet.",
  "analytics.share.expiryNote":
    "The link stops working after 30 days. You can close it sooner.",
  "analytics.share.create": "Create link",
  "analytics.share.linkTitle": "Your link",
  "analytics.share.linkWarning":
    "The link is shown only once. Copy it now; it cannot be retrieved later.",
  "analytics.share.leaveWarning":
    "Leaving without copying discards the link, and a new one must be created.",
  "analytics.share.copy": "Copy link",
  "analytics.share.copied": "Copied",
  "analytics.share.copyFailed": "Select the link above and copy it manually.",
  "analytics.share.done": "Done",
  "analytics.frame": "As of {asOf} · {zone}",
  "review.title": "Checks before the call",
  "review.ready": "Ready",
  "review.readyWithExceptions": "Ready, with notes",
  "review.needsReview": "Needs review",
  "review.checksIncomplete": "Checks incomplete",
  "review.allSourcesRead": "All sources checked.",
  // The sources a run reads, named for the reader rather than by the server's
  // own vocabulary. The unread line printed the wire keys verbatim, so a German
  // reader was told "mail, offers" in English — words that name a table, not a
  // thing they would go and fix.
  "review.source.mail": "mailbox",
  "review.source.calendar": "calendar",
  "review.source.documents": "documents",
  "review.source.contracts": "contracts",
  "review.source.incumbent": "legacy system",
  "analytics.coverageNeverRun":
    "No check has run yet. An unchecked installation is not the same as a healthy one.",
  "review.source.offers": "offers",
  "review.sourcesUnread":
    "Not checked: {sources}. Findings below cover only what was checked.",
  "review.notCheckedYet":
    "Nothing checked yet: no nightly run has completed. The figures above reflect current records.",
  "review.nothingToCheck": "Nothing to check.",
  "review.answer": "Answer",
  "review.colSeverity": "Severity",
  "review.colFinding": "Finding",
  "review.colDeal": "Deal",
  "review.colAtStake": "At stake",
  "review.colSeenSince": "Seen since",
  "review.severityHigh": "High",
  "review.severityMedium": "Medium",
  "review.severityLow": "Low",
  "review.closePast": "Close date has passed",
  "review.closeUnconfirmed": "Close date not confirmed",
  "review.closePushed": "Close date keeps moving",
  "review.amountVsOffer": "Amount disagrees with the offer",
  "review.amountVsContract": "Amount disagrees with the contract",
  "review.noNextStep": "No next step",
  "review.noEconomicBuyer": "No signer identified",
  "review.buyerSilent": "Buyer has gone quiet",
  "review.commitUnpriced": "Committed with no amount",
  "review.unknownCheck": "Unnamed check",
  "review.sheetTitle": "Answer check",
  "review.outcomeLegend": "Answer type",
  "review.fixedRecord": "Record corrected",
  "review.addedEvidence": "Evidence added",
  "review.valueCorrect": "Value is correct",
  "review.notRelevant": "Not relevant to this deal",
  "review.remindLater": "Remind later",
  "review.reassign": "Someone else’s to answer",
  "review.hidesUntilExpiry": "Hides this check until it expires.",
  "review.reason": "Reason",
  "review.reasonHelp":
    "The next person to see this number needs the reason it is not flagged.",
  "review.remindAt": "Remind on",
  "review.expiresAt": "Expires on",
  "review.expiresHelp":
    "At most 90 days: a value correct in May describes May.",
  "review.cancel": "Cancel",
  "review.submit": "Save answer",
  "forecast.question": "Period forecast",
  "forecast.answerWithCall":
    "The current call is {call}. Evidence supports {evidence}.",
  "forecast.answerNoCall":
    "No call recorded for this period. Evidence supports {evidence}.",
  "forecast.partialTitle": "Not every deal is priced",
  "forecast.partial":
    "{priced} of {eligible} deals are priced. Unpriced deals add nothing to the totals above.",
  "forecast.currentCall": "Current call",
  "forecast.currentCallNone": "Not called",
  "forecast.currentCallDetailOver": "Called {date} · {gap} over evidence",
  "forecast.currentCallDetailUnder": "Called {date} · {gap} under evidence",
  "forecast.currentCallDetailEven": "Called {date} · matches evidence",
  "forecast.evidence": "Evidence",
  "forecast.evidenceDetail": "Confirmed close dates",
  "forecast.alreadyWon": "Already won",
  "forecast.alreadyWonDetail": "Closed this period",
  "forecast.updateCall": "Update call",
  "forecast.callExplains":
    "A call is the amount you expect to close. It records your number and changes no deal.",
  "forecast.expectedTotal": "Expected total for this period",
  "forecast.supportingNote": "Supporting note",
  "forecast.cancel": "Cancel",
  "forecast.saveCall": "Save call",
  "analytics.scopeLabel": "Record scope",
  "analytics.scopeFixed": "These numbers cover {scope}.",
  "forecast.period": "Period",
  "forecast.period.quarter": "Quarter",
  "forecast.period.month": "Month",
  "forecast.period.week": "Week",
  "forecast.receipt": "Data and evidence checked",
  "forecast.eligible": "Eligible deals",
  "forecast.priced": "Priced",
  "forecast.confirmed": "Close date confirmed",
  "forecast.fxMissing": "Exchange rate missing",
  "analytics.reportForecast": "Forecast categories",
  "analytics.reportOpenByCompany": "Open deals per company",
  "analytics.forecastBannerTitle": "How to read these tiles",
  "analytics.forecastBanner":
    "Each tile shows the raw total and, below it, the probability-weighted total. Rounding is per deal, so it always matches Explain this number.",
  "analytics.company": "Company",
  "analytics.openStageDeals": "Open the deals in {stage}",
  "analytics.openCompanyDeals": "Open this company’s deals",
  "analytics.noCompany": "No company",
  "analytics.openDeals": "Open deals",
  "explain.sources": "Source rows",
  "explain.col.record": "Deal",
  "explain.col.stage": "Stage",
  "explain.col.owner": "Owner",
  "explain.col.pipeline": "Pipeline",

  "settings.accountCard": "Your account",
  "unsaved.title": "Discard unsaved changes?",
  "unsaved.body":
    "Leaving this page discards what you entered. Go back to save it first.",
  "unsaved.discard": "Discard changes",
  "settings.addedItem": "“{name}” added",
  "settings.removedItem": "“{name}” removed",
  "settings.removed": "Removed",
  "settings.saved": "Saved",
  "settings.signature": "Email signature",
  "settings.signatureSub":
    "Added below every message you send, above the unsubscribe footer.",
  "settings.signatureLabel": "Sign-off",
  "settings.signaturePlaceholder": "Marek Janetzke\nGradion · +49 40 123456",
  "settings.signatureHint":
    "Plain text. Leave empty to send without a signature. AI drafts never add a sign-off.",
  "settings.signatureSaving": "Saving…",
  "settings.signatureEdit": "Edit signature",
  "settings.signatureNone": "No sign-off set",
  "settings.signatureCancel": "Cancel",
  // One line under the readings strip, naming what the day could not read.
  "delivery.morningLabel": "Morning brief",
  "delivery.morningHelp":
    "Also sends the day’s brief by email. It always appears on Home.",
  "delivery.weeklyLabel": "Weekly review",
  "delivery.weeklyHelp": "Also sends Monday’s review by email.",
  "delivery.byEmail": "By email",
  "delivery.none": "Not by email",
  "settings.appearance": "Appearance",
  "settings.appearanceHelp":
    "Light, dark or the device setting. The account menu also changes it.",
  "settings.displayName": "Display name",
  "settings.displayNameHelp":
    "Shown to colleagues on records you edit, in pickers and in the audit log.",
  "settings.displayNameSave": "Save",
  "settings.languageHelp": "Applies to this session.",
  "role.admin": "Admin",
  "role.management": "Management",
  "role.manager": "Team lead",
  "role.rep": "User",
  "role.readOnly": "Read-only",
  "role.ops": "Ops",
  "inlineChoice.change": "Change {field}",
  "rbac.masked": "Masked value",
  "settings.passports": "Agent passports",
  "settings.passportsSub":
    "An agent acts with your permissions and never more: every request rechecks your permissions.",
  // What each passport scope admits, in words. The wire carries `read`/`draft`/
  // `write`/`send`/`enrich`; a human granting them is choosing what an agent may
  // do on their behalf, and the protocol token alone does not say — "write" and
  // "send" read as near-synonyms until one of them names the mailbox.
  "passport.scope.read": "Read records",
  "passport.scope.draft": "Draft messages",
  "passport.scope.write": "Change records",
  "passport.scope.send": "Send messages",
  "passport.scope.enrich": "Buy contact data",
  "passport.select": "Passport",
  "passport.noneOption": "No passport",
  "settings.passportsLendHint":
    "Passports you created for scripts and other tools. An MCP client connection does not use these; it is listed below.",
  "settings.passportLabel": "Agent name",
  "settings.mint": "Mint passport",
  "settings.minting": "Minting…",
  "settings.mintCancel": "Cancel",
  "settings.mintDone": "Done",
  "settings.mintOpen": "New passport",
  "settings.passportScopes": "Agent permissions",
  "settings.passportScopesHint":
    "Select at least one. An agent can never do more than you can.",
  "settings.passportScopesRequired":
    "Select at least one permission for this agent.",
  // What the scheduled agent is doing for this reader, one line per (kind,
  // state). First contact for what Margince did, result first, and never a word
  // that reads as finished on a run that stopped part-way.
  "agent.activity.weeklyReview.queued": "Weekly summary queued",
  "agent.activity.weeklyReview.running": "Summarizing week…",
  "agent.activity.weeklyReview.stalled":
    "Weekly summary is taking unusually long",
  "agent.activity.weeklyReview.done": "Weekly summary ready",
  "agent.activity.weeklyReview.degraded":
    "Week measured, but no summary written",
  "agent.activity.weeklyReview.failed": "Weekly summary failed",
  // What the week TAUGHT, which is a different promise from the summary above:
  // a learning is advice, so the failed and degraded lines say the numbers
  // still stand rather than implying the week went unmeasured.
  "agent.activity.weeklyLearnings.queued": "Weekly observations queued",
  "agent.activity.weeklyLearnings.running": "Analyzing week for observations…",
  "agent.activity.weeklyLearnings.stalled":
    "Weekly observations are taking unusually long",
  "agent.activity.weeklyLearnings.done": "Weekly observations ready",
  "agent.activity.weeklyLearnings.degraded":
    "Week measured, but no observations found",
  "agent.activity.weeklyLearnings.failed": "Weekly observations failed",
  "agent.activity.morningBrief.queued": "Morning brief queued",
  "agent.activity.morningBrief.running": "Preparing Morning brief…",
  "agent.activity.morningBrief.done": "Morning brief ready",
  "agent.activity.morningBrief.degraded": "Morning brief stopped partway",
  "agent.activity.morningBrief.failed": "Morning brief failed",
  "agent.activity.morningBrief.stalled":
    "Morning brief is taking unusually long",
  "agent.activity.riskSweep.queued": "Deal risk check queued",
  "agent.activity.riskSweep.running": "Checking deals for risk…",
  "agent.activity.riskSweep.done": "Deal risk check complete",
  "agent.activity.riskSweep.degraded": "Deal risk check stopped partway",
  "agent.activity.riskSweep.failed": "Deal risk check failed",
  "agent.activity.riskSweep.stalled":
    "Deal risk check is taking unusually long",
  "agent.activity.documentExtract.queued": "Document extraction queued",
  "agent.activity.documentExtract.running": "Extracting document…",
  "agent.activity.documentExtract.stalled":
    "Document extraction is taking unusually long. Reopen the file to retry.",
  "agent.activity.documentExtract.done": "Document extraction complete",
  "agent.activity.documentExtract.degraded":
    "Document extraction stopped partway",
  "agent.activity.documentExtract.failed": "Document extraction failed",
  // The same six, with the document NAMED. A rail that says "I'm reading your
  // document" reports that software is busy; one that says "I'm reading
  // Q3-offer.pdf" reports the reader's own afternoon. The name is the source's
  // snapshot of what the product calls that document elsewhere, so these are
  // used only when it sent one — every kind falls back to the pair above.
  "agent.activity.documentExtractNamed.queued": "Extraction of {name} queued",
  "agent.activity.documentExtractNamed.running": "Extracting {name}…",
  "agent.activity.documentExtractNamed.stalled":
    "Extraction of {name} is taking unusually long. Reopen the file to retry.",
  "agent.activity.documentExtractNamed.done": "Extraction of {name} complete",
  "agent.activity.documentExtractNamed.degraded":
    "Extraction of {name} stopped partway",
  "agent.activity.documentExtractNamed.failed": "Extraction of {name} failed",
  // A company's website being read. The same shape as the document lines: the
  // unnamed pair says which kind of thing, the named one says which company.
  "agent.activity.transcriptRead.queued": "Transcript read queued",
  "agent.activity.transcriptRead.running": "Reading transcript…",
  "agent.activity.transcriptRead.stalled":
    "Transcript read is taking unusually long",
  "agent.activity.transcriptRead.done": "Transcript read complete",
  "agent.activity.transcriptRead.degraded": "Transcript read stopped partway",
  "agent.activity.transcriptRead.failed": "Transcript read failed",
  "agent.activity.voiceBuild.queued": "Writing voice analysis queued",
  "agent.activity.voiceBuild.running": "Learning writing voice…",
  "agent.activity.voiceBuild.stalled":
    "Writing voice analysis is taking unusually long",
  "agent.activity.voiceBuild.done": "Writing voice learned",
  "agent.activity.voiceBuild.degraded": "Writing voice partly learned",
  "agent.activity.voiceBuild.failed": "Writing voice could not be learned",
  "agent.activity.siteRead.queued": "Company website read queued",
  "agent.activity.siteRead.running": "Reading company website…",
  "agent.activity.siteRead.stalled":
    "Company website read is taking unusually long",
  "agent.activity.siteRead.done": "Company website read complete",
  "agent.activity.siteRead.degraded": "Company website read stopped partway",
  "agent.activity.siteRead.failed": "Company website read failed",
  "agent.activity.siteReadNamed.queued": "Read of the {name} website queued",
  "agent.activity.siteReadNamed.running": "Reading the {name} website…",
  "agent.activity.siteReadNamed.stalled":
    "Read of the {name} website is taking unusually long",
  "agent.activity.siteReadNamed.done": "Read of the {name} website complete",
  "agent.activity.siteReadNamed.degraded":
    "Read of the {name} website stopped partway",
  "agent.activity.siteReadNamed.failed": "Read of the {name} website failed",
  // The AI work a contact ASKS for and then waits on. Same rules as the
  // scheduled lines above — first contact, result first, and never a word that
  // reads as finished on a run that stopped part-way.
  //
  // These four were deliberately not narrated until the router could say
  // `running`: reporting them settled-only meant a line that appeared already
  // finished, which tells a waiting reader nothing they did not already know.
  // The account scan: one reader's read of one account, filed under them
  // and named for the account the rail can say which one is ready.
  "agent.activity.accountScan.queued": "Company analysis queued",
  "agent.activity.accountScan.running": "Analyzing company history…",
  "agent.activity.accountScan.stalled":
    "Company analysis is taking unusually long. Reopen the company to retry.",
  "agent.activity.accountScan.done": "Company analysis complete",
  "agent.activity.accountScan.degraded":
    "Company analysis completed as far as the records allowed",
  "agent.activity.accountScan.failed": "Company analysis failed",
  "agent.activity.accountScanNamed.queued": "Analysis of {name} queued",
  "agent.activity.accountScanNamed.running": "Analyzing the history of {name}…",
  "agent.activity.accountScanNamed.stalled":
    "Analysis of {name} is taking unusually long. Reopen the company to retry.",
  "agent.activity.accountScanNamed.done": "Analysis of {name} complete",
  "agent.activity.accountScanNamed.degraded":
    "Analysis of {name} completed as far as the records allowed",
  "agent.activity.accountScanNamed.failed": "Analysis of {name} failed",
  //
  // summarize is five sites over three kinds of record — a company, a contact,
  // a meeting — so the unnamed lines name none of them: "this company" was a
  // guess that was wrong two times in five, and a guess at a name is worse
  // than no name. The NAMED pair below is the line a reader should see; these
  // are what an occurrence without a subject falls back to.
  "agent.activity.summarize.queued": "Summary queued",
  "agent.activity.summarize.running": "Writing summary…",
  "agent.activity.summarize.done": "Summary ready",
  "agent.activity.summarize.degraded": "Summary stopped partway",
  "agent.activity.summarize.failed": "Summary failed",
  "agent.activity.summarize.stalled": "Summary is taking unusually long",
  // The same six with the record NAMED — the company, the contact or the
  // meeting the summary is about, in the name the product shows for it
  // elsewhere. Whenever the rail names what it is working on, it names the
  // actual record: "what I know about Acme", never "about this company".
  "agent.activity.summarizeNamed.queued": "Summary of {name} queued",
  "agent.activity.summarizeNamed.running": "Summarizing {name}…",
  "agent.activity.summarizeNamed.done": "Summary of {name} ready",
  "agent.activity.summarizeNamed.degraded": "Summary of {name} stopped partway",
  "agent.activity.summarizeNamed.failed": "Summary of {name} failed",
  "agent.activity.summarizeNamed.stalled":
    "Summary of {name} is taking unusually long",
  "agent.activity.draftReply.queued": "Reply draft queued",
  "agent.activity.draftReply.running": "Drafting reply…",
  "agent.activity.draftReply.done": "Draft reply ready",
  "agent.activity.draftReply.degraded": "Reply draft stopped partway",
  "agent.activity.draftReply.failed": "Reply draft failed",
  "agent.activity.draftReply.stalled": "Reply draft is taking unusually long",
  "agent.activity.offerDraft.queued": "Offer draft queued",
  "agent.activity.offerDraft.running": "Drafting offer…",
  "agent.activity.offerDraft.done": "Draft offer ready",
  "agent.activity.offerDraft.degraded": "Offer draft stopped partway",
  "agent.activity.offerDraft.failed": "Offer draft failed",
  "agent.activity.offerDraft.stalled": "Offer draft is taking unusually long",
  // ── THE AGENT SURFACE'S OWN WORDS ──────────────────────────────────────────
  //
  // The rail's one line and the panel it opens, in one place. They were split
  // between this catalog and an English-only table in `agentrail-copy.ts`,
  // which is why a German reader met "Across the workspace" over a translated
  // sentence. Every word either surface renders is here now.
  //
  // SHORT. The rail clamps its line to two lines at about 200px (`.arline` in
  // agentrail.css) and the panel is 408px wide, so a heading is a noun phrase
  // and a status is one clause. Every translation is held to the same ceiling.

  // The two regions, for a reader who navigates by landmark.
  "agent.rail.region": "Margince agent",
  "agent.panel.label": "Agent panel",
  // The one button, named for what pressing it does.
  "agent.rail.open": "Open agent panel",
  "agent.rail.close": "Close agent panel",

  // THE STATE IN A WORD, under the agent's name. The Core's own vocabulary is
  // five machine words and the head used to print whichever one it was in, so a
  // panel opened on a broken installation said "error" at a reader in the
  // product's voice. The sentence beside these says what is HAPPENING; these
  // say what the agent is. One word each, because they sit beside a dot.
  "agent.state.idle": "Idle",
  "agent.state.ingest": "Capturing",
  "agent.state.working": "Working",
  "agent.state.warning": "Warning",
  "agent.state.error": "Error",

  // The month's figure: what it names, beside it in the head and in the rail's
  // own line. ONE string for both — they were two identical entries.
  "agent.thisMonth": "this month",
  // The same fact spelled out, for the button's accessible name: on a collapsed
  // rail the name is the only place the figure's scope is said.
  "agent.rail.spend": "Cost this month",

  // The four section titles. Noun phrases: a heading names a thing, and "What
  // it has done" was a sentence pretending to be one.
  "agent.panel.runningNow": "Running now",
  "agent.panel.needsYou": "Needs attention",
  "agent.panel.recent": "Recent activity",
  "agent.panel.runtime": "Runtime",
  "agent.panel.fullLog": "Full log",

  // What the reader acts on, and the two quiet lines for when there is nothing.
  // Three different facts, so three different sentences: the head's resting
  // line, this section's empty arm and the log's empty arm all said some
  // version of "nothing" and two of them said the same four words.
  "agent.panel.decisions": "Approvals",
  "agent.panel.nothingWaiting": "Nothing waiting",
  "agent.panel.nothingToday": "Nothing finished today",
  // Live work of a kind the rail does not narrate: it admits the work and
  // offers nothing to read, because there is no row behind it.
  "agent.panel.unnamedLive": "Working in the background",

  // THE RUNTIME FACTS, as terms beside their values. Sentence case, because a
  // <dt> is a name rather than an inline word in a sentence — which is what
  // these were when the strip was one wrapped line of fragments.
  "agent.fact.model": "Model",
  "agent.fact.tools": "Tools",
  "agent.fact.sources": "Sources",
  "agent.fact.offline": "offline",
  // The two reasons there is no model to name, in the value's own slot.
  "agent.fact.noCalls": "No model calls yet",
  "agent.fact.hidden": "Hidden for your role",
  // The fault a badge carries, above the facts it invalidates.
  "agent.fact.noModel": "No model configured",

  // THE RAIL'S LINE. What the agent is doing, or the fault that stops it, in
  // one clause. A count is a plural base, because "1 decisions" is the tell of
  // a surface that pastes a number onto a noun.
  "agent.line.waiting_one": "{count} approval waiting",
  "agent.line.waiting_other": "{count} approvals waiting",
  "agent.line.allClear": "Nothing needs attention",
  "agent.line.cannotReach": "Cannot reach {sources}",
  "agent.line.runFailed": "A run failed",
  "agent.line.runStopped": "A run stopped early",
  "agent.line.justNow": "just now",

  // The panel's one setting, named for what a reader SEES rather than for the
  // surface that draws it — nobody outside this tree calls it the edge.
  "agent.setting.edgeLight": "Screen edge light",

  // The four things the agent says once it has run out of news. None claims the
  // agent did anything: they are standing facts about the product, which is
  // what lets them be written here rather than read from the installation.
  "agent.tip.day": "Tasks, approvals and duplicates are on {name}.",
  "agent.tip.ask": "Ask the agent from anywhere with ⌘K.",
  "agent.tip.recap": "Open the agent panel for today’s activity.",
  "agent.tip.edge": "The screen edge lights up while the agent works.",

  "agents.connected": "Connected agents",
  "agents.connectedSub":
    "MCP clients with their own credential, limited to the access you approved",
  "agents.noneConnected": "No agents connected yet.",
  "agents.connectedOn": "connected {date}",
  "agents.disconnect": "Disconnect",
  "agents.disconnectOpen": "Disconnect",
  "agents.disconnectNamed": "Disconnect {client}",
  "agents.disconnected": "Disconnected",
  "agents.lapsed": "Credential expired",
  "agents.renewing": "Renewing",
  "agents.renewsBy": "credential renews by {date}",
  "agents.expiredOn": "credential expired {date}",
  "agents.revokeGrantOpen": "End connection",
  "agents.revokeGrantNamed": "End connection to {client}",
  "agents.disconnectConfirm":
    "This ends the whole connection, not one credential. The agent loses access on its next call and cannot renew. Reconnecting requires approving access again.",
  "agents.connectHow": "Connect an agent",
  "agents.connectSteps":
    "Run one of these commands. The client registers itself and returns here so you can choose its access.",
  "agents.connectAntigravityPath":
    "Antigravity has no add command. Put that block in ~/.gemini/config/mcp_config.json.",
  "agents.connectorOff": "The MCP connector is off for this installation.",
  "agents.connectorOffDetail":
    "No agent can connect until an administrator or operations user enables it. Passports still work as REST credentials.",
  "settings.tokenOnce": "Copy it now. This credential is shown only once.",
  "settings.token": "Credential",
  "settings.autonomy": "Autonomy tiers",
  "settings.autonomySub": "What runs immediately and what waits for approval",
  "settings.tierRead":
    "Read, summarize, draft: runs immediately and is fully logged.",
  "settings.tierSend":
    "Send email, book meetings, update a contact or deal: runs immediately if the agent has that permission. Granting the permission is the approval.",
  "settings.tierWait":
    "Enrichment, custom fields, webhooks, tag merges: wait in Approvals.",
  "settings.tierAdvance":
    "Advance a deal stage: waits only when the move closes the deal as won or lost.",
  "settings.locked": "Locked",
  "settings.purposes": "Consent purposes",
  "settings.purposesSub":
    "What this installation asks consent for, and the lawful basis of each purpose.",
  "settings.created": "created {date}",
  "settings.expires": "expires {date}",
  "settings.revoked": "Revoked",
  "settings.revoke": "Revoke",
  "settings.revokeConfirm":
    "The passport’s credential is invalidated immediately. The agent loses access on its next call.",
  "import.withheld":
    "Only an administrator or operations user can import files.",
  "import.title": "Import file",
  "import.sub":
    "Import a CSV of leads, contacts or companies. Nothing is written until you review what the import will do.",
  "import.startLabel": "Import CSV file",
  "import.start": "Start import",
  "import.objectLabel": "Row type",
  "import.object.lead": "Prospects",
  "import.object.company": "Companies",
  "import.object.contact": "Contacts",
  "import.objectHint.lead":
    "Imports an unworked list as leads, for a user to qualify before they become contacts.",
  "import.objectHint.company":
    "Matched by the mapped name, so a re-upload updates instead of duplicating.",
  "import.objectHint.contact":
    "For existing contacts. Matched by email, so a re-upload updates instead of duplicating, and addresses already stored stay unchanged.",
  "import.fileLabel": "CSV file",
  "import.choose": "Choose file",
  "import.chooseAnother": "Choose another file",
  "import.profiled": "Rows profiled from the start of the file: {rows}.",
  // The name the mapping grid announces once it is wider than its box.
  "import.mappingTable": "Column mapping",
  "import.col.column": "Column",
  "import.col.filled": "Filled",
  "import.col.samples": "Values",
  "import.col.destination": "Field",
  "import.dontImport": "Do not import",
  "import.noSamples": "Empty",
  "import.destinationFor": "Field for {column}",
  "import.identifiedBy":
    "Rows are identified by {column}, so re-importing this file updates rows instead of duplicating them.",
  "import.needsIdentifier":
    "Map a column to {field}. Without it, rows cannot be matched on a later upload or undone.",
  "import.validate": "Preview import",
  "import.validating": "Checking…",
  "import.previewTitle": "Import preview",
  "import.outcomeTitle": "Import result",
  // Shown when the card read this run back on mount instead of the reader
  // having just caused it: an outcome with no press behind it reads as an
  // import that ran by itself.
  "import.resumedRun":
    "This import ran on {when}. All actions below remain available.",
  "import.count.created": "Create",
  "import.count.updated": "Update",
  "import.count.unchanged": "Unchanged",
  "import.count.skipped": "Skipped",
  "import.rowsRead": "{rows} rows read, identified by {column}.",
  "import.linksOffered":
    "{offered} rows name an employer; {unresolved} name a company not yet in the CRM.",
  "import.linksApplied": "{applied} of {offered} employer links written.",
  "import.issuesLead":
    "Some rows cannot be imported. Each is listed with its line number in the file.",
  "import.issueLine": "Line {line}:",
  "import.commit_one": "Import 1 row",
  "import.commit_other": "Import {rows} rows",
  "import.importing": "Importing…",
  "import.done": "Import complete",
  "import.failed":
    "The import stopped after {checkpoint} rows. Resume continues from that point.",
  "import.resume": "Resume import",
  "import.uploadFailed": "File could not be read",
  "import.resumedRunTitle": "Earlier import",
  "import.failedTitle": "Import stopped partway",
  "import.commitFailed": "Import did not run",
  "import.undoInterruptedTitle": "Undo stopped partway",
  "import.undoFailed": "Undo did not run",
  "import.validateFailed": "Preview did not run",
  "import.another": "Import another file",
  "import.undo_one": "Undo import (1 row)",
  "import.undo_other": "Undo import ({rows} rows)",
  "import.undoing": "Undoing…",
  "import.undoInterrupted":
    "The undo was interrupted. Continue resumes where it stopped.",
  "import.continueUndo": "Continue undo",
  "import.undone": "Import undone",
  "import.undoReversed_one": "1 row reversed.",
  "import.undoReversed_other": "{rows} rows reversed.",
  "import.undoKeptLead": "Kept because they were edited after the import:",
  "import.undoErroredLead": "Could not be reversed and left unchanged:",
  "settings.dangerZone": "Danger zone",
  "settings.dangerZoneSub":
    "Non-production installations only. Cannot be undone.",
  "settings.resetDataDesc":
    "Resets this installation to its first-boot state. Domain and configuration data is deleted; the company and its users are kept and stay signed in.",
  "settings.resetDataButton": "Reset data",
  "settings.resetDataLabel": "Reset all data",
  "settings.resetDataConfirmButton": "Reset all data",
  "settings.resetDataConfirmTitle": "Reset all data?",
  "settings.resetDataConfirmBody":
    "Type the company name to confirm. This cannot be undone.",
  "settings.resetDataConfirmName": "Type this company name:",
  "settings.resetDataConfirmLabel": "Confirm company name",
  "settings.resetDataResult":
    "Cleared: tables {tables}, job rows {jobs}, event streams {streams}, cache keys {keys}, stored files {objects}.",
  "settings.resetDataDrainWarning":
    "A background job was running when the reset began. It fails against the deleted data and logs one harmless error.",

  "settings.jobs": "Background jobs",
  "settings.jobsSub": "Queued background jobs and failed jobs by owner",
  "jobs.adminOnly":
    "Background job health covers the whole installation and requires a permission your role does not have.",
  "jobs.empty":
    "The background queue is empty. No jobs are waiting, running, retrying or dead.",
  "jobs.workspaceKinds": "This company",
  "jobs.workspaceEmpty": "No background jobs for this company.",
  "jobs.dispatcherKinds": "Fleet dispatchers",
  "jobs.dispatcherSub":
    "Rows with no company: a dispatcher distributes work to every company and runs none itself. Their counts belong to the installation.",
  "jobs.dispatcherEmpty":
    "No dispatcher rows. Periodic ticks re-insert them, so an empty list means none is scheduled now.",
  "jobs.count.waiting": "{count} waiting",
  "jobs.count.running": "{count} running",
  "jobs.count.retrying": "{count} retrying",
  "jobs.count.dead": "{count} dead",
  "jobs.queue": "queue {queue}",
  "jobs.waitedSeconds_one": "oldest has waited {count} second",
  "jobs.waitedSeconds_other": "oldest has waited {count} seconds",
  "jobs.waitedMinutes_one": "oldest has waited {count} minute",
  "jobs.waitedMinutes_other": "oldest has waited {count} minutes",
  "jobs.waitedHours_one": "oldest has waited {count} hour",
  "jobs.waitedHours_other": "oldest has waited {count} hours",
  "jobs.waitedDays_one": "oldest has waited {count} day",
  "jobs.waitedDays_other": "oldest has waited {count} days",
  // The window is in the TITLE, not only the body: a count with no span asks
  // the reader to guess, and the guess is "since forever" — which is what made
  // a finished outage keep this red for a week.
  "jobs.deadTitle": "Dead jobs in the last {hours}h: {count}",
  "jobs.deadTotal":
    "{count} discarded or canceled in the last 7 days, the retention period for these records.",
  "jobs.deadBody":
    "Jobs discarded or canceled that will not run without intervention: {count}. Discarded jobs used every attempt; canceled jobs were stopped on purpose.",
  "jobs.failures": "Recent failures",
  "jobs.failuresSub": "Most recent first, up to 50.",
  "jobs.failuresEmpty": "No failures recorded.",
  "jobs.state.retryable": "Retrying",
  "jobs.state.discarded": "Discarded",
  "jobs.state.cancelled": "Canceled",
  "jobs.attempt": "attempt {attempt} of {max} · {when}",
  "jobs.remedy": "What to do: {remedy}",
  "jobs.jobId": "job {id}",
  "jobs.failingSince": "failing since {when}",
  "jobs.reasonVetted":
    "Reasons, classes and remedies come from the job layer, never the worker’s raw cause. A failure it cannot phrase shows a fixed substitute and no class.",
  "jobs.generatedAt": "As of {time}",

  "settings.extIngest": "Refused connector records",
  "settings.extIngestSub":
    "Records from an installed connector that this CRM could not store.",
  "extIngest.adminOnly":
    "Connector intake health covers the whole installation and requires a permission your role does not have.",
  // The clean state is a FINDING, not an absence. A connector sending nothing
  // and a connector whose every record is refused looked identical before this
  // card, and the reader has to be able to tell this line from "no answer".
  "extIngest.empty":
    "No records refused in the last {days} days. Every record the installed connectors sent was accepted.",
  "extIngest.refusedTotal_one": "{count} record refused",
  "extIngest.refusedTotal_other": "{count} records refused",
  "extIngest.lastRefused": "most recent {when}",
  "extIngest.refusal.key": "record key",
  "extIngest.refusal.activity": "the activity itself",
  "extIngest.refusal.addresses": "addresses",
  "extIngest.refusal.counterparty": "counterparty",
  "extIngest.refusal.participants": "participants",
  "extIngest.refusal.size": "size limits",
  "extIngest.refusalCount": "{count} on {refusal}",
  // Said on the card because an operator who cannot find the detail here goes
  // looking for a bug rather than for the connector's own log.
  "extIngest.noDetail":
    "Shows counts and the refusing check only, never the record, because the refused field can quote sender content. The connector log has the full reason for each.",
  "extIngest.generatedAt": "As of {time}",

  "audit.you": "You",
  "audit.system": "System",
  "audit.unknownBuyer": "Deal Room participant",
  "audit.unknownMember": "Unknown member",
  "audit.viaAgent": "via an agent",
  "audit.viaConnector": "via a connector",
  "audit.viaDealRoom": "in the Deal Room",
  "audit.viaNamed": "via {client}",
  "audit.noHumanAuthority": "No human authority recorded",
  "settings.auditSub": "Every action, attributed to a user, agent or connector",
  "settings.auditAdminOnly":
    "Your role cannot read the full audit log. It records every actor and every record they accessed.",
  "settings.auditFilters": "Filters",
  "settings.auditEntries": "Audit log",
  "settings.auditTrailLabel": "Recorded actions",
  "settings.auditActor": "Actor",
  "settings.auditEntity": "Entity type",
  "settings.auditEntityId": "Entity ID",
  "settings.auditAction": "Action",
  "settings.auditFrom": "From",
  "settings.auditTo": "To",
  "settings.auditExpand": "Show change detail",
  "settings.auditRule": "Authorization rule",
  "settings.auditOnBehalf": "on behalf of",
  "settings.privacy": "Privacy requests",
  "settings.privacySub": "Data subject requests with their statutory deadlines",
  "settings.due": "due {date}",

  "privacy.addPurpose": "Add purpose",
  "privacy.corrections": "Correction requests",
  "privacy.correctionsSub":
    "Contacts entered these through the link they were emailed. None has changed the record yet; a correction is a request until someone accepts it.",
  "privacy.correctionsEmpty": "Nothing waiting",
  "privacy.correctionsEmptySub":
    "Corrections a contact sends through their own link appear here for someone to answer.",
  "privacy.correctionRemoval": "Asked to be removed",
  "privacy.correctionDecide": "Decide",
  "privacy.correctionUnnamed": "Unnamed contact",
  "privacy.correctionAcknowledge": "Mark as read",
  "privacy.correctionNote": "Reason (the contact may ask)",
  "privacy.correctionAccept": "Accept and update",
  "privacy.correctionReject": "Leave as it is",
  "privacy.purposesRegistry": "Registered purposes",
  "privacy.purposesReadOnly":
    "Read-only. Adding a purpose needs a permission your role does not have.",
  "privacy.purposeKey": "Key",
  "privacy.purposeLabel": "Label",
  "privacy.purposeDoi": "Requires double opt-in",
  "privacy.purposeCreate": "Create purpose",
  "privacy.purposeAppendOnly":
    "A purpose cannot be renamed or removed once created; the catalog is append-only. Choose the key carefully.",
  "notice.title": "Disclosure duties",
  "notice.sub":
    "Contacts obtained without asking them, and whether they have been told yet.",
  "notice.facetLabel": "Show duties",
  "notice.facetOwed": "Still owed",
  "notice.facetAll": "All",
  "notice.emptyOwed":
    "Nothing is owed. Every contact obtained has been told, or the duty was excused.",
  "notice.readOnlyForPrivacy":
    "These duties name contacts and how they were obtained, so only users with access to privacy requests can see them.",
  "notice.dueAt": "Due {date}",
  "notice.overdue": "Overdue",
  "privacynotice.title": "What we hold about you",
  "privacynotice.intro":
    "We are telling you this because the law requires it. You do not need to reply or do anything.",
  "privacynotice.source.title": "Where your details came from",
  "privacynotice.source.when": "Obtained on {date}",
  "privacynotice.source.subjectInitiated": "You wrote to us first.",
  "privacynotice.source.customerContract":
    "You are a contact on a business relationship with us.",
  "privacynotice.source.requested": "You asked us for a quote or a meeting.",
  "privacynotice.source.inPerson": "Someone recorded a conversation with you.",
  "privacynotice.source.referral": "Someone else gave us your details.",
  "privacynotice.source.eventOrForm":
    "You filled in a form or registered for something.",
  "privacynotice.source.publicSource":
    "We found your details in a public or business source, such as a directory or a company website.",
  "privacynotice.source.purchasedOrImported":
    "Your details came from a list that was bought or imported.",
  "privacynotice.source.unknown": "We cannot say how your details reached us.",
  "privacynotice.purposes.title": "What we use it for",
  "privacynotice.rights.title": "Your rights over this",
  "privacynotice.rights.how":
    "To use any of these, reply to the message that brought you here, or contact us through the address on our website.",
  "privacynotice.right.access": "Ask for a copy of what we hold about you.",
  "privacynotice.right.rectification":
    "Ask us to correct anything that is wrong.",
  "privacynotice.right.erasure": "Ask us to delete it.",
  "privacynotice.right.restriction":
    "Ask us to stop using it while something is disputed.",
  "privacynotice.right.objection": "Object to how we use it.",
  "privacynotice.right.complain": "Complain to your data protection authority.",
  "notice.claimed": "Claimed",
  "notice.unclaimed": "Unclaimed",
  "notice.excuse": "End without sending",
  "notice.excuseTitle": "End duty without sending disclosure",
  "notice.excuseWhich": "Reason type",
  "notice.excuseProvided": "Already informed elsewhere",
  "notice.excuseExempt": "Duty does not apply",
  "notice.excuseGround": "Reason, in your own words",
  "notice.excuseConfirm": "Record reason",
  "privacy.caseNotHere": "The request this link names is not on this page yet.",
  "privacy.caseNotHereBody":
    "It may be under another status filter, or further down the list, which loads 20 at a time. Select its status, or load more.",
  "privacy.facetAll": "All",
  "privacy.inboxAdminOnly":
    "Viewing subject requests needs a permission your role does not have. The requests name the people who asked, so access is restricted.",
  "privacy.overdue": "Overdue",
  "privacy.closed":
    "Closed. A closed request cannot be reopened; a new concern needs a new request.",
  "privacy.assignee": "Assignee",
  "privacy.assigneeUnassignable":
    "Once set, an assignee cannot be cleared here.",
  "privacy.resolution": "Resolution",
  "privacy.resolutionRequired": "Closing a request needs its answer.",
  "privacy.movedOn":
    "Someone else decided this request first. Review the current state below.",
  "privacy.inProgress": "In progress",
  "privacy.fulfil": "Fulfill",
  "privacy.reject": "Reject",
  "privacy.newRequest": "New request",
  "privacy.queue": "Requests",
  "privacy.kind": "Kind",
  "privacy.contact": "Contact",
  "privacy.subjectRef": "Subject reference",
  "privacy.dueAt": "Due",
  "privacy.openRequest": "Open request",
  "privacy.erasureNeedsContact":
    "An erasure request must name a contact in this company, because fulfilling it erases that record. A free-text subject cannot be erased.",
  "privacy.accessManual":
    "An access request is fulfilled manually: record what you sent in the resolution. This system does not assemble or export the data for you.",
  "privacy.fulfilErasureTitle": "Fulfill erasure request",
  "privacy.erasureIrreversible":
    "This permanently erases the contact across the whole system: record, captured activity and derived values. It cannot be undone. The erasure itself is audited.",
  "privacy.typeErase": "Type ERASE to confirm",
  "privacy.erasureConfirm": "Erase and suppress",
  "privacy.legalHoldTitle": "Blocked by legal hold",
  "privacy.legalHold":
    "This contact is inside a statutory retention window, so erasure does not take precedence here (Art. 17(3)(b)). The block applies to every role, including administrators, with no override. The attempt was audited.",

  "restricted.title": "Restricted records",
  "restricted.sub":
    "Records a statutory retention obligation holds after an erasure: which record, why and until when. The correspondence itself is hidden so that it is not read.",
  "restricted.withheld":
    "Only an administrator or operations user can see which records a statutory obligation holds. It uses the same permission as retention policies.",
  "restricted.empty":
    "No records held. Every erasure so far was completed in full.",
  "restricted.heldLabel": "Records held now",
  "restricted.kind": "Record",
  "restricted.occurred": "Dated",
  "restricted.deals": "Deal or project",
  "restricted.noDeal": "No deal on record",
  "restricted.reason": "Held because",
  "restricted.until": "Held until",
  "restricted.redacted": "Redacted",
  "restricted.nothingRedacted": "Nothing removed",
  "restricted.redactedCount_one": "{count} field removed",
  "restricted.redactedCount_other": "{count} fields removed",
  "restricted.class.commercialCorrespondence": "Commercial correspondence",
  "restricted.kind.email": "Email",
  "restricted.kind.call": "Call",
  "restricted.kind.meeting": "Meeting",
  "restricted.kind.message": "Message",
  "restricted.decide": "Decision",
  "restricted.reasonLabel": "Why",
  "restricted.reasonHint":
    "Recorded in the audit trail with your name. State what you decided and on what basis.",
  "restricted.release.action": "Release",
  "restricted.release.title": "Release record from legal hold?",
  "restricted.release.body":
    "Releasing ERASES the record; it does not return it to use. The erasure request this obligation suspended is still open, so releasing completes it. This cannot be undone.",
  "restricted.release.confirm": "Release and erase",
  "restricted.pin.action": "Pin a record",
  "restricted.pin.submit": "Pin",
  "restricted.pin.idHint":
    "For correspondence the automatic rule cannot recognize, such as supplier and purchasing mail under §257 HGB with no deal here. The record ID is on its audit entry.",
  "restricted.pin.idMalformed":
    "Not a valid record ID. The format is 8-4-4-4-12 hexadecimal characters, shown in full on the record’s audit entry.",
  "restricted.pin.idPlaceholder": "Record ID",
  "restricted.pin.title": "Place record under legal hold?",
  "restricted.pin.body":
    "The record is held for the statutory window: hidden from every ordinary view, locked, and erased when the window closes. Its identifiers are redacted now.",
  "restricted.pin.confirm": "Pin and hold",
  "retention.title": "Retention",
  "retention.sub":
    "How long each record type is kept, and what happens when its window ends",
  "retention.retainOnly": "Retain-only mode",
  "retention.retainOnlyHelp":
    "While on, this installation destroys nothing: no anonymizing and no erasing, whatever a policy below says. Archiving still runs; an archived record is kept, not destroyed.",
  "retention.adminOnly":
    "Only an administrator or operations user can change retention.",
  "retention.withheld":
    "Only an administrator or operations user can see retention policies. They set what this installation keeps for everyone.",
  "retention.addPolicy": "Add policy",
  "retention.create": "Create policy",
  "retention.scope": "Applies to",
  "retention.window": "Window in days",
  "retention.windowDays": "{days} days",
  "retention.windowInvalid": "Enter a whole number of days, at least 1.",
  "retention.action": "Action",
  "retention.actionHint":
    "Archive keeps the record. Anonymize and erase destroy data and are held back in retain-only mode.",
  "retention.lawfulBasis": "Lawful basis",
  "retention.lawfulBasisHint":
    "Optional. The Art. 6 basis this window relies on, for auditors reading the policy.",
  "retention.enabled": "Enabled",
  "retention.edit": "Edit",
  "retention.save": "Save policy",
  "retention.delete": "Delete policy",
  "retention.deleteTitle": "Delete retention policy?",
  "retention.deleteBody":
    "This removes the rule for {scope} entirely, so nothing in that scope ages out anymore. To pause the rule and keep its window, turn off Enabled instead.",
  "retention.duplicateScope":
    "A policy for this scope already exists; each scope has at most 1 rule. Edit the existing policy instead.",
  "retention.empty":
    "No retention policy yet. Nothing in this installation ages out.",
  "retention.effectActing": "Acting nightly",
  "retention.effectSuppressed": "Paused by retain-only mode",
  "retention.effectDisabled": "Disabled",
  "retention.suppressedWhy":
    "Enabled, but retain-only mode holds it back: this rule destroys data, so it does not act until the mode is turned off.",
  "retention.disabledWhy":
    "Turned off and kept. Its window is preserved, and nothing in this scope ages out while it is off.",
  "retention.actionArchive": "Archive",
  "retention.actionAnonymize": "Anonymize",
  "retention.actionErase": "Erase",
  "retention.scopeLeadUnconverted": "Leads that never converted",
  "retention.scopeActivity": "All captured activity",
  "retention.scopeActivityTranscript": "Call transcripts",
  "retention.scopeContactNoConsentNoDeal":
    "Contacts with no consent and no deal",
  "retention.scopeDealLost": "Lost deals",
  "retention.scopeDealWon": "Won deals",
  "retention.scopeAiCallPayloadContent": "AI call payloads",

  "retention.scopeRawCapture": "Stored message originals",
  "settings.pipelines": "Pipelines",
  "settings.pipelinesReadOnly":
    "Read-only. Your role cannot change pipelines or stages.",
  "settings.pipelinesSub":
    "Stages a deal moves through, one sequence per pipeline.",
  "pipeline.new": "New pipeline",
  "pipeline.edit": "Edit pipeline",
  "pipeline.name": "Name",
  "pipeline.default": "Default",
  "pipeline.notDefault": "Not default",
  "pipeline.position": "Position",
  "pipeline.retired": "Retired",
  "pipeline.retire": "Retire",
  "pipeline.retireConfirm":
    "Retire {name}? It leaves pickers and new-deal forms. Deals on it keep their stage, history and forecast contribution, and you can restore it at any time.",
  "pipeline.retireBlocked":
    "This is the default pipeline, and new deals need one. Make another pipeline the default first, then retire this one.",
  "pipeline.retired.done": "{name} retired",
  "pipeline.restore": "Restore",
  "pipeline.restored": "{name} restored",
  "stage.new": "New stage",
  "stage.edit": "Edit stage",
  "stage.name": "Name",
  "stage.semantic": "Stage type",
  "stage.winProb": "Win probability",
  "stage.semOpen": "Open",
  "stage.semWon": "Won",
  "stage.semLost": "Lost",
  "stage.remove": "Remove",
  "stage.removeConfirm": "Remove stage",
  "stage.removeTitle": "Remove this stage?",
  "stage.removeBody":
    "“{name}” leaves the pipeline and later stages move up. Past stage changes stay readable. Move its deals before removing it.",
  "stage.criteria.title": "Exit criteria",
  "stage.criteria.sub": "What must be true before a deal leaves this stage.",
  "stage.criteria.buyerCalloutTitle": "Evidence must come from the buyer",
  "stage.criteria.buyerCallout":
    "A message from your team never satisfies a criterion about a buyer action.",
  "stage.criteria.unreadableTitle": "Criteria did not load",
  "stage.criteria.unreadable":
    "Reload before changing them; the list may be incomplete.",
  "stage.criteria.none": "No exit criteria yet.",
  "stage.criteria.terminal":
    "A won or lost stage is final, so it has no exit criteria.",
  "stage.criteria.add": "Add criterion",
  "stage.criteria.key": "Key",
  "stage.criteria.keyHint":
    "The name evidence cites. Lowercase letters, digits and underscores. It cannot be changed later.",
  "stage.criteria.label": "Label",
  "stage.criteria.kind": "Kind",
  "stage.criteria.required": "Required",
  "stage.criteria.optional": "Optional",
  "stage.criteria.hint": "Hint",
  "stage.criteria.edit": "Edit criterion",
  "stage.criteria.remove": "Remove",
  "stage.criteria.removeTitle": "Remove this criterion?",
  "stage.criteria.removeBody":
    "“{name}” is no longer asked for. Evidence already recorded against it stays readable.",
  "stage.criteria.kindBuyerConfirmed": "Buyer confirmed",
  "stage.criteria.kindEventHeld": "Event held",
  "stage.criteria.kindDocumentSigned": "Document signed",
  "stage.criteria.kindRoleIdentified": "Role identified",
  "stage.criteria.kindTermsAccepted": "Terms accepted",
  "stage.criteria.kindCustom": "Custom",

  "ob.url": "Website",
  "ob.urlScheme": "https://",
  "ob.back": "Back",
  "ob.restoring": "Restoring setup…",
  "ob.readManual": "Enter manually",
  "ob.coreIntroTitle": "Start with the legal company",
  "ob.coreIntroBody":
    "Margince needs the legal name, address and VAT or register number, then what the company sells and to whom.",
  "ob.coreLegalKicker": "Legal identity first",
  "ob.corePathLabel": "What Margince learns",
  "ob.corePathLegal": "Legal identity",
  "ob.corePathOffer": "Offer",
  "ob.corePathCustomer": "Customers",
  "ob.coreReadingPage": "Reading",
  "ob.coreWebsiteTitle": "Which website should Margince read?",
  "ob.coreWebsiteBody":
    "The legal notice is read first, then products, customers and positioning.",
  "ob.corePreparing": "Preparing to read {host}",
  "ob.coreLegalReading": "Reading legal identity on {host}",
  "ob.coreLegalReadingBody":
    "Looking for the legal notice, address and register or VAT number. Unstated details stay empty.",
  "ob.coreBusinessReading": "Learning how the business works",
  "ob.coreBusinessReadingBody":
    "Linking products, customers and positioning to the public text that supports them.",
  "ob.coreReady": "Cited company details found: {count}",
  "ob.corePartial": "Useful details found: {count}. Some gaps remain.",
  "ob.coreReadyBody":
    "Nothing is saved yet. Review the legal identity first, then the offer and customer.",
  "ob.coreDeferredBody": "This read resumes automatically.",
  "ob.coreFailedBody":
    "The site could not be read well enough, so reading stopped rather than guess. Enter the details manually.",
  "ob.coreFindingsTitle": "Supported findings",
  "ob.coreFindingsBody":
    "Each value shows the public wording behind it. Values that cannot be verified stay empty.",
  "ob.ai.identity": "Margince",
  "ob.ai.role": "Company research AI",
  "ob.ai.speaker": "M",
  "ob.ai.speakerName": "Margince",
  "ob.ai.ready": "Ready to research",
  "ob.ai.configured": "Configured AI",
  "ob.ai.modelsUsed": "Models used in this task",
  "ob.ai.route": "Task · tier · provider",
  "ob.ai.calls": "AI calls",
  "ob.ai.tokens": "Tokens",
  "ob.ai.latency": "Model latency",
  "ob.ai.estimatedCost": "Estimated provider cost",
  "ob.ai.partialEstimate": "Partial · unpriced usage exists",
  "ob.ai.awaitingModel": "Shown after the first model call",
  "ob.ai.notAvailableYet": "Not available yet",
  "ob.ai.runtimeUnavailable": "Runtime details unavailable",
  // The runtime disclosure is a chip you can open rather than a permanent
  // band: cost is stated WHILE it is being spent, but a reader deciding
  // whether a legal entity is right should not have to read a billing table
  // to get to it.
  "ob.ai.runtimeChip": "Active model and cost",
  "ob.ai.answeringNow": "Active model",
  "ob.ai.runScope": "This run only. The full log is in Settings under AI.",
  "ob.ai.tier.localSmall": "local, fast",
  "ob.ai.tier.cheapCloud": "cloud, efficient",
  "ob.ai.tier.premium": "premium reasoning",
  "ob.ai.tier.frontier": "frontier reasoning",
  "ob.ai.tier.localLarge": "local, advanced",
  // The rail footer's plain-language line: the exact ids sit one click away
  // in the runtime chip's "Configured AI" row, so this says only what a
  // non-technical reader needs at a glance — how many models, and where.
  "ob.ai.summary.cloud_one": "1 model, running in the cloud",
  "ob.ai.summary.cloud_other": "{count} models, running in the cloud",
  "ob.ai.summary.local_one": "1 model, running locally",
  "ob.ai.summary.local_other": "{count} models, running locally",
  "ob.ai.summary.hybrid_one": "1 model, split between cloud and local",
  "ob.ai.summary.hybrid_other": "{count} models, split between cloud and local",
  "ob.ai.summary.development_one": "1 model, development mode",
  "ob.ai.summary.development_other": "{count} models, development mode",
  "ob.ai.summary.none": "No model configured yet",
  "ob.ai.summaryProviders_one": "1 provider configured",
  "ob.ai.summaryProviders_other": "{count} providers configured",
  "ob.ai.readFirst": "Start company setup before asking about it.",
  "ob.ai.liveArtifact": "Live draft for review",
  "ob.ai.companyKnowledge": "Company knowledge",
  "ob.ai.companyKnowledgeBody":
    "Website evidence stays separate from this chat. You decide what becomes company context.",
  "ob.ai.companyKnowledgeManualBody":
    "Your answers and Margince’s suggestions stay editable here. You decide what becomes company context.",
  "ob.ai.askPlaceholder":
    "Ask about a finding, correct a detail or add what is missing",
  "ob.ai.send": "Send to Margince",
  "ob.ai.reviewBoundary":
    "Margince can suggest changes here and applies them to the draft only when you approve.",
  "ob.ai.confirmBoundary":
    "Nothing becomes company context until you confirm this draft.",
  "ob.ai.confirmCompany": "Confirm and save company",
  "ob.ai.thinking": "Checking the dossier…",
  "ob.ai.suggestedChanges": "Suggested changes to the draft",
  "ob.ai.applyChanges": "Apply to draft",
  "ob.ai.applied": "Applied to draft",
  "ob.ai.finding_one": "cited finding",
  "ob.ai.finding_other": "cited findings",
  "ob.continueManual": "Enter manually",
  "ob.readStatus.queued": "Preparing",
  "ob.readStatus.deferred": "Waiting for AI allowance",
  "ob.readStatus.reading": "Reading",
  "ob.readStatus.ready": "Reading finished",
  "ob.readStatus.partial": "Finished with gaps",
  "ob.readStatus.failed": "Input needed",
  "ob.readStatus.confirmed": "Choices saved",
  "ob.readStatus.abandoned": "Stopped",
  "ob.pagesRead": "pages read",
  "ob.legalEntitiesFound": "legal entities found",
  "ob.coverageDetails": "Coverage and unread pages",
  "ob.legalFoundTitle": "Legal entities found",
  "ob.legalFoundBody":
    "Each block shows the registered name, address and register or VAT number. Select the right one in the review.",
  "ob.legalEntity": "Legal entity",
  "ob.confirmWebsite":
    "Based on {count} public pages. Edit any value; unedited values keep their evidence.",
  "ob.confirmManual":
    "These answers came from you and are stored as human assertions.",
  "ob.legalTitle": "Select legal entity",
  "ob.legalSub":
    "The legal notice names several entities. Select yours to fill in its details.",
  "ob.factsTitle": "Other facts found",
  "ob.factsSelected": "{selected} of {total} selected",
  "ob.factsSub":
    "Clear any fact that should not become company context. Up to 100 facts can be selected.",

  // No step number: the rail decides how many stops a reader gets, so the
  // count belongs to ob.conv.scene.step, which reads it off the rail. A total
  // written into a string here can only ever disagree with the real one.
  "ob.s1.kick": "Confirm",
  "ob.s1.title": "Review company details",
  "ob.s1.sub":
    "Only details supported by the website are filled in. Correct anything that is wrong.",
  "ob.s1.urlPlaceholder": "yourcompany.com",
  "ob.s1.identityLabel": "Legal company",
  "ob.s1.offerLabel": "Products and offer",
  "ob.s1.customerLabel": "Customer",
  "ob.s1.salesLabel": "Positioning and sales context",
  "ob.s1.fieldRequired": "Required",
  "ob.s1.requiredMissing": "Complete these fields to continue: {fields}",
  "ob.s1.saving": "Saving…",
  "ob.s1.saveFailed": "Company not saved",
  "ob.s1.savedNote":
    "Saved to the company. Changes here are saved again when you continue.",
  "ob.readGo": "Read website",
  "ob.urlWillRead": "Reads {host}",
  "ob.readFromSite": "read from site",
  "ob.failTitle": "Not enough could be read from this website",

  "ob.manualChapterLegal": "Legal company",
  "ob.manualChapterOffer": "Products and offer",
  "ob.manualChapterCustomer": "Ideal customer",
  "ob.manualChapterSales": "Sales approach",
  "ob.manualNext": "Next question",
  "ob.manualLater": "Add later",
  "ob.manualReview": "Review answers",
  "ob.manualRequired": "Required for a usable company profile",
  "ob.manualOptional": "Optional. Leave empty to add later.",
  "ob.manual.display_name": "What name do customers know your company by?",
  "ob.manual.display_nameHint":
    "The familiar or trading name shown throughout Margince.",
  "ob.manual.legal_name": "What is the full registered legal name?",
  "ob.manual.legal_nameHint":
    "Include the legal form where it applies, such as GmbH, Ltd, Inc. or AG.",
  "ob.manual.registered_address": "What is the registered address?",
  "ob.manual.registered_addressHint":
    "Use the official address from the commercial register or legal notice.",
  "ob.manual.register_vat": "What are the register and VAT/UID numbers?",
  "ob.manual.register_vatHint":
    "Enter the identifiers exactly as issued. Leave empty if none apply.",
  "ob.manual.legal_form": "What legal form does the company have?",
  "ob.manual.legal_formHint":
    "The form as the register states it, such as GmbH, AG or Ltd.",
  "ob.manual.register_court": "Which court holds the register entry?",
  "ob.manual.register_courtHint":
    "The court named in the legal notice, such as Amtsgericht Charlottenburg.",
  "ob.manual.register_number": "What is the commercial register number?",
  "ob.manual.register_numberHint":
    "The register entry alone, such as HRB 12345 B. The VAT ID goes in the field above.",
  "ob.manual.industry": "Which industry is the company in?",
  "ob.manual.industryHint": "The description customers recognize immediately.",
  "ob.manual.history": "Is there useful company history Margince should know?",
  "ob.manual.historyHint":
    "For example founding year, origin or a major change in the business.",
  "ob.manual.offer_summary": "What products or services do you sell?",
  "ob.manual.offer_summaryHint":
    "1 or 2 concrete sentences. Margince uses this as the commercial description.",
  "ob.manual.value_proposition": "What outcome does the offer create?",
  "ob.manual.value_propositionHint":
    "The value customers receive, beyond product features.",
  "ob.manual.usp": "What makes customers choose you?",
  "ob.manual.uspHint": "The strongest difference from the alternatives.",
  "ob.manual.icp": "Who is your ideal customer?",
  "ob.manual.icpHint":
    "The companies or people that benefit most: size, industry, situation or geography.",
  "ob.manual.buying_center": "Who evaluates, buys or approves the purchase?",
  "ob.manual.buying_centerHint": "Typical roles, and who has the final say.",
  "ob.manual.customer_pains": "What problems bring those customers to you?",
  "ob.manual.customer_painsHint": "Problems as customers describe them.",
  "ob.manual.desired_outcomes": "What are they trying to achieve?",
  "ob.manual.desired_outcomesHint":
    "Practical or business outcomes they care about.",
  "ob.manual.buying_intents": "What usually signals buying interest?",
  "ob.manual.buying_intentsHint":
    "For example a new initiative, hiring pattern, deadline or operational problem.",
  "ob.manual.common_objections": "What objections do you hear most often?",
  "ob.manual.common_objectionsHint":
    "Concerns that often slow or stop a purchase.",
  "ob.manual.sales_motion": "How does a typical sale happen?",
  "ob.manual.sales_motionHint":
    "The path from first contact to decision, including trials or procurement where relevant.",

  "ob.field.display_name": "Company name",
  "ob.field.offer_summary": "Products and services",
  "ob.field.icp": "Ideal customer",
  "ob.field.buying_center": "Buying center",
  "ob.field.value_proposition": "Value proposition",
  "ob.field.usp": "Unique selling point",
  "ob.field.customer_pains": "Customer pains",
  "ob.field.desired_outcomes": "Desired outcomes",
  "ob.field.buying_intents": "Buying intents",
  "ob.field.common_objections": "Common objections",
  "ob.field.sales_motion": "Sales motion",
  "ob.field.legal_name": "Registered legal name",
  "ob.field.registered_address": "Registered address",
  "ob.field.register_vat": "Register and VAT ID",
  "ob.field.legal_form": "Legal form",
  "ob.field.register_court": "Register court",
  "ob.field.register_number": "Register number",
  "ob.field.industry": "Industry",
  "ob.field.history": "Company history",

  // What the answer should contain and why the CRM wants it. Shown on an
  // empty deck card so the card is answerable without guessing; still shown
  // once evidence exists, because the evidence states a claim and this states
  // a purpose, and the two are not the same sentence.
  "ob.fieldHint.display_name":
    "The name customers use, not the legal name. Shown across Margince.",
  "ob.fieldHint.offer_summary":
    "1 or 2 plain sentences on what the company sells.",
  "ob.fieldHint.icp": "Who benefits most, by size, industry or situation.",
  "ob.fieldHint.buying_center": "Roles that evaluate or sign off.",
  "ob.fieldHint.value_proposition":
    "The outcome a customer gets, not a product feature.",
  "ob.fieldHint.usp":
    "The difference that decides a purchase, not a strength every competitor claims.",
  "ob.fieldHint.customer_pains": "The problem in the customer’s own words.",
  "ob.fieldHint.desired_outcomes":
    "What the customer wants to achieve, in business terms.",
  "ob.fieldHint.buying_intents":
    "A signal that a prospect is close to buying, such as a hire or a deadline.",
  "ob.fieldHint.common_objections":
    "The concern that most often slows or stops a deal.",
  "ob.fieldHint.sales_motion":
    "The path from first contact to signed deal, including any trial or procurement step.",
  "ob.fieldHint.legal_name":
    "The name as registered, including the legal form. Used on invoices.",
  "ob.fieldHint.registered_address":
    "The address in the legal notice, not a mailing or showroom address.",
  "ob.fieldHint.register_vat":
    "Both identifiers exactly as issued. They appear on invoices and contracts.",
  "ob.fieldHint.legal_form": "The form as the register states it.",
  "ob.fieldHint.register_court":
    "The court named in the legal notice that holds the register entry.",
  "ob.fieldHint.register_number":
    "The register entry only. The VAT ID has its own field above.",
  "ob.fieldHint.industry":
    "The description customers recognize, not an internal classification code.",
  "ob.fieldHint.history":
    "Only if it changes how the company is understood, such as a founding year or major pivot.",

  // A worked example, not an instruction: what a filled-in answer looks like,
  // never the label restated. The legal fields print a real German imprint's
  // shape because that is the notice the read parses.
  "ob.fieldEg.display_name": "Northwind Robotics",
  "ob.fieldEg.offer_summary":
    "Cloud inventory software for mid-size retailers.",
  "ob.fieldEg.icp": "Retail chains with 20 to 200 stores.",
  "ob.fieldEg.buying_center": "Head of Operations, with Finance approving.",
  "ob.fieldEg.value_proposition":
    "Cuts stock-out incidents by half within a quarter.",
  "ob.fieldEg.usp": "Only supplier offering same-day, on-site support.",
  "ob.fieldEg.customer_pains": "“Stock runs out before anyone notices.”",
  "ob.fieldEg.desired_outcomes": "Never miss a reorder deadline again.",
  "ob.fieldEg.buying_intents": "A new warehouse opening within 90 days.",
  "ob.fieldEg.common_objections": "Worried about migrating off the old system.",
  "ob.fieldEg.sales_motion": "Demo, a two-week pilot, then a yearly contract.",
  "ob.fieldEg.legal_name": "Northwind Robotics GmbH",
  "ob.fieldEg.registered_address": "Musterstraße 12, 10115 Berlin",
  "ob.fieldEg.register_vat": "DE123456789",
  "ob.fieldEg.legal_form": "GmbH",
  "ob.fieldEg.register_court": "Amtsgericht Charlottenburg",
  "ob.fieldEg.register_number": "HRB 12345 B",
  "ob.fieldEg.industry": "E-commerce logistics",
  "ob.fieldEg.history": "Founded 2015, spun off from a logistics startup.",

  "ob.s4.provGoogle": "Google",
  "ob.s4.provMicrosoft": "Microsoft",
  "ob.s4.provImap": "Other mailbox (IMAP/SMTP)",
  "ob.s4.microsoftBtn": "Connect Microsoft",
  "ob.s4.microsoftHint":
    "Reads your mail and can send from it. You grant both on Microsoft’s screen and can disconnect at any time.",
  "ob.s4.microsoftUnverified":
    "An “unverified app” notice may appear. It refers to this self-hosted installation, not a third party.",
  "ob.s4.microsoftFailed": "Microsoft connection not completed",
  "ob.s4.connectOkTitle": "Mailbox connected",
  "ob.s4.connectOkBody": "Capture starts on the next sync.",
  "ob.s4.connectVerifying": "Confirming connection…",
  "ob.s4.connectLive": "Live and capturing",
  "ob.s4.connectConfirmFailed": "The connection could not be confirmed.",
  "ob.s4.connectRetry": "Connect again in Settings under Connections.",
  "ob.s4.connectDenied": "Access was declined. Nothing was connected.",
  "ob.s4.googleBtn": "Connect Gmail",
  "ob.s4.googleHint":
    "Reads your mail and can send from it. You grant both on Google’s screen and can disconnect at any time.",
  "ob.s4.googleUnverified":
    "If Google warns about an “unverified app”, select Advanced, then Continue. Google’s screen lists exactly what is granted.",
  "backfill.title": "Import mailbox history",
  "backfill.intro":
    "Choose how far back to import. The scope and estimated cost are shown before anything runs, and this step can be skipped.",
  "backfill.windowLabel": "Import window",
  "backfill.window36m": "3 years",
  "backfill.window84m": "7 years",
  "backfill.window120m": "10 years",
  "backfill.since": "Imports email since {date}.",
  "backfill.extendNote":
    "The history can be extended later. Emails already imported are kept and not duplicated.",
  "backfill.costFloorNote":
    "This estimate covers only the messages counted so far. The full import can contain more messages and cost more.",
  "backfill.window3m": "3 months",
  "backfill.window6m": "6 months",
  "backfill.window12m": "1 year",
  "backfill.window24m": "2 years",
  "backfill.window60m": "5 years",
  "backfill.previewLoading": "Counting messages…",
  "backfill.scopeIs": "Imports {window} of your mailbox.",
  "backfill.estimateMessagesExact_one": "{count} message in that period.",
  "backfill.estimateMessagesExact_other": "{count} messages in that period.",
  "backfill.estimateMessagesAtLeast_one":
    "At least {count} message in that period. Counting stopped there, so there may be more.",
  "backfill.estimateMessagesAtLeast_other":
    "At least {count} messages in that period. Counting stopped there, so there may be more.",
  "backfill.estimateCost": "Estimated AI cost:",
  "backfill.estimateNote":
    "An estimate, not a bill. Actual usage is metered and shown as it accrues.",
  "backfill.startCta": "Start import",
  "backfill.starting": "Starting…",
  "backfill.skip": "Skip mailbox history import",
  "backfill.skippedNote":
    "No history imported. New mail is still captured, and an import can be started later from Settings.",
  "backfill.loading": "Checking import status…",
  "backfill.statusUnavailable":
    "Import status is unavailable. Capture continues.",
  "backfill.queuedTitle": "Import queued",
  "backfill.runningTitle": "Importing mailbox history",
  // The pill beside a live title: the indigo on the card is a claim that a
  // machine is doing the reading, and this is the same claim in words.
  "backfill.readingBadge": "Analyzing",
  "backfill.doneTitle": "Mailbox history import complete",
  "backfill.errorTitle": "Import error",
  "backfill.cancelledTitle": "Import canceled",
  "backfill.progressLabel": "Import progress",
  "backfill.countScanned": "Messages scanned",
  "backfill.statEmails": "Emails captured",
  "backfill.statContacts": "Contacts",
  // The count is domains this run raised a company question for, not
  // companies created — a domain becomes one only if its site says so.
  "backfill.statCompanies": "Companies to check",
  "backfill.errorNote":
    "The import retries automatically. Everything captured so far is kept.",
  "backfill.cancel": "Stop import",
  "backfill.cancelledNote": "Stopped. Everything captured so far is kept.",
  // On a run that has stopped — cancelled, failed or finished. The window it
  // opens on is the one that ran, because the server only ever widens.
  "backfill.restart": "Start another import",
  "backfill.unsupportedNote":
    "This mailbox type does not support history import. Only new mail is captured.",
  "backfill.narrowingNote":
    "A wider window already ran for this mailbox. The import window can only be widened.",
  "backfill.staleUpdated": "Last updated {duration} ago. No recent progress.",

  // The units an installation composed, offered on the settings page that
  // already holds the kind of credential each one is configured with. The two
  // headings differ because the two pages mean different things — one is your
  // own account somewhere, the other is the installation's.
  "extUnits.open": "Open",
  "extUnits.openNamed": "Open {name} page",
  "extUnits.user.title": "Your other accounts",
  "extUnits.user.sub":
    "Accounts this installation can connect on your behalf. Each is private to you, and disconnecting it affects only you.",
  "extUnits.workspace.title": "Installation add-ons",
  "extUnits.workspace.sub":
    "Add-ons this installation runs with one shared credential. Settings here apply to all users.",

  // Connected inboxes (Settings → Connections): the "manage in Settings"
  // surface the onboarding copy promises.
  "connectors.title": "Connected mailboxes and calendars",
  // The rep's standing overnight authority — one question, asked beside the
  // mailbox connect in onboarding and again in Settings. The danger line names
  // the features that go empty, because "some things stop working" is not
  // something a rep can weigh.
  "overnightGrant.title": "Overnight preparation",
  "overnightGrant.sub":
    "Margince works through your deals overnight and has your Morning brief ready when you arrive. It acts as you, sees only what you can see, and you can stop it at any time.",
  "overnightGrant.label": "Let Margince prepare the Morning brief overnight",
  "overnightGrant.help":
    "It reads your deals and mail to rank today’s priorities, and writes notes back. It cannot send: the permission given here covers reading and writing only, never sending.",
  "overnightGrant.dangerTitle": "Overnight agent will not run",
  "overnightGrant.danger":
    "It cannot read or annotate your Morning brief. Your records, Worklist and scheduled weekly review remain available.",
  "overnightGrant.saveFailedTitle": "Answer was not saved",
  "overnightGrant.saveFailed":
    "Everything else is connected. After signing in, set this in Settings under Connections.",
  "overnightGrant.renewTitle": "Overnight authority expired",
  "overnightGrant.renew":
    "Turn this off and on again to renew it. Until then, your Morning brief is not prepared.",
  "overnightGrant.renewScopeTitle": "Authority no longer covers the work",
  "overnightGrant.writeFailedTitle": "Change was not saved",
  "overnightGrant.renewScope":
    "Margince has gained capabilities since you agreed. Turn this off and on again to extend it. Until then, your Morning brief is not prepared.",
  "aiHealth.title": "Model tiers",
  "aiHealth.sub":
    "Whether each model tier responds. A stopped tier and a cautious tier look the same elsewhere; captured mail stays held in both cases.",
  "aiHealth.noCalls": "No model calls in the last {hours}h.",
  "aiHealth.colTier": "Tier",
  "aiHealth.colState": "State",
  "aiHealth.colCalls": "Last {hours}h",
  "aiHealth.colLatency": "Median",
  "aiHealth.colLast": "Last response",
  "aiHealth.answering": "Responding",
  "aiHealth.notAnswering": "Not responding",
  "aiHealth.callCounts": "{calls} calls, {failures} failed",
  "aiHealth.ms": "{ms} ms",
  "heldThreads.title": "Held threads",
  "heldThreads.sub":
    "Threads your mailbox is withholding. Releasing a thread lets every colleague read it; only you can release yours.",
  "heldThreads.empty": "Your mailbox is withholding no threads.",
  "heldThreads.colThread": "Thread",
  "heldThreads.colWhy": "Reason",
  "heldThreads.colWhen": "Arrived",
  "heldThreads.colActions": "Actions",
  "heldThreads.release": "Share with the team",
  "heldThreads.released": "Shared with the team",
  "heldThreads.noSubject": "First message erased",
  "heldThreads.nothingToShare":
    "No message left to share. The first message was erased, and the hold stays so a later reply does not arrive shared.",
  "heldThreads.pending": "Awaiting classification",
  "heldThreads.attempts": "Attempts: {count}",
  "heldThreads.backlogStalled":
    "Threads asked about repeatedly with no answer: {count}. Mail stays withheld until the classifier responds again; nothing is lost.",
  "heldThreads.heldByOthers":
    "Other mailboxes that imported this message and have not shared it: {count}. A thread opens only when every recipient agrees.",
  "heldThreads.releaseFailed": "The thread was not shared",
  "heldThreads.stillHeldTitle": "Awaiting other mailboxes",
  "heldThreads.backlogStalledTitle": "Classifier not responding",
  "heldThreads.kind.legal": "Legal",
  "heldThreads.kind.financialCorporate": "Company finances",
  "heldThreads.kind.personnel": "Personnel",
  "heldThreads.kind.personal": "Personal",
  "heldThreads.kind.securityIncident": "Security incident",
  "heldThreads.kind.explicitlyConfidential": "Marked confidential",
  "senders.title": "Senders",
  "senders.sub":
    "The decision on each address your mailbox brought in, and your own answer where given. Only you see this list.",
  "senders.emptyTitle": "No senders yet",
  "senders.emptyBody":
    "After your mailbox brings in mail, each sender appears here with its outcome.",
  "senders.colSender": "Sender",
  "senders.colDecision": "Decision",
  "senders.colRecord": "Contact",
  "senders.colActions": "Actions",
  "senders.recordYes": "Yes",
  "senders.recordNo": "No",
  "senders.byYou": "Your decision",
  "senders.deletesOn": "Oldest message deleted on {date}",
  "senders.markBusiness": "Business",
  "senders.keepOut": "Exclude",
  "senders.withdraw": "Undo",
  "senders.keepOutTitle": "Exclude this sender permanently?",
  "senders.keepOutBody":
    "No contact is created, and mail this sender already brought into your mailbox is destroyed. Copies a colleague imported stay theirs.",
  "senders.keepOutConfirm": "Exclude and destroy",
  "senders.kind.contact": "Person",
  "senders.kind.roleMailbox": "Role mailbox",
  "senders.kind.companySender": "Company",
  "senders.kind.newsletter": "Newsletter",
  "senders.kind.transactional": "Automated tool",
  "senders.kind.spam": "Spam",
  "senders.kind.personal": "Personal",
  "senders.kind.advisor": "Advisor",
  "senders.kind.business": "Business",
  "senders.kind.keptOut": "Excluded",
  "senders.kind.undecided": "Undecided",
  "mailSharing.title": "Email sharing",
  "mailSharing.sub":
    "Captured mail is readable by every colleague who can see the contact. On by default, so deal work can be shared.",
  "mailSharing.label": "Share captured mail with the team",
  "mailSharing.help":
    "Individual messages can be limited afterwards, and addresses or domains excluded up front.",
  "mailSharing.danger":
    "With email sharing off, the CRM is hard to use. New mail is visible only to the people on each message.",
  "mailSharing.posture.shared":
    "Newly captured mail is readable by colleagues who can see the contact.",
  "mailSharing.posture.private":
    "Newly captured mail is held to the people on the message and the capturing mailbox.",
  "mailSharing.posture.where": "Change in Capture rules",
  "mailSharing.sharedPosture.label": "Allow mailboxes to share on arrival",
  "mailSharing.sharedPosture.help":
    "Lets a colleague set their mailbox to shared, so captured mail is readable by the team on arrival, before any classification. Off by default.",
  "mailSharing.sharedPosture.warning":
    "Reading an employee’s mailbox into a shared CRM is what a works-council agreement covers in Germany and Austria. Turning this on asserts that your company has one. Margince does not check this.",
  "mailSharing.dangerTitle": "Email sharing is off",
  "mailSharing.sharedPosture.warningTitle": "This asserts a legal basis",
  "mailSharing.saveFailed": "Setting not saved",
  "mailSharing.save": "Save",
  "connectors.originLabel": "Address used in emailed links",
  "connectors.originReachable": "Reachable",
  "connectors.originUnreachable": "Unreachable",
  "connectors.originUnchecked": "Not checked",
  "connectors.sub":
    "Mailboxes and calendars capturing into the CRM. Disconnecting one keeps the records already captured.",
  "connectors.loading": "Loading connectors…",
  "connectors.loadFailed": "Could not load connectors. Reload the page.",
  "connectors.empty": "No mailbox or calendar is connected yet.",
  "connectors.provGmail": "Gmail",
  "connectors.provGcal": "Google Calendar",
  "connectors.provGraph": "Outlook",
  "connectors.provGraphCal": "Outlook Calendar",
  "connectors.provImap": "IMAP mailbox",
  "connectors.provTestMailbox": "Test mailbox",
  "connectors.statusConnected": "Capturing",
  "connectors.statusPending": "Pending confirmation",
  "connectors.statusReauth": "Needs reconnect",
  "connectors.statusError": "Sync error",
  "connectors.statusDisconnected": "Disconnected",
  "connectors.cannotSend": "Capture only, no sending",
  "connectors.reconnectToSend":
    "Reconnect this mailbox to send from it. A mailbox connected before sending existed cannot be upgraded in place; the provider grants sending only on a new connection.",
  "connectors.lastSynced": "Last synced {at}",
  "connectors.neverSynced": "Awaiting first sync",
  "connectors.nextCheck": "Next check around {at}",
  "connectors.polled": "Polled on a schedule (no push subscription)",
  "connectors.pushRenewal": "Push renewal by {at}",
  "connectors.notConfigured":
    "Mail capture is not configured on this installation.",
  "connectors.reconnect": "Reconnect",
  "connectors.disconnect": "Disconnect",
  "connectors.signatureEnrich.label": "Read contact details from this mailbox",
  "connectors.contextTag.label": "Tag for created contacts",
  "connectors.contextTag.none": "No tag",
  "connectors.contextTag.hint":
    "Every contact this connector creates from now on gets this tag. Existing contacts keep their tags.",
  "connectors.contextTag.archived":
    "{name} is archived, so nothing is tagged with it. Choose another tag, or none.",
  "connectors.signatureEnrich.followingDefault":
    "Follows the company setting. Changing it here gives this mailbox its own setting.",
  "connectors.signatureEnrich.ownAnswer":
    "This mailbox’s own setting, independent of the company setting.",
  "hold.sectionTitle": "Private correspondence",
  "hold.notHeld": "Mail with this contact follows your mailbox setting.",
  "hold.heldByAddress":
    "Mail with this address is visible only to the people on it.",
  "hold.heldByDomain":
    "Mail with {domain} is visible only to the people on it.",
  "hold.holdAddress": "Keep private",
  "hold.holdDomain": "Keep all of {domain} private",
  "hold.lift": "Lift",
  "hold.liftingWidensNothing":
    "Lifting applies to new mail. Mail already held stays held.",
  "hold.confirmVerb": "Keep private",
  "hold.confirmTitle": "Keep this correspondence private?",
  "hold.confirmAddressBody":
    "Mail with {address} stays visible only to the people on it. It is still captured and you can still read it; colleagues cannot.",
  "hold.confirmDomainBody":
    "Mail with anyone at {domain}, including subdomains, stays visible only to the people on it. It is still captured and you can still read it; colleagues cannot.",
  "hold.confirmHistoryNote":
    "This covers new mail. Mail already captured keeps its current visibility.",
  "captureNotice.title": "What connecting a mailbox means",
  "captureNotice.whatHappens":
    "Margince reads this mailbox and files what it finds: the messages, who was on them, and the contacts and companies behind the addresses. Attachments are stored with their message.",
  "captureNotice.whoReads":
    "A new mailbox is held by default. A message stays with the people who were on it until a classifier judges the thread to be ordinary business. Only then can colleagues read it. You can set the mailbox to hold everything instead, at any time.",
  "captureNotice.yourControl":
    "You decide per sender and per thread, in Settings under Connections: keep a correspondent out entirely, share a thread with the team, or delete what a sender brought in. Nothing here asks for your agreement. This describes what happens, so you know it before you connect.",
  "connectors.mailPosture.label": "Mail visibility",
  "connectors.mailPosture.classified": "Held until classified",
  "connectors.mailPosture.held": "Always held",
  "connectors.mailPosture.shared": "Shared with the team",
  "connectors.mailPosture.sharedNeedsAdmin":
    "“Shared with the team” requires an administrator to allow it for this company.",
  "connectors.mailPosture.help.classified":
    "New messages are held to the people on them until a classifier rates the thread ordinary. Colleagues see nothing before then.",
  "connectors.mailPosture.help.held":
    "New messages are held to the people on them, regardless of classification. Threads are shared manually, one at a time.",
  "connectors.mailPosture.help.shared":
    "New messages are readable by colleagues on arrival.",
  "connectors.mailPosture.historyTitle": "Apply to captured mail?",
  "connectors.mailPosture.historyBody":
    "This setting applies to mail captured from now on. Mail already in the CRM keeps its visibility unless you narrow it to match.",
  "connectors.mailPosture.historyConfirm": "Change mail visibility",
  "connectors.mailPosture.historyApply": "Also narrow captured mail",
  "connectors.disconnectTitle": "Disconnect this mailbox?",
  "connectors.disconnectBody":
    "This deletes the stored credential for this mailbox. Capture stops immediately; captured records stay in the CRM, and reconnecting asks for permission again.",
  "connectors.disconnectBodyGoogleNote":
    "Google may still list Margince under your account’s third-party access. Remove it there to revoke access fully.",
  "connectors.disconnectBodyMicrosoftNote":
    "Microsoft may still list Margince among your account’s connected apps. Remove it there to revoke access fully.",
  "connectors.errRateLimited":
    "The provider is throttling requests. Capture is slower than usual; nothing is lost.",
  "connectors.errUnreachable":
    "The provider could not be reached. Retrying automatically.",
  "connectors.errAuth":
    "The provider rejected the stored credentials. Reconnect to resume.",
  "connectors.errHistoryGone":
    "The provider’s change history expired. The next sync restarts from a new point.",
  "connectors.errInternal":
    "An internal error occurred. Capture stopped to avoid partial data.",
  "connectors.errUnknown":
    "Capture failed for an unclassified reason. Retrying automatically.",

  // The OAuth return outcome (Task 2): the callback lands back on
  // #/settings/connections/{outcome} — a dismissible inline note driven by
  // that route segment, never a claim the server hasn't confirmed.
  "connectors.oauthOk": "Connected. Your mailbox is capturing.",
  "connectors.oauthDenied": "Access was declined, so nothing was connected.",
  "connectors.oauthError": "The connection could not be completed. Retry.",
  // Three failures that "try again" would be wrong about, each fixed somewhere
  // else: the provider refused the grant (retrying the same way repeats it),
  // its API is not enabled for this deployment (the vendor's console), and it
  // refused this deployment's own client credentials (the app card in
  // Settings). The last two are an administrator's; no user action clears them.
  "connectors.oauthRejected":
    "The provider declined the connection. Accept every requested permission, then connect again.",
  "connectors.oauthMisconfigured":
    "This installation cannot complete the connection because the provider’s API is not enabled. An administrator must enable it; the server log names the API.",
  "connectors.oauthBadClient":
    "The provider refused this installation’s app credentials. An administrator must check the client ID and secret in Settings under General; reconnecting does not fix this.",
  "connectors.dismissOutcome": "Dismiss",
  "connectors.oauthConnected": "Connected",
  "connectors.oauthNotConnected": "Nothing was connected",
  "connectors.connectFailed": "Could not connect",
  "connectors.imapConnectFailed": "Mailbox not connected",

  // The "Add a connection" affordance (Task 1): one verb in the card's header
  // opens a dialog listing the providers still addable, each with the sentence
  // it needs. `addOpen` names the act of OPENING that list, and the picks inside
  // it name the act of connecting — so no two buttons on the surface read the
  // same while both are on screen.
  "connectors.addConnection": "Add connector",
  "connectors.addOpen": "Add connector",
  "connectors.connect": "Connect",
  "connectors.connectProvider": "Connect {provider}",
  "connectors.rosterLabel": "Active connectors",
  "connectors.addGmailBrings":
    "Mail sent and received in Gmail. Margince can also send from it.",
  "connectors.addGcalBrings":
    "Your Google Calendar, connected separately from Gmail.",
  "connectors.addGraphBrings":
    "Mail sent and received on a Microsoft work account. Margince can also send from it.",
  "connectors.addGraphCalBrings":
    "Your Outlook calendar, connected separately from Outlook mail.",
  "connectors.addImapBrings":
    "Any other mail host, using an app password. Capture only.",
  "connectors.addTestMailboxBrings":
    "Test mailbox for QC. No real mail is sent or received.",
  "connectors.providerNotConfigured":
    "{provider} is not configured on this installation.",

  // The inline IMAP connect form (Task 6): first-connect and reconnect for
  // the one credential provider, done in Settings instead of bouncing to
  // onboarding.
  "connectors.imapModalTitle": "Connect IMAP mailbox",
  "connectors.imapHost": "IMAP server",
  "connectors.imapPort": "Port",
  "connectors.imapUsername": "Email address",
  "connectors.imapSecret": "App password",
  "connectors.imapMailbox": "Mailbox",
  "connectors.imapMaxMessages": "Messages per sync",
  "connectors.imapSecretHint":
    "Use an app-specific password. It is stored in the key vault and used to read mail on a schedule until you disconnect, which deletes it.",
  "connectors.imapSubmitCta": "Connect",
  "connectors.imapNeeded": "Required fields",
  "connectors.imapStillNeeded": "Required: {fields}",
  "connectors.imapLoginRejected":
    "The mailbox rejected these credentials. Check host, email and app password.",
  "connectors.imapUnreachable":
    "The mail server could not be reached. Check the host and port.",

  // The Telegram connector panel (Task 17, design §9.1-§9.2): one bot
  // connects for the whole workspace — no OAuth handshake, a BotFather
  // token submitted through the same inline-form shape the IMAP connector
  // uses. Unlike the mail providers, the connection stays editable in
  // place: replacing the token goes through PATCH, never a disconnect.
  "connectors.provTelegram": "Telegram",
  "connectors.telegramTitle": "Telegram bot",
  "connectors.telegramSub":
    "One bot receives and sends messages for the whole company.",
  "connectors.telegramNotConfigured":
    "Messaging channels are not configured on this installation.",
  "connectors.telegramConnectCta": "Connect Telegram bot",
  "connectors.telegramRosterLabel": "Connected bot",
  "connectors.telegramEmpty": "No bot connected.",
  "connectors.telegramEditToken": "Replace token",
  "connectors.telegramDisconnectTitle": "Disconnect this bot?",
  "connectors.telegramDisconnectBody":
    "This deletes the stored token and stops polling the bot. Capture and sending stop immediately; captured records stay in the CRM.",
  "connectors.telegramModalTitle": "Connect Telegram bot",
  "connectors.telegramEditTitle": "Replace bot token",
  "connectors.telegramBotToken": "Bot token",
  "connectors.telegramBotTokenHint":
    "The token BotFather issued for the bot. It is stored in the key vault and never shown again.",
  "connectors.telegramSubmitCta": "Connect",
  "connectors.telegramReplaceCta": "Replace token",
  "connectors.telegramConnectedAs": "Connected as @{username}.",
  "connectors.telegramConnectFailed": "Bot not connected",

  // The workspace's own consumer-mail list (CAP-PARAM-5): what the shipped
  // baseline missed, and what it got wrong. Admin-curated and shared, because
  // whether a domain can name a company is a fact about the domain.
  "consumerMail.title": "Consumer mail domains",
  "consumerMail.sub":
    "Mail from a consumer mailbox creates the contact but never a company. Margince ships a list of these providers; add missing domains or override a wrong entry.",
  "consumerMail.addedTitle": "Added here",
  "consumerMail.addTitle": "Add domain",
  "consumerMail.domainLabel": "Domain",
  "consumerMail.domainPlaceholder": "provider.example",
  "consumerMail.kindLabel": "Domain type",
  "consumerMail.kind.extra": "Consumer mail, never a company",
  "consumerMail.kind.never": "Company domain, overrides shipped list",
  "consumerMail.add": "Add",
  // The header verb names the whole act it opens a dialog for; the dialog's own
  // submit is the bare verb, so no two buttons on this surface read the same.
  "consumerMail.addOpen": "Add domain",
  "consumerMail.remove": "Remove",
  "consumerMail.none":
    "No domains added. The shipped list applies to every domain.",
  "consumerMail.adminOnly": "You do not have permission to change this list.",
  "consumerMail.addOnly":
    "You can add consumer mail domains. Overriding the shipped list or removing entries requires an administrator.",
  "consumerMail.baselineTitle": "Shipped list",
  "consumerMail.baselineCount":
    "Margince ships with {total} known consumer mail domains.",
  "consumerMail.baselineSearchLabel": "Search the shipped list",
  "consumerMail.baselinePlaceholder": "gmail.com",
  "consumerMail.baselineNone": "No shipped domain matches.",
  "consumerMail.removeFailed": "Domain not removed",
  "consumerMail.addFailed": "Domain not added",
  "consumerMail.baselineMore":
    "Showing the first {shown} of {matched} matches.",

  "blockedDomains.title": "Refused domains",
  "blockedDomains.sub":
    "Domains refused as companies, and what decided each: a model result, a heuristic or a person. Allowing a domain reopens the question. Only unowned open questions appear here.",
  "blockedDomains.listTitle": "Recorded decisions",
  "blockedDomains.record": "Record a decision",
  "blockedDomains.recordOpen": "Record a decision",
  "blockedDomains.domainLabel": "Domain",
  "blockedDomains.domainPlaceholder": "supplier.example",
  "blockedDomains.admissionLabel": "Decision",
  "blockedDomains.admission.suppressed": "Never a company",
  "blockedDomains.admission.admitted": "Allowed",
  "blockedDomains.admission.undecided": "Undecided",
  "blockedDomains.reasonLabel": "Reason",
  "blockedDomains.reasonHint": "One sentence a later reviewer can act on.",
  "blockedDomains.reasonPlaceholder": "Supplier, not a customer",
  "blockedDomains.save": "Save decision",
  "blockedDomains.stored": "Saved: {domain}, {admission}",
  "blockedDomains.saveFailed": "Decision not saved",
  "blockedDomains.adminOnly":
    "Only an administrator or operations user can change domain decisions. The list is read-only for you.",
  "blockedDomains.none":
    "No domains refused yet. Bulk-sender results and manual refusals appear here.",
  "blockedDomains.unit": "domain decisions",
  "blockedDomains.openCompany": "Open company",
  "blockedDomains.col.domain": "Domain",
  "blockedDomains.col.admission": "Decision",
  "blockedDomains.col.source": "Decided by",
  "blockedDomains.col.reason": "Reason",
  "blockedDomains.col.decided": "When",
  "blockedDomains.col.revise": "Change",
  "blockedDomains.source.verdict": "Model result",
  "blockedDomains.source.heuristic": "Heuristic",
  "blockedDomains.source.human": "Person",
  "blockedDomains.source.unevidenced": "No company named on the site",
  "blockedDomains.source.staleEvidence": "Supporting mail too old",
  "blockedDomains.source.nearDuplicate": "Company name already exists",
  "blockedDomains.rowAdmit": "Allow",
  "blockedDomains.rowRefuse": "Refuse",
  "blockedDomains.rowReopen": "Reopen",
  "blockedDomains.reopened": "{domain} reopened",
  "blockedDomains.reopenFailed": "Domain not reopened",

  "ob.s4.googleFailed": "Google connection not completed",
  "ob.s4.imapHost": "IMAP host",
  "ob.s4.imapHostPlaceholder": "imap.gmail.com",
  "ob.s4.imapPort": "Port",
  "ob.s4.imapEmail": "Email",
  "ob.s4.imapPassword": "App password",
  "ob.s4.imapMailbox": "Mailbox",
  "ob.s4.imapMax": "Number of recent emails",
  "ob.s4.imapHint":
    "Use an app password. It is stored encrypted and deleted on disconnect.",
  "ob.s4.imapConnect": "Test and connect",
  "ob.s4.connecting": "Connecting…",
  "ob.s4.accessToggle": "Access granted",
  "ob.s4.scope1Lead": "Read access.",
  "ob.s4.scope1Rest":
    "Mail becomes contacts, companies and activities automatically.",
  "ob.s4.scope2Lead": "Sending is included.",
  "ob.s4.scope2Rest":
    "Margince can send from this mailbox when you send, and when you give an agent a passport that allows sending. Giving that passport is the approval. You can withdraw it at any time.",
  "ob.s4.scope3Lead": "Data stays in your company.",
  "ob.s4.scope3Rest": "Export or delete everything at any time.",
  "ob.s4.scope4Lead": "Disconnect in one click.",
  "ob.s4.scope4Rest": "The CRM keeps working but stops capturing.",
  "ob.s4.capturedTitle": "Mailbox connected",
  "ob.s4.capturedBody":
    "New mail appears here as the first sweep runs, usually within minutes.",
  "ob.s4.connectFailed": "Mailbox not connected",
  "ob.s4.notNow": "Not now",

  "ob.conv.threadLabel": "Onboarding conversation",
  "ob.conv.read.started": "Reading {host}. I will report what I find.",
  "ob.conv.read.pages": "Pages read so far: {pages}.",
  "ob.conv.read.learnedField": "Learned {field}: {value}",
  "ob.conv.read.extracting":
    "Crawl finished. Extracting what the site says about the business.",
  "ob.conv.read.warning": "Note: {warning}",
  "ob.conv.read.failed":
    "I could not read that site. Try another URL, or enter the details manually.",
  "ob.conv.read.deferred":
    "The read is paused. I will resume it automatically.",
  "ob.conv.read.pollFailed":
    "The connection dropped during reading. What I found is kept.",
  "ob.conv.clarify.entity":
    "The site names more than one legal entity. Which one is this installation for?",
  "ob.conv.company.confirmed":
    "Company profile confirmed. Every stored value records its source.",
  "ob.conv.manual.chosen": "I will enter it manually.",
  "ob.conv.voice.skipped": "Skip voice for now.",
  "ob.conv.voice.uploadAdded": "Added {name}.",
  "ob.conv.voice.speakerQuestion":
    "This transcript has several speakers. Which one is you? Only your own words count.",
  "ob.conv.voice.speakerOptionDetail": "words: {words} · turns: {turns}",
  "ob.conv.voice.speakerFoot": "Your choice applies to this file only.",
  "ob.conv.voice.speakerContinue": "Use this speaker",
  "ob.conv.voice.continueSkippedStatus": "Skipped. Add it later in Settings.",
  "ob.conv.voice.continueFailedStatus":
    "Your material is kept. Retry now, or continue and finish this later.",
  "ob.conv.voice.continueDeferredStatus":
    "No action needed. Continue; the build finishes automatically.",
  "ob.conv.voice.composer": "Paste the text you wrote here",
  "ob.conv.voice.dropHint":
    "You can also drop files anywhere in this conversation: .txt, .md, .pdf, .docx or a .vtt, .srt or .json transcript.",
  "ob.conv.voice.fileSkipped":
    "I cannot read {name}. Supported formats: .txt, .md, .pdf, .docx, .vtt, .srt, .json.",
  "ob.conv.voice.fileUnreadable":
    "I could not open {name}. If it is password-protected or damaged, paste its text instead.",
  "ob.conv.voice.fileEmpty":
    "There are no words in {name}, so nothing was counted.",
  "ob.conv.voice.reactionTranscript":
    "Words kept: {kept} of {total}. Only your turns count; speech improves the voice most.",
  "ob.conv.voice.reactionDocument":
    "Words counted: {words}. All words in this file are yours.",
  "ob.conv.voice.refusalUnattributed":
    "This looks like a conversation, but I cannot tell which words are yours, so none were counted.",
  "ob.conv.voice.refusalSpeaker":
    "I could not find that speaker in the transcript. Nothing was counted.",
  "ob.conv.voice.refusalUnsupported":
    "I could not parse that file as text or a transcript. Nothing was counted.",
  "ob.conv.voice.ingestFailed": "I could not add that source: {detail}",
  "ob.conv.voice.ingestUnexpected":
    "I could not add that source. Retry shortly.",
  "ob.conv.voice.pasteAdd": "Add to writing samples",
  "ob.conv.voice.pasteDiscard": "Discard",
  "ob.conv.voice.pasteSource": "Pasted text",
  "ob.conv.voice.buildChip": "Build my voice profile",
  "ob.conv.voice.retryBuild": "Retry build",
  "ob.conv.voice.buildPollFailed":
    "The connection dropped during the build. Your texts are kept; retry the build.",
  "ob.conv.voice.statusBuilding": "Building voice profile…",
  "ob.conv.voice.resultTitle": "This is your voice profile.",
  "ob.conv.voice.resultLoading": "Loading build results…",
  "ob.conv.voice.resultEmpty":
    "The build finished but has nothing to show yet. Review it in Settings.",
  "ob.conv.voice.candidateNote":
    "This version needs your review before it goes live. Approve it in Settings.",
  "ob.conv.voice.artifactTitle": "Voice corpus",
  "ob.conv.voice.artifactBody":
    "Only your own words count. All numbers come from the server after speaker filtering.",
  "ob.conv.voice.artifactEmpty":
    "Nothing collected yet. Attach a transcript or a text you wrote.",
  "ob.conv.voice.meterWords": "Own words: {words} of {target}",
  "ob.conv.voice.meterBand": "Quality: {band}",
  "ob.conv.voice.manifestKept": "Kept {kept} of {total} words",
  "ob.conv.voice.manifestWords": "{words} words",
  "ob.conv.voice.registerMix": "Registers: {mix}",
  "ob.conv.voice.stageTitle": "Build progress",
  "ob.conv.corpus.words": "Own words in your corpus now: {words}.",
  "ob.conv.corpus.band": "Corpus quality moved to {band}.",
  "ob.conv.build.snapshot": "Locking in your corpus.",
  "ob.conv.build.extract": "Extracting your writing patterns.",
  "ob.conv.build.evaluate": "Testing drafts against held-out samples.",
  "ob.conv.build.activate": "Activating your voice profile.",
  "ob.conv.build.succeeded": "Your voice profile is ready.",
  "ob.conv.build.deferred":
    "The build is queued until AI allowance is available. It runs automatically.",
  "ob.conv.build.failed":
    "The build did not finish. Your texts are kept; retry at any time.",
  "ob.conv.done": "Setup complete. Your CRM is ready.",
  "ob.conv.clarify.question": "{question}",
  "ob.conv.clarify.optionDetail": "{detail}",
  "ob.conv.clarify.dismiss": "Skip. I will set it myself.",
  "ob.conv.clarify.keepMine": "Keep my value",
  "ob.conv.review.skipped": "You skipped: {fields}. Edit them at any time.",
  "ob.conv.clarify.applyFailed":
    "I could not record that choice: {detail} Select it again.",
  "ob.conv.clarify.applyMissing":
    "The server did not confirm that choice. Select it again.",
  "ob.conv.loadFailed": "I could not check your setup. Retry.",
  "ob.conv.retry": "Retry",
  "ob.conv.connect.persistFailed":
    "I could not save the setup completion. Retry.",
  "ob.conv.review.title":
    "This is everything I found. Correct anything that is wrong.",
  "ob.conv.review.showLess": "Show less",
  "ob.conv.review.continue": "Continue",
  "ob.conv.review.progressLabel": "Required fields completed",
  "ob.conv.review.requiredRemaining_one":
    "{count} field needed before you can continue",
  "ob.conv.review.requiredRemaining_other":
    "{count} fields needed before you can continue",
  "ob.conv.review.requiredDone": "Nothing more needed. You can continue.",
  "ob.conv.review.confirmQuestionOpen":
    "A decision is still open. Answer it to continue.",
  "ob.conv.triage.stateRequired": "required, still empty",
  "ob.conv.triage.stateEmpty": "empty",
  "ob.conv.triage.stateTyped": "entered by you",
  "ob.conv.triage.stateStored": "from your profile",
  "ob.conv.triage.stateStoredBadge": "From your profile",
  // A value the entity census read off the legal notice — the one company the
  // site names, or the candidate the human picked from several. Nothing ever
  // scored it, so the word names WHERE it came from; "chosen by you" would be
  // false on the sole-candidate path, where nobody was asked anything.
  "ob.conv.triage.stateQuoted": "read from your legal notice",
  "ob.conv.triage.stateQuotedBadge": "Read from your legal notice",
  // Where a value would stand on an empty row. It says only that the row is
  // empty: the same line serves the manual path, where nothing ever read the
  // site, and a read-backed board, where the wire says nothing about why any
  // one field came back missing. Naming a cause here would invent one.
  "ob.conv.triage.emptyHint": "Nothing here yet. Add it manually.",
  "ob.conv.triage.legalNotPublished":
    "Not stated on your legal notice or imprint page. Add it manually.",
  "ob.conv.triage.legalNotChecked":
    "I found no legal notice or imprint page on your site. Add it manually.",
  // Several companies stand on the one imprint. The value IS on the page, so
  // saying it is not stated would be false; what is missing is the human's
  // decision about which company this installation belongs to.
  "ob.conv.triage.legalUnpicked":
    "Your imprint names more than one company. Select yours and I will fill this in.",
  // The omission notice on an empty row, once a read has actually run: the
  // field is named as withheld rather than left blank, and the reason is only
  // ever one the read can support for THAT field. The read's crawl-wide
  // warnings belong to the coverage card, which states each under its own
  // heading; beside a field they would name a cause the read never gave.
  "ob.conv.triage.omittedLabel": "Omitted, not guessed",
  "ob.conv.triage.omittedField": "{field}: {reason}",
  "ob.conv.triage.mapLabel": "Jump to a section",
  "ob.conv.triage.sectionBlocking": "{count} needed to continue",
  "ob.conv.triage.sectionAdvisory": "{count} to review",
  "ob.conv.triage.blockingHead": "Needed to continue",
  "ob.conv.triage.advisoryHead": "To review",
  "ob.conv.triage.sectionSettled": "Nothing outstanding here",
  "ob.conv.triage.sectionMore": "+{count} more",
  "ob.conv.triage.restTitle": "Background information",
  "ob.conv.triage.looksSolid": "Looks complete · {count}",
  "ob.conv.triage.companyWebsite": "Website",
  "ob.conv.triage.sourceCount": "{count} source",
  "ob.conv.triage.contactsLabel": "Contacts",
  "ob.conv.triage.contactsCount": "{count} found",
  "ob.conv.triage.contactsEmpty": "No contacts found on your site.",
  "ob.conv.triage.factsLabel": "Facts",
  "ob.conv.triage.factsCount": "{count} found",
  "ob.rail.tokensUnit": "tok",
  "ob.conv.scene.step": "Step {n} of {m} · {label}",
  "ob.conv.scene.detour": "Decision needed",
  "ob.conv.scene.decisionSub":
    "Your site names several legal entities. The one you select appears on every quote and invoice.",
  "ob.conv.scene.continue": "Continue",
  "ob.conv.connect.sceneTitle": "Connect your accounts",
  "ob.conv.connect.sceneSub":
    "I build your contacts, companies and history from the mail already in your mailbox.",
  "ob.conv.connect.mailboxTitle": "Your mailbox",
  "ob.conv.connect.mailboxHint":
    "Select one. Contacts, companies and history come from this mailbox.",
  "ob.conv.connect.networkTitle": "Your network",
  "ob.conv.connect.networkHint":
    "Save your profile so a network imported later is attributed to you. The import is in Settings.",
  "ob.conv.connect.recommended": "Recommended",
  // Neither grant carries calendar or contacts — those are their own,
  // separate consent (Settings → Calendar) — and neither carries sign-in:
  // both connect the SAME two things here, mail read and send, so the two
  // lines say the same thing rather than inventing a difference that is not
  // in the grant.
  "ob.conv.connect.gmailBrings": "Mail read and sent via Google",
  "ob.conv.connect.microsoftBrings": "Mail read and sent via Microsoft",
  "ob.conv.connect.imapBrings":
    "Mail from any host, with your email address and an app password",
  "ob.conv.connect.linkedinAuth": "Profile link, saved to your account",
  "ob.conv.connect.saveCta": "Save",
  "ob.conv.connect.dialogDone": "Done",
  "ob.conv.connect.scopeGoogle": "OAuth, read and send scopes",
  "ob.conv.connect.scopeMicrosoft": "OAuth, Graph API",
  "ob.conv.connect.scopeImap": "Mail address and password",
  "ob.conv.connect.connectCta": "Connect",
  "ob.conv.connect.connectedCta": "Connected",
  "ob.conv.connect.savedCta": "Saved",
  "ob.conv.connect.blockedCard":
    "A mailbox is already selected. Disconnect it in Settings to switch.",
  "ob.conv.connect.guaranteesToggle": "What connecting does",
  "ob.conv.connect.dialogHeadlineAccess": "{name} access needed",
  "ob.conv.connect.dialogHeadlineImap": "Connect mail host",
  "ob.conv.connect.appMissingCard":
    "Your company has not registered its {name} app yet.",
  "ob.conv.connect.appUnusableCard":
    "Your company’s {name} app cannot be opened. An administrator must fix it; a new app is not needed.",
  "ob.conv.connect.unsupportedCard": "This installation does not serve {name}.",
  "ob.conv.connect.appSetupLink": "Set it up in Settings",
  "ob.conv.connect.dialogIntro":
    "{brings}. I read it once to build your contacts and history, then keep it in sync.",
  "ob.conv.connect.linkedinName": "LinkedIn",
  "ob.conv.connect.linkedinSaved": "Profile saved",
  "ob.conv.connect.linkedinSkippedNote": "Skipped: add it later in Settings",
  "ob.conv.connect.rosterFailedTitle": "Mailboxes could not be checked",
  "ob.conv.connect.rosterFailedBody":
    "Connection status did not load. Retry before selecting a provider.",
  "ob.conv.voice.sceneTitle": "Train your writing voice",
  "ob.conv.voice.sceneSub": "Margince drafts every email in your own words.",
  "ob.conv.voice.heroBody":
    "It learns your tone, rhythm and phrasing only from your own writing.",
  "ob.conv.voice.whyToggle": "Why add samples",
  "ob.conv.voice.dropTitle": "Drop your writing here",
  "ob.conv.voice.dropSub": "Sent mail works best.",
  "ob.conv.voice.browse": "Browse files",
  "ob.conv.voice.pasteInstead": "Paste text instead",
  "ob.conv.voice.sourcesTitle": "Sources",
  "ob.conv.voice.meterLabel": "Progress toward the {min}-word minimum",
  "ob.conv.voice.meterProgress": "{words} of {min} words",
  "ob.conv.voice.meterReady":
    "{words} words: enough to build. More words improve it.",
  "ob.conv.voice.footReady":
    "Training takes about a minute. A sample is shown before anything is saved.",
  "ob.conv.voice.footFloor":
    "{min} words minimum. Below that the model copies phrasing.",
  "ob.conv.voice.buildingTitle": "Learning your voice",
  "ob.conv.voice.buildingMeta_one": "{words} words, {sources} source",
  "ob.conv.voice.buildingMeta_other": "{words} words, {sources} sources",
  "ob.conv.voice.resultSub":
    "Read the sample. If it fits, confirm. If not, add more sources and I rebuild.",
  "ob.conv.voice.resultSubNoSample":
    "This build returned no sample draft. This is what it learned; add more writing and I will rebuild.",
  "ob.conv.voice.resultContinue": "That is me",
  "ob.conv.voice.revise": "Not quite me: add more writing",
  "ob.conv.voice.distilling": "Analyzing",
  "ob.conv.voice.hears": "hears",
  "ob.conv.voice.hearsWords":
    "{words} of your own words across {sources} sources",
  "ob.conv.voice.hearsBand": "a {band} corpus so far",
  "ob.conv.voice.hearsRegister": "{words} words of {register} writing",
  "ob.conv.voice.sampleEyebrow": "Sample, not sent",
  "ob.conv.voice.sampleAnother": "Another scenario",
  "ob.conv.voice.sampleSubjectLabel": "Subject",
  "ob.conv.voice.sampleWhyTag": "Why",
  "ob.conv.voice.dimensionsTitle": "Measured dimensions",
  "ob.conv.voice.dimensionsCount": "Measured: {count}",
  "ob.conv.voice.dimSentenceName": "Sentence length",
  "ob.conv.voice.dimSentencePoleLow": "Terse",
  "ob.conv.voice.dimSentencePoleHigh": "Elaborate",
  "ob.conv.voice.dimSentenceMeasured": "Measured",
  "ob.conv.voice.dimSentenceEvidence": "{count} words per sentence on average.",
  "ob.conv.scene.evidence": "evidence",
  "ob.conv.scene.hideEvidence": "hide evidence",
  "ob.conv.scene.whyThis": "What I read",
  "ob.conv.scene.foundOn": "Found on",
  "ob.conv.activity.steps_one": "{count} step",
  "ob.conv.activity.steps_other": "{count} steps",
  "ob.conv.showField": "Show",
  "ob.conv.review.editDirectly": "Edit fields directly",
  "ob.conv.review.backToDossier": "Back to dossier",
  "ob.conv.review.proposalFallback":
    "I could not load the prepared mapping. Review what I read directly; every field keeps its source.",
  "ob.conv.review.confirmFailed":
    "I could not save that: {detail} Fix it and accept again.",
  "ob.conv.review.confirmVersionSkew":
    "Your review received newer information. Check it, then select Continue again.",
  "ob.conv.review.confirmVersionSkewStuck":
    "Nothing has changed yet, so Continue would fail again. Review again, or retry shortly.",
  "ob.conv.review.refusalTitle": "Continue did not finish",
  "ob.conv.review.confirmNotReady":
    "This read has no draft to confirm yet. Check again when it finishes, or start a new read.",
  "ob.conv.review.confirmCheckFailed":
    "This read is confirmed, but I could not load the company it created. Retry shortly.",
  "ob.conv.artifact.empty":
    "Nothing read yet. Enter a website to fill this panel with sourced findings.",
  "ob.conv.results.continue": "Continue",
  "ob.conv.recap.back": "Welcome back. This is where setup stands.",
  "ob.conv.recap.company": "Your company profile for {name} is confirmed.",
  "ob.conv.recap.companyUnsaved":
    "Your company details are not saved yet. Complete them in Settings.",
  "ob.conv.recap.voiceBuilt":
    "Your voice profile is built. Drafts use your voice.",
  "ob.conv.recap.voiceSkipped":
    "You skipped the voice profile. Drafts use a neutral starter voice.",
  "ob.conv.recap.corpus":
    "Your corpus already holds {words} of your own words.",
  "ob.conv.recap.readTerminal":
    "I finished reading {host}: {count} sourced findings, shown below.",
  "ob.conv.recap.readReading":
    "I am still reading {host}. Pages so far: {pages}.",
  "ob.conv.recap.readFailed":
    "My earlier read of {host} did not finish. Enter a website again, or enter the details manually.",
  "ob.conv.recap.readDeferred":
    "My read of {host} is paused. Enter a website again, or enter the details manually.",
  "ob.conv.connect.skip": "Continue without a mailbox",
  "ob.conv.connect.continue": "Continue",
  "ob.conv.connect.mailboxNeeded":
    "A mailbox is still needed: mail is what gets read and drafted. Connect one above, or continue without one for now.",
  "ob.conv.linkedin.cardBody":
    "Your profile address, so an imported connection reads “Anna knows them”, not “the company knows them”.",
  "ob.conv.linkedin.dialogHeadline": "Your LinkedIn profile",
  "ob.conv.linkedin.profileLabel": "Your LinkedIn profile URL",
  "ob.conv.linkedin.profilePlaceholder": "https://www.linkedin.com/in/…",
  "ob.conv.linkedin.profileWhy":
    "Attributes the network to you: “Anna knows them”, not “the company knows them”.",
  "ob.conv.linkedin.save": "Save profile",
  "ob.conv.linkedin.skip": "Skip LinkedIn for now",
  "ob.conv.linkedin.importLater":
    "Connections are imported in Settings from a Connections.csv export.",
  "ob.conv.linkedin.saved":
    "LinkedIn profile saved. Your imported network will be attributed to you.",
  "ob.conv.linkedin.skipped":
    "LinkedIn skipped. Add your profile at any time in Settings.",

  // The setup rail: five stops, one word each. Long enough to name the step,
  // short enough that five of them fit a column at 10px.
  "ob.rail.read": "Read",
  "ob.rail.confirm": "Confirm",
  "ob.rail.basis": "Basis",
  "ob.rail.voice": "Voice",
  "ob.rail.connect": "Connect",

  // The invite: asked once the company is confirmed, before the two steps
  // that are only about the contact answering. An administrator who sets the
  // installation up for a team and never works in it finishes here.
  "ob.conv.invite.title": "Will you work in Margince yourself?",
  "ob.conv.invite.body":
    "The company is set up. The next 2 steps are about you and apply only if you will use Margince.",
  "ob.conv.invite.yes": "Yes, I will work in Margince",
  "ob.conv.invite.yesBody":
    "Train your voice and connect your mailbox and calendar: 2 short steps.",
  "ob.conv.invite.no": "No, I am only setting it up",
  "ob.conv.invite.noBody":
    "Invite the first person who will work here instead, and setup is done.",
  "ob.conv.invite.foot":
    "Voice and accounts can also be set up later in Settings.",
  "ob.conv.invite.continue": "Continue",
  "ob.conv.invite.accepted": "Yes, I will be working in it.",
  "ob.conv.invite.declined": "No, I am only setting it up.",

  // The team act: the first contact who will work here, invited with the
  // same form Settings → Contacts uses.
  "ob.conv.team.title": "Invite the first user",
  "ob.conv.team.body":
    "Someone must be the first person working in Margince. Add them now, or later in Settings under People.",
  "ob.conv.team.invitedLabel": "Invited so far",
  "ob.conv.team.invitedLine": "{name} is invited.",
  "ob.conv.team.skip": "Skip for now",
  "ob.conv.team.finish": "Finish setup",
  "ob.conv.team.done":
    "Setup is complete. Each user you add can train their voice and connect their accounts in Settings.",
  "ob.conv.team.persistFailed":
    "I could not record that setup is complete. Retry, or finish later from Settings.",
  // Reporting settings are settled after the company is confirmed.
  "ob.conv.basis.title": "Set the reporting basis",
  "ob.conv.basis.body":
    "Base currency and reporting timezone apply to every deal, report and brief in the installation. Both are prefilled and can be changed in Settings until a deal fixes the currency.",
  "ob.conv.basis.reportingTitle": "Reporting basis",
  "ob.conv.basis.timezoneNeeded": "A reporting timezone is needed.",
  "ob.conv.basis.continue": "Continue",
  "ob.conv.basis.done": "Reporting basis set.",

  // --- the gate: the first screen after sign-in -------------------------
  // One question and nothing else. Nobody should meet the whole tool on their
  // first screen, so the gate names what it will do, what it costs the reader
  // (two minutes), and who decides (they do) — then asks once.
  "ob.gate.title": "Welcome, {name}",
  "ob.gate.titleAnonymous": "Welcome to Margince",
  "ob.gate.sub":
    "Margince reads your website and drafts the company profile. Nothing is saved until you approve. About 2 minutes.",
  "ob.gate.trustToggle": "How this works",
  "ob.gate.trustBody":
    "Only public pages are read. Nothing is saved until you confirm, and reading the site sends nothing to anyone.",
  "ob.gate.field": "Website address",
  "ob.gate.placeholder": "yourcompany.com",
  "ob.gate.submit": "Read website",
  "ob.gate.altPrompt": "No website?",
  "ob.gate.altAction": "Enter details manually",
  "ob.gate.invalidUrl":
    "This is not a valid web address. Enter it as yourcompany.com.",
  // One string for two failures that look identical to the reader: the request
  // to start never landed, or the read started and did not finish. {detail} is
  // the server's own guidance and may be empty, so the sentence has to stand
  // without it.
  "ob.gate.startFailed":
    "The site could not be read. {detail} Try another address, or enter the details manually.",
  // A deferred read is shelved, not broken: the server will come back to it. So
  // this says what is true and names both doors, without asking the reader to
  // fix anything.
  "ob.gate.readPaused":
    "The read is paused. {detail} It resumes automatically. Or enter another address, or the details manually.",

  // --- the read theatre -------------------------------------------------
  // Volume made visible. The wire gives no page-count denominator, so every
  // number here is an open count — never "14 of 18", never a bar with a known
  // end, because inventing the total would be inventing data.
  "ob.scan.title": "Reading {host}",
  "ob.scan.sub":
    "Every fact keeps the page it came from, so each claim can be checked.",
  "ob.scan.doneTitle": "Read {host}",
  "ob.scan.doneSub":
    "{facts} facts and {fields} profile fields, each with its source page. Opening the review.",
  "ob.scan.phaseCrawling": "Fetching pages",
  "ob.scan.phaseExtracting": "Identifying what the company sells",
  "ob.scan.phaseQueued": "Queued",
  "ob.scan.phaseDeferred": "Paused",
  "ob.scan.pagesRead": "{pages} pages read",
  "ob.scan.pagesSkipped": "{count} skipped",
  "ob.scan.stillReading": "still reading",
  "ob.scan.pageStripLabel": "Pages read so far",
  "ob.scan.logLabel": "Pages read, newest first",
  "ob.scan.pageFetched": "{url}: read",
  "ob.scan.pageSkipped": "{url}: skipped, {reason}",
  "ob.scan.pageFailed": "{url}: could not be read, {reason}",
  "ob.scan.pageNoReason": "no reason recorded",
  "ob.scan.pageStatusFetched": "read",
  "ob.scan.pageStatusSkipped": "skipped: {reason}",
  "ob.scan.pageStatusFailed": "could not be read: {reason}",
  "ob.scan.skipReason.robots": "the site disallows reading it",
  "ob.scan.skipReason.offDomain": "it is on another domain",
  "ob.scan.skipReason.pageCap": "the page limit for one read was reached",
  "ob.scan.skipReason.byteCap": "the text limit for one read was reached",
  "ob.scan.skipReason.unreadable": "the page could not be read",
  "ob.scan.transparency": "Transparency",
  "ob.scan.costLine": "{calls} calls · {tokens} tokens · {cost}",
  "ob.scan.costPending": "no billed model calls yet",
  "ob.scan.costUnpriced": " · unpriced usage exists",

  // --- the live panel: what the read covered, and what it left ----------
  "ob.live.stateDone": "done",
  "ob.live.stateNow": "in progress",
  "ob.live.stateWaiting": "waiting",
  "ob.live.review": "Review",
  "ob.live.hide": "Hide",
  "ob.live.countPages": "{read} read · {skipped} skipped",
  "ob.live.cardCoverage": "Pages read and skipped",
  "ob.live.coverageWarning": "Warning",
  // A bounded read has to say it was bounded: the page counts beside it
  // otherwise read as the whole site.
  "ob.live.coverageStopped": "Stopped early",
  // The same bound, said without alarm: a page or byte cap is the size this
  // read was configured for, not something that went wrong.
  "ob.live.coverageCapped": "Limit reached",
  "ob.live.stoppedPageCap":
    "The page limit for one read was reached, so some pages were not opened.",
  "ob.live.stoppedByteCap":
    "The size limit for one read was reached, so some pages were not opened.",
  "ob.live.stoppedBudget":
    "The AI allowance was reached, so some pages were not opened.",
  "ob.live.stoppedDeadline":
    "The time limit for one read was reached, so some pages were not opened.",
  "ob.live.coverageSkipped": "Skipped",
  "ob.live.coverageFailed": "Could not read",
  "ob.live.coverageClean":
    "Every page returned. Nothing was skipped and nothing failed.",

  // --- facts: saving one, and the ceiling on how many -------------------
  "ob.facts.rowSave": "Save fact: {fact}",
  "ob.facts.capReached":
    "Up to {max} facts can be saved. Clear one to add another.",

  // --- the handoff into the app -----------------------------------------
  "ob.enter.assembling": "Assembling company profile…",

  // --- the mailbox backread ---------------------------------------------
  // A separate operation from connecting, and the copy has to keep them
  // separate: connecting grants access, the backread spends budget reading
  // history. Read-only, and it writes nothing until the reader approves.
  "ob.backread.heading": "Import period",
  "ob.backread.estimating": "Counting messages in this period…",
  "ob.backread.estimate": "About {messages} messages in this period.",
  "ob.backread.estimateAtLeast":
    "At least {messages} messages in this period; counting stopped early.",
  "ob.backread.estimateHeuristic": "Estimated from the mailbox, not counted.",
  "ob.backread.estimateCost": "About {cost} in model calls.",
  "ob.backread.estimateFailed":
    "The period could not be estimated: {detail} Start anyway, or select another period.",
  "ob.backread.note":
    "The mailbox is not changed. Imported emails and contacts appear as the import progresses.",
  "ob.backread.start": "Connect and import",
  "ob.backread.startFailed":
    "The mailbox history import did not start: {detail} Retry, or continue and start it later in Settings.",
  "ob.backread.running": "Importing mailbox history",
  "ob.backread.runningNote":
    "The import continues while you work and resumes where it stopped.",
  "ob.backread.queued": "Queued. Starts shortly.",
  "ob.backread.progress": "{scanned} of about {total} messages",
  "ob.backread.progressNoTotal": "{scanned} messages so far",
  "ob.backread.tallyMessages": "messages read",
  "ob.backread.tallyCaptured": "kept",
  "ob.backread.tallySkipped": "ignored",
  "ob.backread.tallyContacts": "contacts found",
  "ob.backread.tallyCompanies": "companies found",
  "ob.backread.doneHeading": "Import results",
  "ob.backread.doneNote":
    "Nothing is written yet. All findings wait for review in the Worklist.",
  "ob.backread.failed":
    "The mailbox history import stopped: {detail} The connection is intact; restart the import in Settings.",
  "ob.backread.cancelled": "Import stopped. Nothing was written.",
  "ob.backread.cancelledPartial":
    "Import stopped. Records already captured stay and wait for review in the Worklist.",
  "ob.backread.cancelFailed":
    "The import could not be stopped: {detail} It keeps running meanwhile. Retry.",
  "ob.backread.detailUnavailable": "An unexpected error occurred.",
  "ob.backread.cancel": "Stop import",
  "ob.backread.explore": "Continue during import",
  "ob.backread.skip": "Skip history import",

  "auth.title": "Margince",
  "auth.checking": "Checking session…",
  "auth.pageTitle": "Sign in · Margince",
  "auth.loginTitle": "Sign in to Margince",
  // Short declaratives rather than one comma-joined sentence (VOICE-RULE-1):
  // each clause is a separate fact about the installation, and a reader who
  // stops after the first has still read a true sentence. Two of them, not
  // three — "Margince runs on your own server" is a claim about the product,
  // and someone at a login screen is here to get in. What is left is the
  // provisioning fact (A97, invite-only): what to do when there is no sign-up
  // link.
  "auth.loginSub":
    "Accounts are created by an administrator. Self-signup is not available.",
  // Five lines, one voice, and the ORDER is load-bearing: greeting, what the
  // system is for, what it does, the one promise, then the handover to the form.
  // They read as a paragraph somebody is saying, so reordering them breaks a
  // sentence rather than a layout.
  //
  // This region used to admit only limits on the system's own behaviour and
  // server-read facts about the installation — no greeting, and nothing the task
  // depended on. Both of those bounds are lifted here on purpose: the greeting
  // is the first thing said, and the last line exists to point at the form in
  // the other half of the screen.
  "auth.coreGreeting": "This is Margince.",
  "auth.corePurpose": "It takes care of the work around your work.",
  "auth.coreDevelopment": "Development AI",
  "auth.coreModeDevelopment": "Offline development mode",
  // The shortest label that still names the field (VOICE-RULE-1), pinned by the
  // login spec §7.1/§7.2 (Amendment 4) and reconciling
  // single-company-auth-concept.md §12, which already drew "Email".
  "auth.email": "Email",
  // A placeholder is an EXAMPLE, never an instruction and never the label
  // again. "Enter your email" in a placeholder is a label that disappears.
  // The address is the login spec §7.2's, and the reserved example domain
  // rather than a plausible one: `company.com` belongs to somebody.
  "auth.emailPlaceholder": "name@example.com",
  "auth.password": "Password",
  "auth.passwordPlaceholder": "Password",
  "auth.passwordHint": "At least 12 characters",
  "auth.showPassword": "Show password",
  "auth.hidePassword": "Hide password",
  "auth.capsLock": "Caps Lock is on",
  // NOT the label of a served provider button. A real installation's button text
  // is `oidc_providers[].label` off the wire, server-owned, and the client never
  // composes it. This is what the ui-preview fixture uses to stand in for that
  // server in the reader's own language — see app/ui-preview.ts.
  "auth.continueWith": "Continue with {brand}",
  // Labels the password path, not the provider buttons above it: where the
  // installation runs SSO, the form beneath this divider is the fallback door.
  "auth.orDivider": "or",
  // The card at an installation that closed the password door and whose
  // provider is not offering the flow yet — the deployment mounted it, the
  // OAuth app is not stored. It names what is missing without blaming the
  // reader, who cannot fix either half.
  "auth.noMethodOffered":
    "This company signs in through an identity provider that is not available. Ask an administrator to complete its setup.",
  // §7.1 verbatim. The noun is "company", not "workspace": ADR-0061
  // keeps `workspace` internal and §7.3 removed it from authentication. And the
  // line states that ACCESS is restricted, never that data is safe, encrypted or
  // compliant — VOICE-RULE-7 rules those out here, because they are outcome
  // claims the installation's own configuration can contradict, on the screen a
  // CISO reads on the way in.
  "auth.legalProtected": "Access to this company is restricted.",
  "auth.legalTerms": "Terms",
  "auth.legalPrivacy": "Privacy",
  "auth.signingIn": "Signing in…",
  "auth.signIn": "Sign in",
  "auth.failed": "Sign-in failed",
  "auth.errCredentials":
    "Sign-in failed. Check the email and password and retry.",
  "auth.errRateLimited": "Too many sign-in attempts. Retry later.",
  "auth.errUnreachable":
    "Margince could not be reached. Check the connection and retry.",
  "auth.retry": "Retry",
  "auth.noticeSignedOut": "You have been signed out.",
  "auth.noticeSessionExpired":
    "The session expired. Sign in again to continue.",
  "auth.noticeOidcFailed":
    "Sign-in with Google failed. If you were invited, open the link in the invitation email to complete account setup.",
  "auth.connectionTitle": "Margince could not be reached",
  "auth.connectionBody":
    "Check the connection and retry. If the problem continues, the server may be restarting.",
  "auth.unavailableTitle": "Installation not ready",
  "auth.unavailableBody":
    "This Margince installation is not ready for sign-in. An administrator must complete or repair the setup.",
  // The first-run claim (ADR-0105). An installation with no configured admin
  // waits to be claimed; this is the only screen that creates an account
  // without one, so it names what it is creating.
  // Changing your own password from account settings. The current password is
  // the authority, not the session.
  // The account whose password an operator chose. Authenticated, and
  // able to do exactly one thing until it has a password of its own.
  "forcedPassword.pageTitle": "Set password",
  "forcedPassword.title": "Set your own password",
  "forcedPassword.body":
    "This account still uses the password set by the administrator. Set a password only you know to continue.",
  "password.title": "Password",
  "password.body": "Change the password used to sign in.",
  "password.current": "Current password",
  "password.next": "New password",
  "password.confirm": "Confirm new password",
  "password.hint": "At least 12 characters",
  "password.tooShort": "Password is too short. Use at least 12 characters.",
  "password.mismatch": "Passwords do not match.",
  "password.changing": "Changing password…",
  "password.open": "Change password",
  "password.cancel": "Cancel",
  "password.submit": "Save new password",
  "password.doneTitle": "Password changed",
  "password.done": "All other devices were signed out.",
  "password.changeFailedTitle": "Password not changed",
  // Deliberately says nothing about WHICH field: this is the fallback for a
  // refusal the server did not explain, and naming the current password would
  // send someone hunting a mistake that may not be theirs.
  "password.errorGeneric": "No cause was reported. Retry.",
  "setup.pageTitle": "Set up Margince",
  "setup.title": "Claim this installation",
  "setup.body":
    "This installation has no company yet. The administrator has a one-time setup token from the token file the server wrote at first start.",
  "setup.token": "Setup token",
  "setup.tokenHint":
    "From the token file written at first start. The server log names its path, and contains the token if the file could not be written.",
  "setup.company": "Company name",
  "setup.baseCurrency": "Base currency",
  "setup.baseCurrencyHint":
    "All amounts are converted to this currency. It can be changed in Settings only until the first amount is converted.",
  "setup.baseCurrencyMalformed":
    "Enter a 3-letter currency code, like EUR, CHF or USD.",
  "setup.baseLanguage": "Base language",
  "setup.baseLanguageHint":
    "The language AI writes in when the whole team reads the text. Each person chooses their own display language, and customer replies follow the language of the thread.",
  "setup.timezone": "Reporting timezone",
  "setup.timezoneHint":
    "IANA timezone name. All reporting periods are computed in it. Guessed from this browser; change it if the team works elsewhere.",
  "setup.adminName": "Your name",
  "setup.adminEmail": "Your email",
  "setup.adminPassword": "Password",
  "setup.passwordHint": "At least 12 characters",
  "setup.passwordShort": "Password is too short. Use at least 12 characters.",
  "setup.rootWarning":
    "This creates the administrator account for the whole installation, with every permission, including managing all other users.",
  "setup.claim": "Create company",
  "setup.claiming": "Creating…",
  "setup.errorToken":
    "This setup token is not valid for this installation. Check the token file named in the server log at first start.",
  "setup.errorAlready":
    "This installation already has a company. Sign in, or ask the administrator to reset it.",
  "setup.errorFields": "Some fields are invalid. Correct them and retry.",
  "setup.errorServer":
    "Setup did not complete and nothing was created. Retry shortly, and check the server log if it fails again.",
  "setup.errorNetwork":
    "Margince could not be reached. Check the connection and retry.",
  "auth.forgotLink": "Forgot password?",
  "auth.forgotTitle": "Reset password",
  // Two sentences, sentence-cased, with no dash. VOICE-RULE-5 forbids an em or
  // en dash anywhere in user-facing copy, and a lowercase opening mid-surface
  // reads as a fragment rather than as a sentence. Same for auth.resetSub.
  "auth.forgotSub":
    "Enter your email address. If it has an account, a reset link is sent to it.",
  "auth.sendResetLink": "Send reset link",
  "auth.forgotSentTitle": "Check your email",
  "auth.forgotSentBody":
    "If that address has an account, a reset link has been sent. It expires in 1 hour.",
  "auth.resetTitle": "Choose new password",
  "auth.resetSub": "The link is valid. Enter a new password.",
  "auth.newPassword": "New password",
  "auth.setNewPassword": "Set new password",
  "auth.resetFailed": "This reset link is invalid, already used or expired.",
  // The password was refused, not the link — so the link is still good and the
  // user must not be sent to replace it.
  "auth.resetRejectedPassword":
    "The password was refused. Choose a different password.",
  // Neither the link's fault nor the user's: the token is untouched, so retrying
  // the same one is the right advice. Two sentences, no dash (VOICE-RULE-5).
  "auth.resetServerFailed":
    "The password was not set. The link is still valid; retry shortly.",
  // Its own key rather than auth.errRateLimited, which says "sign-in attempts":
  // this user is setting a password, not signing in, and copy that names the
  // wrong action reads as the wrong error.
  "auth.resetRateLimited": "Too many attempts. Set the password again later.",
  "auth.requestNewLink": "Request a new link",
  "auth.askAdminForNewLink":
    "Ask an administrator for a new set-password link.",
  "auth.resetDoneTitle": "Password updated",
  "auth.resetDoneBody":
    "The password was changed and all other sessions were signed out. Sign in with the new password.",
  "auth.backToLogin": "Back to sign in",
  "auth.signOut": "Sign out",

  "client.back": "Back to Margince",
  "client.title": "Margince alongside your mailbox",
  "client.sender": "Sender",
  "client.lookup": "Look up",
  "client.open360": "Open 360 view",
  "client.unknown": "Not in your company yet.",
  "client.unknownDetail":
    "This sender matches no contact you can see. Nothing was fetched from anywhere else.",
  "client.createLead": "Capture as lead",
  "client.isolation": "Connects only to your company",
  "client.attribution": "Every capture is attributed and auditable.",

  "book.title": "Book a meeting",
  "book.min15": "15 min",
  "book.min30": "30 min",
  "book.min60": "60 min",
  "book.attendee": "Attendee email",
  "book.welcomeBack": "Recognized: {name}",
  "book.subject": "Meeting via Margince",
  "book.confirmed": "Meeting booked",
  "book.tellThemYourself":
    "Margince does not send an invitation. Share the time with your attendee directly.",
  "book.failed": "Booking failed. Nothing was scheduled.",
  "book.name": "Your name",
  "book.email": "Your email",
  "book.consentWording":
    "I agree that my name and email are stored to arrange and follow up on this meeting.",

  "prefs.title": "Choose which emails you receive",
  "prefs.sub":
    "Each purpose is separate. Transactional messages cannot be switched off here because you need them; all other purposes are yours to control.",
  "prefs.unsub.title": "Stop receiving these emails?",
  "prefs.unsub.lead":
    "One click stops messages of this kind to your address. Nothing else changes.",
  "prefs.unsub.loading": "Opening email preferences…",
  "prefs.unsub.afterTitle": "What happens next",
  "prefs.unsub.afterBody":
    "Emails of this kind are no longer sent to you. Security and service messages you need for something you requested are not affected.",
  "prefs.unsub.confirm": "Unsubscribe from these emails",
  "prefs.unsub.busy": "Recording your choice\u2026",
  "prefs.unsub.seeAll": "See all preferences",
  "prefs.unsub.privacy":
    "No sign-in needed. This personal link controls only your email preferences. Do not share it.",
  "prefs.unsub.doneTitle": "Unsubscribed",
  "prefs.unsub.doneBody":
    "You will not receive {label} from this sender again. The change applies immediately.",
  "prefs.unsub.manage": "Manage preferences",
  "prefs.unsub.alreadyOff":
    "These emails were already switched off. Nothing changed.",
  "prefs.unsub.lockedTitle": "These messages cannot be switched off",
  "prefs.unsub.lockedBody":
    "They are needed for something you requested, such as a password reset or a confirmation.",
  "prefs.unsub.retry": "Retry",
  "prefs.unsub.unknownPurposeTitle": "This link matches no email type",
  "prefs.unsub.unknownPurpose":
    "Open your preferences to see every type of email sent.",
  "prefs.unsub.deadLinkTitle": "This link is no longer valid",
  "prefs.unsub.deadLinkBody":
    "Preference links expire and can be withdrawn. Request a new one from any recent email.",
  "prefs.unsub.errorTitle": "Preferences could not be opened",
  "prefs.unsub.failedTitle": "Emails not stopped",
  "prefs.purpose.business_correspondence": "Direct correspondence",
  "prefs.purpose.marketing_email": "Product news",
  "prefs.purpose.transactional": "Security and service messages",
  "prefs.sentVia": "Sent via Margince",
  "prefs.noObjection": "On: you have not objected to these",
  "prefs.optedOut": "Off: you asked to stop these",
  "prefs.invalidLink":
    "This link is no longer valid. Preference links expire and can be withdrawn. Request a new one from any recent email.",
  "buyer.opening": "Opening your Deal Room…",
  "buyer.deadTitle": "This link no longer works",
  "buyer.deadAskContact": "Ask your contact for a new link.",
  "buyer.linkDead":
    "The link was already used, has expired or was replaced by a newer one. Request a new link below.",
  "buyer.noLink":
    "Open this page from the link you received. If you no longer have it, request a new one below.",
  "buyer.emailLabel": "Your email address",
  "buyer.emailHint": "The address the invitation was sent to.",
  "buyer.requestLink": "Request new link",
  "buyer.linkRequestedTitle": "Check your email",
  "buyer.linkRequested":
    "If that address was invited, a new link is on its way.",
  "buyer.pausedTitle": "Access is paused",
  "buyer.pausedBody":
    "{steward} has paused this room. Your link remains valid, and you can continue once access resumes.",
  "buyer.expiredTitle": "Access has ended",
  "buyer.expiredBody":
    "Access to this room has expired. Contact {steward}, or request a new link below.",
  "buyer.eyebrow": "Deal Room",
  "buyer.contact": "Your contact: {steward}.",
  "buyer.closed": "This room is closed. Its content is kept as a record.",
  "buyer.previewBannerTitle": "You are previewing this room",
  "buyer.previewBanner":
    "This is the buyer’s view. You can read everything but change nothing.",
  "buyer.previewReadOnly":
    "A preview is read-only. Close this tab to return to the Deal Room page.",
  "buyer.closedNote": "This room is now read-only.",
  "buyer.stewardUnknown": "your contact",
  "buyer.signOut": "Sign out",
  "buyer.signedInAs": "Signed in as {name}.",
  "buyer.contactEyebrow": "Your contact",
  "buyer.contactBody":
    "Ask your question under the relevant document; it goes directly to {steward}.",
  "buyer.closedOn": "Closed on {date}",
  "room.docs.title": "Documents",
  "room.docs.empty": "No documents in the room yet.",
  "room.docs.fileLabel": "File from this deal",
  "room.docs.fileHint":
    "Any file in the deal’s Files area can be added, including uploads and email attachments.",
  "room.docs.pickFile": "Pick a file",
  "room.docs.noFiles": "No files on this deal",
  "room.docs.groupLabel": "Group",
  "room.docs.add": "Add to room",
  "room.docs.upload": "Upload a file",
  "room.docs.remove": "Remove {title} from the room",
  "room.docs.group.commercial": "Commercial",
  "room.docs.group.legal": "Legal",
  "room.docs.group.security_privacy": "Security and privacy",
  "room.docs.group.delivery_operations": "Delivery and operations",
  "buyer.docs.title": "Documents",
  "buyer.docs.empty": "No documents yet.",
  "buyer.docs.download": "Download {title}",
  "buyer.docs.downloadFailed":
    "The download did not start. Retry, or ask your contact.",
  "buyer.docs.downloadShort": "Download",
  "buyer.poweredBy": "Powered by",
  "buyer.poweredByMargince": "Powered by Margince",
  "threads.roomTitle": "Room threads",
  "threads.aboutThis_other": "{count} threads about this document",
  "threads.aboutThis_one": "{count} thread about this document",
  "threads.askAbout": "Ask about this document",
  "threads.read": "Read",
  "threads.readTitle": "Read {title}",
  "threads.unanswered_one": "{count} unanswered",
  "threads.unanswered_other": "{count} unanswered",
  "threads.cancel": "Cancel",
  "threads.empty": "No messages yet.",
  "threads.requiredChange": "Change required",
  "threads.resolved": "Resolved",
  "threads.sideBuyer": "buyer",
  "threads.sideSeller": "seller",
  "threads.replyLabel": "Reply",
  "threads.reply": "Reply",
  "threads.resolve": "Resolve",
  "threads.newLabel": "New thread",
  "threads.requireChangeLabel": "This document needs a change",
  "threads.open": "Post",
  "threads.readOnly": "Your access is read-only.",
  "deal360.blocker": "Blockers",
  "deal360.buyer": "What the buyer wants",
  "deal360.verdict.live": "Live",
  "deal360.verdict.drifting": "Drifting",
  "deal360.verdict.blocked": "Blocked",
  "deal360.verdict.cold": "Cold",
  "dealmail.title": "Email",
  "dealmail.reply": "Draft reply",
  "dealmail.send": "Send email",
  "recordmail.title": "Email",
  "recordmail.reply": "Draft reply",
  "recordmail.send": "Write email",
  "deal360.rewrite": "Regenerate",
  "deal360.readFull": "Read full brief",
  "deal360.openTask": "Open task",
  "deal360.createTask": "Add task",
  "deal360.openBrief": "Open meeting brief",
  "deal360.unreadable":
    "This brief did not load. Reload the page, or regenerate it.",
  "prefs.rateLimited":
    "Too many attempts from this device. Wait a minute and reload.",
  "prefs.subscribed": "On: you asked for these",
  "prefs.alwaysOn": "Always on",
  // The public confirm-your-details page. Margince speaks in first contact here,
  // as it does in onboarding: short flat sentences, says what it will and will
  // not do, no em dashes.
  "confirm.title": "Your details",
  "confirm.intro":
    "Margince, an AI system, runs this CRM. Below is everything recorded about you. You can correct any of it, or request its removal.",
  "confirm.card.title": "Recorded details",
  "confirm.field.fullName": "Name",
  "confirm.field.title": "Job title",
  "confirm.field.email": "Email",
  "confirm.field.phone": "Phone",
  "confirm.field.company": "Company",
  "confirm.field.none": "Not recorded",
  "confirm.marketing.title": "Receive occasional news?",
  "confirm.marketing.ask":
    "News from time to time, roughly once a month. Your choice is respected.",
  "confirm.marketing.yes": "Subscribe to news",
  "confirm.marketing.no": "Do not send news",
  "confirm.provenance.title": "Sources of your details",
  "confirm.provenance.empty": "No source is recorded for these details.",
  "confirm.provenance.line": "{field}: from {source}, recorded {date}",
  "confirm.erasure.ask": "Request removal",
  "confirm.erasure.staged":
    "Removal requested. Confirm below to send the request.",
  "confirm.submit": "Confirm",
  "confirm.subscription.title": "Confirm your subscription",
  "confirm.subscription.ask": "Confirm that you want to receive {purpose}.",
  "confirm.subscription.confirm": "Confirm subscription",
  "confirm.subscription.alreadyTitle": "You are subscribed",
  "confirm.subscription.alreadyBody":
    "Your subscription to {purpose} is confirmed. You can stop it at any time from any email sent to you.",
  "confirm.done.title": "Thank you",
  "confirm.receipt.title": "What happens next",
  "confirm.receipt.body":
    "Quote this reference in any question about your request. A response follows within one month.",
  "confirm.receipt.rectify": "Correction requested",
  "confirm.receipt.erasure": "Removal requested",
  "confirm.done.body":
    "Your answer is recorded. Any changes go to a person here to apply, and this link is now used up.",
  "confirm.invalidLink":
    "This link is no longer valid. It may have been used or may have expired.",
  "prefs.lockedWhy": "Needed for something you requested, so it stays on.",
  "prefs.confirmationNeededWhy":
    "To switch this on, use the confirmation link in the email sent to you. You can switch it off here at any time.",
  "prefs.notSaved": "Not saved yet.",
  "prefs.savePending": "Pending: {changes}.",
  "prefs.saveProof":
    "The exact wording you saw and a timestamp are recorded as proof. The choice then applies to every future email.",
  "prefs.save": "Save preferences",
  "prefs.discard": "Discard",
  "prefs.cannotGrant":
    "{purposes} cannot be started for this record. If you think that is wrong, reply to any email from this sender to have it reviewed.",
  "prefs.cannotGrantWhy": "This cannot be switched on for this record.",
  "prefs.choiceNotApplied":
    "One of your choices did not take effect. Your current settings are shown above.",
  "prefs.confirmationSent":
    "Check your email and click the link to confirm {purposes}. The subscription does not start until you confirm.",
  "prefs.confirmationUnavailable":
    "The confirmation email for {purposes} could not be sent, so the subscription has not started. Retry later.",
  "prefs.partialSave":
    "Saving stopped partway. Some of your choices may have been saved; your current settings were reloaded so you can see exactly what applies.",
  "prefs.wording.business_correspondence":
    "“Send me replies and direct messages about our conversations.”",
  "prefs.wording.transactional":
    "“Send me what I need for something I asked for.”",
  "prefs.wordingGeneric": "“Send me {label}.”",
  "prefs.wording.marketing_email":
    "“Send me product updates and occasional marketing email.”",
  "prefs.wording.events": "“Send me event and webinar invitations.”",
  "prefs.unsubscribeAll": "Stop all marketing",
  "prefs.unsubscribeAllHint":
    "Switches off every marketing purpose above. Replies to your own inquiries and anything you requested continue, because no one subscribed you to those.",
  "prefs.oneClickDone":
    "You are unsubscribed from this sender’s marketing email. This applies immediately to every campaign.",
  "prefs.oneClickAlreadyOff": "These were already off. Nothing changed.",
  "prefs.undo": "Undo and keep receiving marketing",
  "prefs.undoExplicit":
    "Resubscribing is an explicit opt-in and is never switched back on automatically. Save below to record your consent, or discard.",

  "auto.tier.runs": "Runs",
  "auto.tier.approval": "Approval",
  "auto.sub":
    "A rule marked “runs” acts on its own. A rule marked “approval” sends its actions to Approvals.",
  "auto.readOnly":
    "Read-only: you do not have permission to change automations.",
  "auto.catalog": "Starter library",
  "auto.catalogSub": "Available automation types",
  "auto.instances": "Configured automations",
  "auto.use": "Use template",
  "auto.name": "Name",
  "auto.create": "Create",
  "auto.createdPaused": "Created paused. Nothing runs until it is enabled.",
  "auto.delete": "Delete",
  "auto.statusEnabled": "Enabled",
  "auto.statusPaused": "Paused",
  "auto.dateField.placeholder": "Select date field",
  "auto.dateField.needsObject":
    "Choose an object first to list its date fields.",
  "auto.dateField.empty": "This object has no active date fields yet.",
  "auto.dateField.loadError": "Could not load date fields. Retry.",
  "auto.enabledFor": "{name} is enabled",
  "auto.rowActions": "Actions for {name}",
  "auto.withheld": "Configured automations are hidden for your role.",
  "auto.deleteTitle": "Delete this automation?",
  "auto.deleteBody":
    "“{name}” and its settings are permanently deleted. To stop it without losing the rule, turn it off instead.",

  "auto.runs.open": "Runs",
  "auto.runs.title": "Run history",
  "auto.runs.filterAll": "All",
  "auto.runs.filterFired": "Fired",
  "auto.runs.filterFailed": "Failed",
  "auto.runs.filterBlocked": "Blocked",
  "auto.runs.filterSkipped": "Skipped",
  "auto.runs.filterQueued": "Queued for approval",
  "auto.runs.empty": "This automation has not fired yet.",
  "auto.runs.emptyFiltered": "No runs with this outcome.",
  "auto.runs.needsApproval": "Needs approval",
  "auto.runs.why": "Trigger",
  "auto.runs.target": "Target",
  "auto.runs.result": "Result",
  "auto.runs.reason": "Reason",
  "auto.runs.outcomeFired": "Fired",
  "auto.runs.outcomeFailed": "Failed",
  "auto.runs.outcomeBlocked": "Blocked",
  "auto.runs.outcomeSkipped": "Skipped",
  "auto.runs.outcomeQueued": "Queued",

  "auto.preview.open": "Preview",
  "auto.preview.title": "Dry-run impact",
  "auto.preview.window": "Window",
  "auto.preview.window7": "7 days",
  "auto.preview.window30": "30 days",
  "auto.preview.window90": "90 days",
  "auto.preview.matchesNow": "Matches now: {n}",
  "auto.preview.wouldFire": "Would fire: about {n} in {days} days",
  "auto.preview.notComputable": "Trailing estimate unavailable",
  "auto.preview.hidden": "{n} hidden (no access)",
  "auto.preview.explainer":
    "Read-only dry run. No records are changed and nothing is sent.",

  "strength.title": "Relationship strength",
  "strength.score": "Score {score} of 100",
  "strength.bucket.none": "Dormant",
  "strength.bucket.weak": "Weak",
  "strength.bucket.moderate": "Warm",
  "strength.bucket.strong": "Strong",
  "strength.factor.recency": "Recency",
  "strength.factor.frequency": "Frequency",
  "strength.factor.reciprocity": "Reciprocity",
  "strength.factor.direction": "Direction",
  "strength.lastInteraction": "Last interaction: {when}",
  "strength.none": "No interactions yet",
  "strength.inout": "{in} in · {out} out (90 days)",
  "strength.computedFrom": "Computed from {count} activities",

  // The relationship-graph cards (ADR-0078). The colleague bands are PO-F-3b's
  // own vocabulary and deliberately differ from the workspace-wide card's:
  // the two measure different things and must not read as comparable.
  "network.title": "Colleague connections",
  "network.empty": "No colleague has been in touch with this contact yet.",
  "network.interactions": "{count} interactions (90 days)",
  "network.neverSpoken": "No recorded contact",
  "network.bucket.none": "No contact",
  "network.bucket.weak": "Weak",
  "network.bucket.moderate": "Moderate",
  "network.bucket.strong": "Strong",
  "coverage.engaged": "Engaged",
  "coverage.quiet": "No two-way contact",
  "coverage.seatWithheld": "A contact you cannot read",
  "coverage.daysSinceTouch": "{days} days",
  "coverage.risk.single_threaded_theirs": "Single-threaded",
  "coverage.risk.single_threaded_ours": "Covered by one colleague",
  "coverage.risk.coverage_gap": "No engaged champion",
  "coverage.risk.champion_left": "Champion has left",
  "coverage.risk.stakeholder_left": "Stakeholder has left",
  "coverage.risk.going_cold": "Going cold",

  "cf.title": "Custom fields",
  "cf.formSection": "Custom fields",
  "cf.subtitle":
    "Add a typed field to an existing object at runtime, without code or a deploy. New objects and relationships require code changes.",
  "cf.object": "Object",
  "cf.obj.deal": "Deal",
  "cf.obj.company": "Company",
  "cf.obj.contact": "Contact",
  "cf.obj.lead": "Lead",
  "cf.obj.project": "Project",
  "cf.obj.contract": "Contract",
  "cf.listLabel": "Fields on {object}",
  "cf.col.field": "Field",
  "cf.col.type": "Type",
  "cf.col.addedBy": "Added by",
  "cf.addedByYou": "You",
  "cf.addedByAdmin": "Administrator",
  "cf.empty.deal":
    "No custom fields on Deal yet. Add one to track data the core fields do not cover.",
  "cf.empty.company":
    "No custom fields on Company yet. Add one to track data the core fields do not cover.",
  "cf.empty.contact":
    "No custom fields on Contact yet. Core fields cover the contact record; add one to track more.",
  "cf.empty.lead":
    "No custom fields on Lead yet. A field added here also appears after a lead is qualified as a contact.",
  "cf.empty.project":
    "No custom fields on Project yet. Add one to track delivery data the core fields do not cover.",
  "cf.empty.contract":
    "No custom fields on Contract yet. Add one for a classification the standard terms do not cover.",
  "cf.type.text": "Text",
  "cf.type.number": "Number",
  "cf.type.date": "Date",
  "cf.type.currency": "Currency",
  "cf.type.multiselect": "Multiple choice",
  "cf.type.picklist": "Picklist",
  "cf.type.boolean": "Yes/No",
  "cf.builder.addTo": "Add field to {object}",
  "cf.builder.open": "New field",
  "cf.builder.noCode": "No code",
  "cf.builder.intro":
    "A new field is a real column on the existing table. It works in filters, reports, exports and the API like any core field. It is not a new object.",
  "cf.label": "Label",
  "cf.apiKey": "API key",
  "cf.apiKeyHint":
    "Derived automatically and fixed once live. The cf_ prefix prevents collisions with core fields.",
  "cf.typeLabel": "Type",
  "cf.currencyCode": "Currency code",
  "cf.currencyHint":
    "Three-letter ISO 4217 code, for example EUR or USD. Amounts are stored to the cent.",
  "cf.options": "Options",
  "cf.addOption": "Add option",
  "cf.removeOption": "Remove option",
  "cf.optionPlaceholder": "Option label",
  "cf.lastOptionBlocked": "A picklist needs at least one option",
  "cf.gate.title": "Add this field?",
  "cf.gate.body":
    "On confirm, it becomes a live column on every {object}: on the 360 page, in search and filters, lists, export and the API. The addition is recorded in the audit log.",
  "cf.refuse.title": "This looks like a new object or relationship",
  "cf.refuse.body":
    "This builder adds simple fields to existing records only. A new object, a link between objects or a calculated roll-up is a structural change. It ships as a reviewed change in a new Margince version, made by people, not by the product editing its own code.",
  "cf.refuse.route":
    "Route it through development: your own engineers, an implementation partner or Margince services.",
  "cf.confirm": "Add field",
  "cf.writing": "Saving…",
  "cf.added":
    "Field “{label}” added. It appears on records and in filters, exports and the API.",
  "cf.edit": "Edit label",
  "cf.archive": "Archive field",
  "cf.archived":
    "“{label}” archived. It is hidden from new records, kept in audit and history, and can be restored.",
  "cf.renamePrompt": "New label",
  "cf.renamed": "Renamed to “{label}”",
  "cf.audit.title": "Recent field changes",
  "cf.audit.empty": "No custom field changes yet.",
  "cf.audit.footer":
    "Every addition, edit and archive is recorded permanently in the audit log.",
  "cf.noPermission": "You have read-only access to custom fields.",
  "cf.retired": "Retired",
  // The settings level, in the order the sidebar prints it. "General" rather
  // than "Company" for the first company entry: the group heading above it
  // already says that word, and a row repeating its own heading names nothing.
  // The same reason keeps the possessive off "Agents" — the group is "You".
  //
  // "Connections" and "Integrations" are the same distinction the two groups
  // are: the mailbox and the network a CONTACT connected, against the outside
  // systems the INSTALLATION is wired to. One row carried both before, which
  // is why it had to be ungated to keep a rep's own mailbox reachable.
  "settings.home": "Overview",
  "settings.home.yours": "Your settings",
  "settings.home.manage": "Editable settings",
  "settings.home.lookUp": "Read-only settings",
  "settings.home.rolesLabel": "Your role",
  "settings.home.seatLabel": "Your seat",
  "settings.home.seat.full": "Full seat: changes allowed",
  "settings.home.seat.read": "Read-only seat: view only",
  "settings.home.reachLabel": "Visible records",
  "settings.home.reach.own": "Your own records",
  "settings.home.reach.team": "Your team\u2019s records",
  "settings.home.reach.all": "All company records",
  "settings.home.access": "Your access",
  "settings.boundary.deniedTitle": "No access to this settings page",
  "settings.boundary.deniedBody":
    "This page exists, but your role cannot open it. Copy the address to ask someone who has access.",
  "settings.boundary.unknownTitle": "Settings page not found",
  "settings.boundary.unknownBody":
    "The link may be outdated or mistyped. The settings overview lists every page your role can open.",
  "settings.page.account.sub": "How you appear and sign in.",
  "settings.page.voice.sub": "The wording drafts use when they write as you.",
  "settings.page.agents.sub":
    "What an agent may do unattended, and which clients hold your credentials.",
  "settings.page.connections.sub":
    "Mailboxes and addresses this seat reads from.",
  "settings.page.capture-activity.sub":
    "What capture did with your mail, and why.",
  "settings.page.company.sub":
    "Company name, currency and business context used across all records.",
  "settings.page.authentication.sub":
    "How people sign in to this installation, and which apps may act for it.",
  "settings.page.members.sub":
    "Everyone with a seat, and what each can access.",
  "settings.page.teams.sub":
    "Team membership, which decides team-scoped record access.",
  "settings.page.seats.sub":
    "Seats in use against this installation’s entitlement.",
  "settings.page.stageautomation.sub":
    "The track record of each stage transition before it moves deals automatically.",
  "settings.page.pipelines.sub":
    "Stages a deal moves through, for the whole company.",
  "settings.page.acquisition.sub":
    "Business channels a deal can be attributed to.",
  "settings.page.recordroles.sub":
    "What a colleague or team can be responsible for on a record. Grants no access.",
  "settings.page.leads.sub": "Lead source terms used by this company.",
  "settings.page.fields.sub":
    "Custom fields this company adds to the standard fields.",
  "settings.page.tags.sub":
    "Shared tags. Renaming a tag here renames it on every record.",
  "settings.page.products.sub":
    "Products this company sells, and offer templates.",
  "settings.page.capture.sub":
    "Which mail becomes a record and which is ignored.",
  "settings.page.integrations.sub":
    "Systems this installation exchanges records with.",
  "settings.page.knowledge.sub": "Documents that drafts and answers may use.",
  "settings.page.import.sub": "Import records from a file, one run at a time.",
  "settings.page.models.sub":
    "Which provider handles each kind of work, and whether it is available.",
  "settings.page.automations.sub": "Rules that run automatically.",
  "settings.page.usage.sub": "AI spend this month against the allowance.",
  "settings.page.model-calls.sub": "Each model request and its response.",
  "settings.page.privacy.sub":
    "Data subject requests, consent purposes and record retention.",
  "settings.page.audit.sub": "Every actor and every record they accessed.",
  "settings.page.system-health.sub":
    "Whether background processing is keeping up.",
  "settings.page.extensions.sub":
    "Extensions in this build, and which roles can access them.",
  "settings.page.reset.sub":
    "Delete this installation’s data. Cannot be undone.",
  "settings.scope.self": "Only you",
  "settings.scope.mixed": "Mixed",
  "settings.scope.workspace": "Company",
  "settings.scope.installation": "Installation",
  "settings.scopeAria": "Applies to: {scope}",
  "settings.scopeAriaMixed":
    "Settings on this page affect different people; each one states which.",
  "settings.readOnlyPageTitle": "Read-only settings",
  "settings.readOnlyPage": "Your role cannot change these settings.",
  "settings.saveFailed": "Change not saved",
  "settings.mintFailed": "Passport not created",
  "settings.search.label": "Search settings",
  "settings.search.placeholder": "Search settings",
  "settings.search.none": "No settings page matches.",
  "settings.search.count": "Matching settings pages: {n}",
  "settings.tab.company": "Company profile",
  "settings.tab.authentication": "Sign-in and apps",
  "settings.tab.members": "Members",
  "settings.tab.teams": "Teams",
  "settings.tab.seats": "Seats and license",
  "settings.tab.stageautomation": "Stage automation",
  "settings.tab.pipelines": "Pipelines",
  "settings.tab.acquisition": "Acquisition sources",
  "settings.tab.recordroles": "Responsibility roles",
  "settings.tab.leads": "Lead handling",
  "settings.tab.fields": "Fields",
  "settings.tab.tags": "Tags",
  "settings.tab.products": "Products and offers",
  "settings.tab.import": "Data import",
  "settings.tab.models": "Models and routing",
  "settings.tab.automations": "Automations",
  "settings.tab.usage": "AI usage",
  "settings.tab.model-calls": "Model calls",
  "settings.tab.audit": "Audit log",
  "settings.tab.system-health": "System health",
  "settings.tab.reset": "Reset data",
  "settings.group.me": "You",
  "settings.group.company": "Company",
  "settings.group.people": "People",
  "settings.group.sales": "Sales",
  "settings.group.data": "Data",
  "settings.group.ai": "AI",
  "settings.group.governance": "Governance",
  "settings.tab.account": "Account",
  "settings.tab.voice": "Writing voice",
  "settings.tab.agents": "Agents",
  "settings.tab.connections": "Connections",
  "settings.tab.general": "General",
  "settings.tab.users": "Users and teams",
  "settings.tab.extensions": "Extensions",
  "settings.tab.integrations": "Integrations",
  "settings.tab.capture": "Capture rules",
  "settings.tab.data-model": "Data model",
  "settings.tab.ai": "AI",
  "settings.tab.knowledge": "Knowledge",
  "corpusAsk.title": "Ask your documents",
  "corpusAsk.sub":
    "Ask in your own words. Answers come only from one document set, questions the set does not cover are refused, and every sentence cites its passage.",
  "corpusAsk.whichSet": "Document set",
  "corpusAsk.question": "Your question",
  "corpusAsk.submit": "Ask",
  "corpusAsk.byModel": "Written by Margince from your documents",
  "corpusAsk.byPassages": "Source passages only. No answer was written.",
  "corpusAsk.notReady":
    "This set is still being read: {embedded} of {total} passages are searchable. Retry shortly. The question is not the problem.",
  "corpusAsk.retrievalUnavailable":
    "Nothing was searched. This installation has no search index configured.",
  "corpusAsk.unreviewed":
    "These passages are closest to your question. They have not been reviewed for whether they answer it.",
  "corpusAsk.failed": "Question not answered",
  "corpusAsk.unreviewedTitle": "Passages not reviewed",
  "corpusAsk.notReadyTitle": "This set is still being read",
  "corpusAsk.retrievalUnavailableTitle": "No search index is configured",
  "corpusAsk.noGrantTitle": "You cannot open this company’s documents",
  "corpusAsk.noGrant":
    "You do not have access to this document set. An administrator can grant access.",
  "corpusAsk.noSetsTitle": "No document sets",
  "corpusAsk.noSets":
    "This company has no documents yet, so there is nothing to search.",
  "corpusAsk.citeAtLine": "{number}: {document}, line {line}",
  "corpusAsk.citeInDocument": "{number}: in {document}",
  "corpusAsk.documentLoading": "Loading document…",
  "corpusAsk.documentFailedTitle": "Document could not be opened",
  "corpusAsk.documentFailed":
    "{document} could not be loaded. The quoted passage is below.",
  "corpusAsk.openFile": "Open file",
  "corpusAsk.quoteNotPinpointed":
    "This quote spans a line break and could not be highlighted exactly. The document is open at its source.",
  "corpusAsk.notCovered.title": "Not covered by this set",
  "corpusAsk.notCovered.body":
    "{name} was searched in full and has nothing close enough to answer this. It covers:",
  "knowledge.title": "Document sets",
  "knowledge.sub":
    "Document collections this company can query. Answers use only filed documents, and questions they do not cover are refused.",
  "knowledge.withheld": "You do not have access to the list of document sets.",
  "knowledge.coverage":
    "{documents} documents · {embedded} of {total} passages searchable",
  "knowledge.reindexingTitle": "Reindexing this set",
  "knowledge.reindexing":
    "A change to text indexing started the reindex. Questions return “not ready” until it finishes. No data was lost.",
  "knowledge.showDocuments": "Show documents",
  "knowledge.hideDocuments": "Hide documents",
  "knowledge.documents": "Documents",
  "knowledge.noDocuments": "No documents yet.",
  "knowledge.archive": "Archive set",
  "knowledge.archiveFailed": "Set not archived",
  "knowledge.archiveConfirm.title": "Archive this document set?",
  "knowledge.archiveConfirm.body":
    "The set and its documents are no longer searchable. Nothing is deleted.",
  "knowledge.deleteDocument": "Delete",
  "knowledge.deleteFailed": "The document was not deleted. Retry.",
  "knowledge.deleteConfirm.title": "Delete this document?",
  "knowledge.deleteConfirm.body":
    "The file, its extracted text and its search index are permanently deleted.",
  "knowledge.ingest.queued": "Queued",
  "knowledge.ingest.running": "Indexing…",
  "knowledge.ingest.done": "Searchable",
  "knowledge.ingest.failed": "Could not be read",
  "knowledge.ingestDetailTitle": "Why this file could not be read",
  "knowledge.upload.label": "Add document",
  "knowledge.upload.hint":
    "Plain text, Markdown, CSV or JSON. PDF and Word files are not supported and are refused.",
  "knowledge.upload.empty": "Drop a text file here, or select one",
  "knowledge.upload.submit_other": "Add {count} documents",
  "knowledge.upload.refusedTitle_one": "1 file was not added",
  "knowledge.upload.refusedTitle_other": "{count} files were not added",
  "knowledge.upload.refused": "{filename}: {message}",
  "knowledge.upload.submit_one": "Add document",
  "knowledge.new.title": "New document set",
  "knowledge.new.name": "Name",
  "knowledge.new.topic": "What this set covers",
  "knowledge.new.topicHint":
    "One sentence. It is shown to anyone whose question this set does not cover.",
  "knowledge.new.submit": "Create set",
  "knowledge.new.failed": "The set was not created. Retry.",
  "settings.tab.privacy": "Privacy and retention",
  "settings.tab.capture-activity": "Capture activity",
  "verdictPass.subject.senders": "Senders",
  "verdictPass.subject.threads": "Threads",
  "verdictPass.every_one": "{subject} are checked every minute.",
  "verdictPass.every_other": "{subject} are checked every {minutes} minutes.",
  "verdictPass.next": "Next check {when}.",
  "verdictPass.queued": "A check is due and waiting to start.",
  "verdictPass.running": "A check is running now.",
  "captureActivity.title": "Capture activity",
  "captureActivity.sub":
    "What the last 24 hours of your mail produced. Excluded senders are listed above.",
  "captureActivity.scope.label": "Whose activity",
  "captureActivity.outcomes": "Outcomes",
  "captureActivity.messages": "Messages",
  "captureActivity.scope.mine": "Mine",
  "captureActivity.scope.workspace": "Shared channels",
  "captureActivity.scopeNote":
    "Counted from when a connector hands a message to this CRM. Items a connector filtered on its own side, such as a chat reaction or a mail rule, are not included. Covers messages only; lead capture is not shown here.",
  "captureActivity.filtered":
    "Showing {shown} of {total} {outcome} in this window.",
  "captureActivity.openTrace": "Show every processing step for this message",
  "captureActivity.emptyFiltered":
    "No loaded rows match. Load more to see the rest of the window.",
  "captureActivity.loadMore": "Load more",
  "captureActivity.empty": "No capture activity in the last 24 hours.",
  "captureActivity.payloadsOff":
    "This installation does not record message senders or subjects, so the rows below show only the result.",
  "captureActivity.contentNone": "No sender recorded",
  "captureActivity.outcome.captured": "Captured",
  "captureActivity.outcome.internal": "Dropped as internal",
  "captureActivity.outcome.suppressed": "No contact created",
  "captureActivity.outcome.deferred": "Awaiting sender check",
  "captureActivity.outcome.fault": "Derivation failed",
  "captureActivity.funnel.captured": "Captured",
  "captureActivity.funnel.internal": "Internal",
  "captureActivity.funnel.suppressed": "No contact",
  "captureActivity.funnel.deferred": "Awaiting check",
  "captureActivity.funnel.fault": "Failed",
  "captureActivity.reason.internal_only":
    "every party was on your company’s domains",
  "captureActivity.reason.deferral_capped":
    "the open-question limit was reached, so no result is coming",
  "captureActivity.reason.noise_prior":
    "an earlier check marked this sender as noise, so the message is archived",
  "captureActivity.reason.decided_prior":
    "this sender was already decided, so no contact is created",
  "captureActivity.reason.no_granting_human":
    "the connection names no user to act for",
  "captureActivity.reason.invisible_incumbent":
    "it matched a record you cannot see",
  "captureActivity.reason.derivation_failed":
    "the contact step failed; the message itself is unaffected",
  "captureActivity.reason.no_counterparty": "no sender that can be recorded",
  "captureActivity.reason.role_mailbox":
    "a shared mailbox, not a person: kept, but no contact created",
  "captureActivity.reason.private_thread":
    "a private thread: kept for you, but no contact created",
  "captureActivity.reason.transactional_infra":
    "the sender is mail infrastructure, not a company you work with",
  "captureActivity.reason.transactional_prefix":
    "the sender looks like an automated mailer, not a person",
  "captureActivity.outcome.deferred_capped": "Not queued",
  "captureActivity.outcome.deferred_sent": "Sent for a sender check",
  "captureActivity.resolution.pending": "still waiting",
  "captureActivity.resolution.unsure": "sent for review",
  "captureActivity.resolution.real": "judged a real person",
  "captureActivity.resolution.noise": "judged noise",
  "captureActivity.resolution.rejected": "declined by a human",
  "captureActivity.resolution.suppressed": "suppressed",
  "pipeline.title": "How this message was handled",
  "pipeline.sub":
    "Each capture step, in the order this message passed through it.",
  "pipeline.payloadsOff":
    "No sender or subject is stored for any step: payload capture is turned off on this installation.",
  "pipeline.transport": "Received via",
  "pipeline.unavailable": "Capture steps for this message did not load. Retry.",
  "pipeline.status.done": "Done",
  "pipeline.status.skipped": "Skipped",
  "pipeline.status.pending": "Waiting",
  "pipeline.status.failed": "Failed",
  "pipeline.status.not_applicable": "Not applicable",
  "pipeline.status.unknown": "Unknown",
  "pipeline.reason.record_not_available":
    "this step’s record is no longer kept or is not visible to you; once a record is deleted the two cannot be distinguished",
  "pipeline.status.not_reported": "Not reported here",
  "pipeline.subject.message": "about this message",
  "pipeline.subject.sender": "about the sender, not this message alone",
  "pipeline.subject.domain": "about the sender’s domain",
  "pipeline.subject.thread": "about the whole thread",
  "pipeline.stage.connector_filter": "Connector filter",
  "pipeline.stage.ingress_gate": "Intake check",
  "pipeline.stage.erasure_check": "Erasure check",
  "pipeline.stage.internal_drop": "Internal-only check",
  "pipeline.stage.activity_write": "Saved to timeline",
  "pipeline.stage.tier_ladder": "Contact decision",
  "pipeline.stage.contact_create": "Contact created",
  "pipeline.stage.verdict": "Sender check",
  "pipeline.stage.company_triage": "Company check",
  "pipeline.stage.attention_label": "Attention label",
  "pipeline.stage.material_events": "Thread analysis",
  "pipeline.stage.claim_extraction": "Commitments and open loops",
  "pipeline.reason.internal_only": "every party was on your company’s domains",
  "pipeline.reason.invisible_incumbent": "it matched a record you cannot see",
  "pipeline.reason.transactional_infra":
    "the sender is mail infrastructure, not a company you work with",
  "pipeline.reason.transactional_prefix":
    "the sender looks like an automated mailer, not a person",
  "pipeline.reason.deferral_capped":
    "the open-question limit was reached, so no result is coming",
  "pipeline.reason.noise_prior":
    "an earlier check marked this sender as noise, so the message is archived",
  "pipeline.reason.decided_prior":
    "this sender was already decided, so no contact is created",
  "pipeline.reason.no_counterparty": "no sender that can be recorded",
  "pipeline.reason.role_mailbox":
    "a shared mailbox, not a person: kept, but no contact created",
  "pipeline.reason.private_thread":
    "a private thread: kept for you, but no contact created",
  "pipeline.reason.no_granting_human":
    "the connection names no user to act for",
  "pipeline.reason.derivation_failed":
    "the contact step failed; the message itself is unaffected",
  "pipeline.reason.not_linked_yet": "no contact is linked to this message yet",
  "pipeline.reason.no_contact_intended":
    "the contact decision concluded that none was to be made",
  "pipeline.reason.awaiting_verdict": "the sender check is still pending",
  "pipeline.reason.judged_real": "this sender was judged a real person",
  "pipeline.reason.judged_noise":
    "this sender was judged noise, so no record was made",
  "pipeline.reason.judged_rejected":
    "this sender was declined, so no record was made",
  "pipeline.reason.judged_suppressed":
    "this sender was suppressed, so no record was made",
  "pipeline.reason.no_open_question":
    "there was no open question about this sender",
  "pipeline.reason.thread_not_captured":
    "no captured message remains on this thread, so there was nothing to read",
  "pipeline.reason.events_raised":
    "this thread was read, and its events were filed on the company",
  "pipeline.reason.nothing_material":
    "this thread was read and contained nothing worth filing",
  "pipeline.reason.thread_still_moving":
    "this thread is still active; it is read once it has been quiet for a while",
  "pipeline.reason.awaiting_scan":
    "this thread is due to be read and has not been reached yet",
  "pipeline.reason.reading_parked":
    "reading this thread was refused several times, so it is paused until the thread changes or the pause expires",
  "pipeline.reason.no_single_account":
    "this thread does not map to exactly one company, so its findings have no certain destination",
  "pipeline.reason.two_bodies_of_work":
    "this thread spans 2 projects, so its findings would be wrong for one of them",
  "pipeline.reason.thread_not_all_open":
    "a message on this thread is hidden from some of its readers, so a summary of the whole would present a partial view as complete",
  "pipeline.reason.answered_on_the_company":
    "this step concerns the sender’s domain, so it is answered once on the company, not on every message",
  "pipeline.reason.company_warranted":
    "this domain was judged to warrant a company record, and this is it",
  "pipeline.reason.no_site_identified":
    "nothing on this domain’s site identified a company, so the sender’s own name was used instead",
  "pipeline.reason.triage_queued": "this domain is waiting for review",
  "pipeline.reason.triage_unevidenced":
    "nothing seen from this domain yet indicates a company",
  "pipeline.reason.triage_stale_evidence":
    "what was seen from this domain is too old to decide on",
  "pipeline.reason.triage_near_duplicate":
    "this domain looks like one already recorded, so it is held rather than duplicated",
  "companyTriage.title": "Company origin",
  "companyTriage.sub": "Mail domains checked and the result of each check.",
  "companyTriage.empty":
    "No mail domain was checked for this company. It was created manually or imported.",
  "companyTriage.checkedAt": "checked {when}",
  "pipeline.reason.no_named_reader":
    "this thread has no reader a finding could be addressed to",
  "pipeline.reason.transport_not_read":
    "this step reads email only, and the message arrived by another channel",
  "pipeline.reason.sender_undecided":
    "the sender check is still pending, so the message is held back",
  "pipeline.reason.archived": "the message is archived",
  "pipeline.reason.not_connector_captured":
    "the message was not captured by a connector",
  "pipeline.reason.awaiting_batch":
    "it is eligible and waiting for the next batch",
  "pipeline.reason.labelled": "the message was labeled",
  "pipeline.reason.not_comparable":
    "connector-side filtering is not counted here; the numbers mean different things per connector",
  "pipeline.reason.connector_side_defect":
    "intake failures are a fault of the connection, not of one message",
  "pipeline.reason.would_restore_erased":
    "reporting this would restore data an erasure removed",
  "pipeline.reason.no_writer_yet": "this step does not exist yet",
  "settings.tab.maintenance": "Maintenance",
  "settings.tab.license": "License",
  "license.card.title": "License and seats",
  "license.state.licensed": "Licensed",
  "license.state.uncapped": "Licensed, no seat limit",
  "license.state.unlicensed": "No license configured",
  "license.state.refused": "License refused",
  "license.absent.title": "This installation has no license",
  "license.absent.body":
    "Everything keeps working and nothing is capped. Configure a license token for this installation to count seats against a grant.",
  "license.refused.title": "This installation’s license was refused",
  "license.refused.body":
    "The license token configured for this installation was presented and rejected. Everything keeps working, uncapped, until it is replaced. Check the token and the installation’s clock.",
  "license.seats.capacityOnly":
    "Full seats this installation is using. The entitlement is not visible to this role.",
  "license.seats.title": "Seats",
  "license.seats.uncapped": "No limit",
  "license.seats.ofGranted": "{used} of {granted}",
  "license.seats.left": "{count} left",
  "license.seats.over": "{count} over the grant",
  "license.over.title": "Seats in use exceed the entitlement",
  "license.over.body":
    "{used} seats are in use and the license grants {granted}. No one loses access and no seat is removed, but no new member can be invited until the count is within the entitlement. Deactivate a member or raise the entitlement.",
  "license.holder.title": "Licensed to",
  "license.holder.company": "Company",
  "license.holder.contact": "Contact",
  "license.holder.installation": "Installation",
  "license.holder.validUntil": "Valid until",
  "license.holder.expiredOn": "Expired on",
  "license.holder.id": "License ID",
  "license.grace.title": "This license expired",
  "license.grace.body":
    "The license expired on {expiry}. It still works for a limited period. Renew it to keep the installation in service.",
  "license.renewal.title": "License needs renewal",
  "license.renewal.body":
    "The license expires on {expiry}. Nothing changes before that date.",
  "license.counting":
    "Full seats that are neither deactivated nor suspended, agents included. Read-only seats are unlimited and never counted. New members are admitted against this count.",
  "settings.rates.fxTitle": "Currency rates",
  "settings.rates.fxIntro":
    "Exchange rates that convert foreign-currency amounts to the base currency. New rates take effect today or later; past rates never change.",
  "settings.rates.fxWithheld":
    "Only an administrator or operations user can see currency rates. Every roll-up in the installation is converted with them.",
  "settings.rates.modelWithheld":
    "Only an administrator or operations user can see model prices.",
  "settings.rates.readOnly": "Read-only. Your role cannot change rates.",
  "settings.rates.fxTableLabel": "Rates in force",
  "settings.rates.fxAdd": "Set rate",
  "settings.rates.fxEmpty": "No currency rates yet.",
  "settings.rates.fxModalTitle": "Set currency rate",
  "settings.rates.rateToBase": "Rate (to base currency)",
  "settings.rates.modelTitle": "AI model costs",
  "settings.rates.modelIntro":
    "Price per model in USD per 1M tokens, used to estimate AI spend. Prices never affect model routing.",
  "settings.rates.modelTableLabel": "Prices in force",
  "settings.rates.modelAdd": "Add model rate",
  "settings.rates.modelEmpty": "No model rates yet.",
  "settings.rates.modelModalTitle": "Set model price",
  "settings.rates.notSaved": "Rate not saved",
  "settings.rates.setRate": "Save",
  "settings.rates.refresh": "Refresh from sources",
  "settings.rates.refreshEnqueued":
    "Refresh requested. Proposed changes appear in Approvals.",
  "settings.rates.colFrom": "From",
  "settings.rates.colRate": "Rate (→{base})",
  "settings.rates.colEffective": "Effective",
  "settings.rates.colProvider": "Provider",
  "settings.rates.colModel": "Model",
  "settings.rates.colInput": "Input $/M",
  "settings.rates.colOutput": "Output $/M",
  "settings.rates.colCacheRead": "Cache read $/M",
  "settings.rates.colCacheWrite": "Cache write $/M",
  "settings.voice.title": "Voice DNA",
  "settings.voice.intro":
    "Your personal writing voice. It shapes drafts written for you, is visible only to you and learns only from samples you add.",
  "settings.voice.readOnly": "Read-only. Your role cannot change Voice DNA.",
  "settings.voice.emptyBody":
    "Add writing samples to build your Voice DNA. It takes about a minute.",
  "settings.voice.status.collecting": "Collecting",
  "settings.voice.status.ready": "Ready",
  "settings.voice.status.stale": "Rebuild needed",
  "settings.voice.bandThin": "thin",
  "settings.voice.bandGood": "good",
  "settings.voice.bandRich": "rich",
  "settings.voice.bandSharp": "sharp",
  "settings.voice.version": "version {n}",
  "settings.voice.derivedLabel": "Derived voice",
  "settings.voice.derivedEmpty":
    "Not built yet. Add samples and build to see the derived voice.",
  "settings.voice.personalityLabel": "Preferences",
  "settings.voice.personalityPlaceholder":
    "Notes on how you want to sound. Kept as written; the model never overwrites them.",
  "settings.voice.savePreferences": "Save preferences",
  "settings.voice.corpusLabel": "Writing samples",
  "settings.voice.corpusRowLabel": "Current samples",
  "settings.voice.meter": "{count} of {target} words",
  "settings.voice.register.email": "email",
  "settings.voice.register.social": "social",
  "settings.voice.register.long_form": "long-form",
  "settings.voice.register.spoken": "spoken",
  "settings.voice.register.general": "general",
  "settings.voice.bandDrop":
    "Removing this lowers the voice from {from} to {to}. Select remove again to confirm.",
  "voice.insights.avoidLabel": "What your voice avoids",
  "voice.insights.voiceScore": "Voice match {pct}%",
  "voice.insights.next.addTranscript":
    "Add a call or meeting transcript; spoken words are the strongest source.",
  "voice.insights.next.addEmail":
    "Add sent emails, the main source for how you write at work.",
  "voice.insights.next.addWords":
    "Add about {count} more words to reach the target range.",
  "voice.insights.next.atTarget":
    "Your writing samples are at target. Add recent writing occasionally to keep them current.",
  "voice.status.active": "Active",
  "voice.status.candidate": "Awaiting review",
  "voice.status.superseded": "Superseded",
  "voice.status.rejected": "Rejected",
  "voice.classification.routine": "routine change",
  "voice.classification.material": "material change",
  "voice.outcome.autoActivated": "activated automatically",
  "voice.outcome.reviewRequired": "review required",
  "voice.outcome.manuallyActivated": "activated by you",
  "voice.outcome.rejected": "rejected",
  "voice.outcome.rollback": "restored",
  "voice.history.versionRow": "v{n} \u00b7",
  "voice.history.loadMore": "Show older entries",
  "voice.insights.provenance": "Built from your writing samples · v{n}",
  "voice.insights.statWords": "Words: {count}",
  "voice.insights.statSources": "Sources: {count}",
  "voice.insights.statSentence": "About {count} words per sentence",
  "voice.insights.thinkingLabel": "How you think",
  "voice.insights.movesLabel": "Your signature phrases",
  "voice.insights.samplesLabel": "Sample drafts in your voice",
  "voice.insights.draftOnly": "Draft only, never sent",
  "voice.insights.disclosure":
    "AI-assisted drafts. Every send remains a human decision.",
  "voice.insights.nextBestLabel": "To improve:",
  "voice.candidate.title": "Voice v{n} ready for review",
  "voice.candidate.whatItIs":
    "What the build learned from your samples. It is not used for drafts until you select it.",
  "voice.candidate.reviewLabel": "What this version says about your writing",
  "voice.candidate.concernsLabel": "Why this version needs review",
  "voice.candidate.applyHint":
    "If it reads like you, use it. If not, keep your current voice and add more writing samples; the next build learns from them.",
  "voice.candidate.reason.malformed":
    "The scoring check could not read some sample drafts, so the score is based on fewer samples than usual.",
  "voice.candidate.reason.lowScore":
    "Sample drafts in this voice scored {score} against your writing, below the minimum of {floor} this installation requires for automatic activation.",
  "voice.candidate.reason.hardFailures":
    "Phrases this voice should avoid that appeared in the sample drafts: {n}.",
  "voice.candidate.reason.rulesRemoved":
    "Rules about what to avoid dropped since your previous version: {n}.",
  "voice.candidate.apply": "Use this version",
  "voice.candidate.reject": "Keep current voice",
  "voice.history.label": "Versions and learning",
  "voice.history.empty": "No versions yet. Build your voice first.",
  "voice.history.deltasLabel": "What changed",
  "voice.history.deltasEmpty":
    "Nothing to compare yet. Changes appear from the second build on.",
  "voice.history.deltaRow": "v{from} \u2192 v{to}",
  "voice.history.learning":
    "Learning continuously: drafts served {drafted}, edited before sending {edited}, rejected {rejected}.",
  "voice.history.rollback": "Restore version {n}",
  "settings.voice.corpusEmpty": "No samples yet.",
  "settings.voice.excluded": "excluded",
  "settings.voice.removeSource": "Remove sample",
  "settings.voice.addSource": "Add writing samples",
  "settings.voice.addFirstLabel": "First writing sample",
  "settings.voice.dropHint":
    "Drop or choose files (.txt, .md, .pdf, .docx, .vtt, .srt or .json), several at once.",
  "settings.voice.dropEmpty": "Drop files here, or click to choose",
  "settings.voice.whyToggle": "Why add samples",
  "settings.voice.whyBody":
    "Margince drafts emails in your own words. It learns tone, rhythm and phrasing only from your writing. Samples are visible only to you.",
  "settings.voice.worksTitle": "Best samples",
  "settings.voice.worksEmails":
    "Sent emails saved as .txt, .md, .pdf or .docx.",
  "settings.voice.worksDocs":
    "Proposals, posts and other text you wrote yourself.",
  "settings.voice.worksTranscripts":
    "Call or meeting transcripts (.vtt, .srt, .json or a text export). Margince asks which speaker is you and keeps only your turns.",
  "settings.voice.worksNot":
    "Leave out text others wrote and AI drafts. They teach a different voice.",
  "settings.voice.floorNote":
    "A first build needs at least {min} words. Below that the model copies phrasing.",
  "settings.voice.floorLabel": "Progress to first build ({min} words)",
  "settings.voice.floorProgress": "{words} of {min} words for a first build",
  "settings.voice.speakerQuestion":
    "“{name}” has several speakers. Which speaker are you?",
  "settings.voice.speakerWhy":
    "Only your turns are kept. Other speakers’ words are dropped.",
  "settings.voice.speakerDetail": "{words} words, {turns} turns",
  "settings.voice.speakerConfirm": "Use this speaker",
  "settings.voice.speakerDismiss": "Skip this file",
  "settings.voice.noticeKept":
    "{name}: kept {kept} of {total} words. Only your turns count.",
  "settings.voice.noticeAdded": "{name}: {words} words added.",
  "settings.voice.noticeSkippedType":
    "{name} was skipped. Supported formats: .txt, .md, .pdf, .docx, .vtt, .srt, .json.",
  "settings.voice.noticeSkippedUnreadable":
    "{name} could not be opened. If it is password-protected or damaged, paste its text instead.",
  "settings.voice.noticeSkippedEmpty":
    "{name} was skipped. It contains no text.",
  "settings.voice.noticeDismissed":
    "{name} was skipped. None of it could be attributed to you.",
  "settings.voice.noticeAskQueueFull":
    "{name} was not added. Answer the speaker questions above, then add it again.",
  "settings.voice.noticeFailed": "{name} could not be added: {detail}",
  "settings.voice.noticeUnexpected": "{name} could not be added.",
  "settings.voice.refusalUnattributed":
    "{name} has several speakers and none of the text could be attributed to you, so nothing was added.",
  "settings.voice.refusalSpeaker":
    "That speaker was not found in {name}, so nothing was added.",
  "settings.voice.refusalUnsupported": "{name} is in an unsupported format.",
  "settings.voice.buildsTitle": "Builds",
  "settings.voice.buildRowLabel": "Build from samples",
  "settings.voice.building": "Building…",
  "settings.voice.buildRunning":
    "Building voice. This takes about a minute and continues if you leave this page.",
  "settings.voice.rebuild": "Rebuild Voice DNA",
  "settings.voice.buildFirst": "Build Voice DNA",
  "settings.voice.buildNeedsWords":
    "A first build needs about {n} more words. Below that there is not enough writing to learn from.",
  "settings.voice.buildProvisional":
    "Enough to build. About {n} more words give a fuller picture of your writing.",
  "settings.voice.buildStatus.succeeded": "Voice DNA updated",
  "settings.voice.buildStatus.failed": "Build did not finish. Retry.",
  "settings.voice.buildStatus.deferred":
    "Queued. This page updates when the build finishes.",
  "settings.voice.buildStatus.pending":
    "Still building. This page updates when the build finishes.",
  "extAccess.title": "Extensions and access",
  "extAccess.sub":
    "What each extension unit adds to this installation, and which roles may use it. Administrators only.",
  "extAccess.adminOnly":
    "This page requires permission to read installation extensions and roles. Your role lacks one or both.",
  "extAccess.readOnly":
    "Your role can read this page. Changing a grant requires a full seat.",
  "extAccess.empty": "No extension units are installed.",
  "extAccess.version": "Version {version}",
  "extAccess.openUnit": "Open {name} page",
  "extAccess.noPage":
    "{name} is installed on the API, but this app build has no page for it. The app is probably older than the server.",
  "extAccess.brings.heading": "What this unit adds",
  "extAccess.brings.objects": "Permission objects",
  "extAccess.brings.routes": "Routes",
  "extAccess.brings.jobs": "Background jobs",
  "extAccess.brings.none": "None",
  "extAccess.noObjects":
    "This unit registers no permission objects, so there is nothing to grant.",
  "extAccess.roleColumn": "Role",
  "extAccess.action.read": "Read",
  "extAccess.action.create": "Create",
  "extAccess.action.update": "Update",
  "extAccess.action.delete": "Delete",
  "extAccess.matrixCaption": "Permissions for {object}",
  "extAccess.cell": "Allow {role} to {action} {object}",
  "extAccess.versionSkew":
    "Another user changed this role while you were viewing it, so your change was not applied. The grants above are current; make the change again if needed.",
  "extAccess.systemRole": "Built-in role",
  "extAccess.readOnlyTitle": "Read-only for your role",
  "extAccess.nobodyReadsTitle": "No role can read this extension",
  "extAccess.grantFailed": "Grant not changed",
  "extAccess.nobodyReads":
    "No role has read access to {object}, so every member sees an empty screen for this extension. Grant read to at least one role below.",
  "users.empty": "No users yet.",
  "users.adminOnly": "Your role cannot manage users.",
  "users.inviteTitle": "Invite user",
  "users.teamsLabel": "Teams",
  "users.noTeamsYet": "No teams yet.",
  "users.teamMembersLabel": "Team members",
  "users.teamMembersAdminOnly": "Your role cannot see team members.",
  "users.teamNobodyToAdd": "No users to add.",
  "users.teamsTitle": "Teams",
  "users.teamsSub":
    "Named groups that records can be shared with. For most roles, membership grants no access. A team lead added to a team can read and edit that team’s records without a share.",
  "users.teamsAdminOnly": "Your role cannot manage teams.",
  "users.deactivated": "{name} deactivated",
  "users.reactivated": "{name} reactivated",
  "users.roleSaved": "Role changed for {name}",
  "users.teamArchived": "Team “{name}” archived",
  "users.teamRestored": "Team “{name}” restored",
  "users.archiveTeam": "Archive team {name}",
  "users.newTeamLabel": "New team",
  "users.newTeamOpen": "New team",
  "users.teamNameLabel": "Team name",
  "users.newTeamPlaceholder": "For example, DACH Sales",
  "users.createTeam": "Create team",
  "users.notArchived": "Team not archived",
  "users.notSaved": "Change not saved",
  "users.notCreated": "Team not created",
  "users.inviteFailed": "Invitation not sent",
  "users.access.title": "User access",
  "users.access.identity":
    "Reads every contact, company, lead and deal in the company.",
  "users.access.writesAll": "Edits every record.",
  "users.access.writesTeam":
    "Edits their own records and those of the teams {teams}.",
  "users.access.writesTeamNone":
    "Edits only their own records. Not on a team yet.",
  "users.access.writesOwn": "Edits only their own records.",
  "users.access.none": "no access",
  "users.access.read": "read",
  "users.access.write": "write",
  "users.access.delete": "delete",
  "users.access.object.contact": "Contacts",
  "users.access.object.company": "Companies",
  "users.access.object.lead": "Leads",
  "users.access.object.deal": "Deals",
  "users.access.object.project": "Projects",
  "users.access.mask": "{field} is withheld {when}.",
  "users.access.maskAlways": "always",
  "users.access.maskOutside": "on records they may not edit",
  "users.inviteSub":
    "Add a user to this installation and choose their starting role.",
  "users.membersTitle": "Users",
  "users.membersSub": "Everyone with a seat, including deactivated accounts.",
  "users.memberCount_one": "{count} user",
  "users.memberCount_other": "{count} users",
  "users.teamMemberCount_one": "{count} member",
  "users.teamMemberCount_other": "{count} members",
  "users.emailLabel": "Email",
  "users.nameLabel": "Full name",
  "users.emailPlaceholder": "name@company.com",
  "users.namePlaceholder": "Full name",
  "users.deactivateConfirmTitle": "Deactivate {name}?",
  "users.deactivateConfirmBody":
    "They are signed out everywhere and their agent passports are revoked immediately. They can be reactivated later and must then sign in again.",
  "users.deactivateAgentConfirmBody":
    "This is the company’s agent identity. It signs in nowhere, so no person loses access. Scheduled extension jobs keep running, each acting as itself and capturing under the authority of the member whose connection produced the record.",
  "users.agentSeat": "Agent",
  "users.agentSeatRole": "Acts through a passport, not a role",
  "users.roleLabel": "Role",
  "users.inviteOpen": "Invite user",
  "users.invite": "Invite",
  "users.setRole": "Set role…",
  "users.setRoleFor": "Set role for {name}",
  "users.rowActions": "Actions for {name}",
  "users.rolesHeld": "Holds {roles}. Choosing a role replaces all of them.",
  "users.deactivate": "Deactivate",
  "users.reactivate": "Reactivate",
  "users.status.active": "Active",
  "users.status.invited": "Invited",
  "users.status.deactivated": "Deactivated",
  "users.status.suspended": "Suspended",
  "users.link.action": "Get set-password link",
  "users.link.title": "Set-password link for {name}",
  "users.link.pending": "Creating link…",
  // Two sentences, no dash (VOICE-RULE-5).
  "users.link.body":
    "Send this link to the member through a trusted channel. It works once and is shown only now. A new link can be created from the member’s row.",
  "users.link.urlLabel": "Set-password link",
  "users.link.copy": "Copy link",
  "users.link.copied": "Copied",
  "users.link.copyFailed":
    "Copy failed. Select the link in the field and copy it manually.",
  "users.link.expires": "Expires {when}.",
  "users.link.failedTitle": "Link not created",
  "users.link.failed":
    "The member exists but cannot sign in until they receive a link.",
  "users.link.offline":
    "The server could not be reached. Check the connection and retry.",
  "users.link.retry": "Retry",
  "users.link.done": "Done",
  "settings.companyTitle": "Company profile",
  "settings.companyReadOnly":
    "Read-only. Your role cannot change the company profile.",
  "settings.companySub":
    "Shared business context for drafts, offers, search and agents. Each statement records who supplied it and its source.",
  "settings.companyTrust":
    "Confirmed statements only. Website text never becomes instructions.",
  "settings.companyConfirmed": "confirmed statements",
  "settings.companyMark": "Company logo",
  "settings.companyMarkIntro":
    "The sidebar shows the company at 2 widths. The collapsed sidebar uses the wide logo until a square icon is added.",
  "settings.companyMarkWide": "Wide logo",
  "settings.companyMarkWidePresent":
    "Shown here and at the top of the expanded sidebar.",
  "settings.companyMarkWideNone":
    "No logo yet; initials are shown. Add one here, or read the website to find one.",
  "settings.companyMarkIcon": "Square icon",
  "settings.companyMarkIconPresent":
    "Shown in the collapsed sidebar, where the wide logo is too small to read.",
  "settings.companyMarkIconNone":
    "No icon yet. The collapsed sidebar uses the wide logo.",
  "settings.companyMarkAdd": "Add",
  "settings.companyMarkReplace": "Replace",
  "settings.companyMarkRemove": "Remove",
  "settings.companyMarkUploadFailed": "Logo not uploaded",
  "settings.companyMarkRemoveFailed": "Logo not removed",
  "settings.companyMarkAddWide": "Add wide logo",
  "settings.companyMarkReplaceWide": "Replace wide logo",
  "settings.companyMarkRemoveWide": "Remove wide logo",
  "settings.companyMarkAddIcon": "Add square icon",
  "settings.companyMarkReplaceIcon": "Replace square icon",
  "settings.companyMarkRemoveIcon": "Remove square icon",
  "settings.companyMarkWideHint":
    "SVG or transparent PNG, about 800 × 240 px (up to 4:1), under 5 MB. JPEG, GIF, WebP and ICO also work. Proportions are kept.",
  "settings.companyMarkIconHint":
    "SVG or transparent PNG, about 256 × 256 px, square, under 5 MB. A wide file is not cropped; it is shown small inside the square.",
  "settings.companyMarkEmpty": "Drop logo here, or choose a file",
  "settings.companyMarkIconEmpty": "Drop icon here, or choose a file",
  "settings.companyWebsite": "Public company website",
  "settings.companyWebsiteHint":
    "The public site that website reads start from.",
  "settings.companySourceTitle": "Source",
  "settings.companyRefreshRow": "Read website again",
  "settings.companyRefreshHint":
    "Margince reads the public pages and proposes changes. Nothing reaches the profile until you review and apply them.",
  "settings.companyEdit": "Edit",
  "settings.companyEditField": "Edit {field}",
  "settings.companyWebsiteRequired": "Add a company website before refreshing.",
  "settings.companyRefresh": "Refresh from website",
  "settings.companyEssentials": "Essentials",
  "settings.companyPositioning": "Positioning, buyers and sales motion",
  "settings.companyIdentity": "Identity and legal details",
  "settings.companySave": "Save company context",
  "settings.companySaved": "Saved",
  "settings.companySaveFailed": "Company context not saved",
  "settings.companyRefreshFailed": "Website read did not finish",
  "settings.companyApplyFailed": "Changes not applied",
  "settings.companyRefreshWarnings": "Warnings from this read",
  "settings.companyRefreshUnreadable":
    "The status of this website read was lost. Start the refresh again.",
  "settings.companyRefreshStale":
    "The website proposal changed. Review the updated comparison before applying it.",
  "settings.companyRefreshReview": "Website comparison",
  "settings.companyRefreshReady": "Review what changed",
  "settings.companyRefreshReading": "Reading website…",
  "settings.companyCoverage": "page coverage",
  "settings.companyResolveAll":
    "Choose an outcome for every conflict with a user-entered value.",
  "settings.companyApplyRefresh": "Apply selected changes",
  "settings.companySelectChange": "Select change to {field}",
  "settings.companyClass.new": "New",
  "settings.companyClass.machine_change": "Website changed",
  "settings.companyClass.human_conflict": "Needs your decision",
  "settings.companyClass.unchanged": "Unchanged",
  "settings.companyResolution.keep_current": "Keep current",
  "settings.companyResolution.accept_proposal": "Accept website",
  "settings.companyResolution.useValueFor": "Value to keep for {field}",
  "settings.companyResolution.use_value": "Use edited value",
  "settings.companyManualKicker": "Manual setup",
  "settings.companyManualTitle": "Company essentials",
  "settings.companyManualSub":
    "Website reading is not enabled for this installation. These 3 answers create usable company context without a model call or external request.",
  "settings.companyCreateWorkspace": "Create company context",
  "product.title": "Products",
  "product.settingsSub": "Price list entries that offer lines copy from.",
  "product.readOnly": "Read-only. Your role cannot change products.",
  "product.new": "New product",
  "product.edit": "Edit product",
  "product.archive": "Archive product",
  "product.archiveConfirm":
    "Archive this product? Existing offer lines keep their snapshot.",
  "product.name": "Name",
  "product.sku": "SKU",
  "product.description": "Description",
  "product.unit": "Unit",
  "product.unitPrice": "Unit price",
  "product.currency": "Currency",
  "product.taxRate": "Default tax rate %",
  "product.billingModel": "Billing",
  "product.billingUnclassified": "Not specified",
  "product.billingOneTime": "One-time",
  "product.billingRecurring": "Recurring",
  "product.billingInterval": "Billing period",
  "product.billingNoInterval": "Not specified",
  "product.billingMonthly": "Monthly",
  "product.billingQuarterly": "Quarterly",
  "product.billingHalfYearly": "Every six months",
  "product.billingYearly": "Yearly",
  "product.active": "Active",
  "product.activeFilter": "Active only",
  "product.activeFilterAll": "All",
  "product.inactive": "Inactive",
  "product.archived": "Archived",

  "template.title": "Offer templates",
  "template.settingsSub":
    "Branded PDF layouts for offers in German and English.",
  "template.readOnly": "Read-only. Your role cannot change offer templates.",
  "template.new": "New template",
  "template.edit": "Edit template",
  "template.archive": "Archive template",
  "template.archiveConfirm":
    "Archive this template? Offers that reference it fall back to the locale default.",
  "template.name": "Name",
  "template.locale": "Locale",
  "template.isDefault": "Default for locale",
  "template.header": "Header text",
  "template.footer": "Footer text",
  "template.localeFilter": "Locale",
  "template.localeFilterAll": "All locales",
  "template.localeDE": "German (DE)",
  "template.localeEN": "English (US)",

  "tools.title": "Agent tools",
  "tools.sub":
    "Tools a passport can call; the same inventory an MCP client sees.",
  "tools.egress": "External access",
  "tools.scopeAll": "All passports",
  "tools.inventory": "All {count} tools",
  "tools.scopeLabel": "Filter by passport",
  "tools.scopedTo": "Reachable by {label}",
  "tools.unreachable": "Scope not granted",

  "aiusage.title": "Estimated AI spend and usage",
  "aiusage.withheld":
    "Only an administrator or operations user can see AI spend. The figures cover the whole installation.",
  "aiusage.sub":
    "Historical usage for the selected month. Estimates are separate from the live token allowance and from your provider bill.",
  "aiusage.col.task": "Task",
  "aiusage.col.tier": "Tier",
  "aiusage.col.calls": "Calls",
  "aiusage.col.cached": "Cached",
  "aiusage.col.tokensIn": "Tokens in",
  "aiusage.col.tokensOut": "Tokens out",
  "aiusage.col.cost": "Estimated cost",
  "aiusage.costNote": "Costs are estimates at configured rates.",
  "aiusage.costPartial":
    "This total covers priced calls only: {calls} more had no configured rate, and their usage appears only in the token counts.",
  "aiusage.monthLabel": "Month",
  "aiusage.spendLabel": "Spend by task",
  "aiusage.days.show": "Show days",
  "aiusage.empty": "No AI calls this month.",
  "aiusage.prevMonth": "Previous month",
  "aiusage.nextMonth": "Next month",

  "aibanner.degraded": "80% of AI allowance reached. Review affected features.",
  "aibanner.queued": "AI allowance reached. Review deferred work.",
  "aibanner.unknown": "AI allowance status not recognized",
  "aibanner.link": "Manage allowance",
  "aibanner.dismiss": "Dismiss",

  "licensebanner.body":
    "This installation’s license token was checked and refused.",
  "licensebanner.link": "Open {tab}",

  "aicalls.title": "AI call trace",
  "aicalls.withheld":
    "Only an administrator or operations user can read the call trace. It records every model call the installation made.",
  "aiHealth.withheld":
    "Only an administrator or operations user can see whether model tiers respond. This is installation infrastructure, not data about your work.",
  "aicalls.sub":
    "Every model call: routing identity, tokens, retries, captured payload.",
  "aicalls.col.detail": "Detail",
  "aicalls.expandCall": "Show attempts for {task} at {when}",
  "aicalls.col.when": "When",
  "aicalls.col.task": "Task",
  "aicalls.col.model": "Model",
  "aicalls.col.tokens": "Tokens",
  "aicalls.col.latency": "Latency",
  "aicalls.ms": "{value} ms",
  "aicalls.badge.cacheHit": "Cache hit",
  "aicalls.badge.degraded": "Degraded",
  "aicalls.badge.retries": "Retry ×{count}",
  "aicalls.callsLabel": "Recent calls",
  "aicalls.filter.all": "All tasks",
  "aicalls.loadMore": "Load more",
  "aicalls.empty": "No AI calls recorded yet.",
  "aicalls.detail.identity":
    "Served {served} via {provider} (configured: {configured})",
  "aicalls.detail.source": "Served identity source: {source}",
  "aicalls.detail.context": "Injected context: {scopes}",
  "aicalls.detail.contextNone": "No company context injected",
  "aicalls.detail.attempts": "Attempts",
  "aicalls.detail.request": "Request payload",
  "aicalls.detail.response": "Response payload",
  "aicalls.payload.off":
    "Payload capture is off. Set ai.capture_payloads: true in margince.yaml to record request and response content.",
  "aicalls.payload.none": "No payload captured for this call.",

  "aiexport.button": "Export certification scenario",
  "aiexport.title": "Export run as certification scenario",
  "aiexport.nameLabel": "Scenario name",
  "aiexport.checklist":
    "Secrets were removed at capture. Personal data was not: review and remove it, then replace sanitized_by before committing this file to the corpus.",
  "aiexport.copy": "Copy YAML",
  "aiexport.copied": "Copied",
  "aiexport.download": "Download .yaml",
  "aiexport.copyFailed": "Copy failed. Use the preview or download instead.",
  "aiexport.close": "Close",
  "aiexport.previewLabel": "Scenario preview",
  "aiexport.responseLabel": "Model response",

  "countdown.daysHours": "{days}d {hours}h",
  "countdown.hoursMinutes": "{hours}h {minutes}m",
  "countdown.minutesSeconds": "{minutes}m {seconds}s",
  "countdown.expired": "Expired",

  "installationSettings.companyTitle": "Installation",
  "installationSettings.companySub":
    "Installation name, and the timezone reporting periods are computed in.",
  "installationSettings.currencyTitle": "Currency",
  "installationSettings.dateFormat": "Date format",
  "installationSettings.timeFormat": "Time format",
  "installationSettings.formatsHint":
    "Applies throughout the interface. Stored dates and timezones stay the same.",
  "installationSettings.dateFormat.locale": "Use interface language",
  "installationSettings.dateFormat.dmy": "DD.MM.YYYY · 23.09.2026",
  "installationSettings.dateFormat.mdy": "MM/DD/YYYY · 09/23/2026",
  "installationSettings.dateFormat.ymd": "YYYY-MM-DD · 2026-09-23",
  "installationSettings.timeFormat.locale": "Use interface language",
  "installationSettings.timeFormat.24h": "24-hour · 17:30",
  "installationSettings.timeFormat.12h": "12-hour · 05:30 pm",
  "installationSettings.name": "Company name",
  "installationSettings.nameHint":
    "Shown wherever the product names the company.",
  "installationSettings.timezone": "Reporting timezone",
  "installationSettings.timezoneHint":
    "IANA timezone name, for example Europe/Berlin. Report periods and record dates are computed and shown in it for the whole team, separate from each user’s display timezone.",
  "installationSettings.fiscalYearStart": "Fiscal year starts",
  "installationSettings.fiscalYearStartHint":
    "Month the fiscal year begins. Reports group by this year and quarter; a year that does not start in January is labeled with both years, like FY2026/27. Changing it relabels every report, and saved views filtered by period then cover different months.",
  "installationSettings.forwardMeasure": "Projection basis",
  "installationSettings.forwardMeasureHint":
    "Which remaining deal value the projection adds to won revenue. Commit evidence: committed deals with a confirmed close date. Weighted: every open deal at its stage probability. Manager’s call: replaces the projection; a period with no call falls back to commit evidence and says so.",
  "installationSettings.forwardMeasure.commit_evidence":
    "Commit evidence: confirmed close dates only",
  "installationSettings.forwardMeasure.weighted":
    "Weighted: every open deal at its stage probability",
  "installationSettings.forwardMeasure.manager_call":
    "Manager’s call: the number set for the period",
  "forecast.landing": "Landing",
  "forecast.landingFrom": "Projected · {won} won + {remaining} to come",
  "forecast.landingFromCall": "From the call · {won} won so far",
  "forecast.landingCaveat": "Landing has a caveat",
  "forecast.landing.caveat.call_absent":
    "No call recorded for this period, so this shows commit evidence.",
  "forecast.landing.caveat.call_below_actual":
    "The call is below the amount already won. It is shown as recorded, not corrected.",
  "forecast.pipelineNeeded": "Deal value needed",
  "forecast.pipelineNeededDetail": "{open} open · to reach {landing}",
  "forecast.pipelineBasisWhy.manager_call":
    "Measured against the call for this period.",
  "forecast.pipelineBasisWhy.historical_median":
    "Measured against the median of the last 4 comparable periods.",
  "forecast.pipelineAbsentTitle": "No coverage figure for this period",
  "forecast.pipelineAbsent.insufficient_basis":
    "No call is recorded for this period, and fewer than 4 comparable periods have finished. Measuring against these open deals themselves would always look sufficient.",
  "forecast.pipelineAbsent.insufficient_history":
    "Too few closed deals for a conversion rate. A rate from a handful of deals is unreliable.",
  "forecast.coverage": "{percent}% of required deal value",
  "installationSettings.baseCurrency": "Base currency",
  "installationSettings.baseCurrencyHint":
    "ISO 4217 code that all amounts convert to for roll-ups. Can be changed until the first amount is converted.",
  "installationSettings.baseCurrencyLocked":
    "Locked: amounts are already converted to this currency, so changing it would alter every roll-up.",
  "installationSettings.baseLanguage": "Base language",
  "installationSettings.baseLanguageHint":
    "Language AI writes in when the whole team reads the text. Display language is separate, and customer replies follow the language of the thread.",
  "installationSettings.saveFailed": "Settings not saved",
  "installationSettings.readOnly":
    "Only an administrator or operations user can change these settings.",
  "installationSettings.edit": "Edit",
  "installationSettings.editField": "Edit {field}",
  "installationSettings.save": "Save",
  "signInMethods.title": "Sign-in methods",
  "signInMethods.saveFailed": "Change not saved",
  "signInMethods.sub":
    "How people can sign in. The list shows the providers this installation has credentials for: an administrator can turn one off but cannot add one.",
  "signInMethods.password": "Email and password",
  "signInMethods.passwordAlways":
    "Always available. Every user can sign in this way, so the other methods can be turned off safely.",
  "signInMethods.passwordReason":
    "Password sign-in cannot be turned off. It keeps the installation accessible.",
  "signInMethods.providerHint":
    "Shows this provider on the sign-in screen. Turning it off stops sign-ins in progress; existing sessions continue.",
  "signInMethods.noneConfigured":
    "This installation has no external provider configured. Password is the only sign-in method.",
  "oauthApp.google.title": "Google app",
  "oauthApp.google.sub":
    "Mailboxes are connected, and people sign in with Google, through a Google OAuth app your company owns, using its own credentials.",
  "oauthApp.google.absent":
    "No app is available from any source. Gmail and Calendar cannot be connected, and Google sign-in cannot be offered.",
  "oauthApp.google.redirectSub":
    "Register every URI below on the OAuth client in the Google console. A missing URI fails at the consent screen with redirect_uri_mismatch, which does not name the URI.",
  "oauthApp.google.clientIdPlaceholder":
    "000000000000-xxxx.apps.googleusercontent.com",
  "oauthApp.google.removeConfirmTitle": "Remove the Google app?",
  "oauthApp.google.removeConfirmBody":
    "The client secret cannot be read back, so restoring the app requires re-entering the client ID and secret from the Google console. Gmail and Calendar connections use this app; Microsoft and IMAP mailboxes are unaffected. First-run setup asks for an app again.",
  "oauthApp.microsoft.title": "Microsoft app",
  "oauthApp.microsoft.sub":
    "Outlook mailboxes and calendars are connected, and people sign in with Microsoft, through an Entra app registration your company owns, using its own credentials.",
  "oauthApp.microsoft.absent":
    "No app is available from any source. Outlook mail and calendar cannot be connected, and Microsoft sign-in cannot be offered.",
  "oauthApp.microsoft.redirectSub":
    "Register every URI below under Authentication on the Entra app registration as a Web platform. A missing URI fails at consent with AADSTS50011, which does not name the URI.",
  "oauthApp.microsoft.clientIdPlaceholder":
    "00000000-0000-0000-0000-000000000000",
  "oauthApp.microsoft.removeConfirmTitle": "Remove the Microsoft app?",
  "oauthApp.microsoft.removeConfirmBody":
    "The client secret cannot be read back, so restoring the app requires re-entering the client ID and secret from the Entra portal. Outlook mail and calendar connections use this app; Google and IMAP mailboxes are unaffected. First-run setup asks for an app again.",
  "oauthApp.configured": "In use: {clientId}",
  "oauthApp.fromEnvironment":
    "In use from this installation’s configuration: {clientId}. An app stored here overrides it until removed.",
  "oauthApp.pinnedToDirectory": "Pinned to directory {tenant}.",
  "oauthApp.replaceHint":
    "A new pair replaces the stored one. Existing connections keep working until reconnected.",
  "oauthApp.store": "Store app",
  "oauthApp.replace": "Replace app",
  "oauthApp.writeFailed": "Change not saved",
  "oauthApp.remove": "Remove app",
  "oauthApp.redirectCopied": "Copied",
  "oauthApp.redirectCopyFailed": "Select the address and copy it manually.",
  "oauthApp.redirectCopy": "Copy {purpose} URI",
  "oauthApp.redirect.mailbox_connect": "Mailbox",
  "oauthApp.redirect.calendar_connect": "Calendar",
  "oauthApp.redirect.sign_in": "Sign-in",
  "oauthApp.redirectTitle": "Authorized redirect URIs",
  "oauthApp.clientId": "Client ID",
  "oauthApp.clientSecret": "Client secret",
  "oauthApp.tenant": "Directory (tenant) ID",
  "oauthApp.tenantHint":
    "Optional. Pins the app to one Entra directory: only its members can connect a mailbox, and Microsoft sign-in uses it. Leave empty to let any company connect; sign-in then waits until the server names your directories.",
  "oauthApp.tenantPlaceholder": "00000000-0000-0000-0000-000000000000",
  "oauthApp.saveFailed": "App not saved",
  "firstRun.continue": "Continue",
  "firstRun.ai.title": "Choose a model provider",
  "firstRun.ai.sub":
    "Margince uses your AI provider account. All of this can be changed later in Settings under AI.",
  "firstRun.ai.provider": "Provider",
  "firstRun.ai.key": "API key",
  "firstRun.ai.keyHint": "Stored in the key vault and never shown again.",
  "firstRun.ai.chatModel": "Model",
  "firstRun.ai.modelHint":
    "A starting point. Any model the provider serves works.",
  "firstRun.ai.embedModel": "Embedding model",
  "firstRun.ai.keyFailed": "Key not stored",
  "firstRun.ai.bindFailed": "Provider not bound",
  // Which vendor this installation's text is sent to. Admin/ops only, on both
  // verbs â see the ai_routing RBAC object.
  "aiSettings.withheld": "Restricted",
  "aiSettings.unread": "Unavailable",
  "aiSettings.pending": "Loading…",
  "aiSettings.spend.label": "Tokens · this month",
  "aiSettings.spend.value": "{spent} of {budget}",
  "aiSettings.spend.estimated": "≈ {amount} spent",
  "aiSettings.spend.notPriced": "Not priced",
  "aiSettings.providers.label": "Provider keys",
  "aiSettings.providers.value": "{count} of {total}",
  "aiSettings.providers.missing": "{count} bound, no key",
  "aiSettings.providers.lastCall": "last call {elapsed}",
  "aiSettings.providers.lastCallOnly": "Last call {elapsed}",
  "aiSettings.providers.neverCalled": "Never called",
  "aiSettings.providers.traceFailed": "Call trace unavailable",
  "elapsed.justNow": "just now",
  "elapsed.minutes": "{minutes} min ago",
  "elapsed.hours": "{hours} h ago",
  "elapsed.days": "{days} d ago",
  "aiRouting.lane.local_small":
    "Lowest routing tier; the binding determines processing location",
  "aiRouting.lane.cheap_cloud": "Everyday work: enrichment, summaries, triage",
  "aiRouting.lane.premium": "Text a customer reads",
  "aiRouting.lane.frontier":
    "Advanced reasoning tier; availability does not mean it is used",
  "aiRouting.lane.local_large":
    "Higher local-tier route; inspect the configured endpoint",
  "aiRouting.lane.embeddings": "Search and retrieval across records",
  "aiRouting.lanes.title": "Routing tiers",
  "aiRouting.priceSheet": "Price sheet",
  "aiRouting.provider.label": "Provider",
  "aiRouting.change": "Change",
  "aiRouting.done": "Done",
  "aiRouting.noKey": "No key",
  "aiRouting.unpriced": "Unpriced",
  "aiRouting.effect":
    "Saved bindings reach every process within a minute, without a restart.",
  "aiProviderKeys.title": "Model provider keys",
  "aiProviderKeys.keyless": "No key needed",
  "aiProviderKeys.field": "API key",
  "aiProviderKeys.save": "Save key",
  "aiProviderKeys.adminOnly":
    "Only an administrator or operations user can change a provider key.",
  "aiProviderKeys.saveFailed": "Provider not updated",
  "aiProviderKeys.configured": "Configured",
  "aiProviderKeys.absent": "Not set",
  "aiProviderKeys.configuredHint":
    "Stored in the key vault and cannot be read back. Paste a new key to replace it. It can also be supplied as {envVar}.",
  "aiProviderKeys.absentHint":
    "This provider has no key, so models bound to it cannot be called. It can also be supplied as {envVar}.",
  "aiProviderKeys.addPlaceholder": "Paste API key",
  "aiProviderKeys.replacePlaceholder": "Paste new key",
  "aiProviderKeys.add": "Add",
  "aiProviderKeys.replace": "Replace",
  "aiProviderKeys.removeConfirmTitle": "Remove {provider} key?",
  "aiProviderKeys.removeConfirmBody":
    "The key is deleted from the key vault and cannot be recovered; no readable copy exists. Every AI tier bound to this provider stops until a new key is added.",
  "aiProviderKeys.withheld":
    "Only an administrator or operations user who can change model bindings can see which providers have a key.",
  "aiProviderKeys.remove": "Remove",
  "aiRouting.withheld":
    "Only an administrator or operations user who can change model bindings can see which models this installation uses.",
  "aiRouting.title": "Model routing",
  "aiRouting.sheetAsOf":
    "Model lists come from the price sheet as of {date}. Any newer model ID the provider serves also works; type it.",
  "aiRouting.sheetUnknown":
    "Model lists come from the price sheet, which your role cannot read. Any model ID the provider serves works; type it.",
  "aiRouting.unboundTitle": "No models bound",
  "aiRouting.unboundUnkeyed":
    "No models are bound, so AI features are off. Add a provider key below, then bind the tiers here. An installation can also set its first binding under seeds.ai_routing in margince.yaml, read once when the company is created.",
  "aiRouting.unboundKeyed":
    "No models are bound, so AI features are off. Start from a provider’s defaults, adjust as needed and save.",
  "aiRouting.unboundStart": "Start from {provider}",
  "aiRouting.profile.card": "Installation profile",
  "aiRouting.profile.label": "Location",
  "aiRouting.profile.help":
    "Where inference runs. Sovereign means no egress: only models on your own hosts; others are refused when saving.",
  "aiRouting.profile.eu_hosted": "EU-hosted",
  "aiRouting.profile.sovereign": "Sovereign (no egress)",
  "aiRouting.profile.cloud_frontier": "Cloud frontier",
  "aiRouting.dimensions.label": "Vector width",
  "aiRouting.dimensions.help":
    "Leave blank for the provider default. Values outside 1 to 2000 are refused.",
  "aiRouting.baseUrl.placeholder": "https://openrouter.ai/api",
  "aiRouting.baseUrl.label": "Host",
  "aiRouting.baseUrl.help":
    "Provider host root without a version segment; /v1 is added. Required for openai_compatible, which has no default.",
  "aiRouting.models.noKey":
    "Price sheet only: this provider has no key, so its model list cannot be requested. Any model ID it serves still works; type it.",
  "aiRouting.models.noEndpoint":
    "Price sheet only: enter the host above to request this provider’s model list. Any model ID it serves still works; type it.",
  "aiRouting.models.profileForbids":
    "Price sheet only: this installation profile does not allow access to this provider.",
  "aiRouting.models.notPublished":
    "Price sheet only: this provider publishes no model list.",
  "aiRouting.models.unreachable":
    "Price sheet only: this provider did not respond. Any model ID it serves still works; type it.",
  "aiRouting.model.label": "Model",
  "aiRouting.model.help":
    "Listed models are the ones this installation can price, per 1M tokens in → out. Any other model ID the provider serves also works; type it.",
  "aiRouting.save": "Save routing",
  "aiRouting.saving": "Saving binding…",
  "aiRouting.savedTitle": "Routing saved",
  "aiRouting.saved": "All processes now use it.",
  "aiRouting.saveFailed": "Routing not saved",
  "aiRouting.adminOnly":
    "Changing model routing requires routing-update and allowance-read permission.",
  "workingHours.title": "Bookable hours",
  "workingHours.sub": "Personal setting. Only you set your hours.",
  "workingHours.unsetTitle": "Not set yet",
  "workingHours.unset":
    "Customers are offered 09:00 to 17:00, Monday to Friday, in the installation timezone.",
  "workingHours.start": "Day starts",
  "workingHours.end": "Day ends",
  "workingHours.days": "Working days",
  "workingHours.timezone": "Timezone",
  "workingHours.timezoneHelp":
    "Timezone for the start and end times. Prefilled from this browser.",
  "workingHours.narrowedTitle": "Fewer bookable hours",
  "workingHours.narrowed": "Customers have fewer times to choose from.",
  "workingHours.saveFailed": "Working hours not saved",
  "workingHours.save": "Save working hours",
  "workingHours.day.1": "Monday",
  "workingHours.day.2": "Tuesday",
  "workingHours.day.3": "Wednesday",
  "workingHours.day.4": "Thursday",
  "workingHours.day.5": "Friday",
  "workingHours.day.6": "Saturday",
  "workingHours.day.7": "Sunday",
  "autonomy.title": "Automatic changes",
  "autonomy.sub":
    "Automatic changes start on. Existing settings are kept. Each switch applies to your work, not the whole team.",
  "autonomy.noneDecidedYetTitle": "No reviews yet",
  "autonomy.updateFailed": "Setting not saved",
  "autonomy.noneDecidedYet":
    "You have not reviewed any of these proposals yet. Automatic changes can already run according to the switches below. Proposals depend on your records and the work routed to you.",
  "autonomy.noRecord": "No decisions of this kind yet.",
  "autonomy.record":
    "So far: {clean} approved as proposed, {edited} approved after edits, {rejected} rejected.",
  "autonomy.kind.close_date_correction.label": "Close dates",
  "autonomy.kind.close_date_correction.help":
    "Maintains deals overnight: estimates missing or overdue close dates from deal pace and reviews deals that have gone quiet. Turn off to stop this maintenance.",
  "autonomy.kind.company_name_promotion.label": "Company names",
  "autonomy.kind.company_name_promotion.help":
    "Accepts proposed company names from email signatures for companies named after their domain. Turn off to review these proposals yourself. Names confirmed by independent sources can still be updated automatically.",
  "autonomy.kind.lifecycle_change.label": "Lifecycle stages",
  "autonomy.kind.lifecycle_change.help":
    "Moves a company to another lifecycle stage based on activity. Turn off to review these proposals yourself. Stage changes can affect who sees the company and which automations run.",
  "captureSettings.title": "Enrichment",
  "captureSettings.sub":
    "How captured companies and contacts are enriched after they are created.",
  "captureSettings.autoEnrich.label": "Auto-enrich captured companies",
  "captureSettings.autoEnrich.help":
    "Each new company created from captured mail gets an automatic web profile built from its site. Runs within a daily limit.",
  "captureSettings.signatureEnrich.label": "Read contact details from mail",
  "captureSettings.signatureEnrich.help":
    "When on, Margince reads what a person states under their own name in mail they sent you: a signature, or a business card attached to it, with title, phone number, address or company. This happens within minutes of arrival. Nothing is inferred; a detail the mail does not state is not written. This is the company default; a mailbox with its own setting keeps it.",
  "captureSettings.removeFailed": "Exclusion not removed",
  "captureSettings.addFailed": "Exclusion not added",
  "captureSettings.adminOnly":
    "Only an administrator or operations user can change this.",
  "captureSettings.updateFailed": "Setting not changed",

  "ownDomains.companyTitle": "Company domains",
  "captureExclusions.title": "Capture exclusions",
  "captureExclusions.sub":
    "Addresses and domains whose messages never enter the CRM. Your rules apply only to mailboxes you connected; company rules apply to everyone.",
  "captureExclusions.notRetroactive":
    "Applies from the next message. Messages already captured stay.",
  "captureExclusions.current": "Rules in effect",
  "captureExclusions.empty": "No exclusions.",
  "ownerIdentities.title": "Your other addresses",
  "ownerIdentities.sub":
    "Addresses that are also yours: a send-as alias, a private domain you read, an address you forward from. Mail between them is not captured and never creates a contact.",
  "ownerIdentities.add": "Add address",
  "ownerIdentities.addLabel": "Add your own address",
  "ownerIdentities.addDescription":
    "Private to you. Colleagues never see this list.",
  "ownerIdentities.current": "Declared",
  "ownerIdentities.notRetroactive":
    "Applies from the next message. Mail already captured stays, and a contact already created from an alias stays until you merge or remove it.",
  "ownerIdentities.empty": "No other addresses added.",
  "ownerIdentities.remove": "Remove address",
  "ownerIdentities.added": "Address added",
  "ownerIdentities.confirm": "Add",
  "ownerIdentities.kindLabel": "Type",
  "ownerIdentities.kind.address": "One address",
  "ownerIdentities.learned.deliveredTo":
    "Found automatically: mail to this address arrives in your connected mailbox. Remove it if it is not yours.",
  "ownerIdentities.learned.provider":
    "Your mail provider reports this as one of your addresses. Remove it if it is not yours.",
  "ownerIdentities.kind.domain": "A whole domain",
  "ownerIdentities.valueLabel": "Address or domain",
  "ownerIdentities.addressPlaceholder": "you@example.com",
  "ownerIdentities.removeFailed": "Address not removed",
  "ownerIdentities.addFailed": "Address not added",
  "ownerIdentities.domainPlaceholder": "example.com",
  "captureExclusions.scope.user": "Your mailboxes",
  "captureExclusions.scope.workspace": "Whole company",
  "captureExclusions.kind.address": "Address",
  "captureExclusions.kind.domain": "Domain",
  "captureExclusions.kind.container": "Label, folder or mailbox",
  "captureExclusions.scopeLabel": "Applies to",
  "captureExclusions.kindLabel": "Kind",
  "captureExclusions.addLabel": "Exclude address or domain",
  "captureExclusions.placeholder.address": "name@example.com",
  "captureExclusions.placeholder.domain": "example.com",
  "captureExclusions.add": "Exclude",
  "captureExclusions.addOpen": "New exclusion",
  "captureExclusions.remove": "Resume capture of {value}",
  "ownDomains.title": "Own email domains",
  "ownDomains.sub":
    "Domains that belong to this company. Messages between colleagues are not stored for anyone, including you.",
  "ownDomains.curatedTitle": "Managed here",
  "ownDomains.irreversible":
    "Adding a domain applies from the next message; removing it resumes capture from then on. Mail skipped while registered is never offered again by any mailbox. Captured mail stays.",
  "ownDomains.fromCompany": "From the company profile. Change them there:",
  "ownDomains.openCompany": "Open the company profile",
  "ownDomains.empty":
    "No additional domains registered. Add one if your company also sends from another domain.",
  "ownDomains.confirmed": "Confirmed",
  "ownDomains.candidate": "Seen on a connected mailbox, not confirmed",
  "ownDomains.add": "Add",
  "ownDomains.addOpen": "Add domain",
  "ownDomains.addLabel": "Add own domain",
  "ownDomains.placeholder": "example.com",
  "ownDomains.removeFailed": "Domain not removed",
  "ownDomains.addFailed": "Domain not added",
  "ownDomains.remove": "Remove {domain}",

  "webhooks.title": "Webhooks",
  "webhooks.readOnly":
    "Read-only: only an administrator or operations user can change subscriptions.",
  "webhooks.sub":
    "Outbound subscriptions that receive signed HTTP POSTs for chosen events.",
  "webhooks.new": "New subscription",
  "webhooks.notConfigured":
    "Outbound webhooks are not enabled on this installation. Configure a signing key first.",
  "webhooks.state.active": "Active",
  "webhooks.state.paused": "Paused",
  "webhooks.updated": "Updated {date}",
  "webhooks.field.targetUrl": "Target URL",
  "webhooks.field.eventTypes": "Event types",
  "webhooks.field.state": "State",
  "webhooks.edit": "Edit",
  "webhooks.saveDone": "Webhook saved",
  "webhooks.archiveDone": "Webhook archived",
  "webhooks.archive": "Archive",
  "webhooks.archiveConfirm":
    "Archiving stops all delivery for this subscription. This cannot be undone.",
  "webhooks.rotate": "Rotate secret",
  "webhooks.rotateConfirm.title": "Rotate signing secret?",
  "webhooks.rotateConfirm.body":
    "The current secret is invalidated immediately and the new secret is shown once. Copy it and update your receiver right away.",
  "webhooks.secret.title": "Signing secret",
  "webhooks.secret.warning":
    "This secret is shown once and cannot be retrieved. Store it now; deliveries are signed with it.",
  "webhooks.secret.copy": "Copy",
  "webhooks.secret.copied": "Copied",
  "webhooks.secret.copyFailed": "Select the secret above and copy it manually.",
  "webhooks.secret.done": "Done",
  "webhooks.secret.leaveWarning":
    "Leaving destroys the only copy of this secret. Copy it first.",

  "webhooks.deliveries.show": "View deliveries",
  "webhooks.deliveries.hide": "Hide deliveries",
  "webhooks.deliveries.empty": "No delivery attempts yet.",
  "webhooks.deliveries.title": "Delivery attempts",
  "webhooks.deliveries.deadLetterGroup": "Dead-lettered ({count})",
  "webhooks.deliveries.allGroup": "Other attempts",
  "webhooks.deliveries.column.status": "Status",
  "webhooks.deliveries.column.event": "Event",
  "webhooks.deliveries.column.attempts": "Attempts",
  "webhooks.deliveries.column.lastStatusCode": "Last status",
  "webhooks.deliveries.column.lastError": "Last error",
  "webhooks.deliveries.column.created": "Created",
  "webhooks.deliveries.column.resolved": "Resolved or next retry",
  "webhooks.deliveries.status.pending": "Pending",
  "webhooks.deliveries.status.delivered": "Delivered",
  "webhooks.deliveries.status.retrying": "Retrying",
  "webhooks.deliveries.status.dead_lettered": "Dead-lettered",
  "webhooks.deliveries.status.visibility_revoked": "Stopped: no longer visible",
  "webhooks.deliveries.replay": "Replay",
  "webhooks.deliveries.replayConfirm.title": "Replay this delivery?",
  "webhooks.deliveries.replayConfirm.body":
    "Retries delivery now with the current secret and a new timestamp, without waiting for the next scheduled retry.",
  "reindexbanner.needed": "Reindex needed",
  "reindexbanner.link": "Review in settings",

  "embedreindex.title": "Search index",
  "embedreindex.sub":
    "Reindex status of the search index. Visible to administrators and operations users only.",
  "embedreindex.withheld":
    "Only an administrator or operations user can see the search index. Rebuilding it spends tokens for the whole installation.",
  "embedreindex.unbound":
    "No embedding model is bound, so there is no search index to rebuild.",
  "embedreindex.unboundLink": "Open AI settings",
  "embedreindex.statusLabel": "Index status",
  "embedreindex.reindexLabel": "Reindex changes",
  "embedreindex.reindexHelp":
    "Re-embeds only records whose text changed since the last run.",
  "embedreindex.rebuildLabel": "Rebuild entire index",
  "embedreindex.rebuildHelp":
    "Re-embeds every record. Use it when a run is stuck or the embedding model changed.",
  "embedreindex.statusIdle": "Up to date",
  "embedreindex.statusNeeded": "Reindex needed",
  "embedreindex.statusReembedding": "Reindexing…",
  "embedreindex.lastProgress": "Last progress {duration} ago",
  "embedreindex.entitiesPending": "{count} records pending",
  "embedreindex.workspacePending": "{count} pending",
  "embedreindex.reviewCta": "Review and reindex",
  "embedreindex.rebuildCta": "Rebuild index",
  "embedreindex.confirmTitle": "Start reindex?",
  "embedreindex.rebuildTitle": "Rebuild search index?",
  "embedreindex.confirmCta": "Start reindex",
  "embedreindex.rebuildConfirmCta": "Rebuild now",
  "embedreindex.previewLoading": "Estimating scope…",
  "embedreindex.previewFailed": "Estimate unavailable",
  "embedreindex.estimateEntities": "Records to embed:",
  "embedreindex.estimateTokens": "Estimated AI tokens:",
  "embedreindex.estimateCost": "Estimated cost:",
  "embedreindex.estimateQualityHeuristic":
    "Heuristic estimate: a minimum baseline, not observed spend.",
  "embedreindex.utilizationTitle": "Budget impact",
  "embedreindex.impact.normal": "Normal",
  "embedreindex.impact.degraded": "Would enter economy mode",
  "embedreindex.impact.queued": "Would be queued",

  "consent.title": "Authorize access",
  "consent.asks":
    "{client} will be able to act in Margince as you, with the access checked below.",
  "consent.redirectsTo": "Margince will send the authorization back to {host}.",
  "consent.redirectsToLoopback":
    "That is an address on this computer, and this connection cannot prove which program is listening on it.",
  "consent.scopeNote.read": "sees what you can see",
  "consent.scopeNote.draft": "prepares messages for your review",
  "consent.scopeNote.write": "creates, edits and archives records as you",
  "consent.scopeNote.send": "sends messages as you, without asking first",
  "consent.scopeNote.enrich":
    "spends enrichment credits; each purchase still asks you first",
  "consent.ceiling":
    "Never more than your own permissions. You can disconnect at any time in Settings under Agents.",
  "consent.pickOne": "Pick at least one, or deny.",
  "consent.offline":
    "It stays connected and renews access without asking again until you revoke it.",
  "consent.approve": "Authorize",
  "consent.deny": "Deny access",
  "consent.reentering": "Reconnecting…",
  "consent.backToApp": "Back to Margince",
  "consent.staleTitle": "This request has expired",
  // No {client}: this card renders without the consent-request fetch, so the
  // client's name is not available to name here.
  "consent.staleBody":
    "The connection request is no longer valid. Return to the app you were connecting and start again; reloading this page does not help.",
  "consent.invalidTitle": "This connection request could not be completed",
  "consent.invalidBody":
    "This installation will not authorize the request as it stands; the app may no longer be registered here. Return to the app you were connecting and start again.",
  "contact.thin.title": "Known details",
  "contact.thin.known":
    "On file for {name}: {what}. No one in the company has a recorded exchange with them yet.",
  "contact.thin.remediation.capture":
    "Connect a mailbox that writes to this contact to fill in this page, with the source of each field.",
  "contact.thin.remediation.employer":
    "Add their employer, and Margince can read that company’s website for their role.",
  "contact.thin.logFirst": "Log first interaction",
  "contact.enriched.title": "Enriched details",
  "contact.confirm.title_one": "{count} detail to confirm",
  "contact.confirm.title_other": "{count} details to confirm",
  "contact.confirm.body": "{fields} were read from their emails on {when}.",
  "contact.confirm.review": "Review",
  "contact.enriched.sub":
    "Each value with the text it was read from. A corrected value is kept.",
  "contact.enriched.field.title": "Title",
  "contact.enriched.field.phone": "Phone",
  "contact.enriched.field.role": "Role",
  "contact.enriched.field.linkedin": "LinkedIn",
  "contact.enriched.field.company_name": "Company",
  "contact.enriched.field.address": "Address",
  "contact.enriched.field.website": "Website",
  "contact.enriched.readFrom": "Read from {source} on {when}",
  "contact.enriched.undo": "Undo",
  "contact.enriched.replaced": "Replaced the older value “{was}”.",
  "contact.enriched.correctedByYou": "Corrected by you",
  "contact.enriched.confirmed": "Confirmed",
  "contact.enriched.confirm": "Confirm value",
  "contact.enriched.save": "Save correction",
  "contact.enriched.cancel": "Cancel",
  "contact.graph.loading": "Loading contact network…",
  "contact.graph.routeDirect": "{name} already corresponds with them.",
  "contact.graph.routeVia":
    "{name} corresponds with {through} at the same company.",
  // The same two sentences when the colleague is the reader. A route to the
  // contact reading it is the strongest one this page can find, and printing
  // their own name back at them read as a third party they would have to go
  // and ask.
  "contact.graph.routeDirectYou": "You already correspond with them.",
  "contact.graph.routeViaYou":
    "You correspond with {through} at the same company.",
  "contact.graph.noRoute":
    "No one in the company corresponds with this contact or their company yet.",
  "contact.graph.noDirect":
    "No one in the company has corresponded with this contact.",
  // Names the column beside the graph, for a reader who lands in it from the
  // landmark list rather than by scrolling to it.
  "contact.graph.sideColumn": "Introductions and changes",
  "contact.graph.recordWorksWith": "Record: works with {name}",
  "contact.graph.noEdge": "No recorded correspondence with {name}.",
  "contact.graph.withColleague": "with {name}",
  "contact.graph.withContact": "with this contact",
  "contact.graph.counts":
    "{total} interactions in 90 days · {inbound} in, {outbound} out",
  "contact.graph.untitledMessage": "Untitled",
  "contact.graph.countsOnly": "Counts only. The messages stay on the timeline.",
  "contact.intro.routesTitle": "Routes",
  "contact.graph.droppedNote": "{count} more not shown.",
  "contact.graph.withheldDirect": "Some colleagues are not shown.",
  "contact.graph.withheldAccount":
    "Some contacts at this company are not shown.",
  "contact.intro.askFirstName": "Ask {name} to introduce you",
  "contact.intro.leadRouteBadge": "Strong route",
  "contact.intro.heroDirect": "knows them directly",
  "contact.intro.heroIndirect": "reaches them through {through}",
  "contact.intro.heroYou": "You",
  "contact.intro.heroDirectYou": "know them directly",
  "contact.intro.heroIndirectYou": "reach them through {through}",
  "contact.intro.factReciprocal": "Reciprocal",
  "contact.intro.factOneSided": "One-sided",
  "contact.intro.factDirect": "Direct relationship",
  "contact.intro.factIndirect": "Through a colleague",
  "contact.intro.factReceipts": "{count} visible receipts",
  "contact.intro.verdictDirect":
    "Ask {name}. They already correspond with this contact.",
  "contact.intro.verdictOneSided":
    "Ask {name}. They wrote to this contact with no reply yet.",
  "contact.intro.verdictVia":
    "Ask {name}. They reach this contact through {through}.",
  "contact.intro.verdictDirectYou": "You already correspond with this contact.",
  "contact.intro.verdictOneSidedYou":
    "You wrote to this contact with no reply yet.",
  "contact.intro.verdictViaYou": "You reach them through {through}.",
  // What stands where the ask would be on the reader's own route. The panel
  // is the page's one recommendation, so it says what to do rather than
  // leaving the strongest way in with no move under it.
  "contact.intro.ownRouteNoAsk":
    "No one to ask. Write to this contact directly.",
  "contact.intro.evidenceEyebrow": "Evidence",
  "contact.intro.evidenceExchanges": "Exchanges",
  "contact.intro.evidenceWindow": "in 90 days",
  "contact.intro.evidenceFrom": "{count} from {name}",
  "contact.intro.evidenceFromYou": "{count} from you",
  "contact.intro.evidenceLastContact": "Last contact",
  "contact.intro.lastToday": "Today",
  "contact.intro.lastYesterday": "Yesterday",
  "contact.intro.lastDays": "{days} days ago",
  "contact.intro.lastNever": "None in 90 days",
  "contact.intro.stripWho": "Routes",
  "contact.intro.stripWhoMix": "{direct} direct · {indirect} via a contact",
  "contact.intro.stripWhoOwn": "Includes your own relationship",
  "contact.intro.otherRoutesTitle": "Other routes",
  "contact.intro.otherRoutesSub": "Ranked by two-way correspondence.",
  "contact.intro.relayDue": "due {date}",
  "contact.intro.stripDirect": "Direct relationship",
  "contact.intro.stripNoPath": "No one corresponds with this contact",
  "contact.intro.stripNoRoutes": "None",
  "contact.intro.stripWhyNow": "Latest change",
  "contact.intro.stripNoMoment": "Nothing new",
  "contact.intro.change.replied": "Replied",
  "contact.intro.change.repliedSub": "After {days} d quiet",
  "contact.intro.change.quiet": "Quiet",
  "contact.intro.change.quietSub": "{days} d",
  "contact.intro.change.warmed": "Warmer",
  "contact.intro.change.cooled": "Cooler",
  "contact.intro.change.buckets": "{from} → {to}",
  "contact.intro.stripHandoff": "Intro request",
  "contact.intro.handoffNotStarted": "Not started",
  "contact.intro.handoffOwner": "Next: {name}",
  "contact.intro.ownerColleague": "a colleague",
  "contact.intro.ownerNobody": "no one",
  "contact.intro.relayTitle": "Introduction status",
  "contact.intro.stepRoute": "Choose route",
  "contact.intro.stepRoutePick": "select who to ask",
  "contact.intro.stepRequest": "Request",
  "contact.intro.stepNotSent": "not sent",
  "contact.intro.stepAwaitingAnswer": "awaiting colleague",
  "contact.intro.stepIntroduction": "Introduction",
  "contact.intro.stepNameDrop": "Name used",
  "contact.intro.stepWaiting": "waiting",
  "contact.intro.stepRecorded": "recorded",
  "contact.intro.stepReply": "Reply",
  "contact.intro.stepObserved": "observed from activity",
  "contact.intro.stepDone": "Done",
  "contact.intro.stepCurrent": "Now",
  "contact.intro.stepPending": "Later",
  "contact.intro.laneOurs": "Your team",
  "contact.intro.laneTheirs": "Their company",
  "contact.intro.lanePeers": "Their contacts",
  "contact.intro.laneTarget": "Target",
  "contact.intro.useThisRoute": "Use this route",
  "contact.intro.mapRegion": "Routes to this contact",
  "contact.intro.edgeDirect": "{name} corresponds with them directly",
  "contact.intro.edgeAccount": "works with {name}",
  "contact.intro.routesSub":
    "Best route first. Alternatives apply when the first is unavailable.",
  "contact.intro.best": "Best",
  "contact.intro.evidenceTwoWay_one":
    "{total} two-way exchange in 90 days · {when}",
  "contact.intro.evidenceTwoWay_other":
    "{total} two-way exchanges in 90 days · {when}",
  "contact.intro.evidenceOneSided_one":
    "{total} interaction in 90 days, one-sided · {when}",
  "contact.intro.evidenceOneSided_other":
    "{total} interactions in 90 days, one-sided · {when}",
  "contact.intro.whenToday": "last contact today",
  "contact.intro.whenYesterday": "last contact yesterday",
  "contact.intro.whenDays": "last contact {days} days ago",
  "contact.intro.whenNever": "no recent contact",
  "contact.intro.askTitle": "Ask for an introduction to {name}",
  "contact.intro.cancel": "Cancel",
  "contact.intro.askAction": "Request introduction",
  "contact.intro.askFailed": "The request was not recorded. Retry.",
  "contact.intro.reasonLabel": "Reason for request",
  "contact.intro.reasonHint":
    "Your colleague reads this, not the contact. State why the introduction is worth making.",
  "contact.intro.valueLabel": "Value for the contact",
  "contact.intro.valueHint": "Why the contact would want this introduction.",
  "contact.intro.noteLabel": "Forwardable note",
  "contact.intro.noteHint":
    "The only part the contact reads. Write it so it can be pasted as is.",
  "contact.intro.nameDropAsk": "Ask permission to mention their name",
  "contact.intro.fallbackLegend": "If they say no",
  "contact.intro.fallbackNone": "Nothing further",
  "contact.intro.fallbackNoneHelp":
    "The request closes and you decide the next step.",
  "contact.intro.fallbackNameDrop": "Ask to use their name instead",
  "contact.intro.fallbackNameDropHelp":
    "You contact them yourself and mention your colleague.",
  "contact.intro.fallbackNextRoute": "Try the next route",
  "contact.intro.fallbackNextRouteHelp":
    "Move to the next colleague on the list.",
  "contact.intro.decideTitle": "Introduction to {name}",
  "contact.intro.decideLegend": "Your answer",
  "contact.intro.decideAction": "Record answer",
  "contact.intro.decideFailed": "The answer was not recorded. Retry.",
  "contact.intro.decideReasonLabel": "Comment",
  "contact.intro.decideReasonHint": "Your colleague sees this as written.",
  "contact.intro.noteByModel": "Drafted by Margince",
  "contact.intro.nameDropRequested":
    "They also asked whether they may mention your name.",
  "contact.intro.answerAccept": "Make introduction",
  "contact.intro.answerAcceptHelp": "You make the introduction yourself.",
  "contact.intro.answerNameDrop": "Let them mention you",
  "contact.intro.answerNameDropHelp":
    "Your colleague contacts this contact directly and mentions you. This is not recorded as an introduction.",
  "contact.intro.answerSuggest": "Suggest someone else",
  "contact.intro.answerSuggestHelp":
    "Name the colleague better placed to help.",
  "contact.intro.answerDecline": "Decline",
  "contact.intro.answerDeclineHelp":
    "The request closes. Add a reason if useful.",
  "contact.intro.asksTitle": "Introductions",
  "contact.intro.answerAction": "Answer",
  "contact.intro.completeIntroducedAction": "Mark introduced",
  "contact.intro.completeNameDroppedAction": "Mark name used",
  "contact.intro.completeFailed": "The outcome was not recorded. Retry.",
  "contact.intro.withdrawAction": "Withdraw",
  "contact.intro.withdrawFailed": "The request was not withdrawn. Retry.",
  "contact.intro.stateRequested": "Awaiting reply",
  "contact.intro.stateAccepted": "Intro agreed",
  "contact.intro.stateNameDropApproved": "Name approved",
  "contact.intro.stateSuggestOther": "Other route",
  "contact.intro.handoffOtherSub": "Someone else suggested",
  "contact.intro.stateDeclined": "Declined",
  "contact.intro.stateIntroduced": "Introduced",
  "contact.intro.stateNameDropped": "Name used",
  "contact.intro.stateReplied": "Replied",
  "contact.intro.stateExpired": "Expired",
  "contact.intro.handoffExpiredSub": "No answer in time",
  "contact.intro.stateCancelled": "Withdrawn",
  "contact.intro.alreadyRequested": "Already asked",
  "contact.intro.declined": "Declined before",
  "contact.intro.unavailable": "Not available",
  "contact.network.momentsTitle": "Recent changes",
  "contact.network.momentsSub":
    "Changes in this relationship, from the messages.",
  "contact.network.noMoments": "No recent changes in this relationship.",
  "contact.change.repliedAfterGap": "They replied after {days} quiet days.",
  "contact.change.wentQuiet": "No activity for {days} days.",
  "contact.change.warmed": "The relationship moved from {from} to {to}.",
  "contact.change.cooled": "The relationship cooled from {from} to {to}.",
  "contact.band.none": "no contact",
  "contact.band.weak": "weak",
  "contact.band.moderate": "moderate",
  "contact.band.strong": "strong",
  "contact.bandBadge.none": "No contact",
  "contact.bandBadge.weak": "Weak",
  "contact.bandBadge.moderate": "Moderate",
  "contact.bandBadge.strong": "Strong",
  "contact.pulse.title": "Relationship",
  "contact.pulse.warmestIs":
    "{name} has the strongest relationship with this contact.",
  "contact.pulse.nobodyYet":
    "No one in the company has a recorded exchange with this contact yet.",
  "contact.pulse.lastInbound": "They last wrote",
  "contact.pulse.lastOutbound": "Your team last wrote",
  "contact.pulse.neverInbound": "never",
  "contact.pulse.neverOutbound": "never",
  "contact.pulse.why": "How this is computed",
  "contact.pulse.arithmetic":
    "Score {score}/100 = 100 × recency {recency} × frequency {frequency} × reciprocity {reciprocity}. Computed on load from captured activity and not stored.",
  "contact.identity.title": "Identity",
  "contact.identity.emailDead":
    "Bouncing. Mail to this address is not delivered.",
  "contact.identity.email": "Email",
  "contact.identity.phone": "Phone",
  "contact.identity.currentRole": "Current role",
  "contact.identity.buyingRole": "Buying role",
  "contact.career.title": "Former roles",
  "contact.consent.title": "Outbound consent",
  "contact.consent.allowed": "Allowed: {purposes}",
  "contact.consent.noneGranted":
    "No purpose granted, so outbound messages are blocked.",
  "contact.consent.blocked": "Blocked: {purposes}",
  "contact.network.title": "Colleagues who know this contact",
  "contact.network.twoWay": "{count} two-way exchanges in 90 days",
  "contact.network.oneSided": "{count} interactions in 90 days, one-sided",
  "contact.network.replied": "replied {when}",

  // The contact record page V2 (ADR-0096). The strip, rail and card words are
  // split by SLOT rather than by sentence, because the same word means
  // different things in different slots: "Never" under a direction is an
  // absence of correspondence, "None" under a meeting is an absence of a
  // booking, and German renders them differently.
  "contact.page.loading": "Loading…",
  "contact.page.notOpened": "This contact could not be opened.",
  "contact.page.buyingRole": "Buying role",
  "contact.page.owner": "Owner",
  "contact.page.ownerUnassigned": "Unassigned",
  "contact.page.linkedin": "LinkedIn",

  // The rail's own details grid: the contact's own fields, at a glance above
  // the six relationship sections below it.
  "contact.rail.detailsTitle": "Details",
  "contact.rail.archivedReadOnly":
    "This contact is archived. Restore the contact to make changes.",
  "contact.notYoursToChange":
    "You cannot edit this contact. Ask the owner to share it, or an administrator for edit rights.",
  // Fired when an employment row's version could not be read back before a
  // write — the row is not saved unpinned, so the reader is told to reload
  // rather than left to think the edit landed.
  "contact.rail.employmentVersionUnresolved":
    "This row’s current version could not be loaded. Reload and retry.",
  // The employers section: every employment edge this contact holds, current
  // one first — a contact can work at more than one company at once.
  "contact.rail.employmentTitle": "Companies",
  "contact.rail.noEmployment": "No employment on record.",
  "contact.rail.noPrimaryEmployer": "No primary employer. Select one.",
  "contact.rail.addEmployment": "Add company",
  "contact.employer.contacts_one": "{count} contact",
  "contact.employer.contacts_other": "{count} contacts",
  "contact.employer.openDeals_one": "{count} open deal",
  "contact.employer.openDeals_other": "{count} open deals",
  "contact.employer.noOpenDeals": "no open deals",
  "contact.rail.employer": "Employer",
  "contact.rail.allCompaniesConnected":
    "All matches are already linked to this contact.",
  "contact.rail.isCurrentEmployer": "This is their current employer",
  "contact.rail.markEnded": "Mark as ended",
  "contact.rail.removeEmploymentTitle": "Remove company link?",
  "contact.rail.removeEmploymentBody":
    "The link to {company} and its history are deleted permanently. {company} itself stays. If they left, mark the role ended instead.",
  "contact.timeline.empty": "No activity logged with this contact yet.",
  "contact.deals.empty": "This contact is not on any deal.",
  "contact.deals.untitled": "Untitled deal",
  "contact.deals.noStage": "No stage yet",
  "contact.meetings.upcoming": "Upcoming",
  "contact.meetings.past": "Past meetings",
  "contact.meetings.noneBooked": "No upcoming meetings.",
  "contact.meetings.noneLogged": "No meetings logged.",
  "contact.meetings.untitled": "Untitled meeting",
  "contact.documents.empty": "No files for this contact.",
  "contact.research.empty": "No research on this contact yet.",
  "contact.research.fields": "Enrichment evidence",
  "contact.research.fieldsEmpty": "No enriched fields have evidence yet.",
  "contact.action.email": "Email",
  // The lead verb when the record leaves the transport open: either the
  // composer will ask which way to send, or there is no way to send at all.
  "contact.action.write": "Write",
  // The lead verb when a chat channel is the ONLY way to reach them. The
  // provider is named by the transport directory, so an extension unit this
  // build has never heard of still reads as itself.
  "contact.action.messageOn": "Message on {transport}",
  // Why the lead verb is refused, in two sentences that are never merged: no
  // way to reach them, and consent that says not to.
  "contact.action.noTransport": "No address and no thread to reply to.",
  "contact.action.call": "Call",
  "contact.action.meetings": "Meetings",
  "contact.action.addTask": "Add task",
  "contact.action.research": "Research",

  "contact.strip.never": "Never",
  "contact.strip.today": "Today",
  "contact.strip.yesterday": "Yesterday",
  "contact.strip.days": "{count} days",
  "contact.consent.allowedWord": "Allowed",
  "contact.consent.blockedWord": "Blocked",
  "contact.consent.unknownWord": "Unknown",

  "contact.moment.rule.meeting_prep": "Meeting soon",
  "contact.moment.rule.re_engaged": "Re-engaged",
  "contact.moment.rule.job_change": "Changed jobs",
  "contact.moment.rule.overdue_promise": "Commitment overdue",
  "contact.moment.rule.gone_quiet": "Gone quiet",
  "contact.moment.rule.open_promise": "Commitment due",
  "contact.moment.rule.public_signal": "In the news",
  "contact.moment.rule.missing_next_step": "No next step",
  "contact.moment.rule.thin_relationship": "No interactions recorded",
  "contact.moment.rule.nothing_needed": "Nothing needed",

  "contact.overview.detailsPermissions": "Details and permissions",
  "contact.overview.detailsShow": "Show details and permissions",
  "contact.overview.detailsHide": "Hide details and permissions",
  "contact.overview.partial":
    "Some sections are not available to your role. This summary covers the records you can see.",
  "contact.overview.coverage": "Based on the records you can see.",
  "contact.overview.about": "About this contact",
  "contact.overview.profileOnly":
    "Based on the contact details on file. Add context as you learn more.",
  "contact.overview.briefFailed": "The relationship brief could not be loaded.",

  "contact.brief.title": "Relationship brief",
  "contact.brief.sources": "Sources",
  "contact.brief.updatedAt": "Updated {when}",
  "contact.brief.reading": "Loading relationship brief…",
  "contact.brief.sourceActivity": "Activity",
  "contact.brief.sourceDeal": "Deal notes",

  "contact.matters.title": "What matters to {name}",
  "contact.matters.priorities": "Priorities",
  "contact.matters.objections": "Objections",
  "contact.matters.successCriteria": "Success criteria",
  "contact.matters.absent": "Nothing captured yet",

  "contact.commercial.title": "Open deal and buying role",
  "contact.commercial.withheld":
    "You do not have access to this contact’s deals.",
  "contact.commercial.noDeal": "No open deal.",
  "contact.commercial.closes": "closes {date}",
  "contact.commercial.committee": "Buying committee",
  "contact.commercial.openDeal": "Open deal",

  "contact.loops.title": "Commitments and open questions",
  "contact.loops.empty":
    "No commitments or questions in the captured messages.",
  "contact.loops.ours": "You",
  "contact.loops.question": "Open question",
  "contact.loops.overdue_one": "overdue {count} day",
  "contact.loops.overdue_other": "overdue {count} days",
  "contact.loops.overdueUnderDay": "overdue by less than a day",
  "contact.loops.due": "due {when}",
  "contact.loops.dueToday": "today",
  "contact.loops.dueTomorrow": "tomorrow",
  "contact.loops.dueInDays": "in {count} days",
  "contact.loops.waiting": "Waiting",
  "contact.loops.openBadge": "Open",

  "contact.memory.title": "Activity",
  "contact.memory.showAll": "Show all activity",
  "contact.memory.empty": "No activity captured on this channel yet.",
  "contact.memory.all": "All",
  "contact.memory.email": "Email",
  "contact.memory.meetings": "Meetings",
  "contact.memory.calls": "Calls",
  "contact.memory.notes": "Notes",
  "contact.memory.channelEmail": "Email",
  "contact.memory.channelMeeting": "Meeting",
  "contact.memory.channelCall": "Call",
  "contact.memory.channelNote": "Note",
  "contact.memory.channelMessage": "Message",
  "contact.memory.channelTask": "Task",
  "contact.memory.replied": "Replied",
  "contact.memory.unanswered": "Unanswered",

  "contact.rail.blocked": "Blocked",
  "contact.rail.direction": "Direction",
  "contact.rail.lastReply": "Last reply",
  "contact.rail.trend": "Trend",
  "contact.rail.twoWay": "Two-way",
  "contact.rail.noDirection": "No direction recorded",
  "contact.overview.unavailable": "Not shown: {sections}.",
  "contact.rail.inboundOnly": "Inbound only",
  "contact.rail.outboundOnly": "Outbound only",
  "contact.rail.coverage": "Coverage",
  "contact.rail.exchanges": "{count} exchanges",
  "contact.rail.colleagues_one": "{count} colleague",
  "contact.rail.colleagues_other": "{count} colleagues",
  "contact.rail.noInbound": "No inbound",
  "contact.rail.cooling": "Cooling",
  "contact.rail.warming": "Warming",
  "contact.rail.overall": "Overall",
  "contact.rail.thin": "Thin",
  "contact.rail.atRisk": "At risk",
  "contact.rail.strong": "Strong",
  "contact.standing.why.strong":
    "They wrote within the last {days} days, so the relationship is rated strong.",
  "contact.standing.why.atRisk":
    "No message from them in over {days} days, so the relationship is rated at risk.",
  "contact.standing.why.thin":
    "They have never written, so there is no rating yet.",
  "contact.standing.trend.warming":
    "Warming: their last message is newer than your team’s.",
  "contact.standing.trend.cooling":
    "Cooling: your team wrote last and is waiting on them.",
  "contact.standing.direction.twoWay": "Both sides have written.",
  "contact.standing.direction.inboundOnly":
    "Only they have written so far. Nothing has gone out from your team.",
  "contact.standing.direction.outboundOnly":
    "Only your team has written so far. No reply yet.",
  "contact.standing.direction.none": "No messages either way yet.",
  "contact.rail.consentTitle": "Communication permissions",
  "contact.rail.email": "Email",
  "contact.rail.phone": "Phone",
  "contact.rail.noEmailAddress": "No address on file",
  "contact.rail.noPhoneNumber": "No number on file",
  "contact.rail.channelNotDeliverable": "Not deliverable",
  "contact.drawer.close": "Close",
  "richtext.bold": "Bold",
  "richtext.italic": "Italic",
  "richtext.bulletList": "Bulleted list",
  "richtext.numberList": "Numbered list",
  "richtext.link": "Link",
  "richtext.linkPrompt": "Link URL (leave empty to remove the link)",
  "contact.composer.intentAgenda": "propose an agenda for the upcoming meeting",
  "contact.composer.intentReply": "reply to their last message",
  "contact.composer.intentCommitment": "deliver on a commitment",
  "contact.composer.intentFollowUp": "follow up after a quiet period",
  "contact.research.title": "Deep research · {name}",
  "contact.research.publicOnly": "Public sources only",
  "contact.research.running": "Reading public sources…",
  // Deep research and bought contact data are two capabilities, and this
  // sentence sits directly under the bought data on the same drawer. It names
  // the RESEARCH provider for that reason: "data provider" is the licensed
  // contact-data vocabulary (provider.profile.*), and using it here told a
  // reader nothing was connected while eight purchased claims sat above it.
  "contact.research.notConnected":
    "No research provider is connected, so no public source was read for this contact. This is separate from any purchased contact data above. Margince never researches a contact on its own authority, and deep research needs a licensed provider that holds the lawful basis for it.",
  "contact.research.staged":
    "Research is staged. {name}’s record does not change until you review and save.",
  "contact.research.stats": "{sources} sources read · {claims} cited claims",
  "contact.research.dismiss": "Dismiss",
  "contact.research.discard": "Discard",
  "contact.research.save_one": "Review and save {count} claim",
  "contact.research.save_other": "Review and save {count} claims",
  // A prose claim becomes a stored value only once a reader says which field it
  // fills — the judgement the closed field enum exists to force. The value and
  // its citation are prefilled from the run and stay editable.
  "contact.research.mapField": "Profile field",
  "contact.research.mapFieldPlaceholder": "Select field",
  "contact.research.mapValue": "Value",
  "contact.research.mapQuote": "Source quote",
  "contact.research.mapUrl": "Source link",
  "contact.research.mapUrlInvalid": "Enter an http or https link.",
  "contact.research.mapIncomplete":
    "Add a value, quote and link to save this claim.",
  "contact.research.saved_one": "{count} claim added to the record",
  "contact.research.saved_other": "{count} claims added to the record",
  "contact.research.field.title": "Job title",
  "contact.research.field.role": "Role",
  "contact.research.field.company_name": "Company",
  "contact.research.field.phone": "Phone",
  "contact.research.field.linkedin": "LinkedIn",
  "contact.research.field.address": "Address",
  "contact.research.field.website": "Website",
  "contact.research.evidenceOrOmit":
    "AI-assisted · claims without evidence omitted · public information only",
  "contact.meeting.title": "Meeting brief",
  "contact.meeting.brief": "Prepare brief",
  "contact.meeting.empty": "Nothing recorded for this meeting yet.",
  "contact.meeting.loading": "Preparing brief…",
  "contact.meeting.assembledNow": "Prepared from the latest data",
  "contact.meeting.header": "At a glance",
  "contact.meeting.what_changed": "Since you last spoke",
  "contact.meeting.goal": "Goal for this meeting",
  "contact.meeting.attendees": "Attendees",
  "contact.meeting.commitments": "Open commitments",
  "contact.meeting.deal_state": "Where the deal stands",
  "contact.meeting.risks": "Risks and watch-outs",
  "contact.meeting.talking_points": "Suggested talking points",
  "contact.meeting.company_context": "When you last met",
  "contact.meeting.objective": "Target outcome",
  "contact.meeting.openWith": "Open with",
  "contact.meeting.arc": "Relationship history",
  "contact.meeting.arcSub": "Only the events relevant to this meeting.",
  "contact.meeting.close": "Close the meeting",
  "contact.meeting.advance.minimum": "Minimum advance",
  "contact.meeting.advance.best": "Best advance",
  "contact.meeting.advance.fallback": "Fallback",
  "contact.meeting.unknowns": "Gaps in the record",
  "contact.meeting.likelyAsks": "Likely questions",
  "contact.meeting.beReady": "Be ready for",
  "contact.meeting.say": "Say",
  "contact.meeting.show": "Show",
  "contact.meeting.avoid": "Avoid",
  "contact.meeting.scenarios": "Alternative scenarios",
  "contact.meeting.relevance.high": "Likely",
  "contact.meeting.relevance.medium": "Possible",
  "contact.meeting.relevance.low": "Less likely",
  "contact.meeting.coach.title": "Coaching focus",
  "contact.meeting.coach.eyebrow": "Manager view",
  "contact.meeting.coach.listenFor": "Listen for",
  "contact.meeting.coach.watchFor": "Watch for",
  "contact.meeting.coach.interveneIf": "Step in only if",
  "contact.meeting.coach.paths": "Possible outcomes",
  "contact.meeting.background": "Background and sources",
  "contact.meeting.omittedSource": "Not in this brief",
  "contact.meeting.preparedFor": "Prepared for {name}",
  "contact.meeting.preparedForAt": "Prepared for {name} · {company}",

  "today.source.suggestions": "suggestions",

  // The licensed data provider (ADR-0101). Two surfaces share this
  // vocabulary — the Settings card and the contact page — so a state reads
  // the same wherever it appears.
  "today.scan.queued": "Margince will analyze this company shortly.",
  "today.scan.reading":
    "Margince is analyzing this company’s exchanges and deals.",
  "today.scan.read": "Analyzed {exchanges} and {deals}",
  "today.scan.readExchanges_one": "{count} exchange",
  "today.scan.readExchanges_other": "{count} exchanges",
  "today.scan.readDeals_one": "{count} deal",
  "today.scan.readDeals_other": "{count} deals",
  "today.scan.stale":
    "The company has changed since. It is analyzed again within the hour.",
  "today.scan.resumes": "Analysis resumes {when}. Paused by the AI budget.",
  "provider.title": "Contact data",
  "provider.readOnly":
    "Read-only: connecting a provider costs money, so only an administrator or operations user can do it.",
  "provider.sub":
    "Buy verified contact details for your contacts. The provider charges credits; spend is shown below.",
  "provider.notConfigured":
    "No data provider is available on this installation, so nothing can be bought.",
  "provider.status.connected": "Connected",
  "provider.status.disconnected": "Not connected",
  "provider.status.validating": "Checking API key…",
  "provider.status.invalidCredentials": "API key refused",
  "provider.status.insufficientCredits": "Out of credits",
  "provider.status.rateLimited": "Rate limited",
  "provider.status.providerError": "Provider error",
  "provider.connect": "Connect",
  "provider.reconnect": "Replace API key",
  "provider.apiKey": "API key",
  "provider.apiKeyHint":
    "Stored in the key vault once verified. It is never shown again and never leaves this installation except to the provider.",
  "provider.apiKeyStored": "Replace API key",
  "provider.apiKeyReplaceHint":
    "A key is stored and in use. It cannot be shown again; paste a new key only to replace it.",
  "provider.apiKeyReplacePlaceholder":
    "Paste a new key to replace the stored one",
  "provider.connectConfirm.title": "Connect this data provider?",
  "provider.connectConfirm.body":
    "The key is verified with the provider before it is saved. Once connected, enriching a contact spends credits.",
  "provider.disconnect": "Disconnect",
  "provider.disconnectConfirm.title": "Disconnect this provider?",
  "provider.disconnectConfirm.body":
    "New lookups stop immediately and the key is destroyed. Purchased data stays on your records; disconnecting does not delete it.",
  "provider.deleteData": "Delete purchased data",
  "provider.deleteDataConfirm.title": "Delete all data from this provider?",
  "provider.deleteDataConfirm.body":
    "Every value this provider supplied is removed from every contact. Spend records stay; the data does not. This cannot be undone.",
  "provider.deleteDataConfirm.typed": "Enter the provider name to confirm",
  "provider.automaticLookup": "Look up contacts automatically",
  "provider.automaticLookupHint":
    "Each contact is looked up once for the details this connection selects that the provider charges nothing for, typically the professional profile link, current role and employer, and work history. Email addresses and mobile numbers are never bought this way; they cost credits and remain a per-contact decision.",
  "provider.automaticLookupJurisdiction":
    "Switch this off if the contacts in your CRM fall under a law that forbids trading personal data, such as Vietnam’s. The button on each contact still works, so the decision stays with the person making it.",
  "provider.buyable": "Allow buying {category}",
  "provider.buyableWriteFailed": "Category not changed",
  "provider.postureReadFailed": "Current setting unknown",
  "provider.postureWriteFailed": "Automatic lookup not changed",
  "provider.buyableHint_one":
    "Switching this on buys nothing. It adds a button to each contact, priced at {credits} credit, to buy this detail for one contact at a time.",
  "provider.buyableHint_other":
    "Switching this on buys nothing. It adds a button to each contact, priced at {credits} credits, to buy this detail for one contact at a time.",
  "provider.buyableNeeds":
    "The provider searches for this only together with the {prerequisite}, so it cannot be bought alone. Allow that first.",
  "provider.backlog": "Still to look up",
  "provider.backlogRemaining_one": "{count} contact",
  "provider.backlogRemaining_other": "{count} contacts",
  "provider.backlogWorking":
    "Contacts that existed when the provider was connected are being looked up a few at a time.",
  "provider.backlogPaused":
    "No lookups are running: automatic lookups are off, the daily limit is reached, or the provider is unavailable.",
  "provider.credits": "Provider credit balance",
  "provider.credits.none": "The provider has not reported a balance yet.",
  "provider.credits.notConnected":
    "Connect an API key to see the provider credit balance.",
  "provider.constraints": "Limits in force",
  "provider.spend": "Credits used",
  "provider.spend.hint":
    "Margince’s own record of enrichment spend, not the provider’s invoice. Credits can also be spent in the provider’s app, so the figures can differ.",
  "provider.spend.thisMonth": "This month",
  "provider.spend.month": "Month",
  "provider.spend.pool": "Pool",
  "provider.spend.chargedHead": "Credits",
  // "Held" rather than "unknown": the column holds credits whose outcome the
  // provider never reported, which is what a human reconciles against the
  // invoice. Kept out of the Credits column on purpose.
  "provider.spend.heldHead": "Held",
  "provider.spend.runsHead": "Lookups",
  "provider.spend.none": "Nothing purchased yet.",

  // The contact page's section. The three "nothing here" states are three
  // different sentences on purpose: only one of them is something the
  // reader can act on.
  "provider.profile.title": "Purchased contact data",
  "provider.profile.notConnected":
    "No data provider is connected, so nothing has been purchased.",
  "provider.profile.notEligible":
    "This contact is not eligible: they objected, or the record is archived.",
  "provider.profile.nothingToLookUp":
    "No identifier to look up this contact. Add their LinkedIn URL or employer to run the lookup.",
  "provider.profile.neverRun": "This contact has not been looked up yet.",
  "provider.profile.queued": "Queued",
  "provider.profile.inProgress": "Looking up…",
  "provider.profile.workingTitle": "Querying {provider}",
  "provider.profile.working": "This takes up to a minute.",
  "provider.profile.landingTitle": "Response received",
  "provider.profile.landing": "Saving to the record…",
  "provider.profile.lookupRefused": "Lookup did not run",
  "provider.profile.completed": "Found",
  "provider.profile.noMatch": "The provider has no data for this contact.",
  "provider.profile.stale":
    "Purchased earlier. The provider is no longer connected, so this cannot be refreshed.",
  "provider.profile.invalidCredentials":
    "The provider refused the API key, so the lookup did not run.",
  "provider.profile.insufficientCredits":
    "Not purchased: this month’s credit budget is spent.",
  "provider.profile.rateLimited":
    "Not purchased: the provider is rate limiting requests.",
  "provider.profile.providerError":
    "The last lookup failed. Retry, or check the provider card in Settings if it persists.",
  "provider.profile.submissionUnknown":
    "The outcome of this lookup is unknown. It may have used credits.",
  "provider.profile.claimsUnwritten":
    "Paid for, but the details did not reach this record.",
  "provider.profile.enrichNow": "Look up contact · free",
  "provider.profile.recheck": "Check again · free",
  "provider.profile.lookingUp": "Querying the provider…",
  "provider.profile.emptyTitle": "No data purchased for this contact",
  "provider.profile.emptyBody":
    "A lookup queries {provider} for the details this connection buys and spends {provider} credits. Results appear beside the record and never overwrite colleague entries.",
  "provider.profile.emails": "Email addresses",
  "provider.profile.emailType.provider": "{type}, as labeled by the provider",
  "provider.profile.emailType.requested": "{type}, as requested",
  "provider.profile.mobiles": "Mobile numbers",
  "provider.profile.confidence": "{percent}% confidence",
  "provider.profile.linkedin": "LinkedIn",
  "provider.profile.employment": "Current role",
  "provider.profile.jobHistory": "Earlier roles",
  "provider.profile.location": "Location",
  "provider.profile.departments": "Departments",
  "provider.profile.seniorities": "Seniority",
  "provider.profile.notRequested": "Not requested: {categories}.",
  // The receipt. Without it a lookup that returned one detail out of six read
  // exactly like one that returned all six, and nothing on the page said when
  // the answer arrived.
  // The price rides the button because the decision IS the spend.
  "provider.profile.buy_one": "Buy {category} · {credits} credit",
  "provider.profile.buy_other": "Buy {category} · {credits} credits",
  "provider.profile.buyRebuys":
    "The price includes the {categories} again: the provider requires it for this search and charges for everything it returns.",
  "provider.freeTier.hint":
    "LinkedIn profile, current role and work history cost no credits. Leave this on so every new contact gets them automatically.",
  "provider.pricedTier.hint":
    "Never bought automatically. Bought one contact at a time, with the price on the button.",
  "provider.profile.receiptAt": "Looked up {at}.",
  "provider.profile.receipt":
    "Looked up {at} · {answered} of {asked} details returned.",
  "provider.profile.noAnswer": "Requested, not found: {categories}.",
  // The provider's own vocabulary, in words a reader knows. Not translated
  // one-for-one from the key: these are what a rep would call the thing.
  "provider.category.professionalEmail": "work email",
  "provider.category.personalEmail": "personal email",
  "provider.category.mobile": "mobile number",
  "provider.category.linkedin": "LinkedIn profile",
  "provider.category.currentEmployment": "current role",
  "provider.category.jobHistory": "earlier roles",

  // The predicate builder (AC-filters-and-views-3/4). Operator labels are keyed
  // per reading rather than per symbol: the same `gte` is "on or after" a date
  // and "at least" a quantity, and one label for both would send a reader
  // looking for a calendar on a score.
  "filters.joinAll": "All (AND)",
  "filters.joinAny": "Any (OR)",
  "filters.joinLabel": "Match mode",
  "filters.removeGroup": "Remove group",
  "filters.addGroup": "Add group",
  "filters.addClause": "Add clause",
  "filters.emptyGroup":
    "No clauses yet. An empty group matches nothing; add a clause.",
  "filters.field": "Field",
  "filters.choosePlaceholder": "Select field",
  "filters.customBadge": "Custom field",
  "filters.operator": "Operator",
  "filters.value": "Value",
  "filters.values": "Values",
  "filters.addValue": "Add value",
  "filters.removeClause": "Remove {field} clause",
  "filters.existsLabel": "Field has value",
  "filters.hasValue": "has a value",
  "filters.isEmpty": "is empty",
  "filters.yes": "yes",
  "filters.no": "no",
  "filters.op.eq": "is",
  "filters.op.neq": "is not",
  "filters.op.in": "is any of",
  "filters.op.contains": "contains",
  "filters.op.exists": "has a value",
  "filters.op.afterDate": "is after",
  "filters.op.onOrAfterDate": "is on or after",
  "filters.op.beforeDate": "is before",
  "filters.op.onOrBeforeDate": "is on or before",
  "filters.op.moreThan": "is more than",
  "filters.op.atLeast": "is at least",
  "filters.op.lessThan": "is less than",
  "filters.op.atMost": "is at most",

  // The Filters & views screen's own chrome. The match line is keyed per object
  // because "3 contacts match" and "3 companies match" are different sentences in
  // every language, and a shared "{count} match" would make the object a
  // placeholder that some grammars cannot place.
  "filters.title": "Filters and views",
  "filters.subtitle":
    "Build a filter, preview its matches and save it as a view.",
  "filters.objectLabel": "Record type",
  "filters.tab.contacts": "Contacts",
  "filters.tab.companies": "Companies",
  "filters.tab.deals": "Deals",
  "filters.builderTitle": "Filter",
  "filters.dynamic": "Dynamic: updates on every event",
  "filters.matchContacts": "{count} contacts match",
  "filters.matchCompanies": "{count} companies match",
  "filters.matchDeals": "{count} deals match",
  "filters.noFilterYet": "Add a clause to preview matches",
  // The count when the server was asked and did not answer. It must not fall
  // back to noFilterYet: a reader looking at a finished clause would read a
  // refusal as their own unfinished work. Three words, because this sits in a
  // header row beside two buttons; the reason and the retry go in the results
  // card below, which is the only row wide enough for a sentence.
  "filters.countUnavailable": "Count unavailable",
  "filters.loadingVocabulary": "Loading fields…",
  "filters.noFields": "No filterable fields for this record type.",
  "filters.resultsTitle": "Matching records",
  "filters.resultsCaption":
    "First page of matches, for checking the filter. Not the full selection.",
  "filters.noMatches": "No records match this filter.",
  "filters.loadView": "Load saved filter",
  "filters.pickRecord": "Select record",
  "filters.searchRecords": "Search companies",
  "filters.typeToSearch": "Type to search",
  "filters.searching": "Searching…",
  "filters.searchFailed": "Search failed",
  "filters.noRecordMatches": "No companies match",
  "filters.changeRecord": "Change",
  "filters.removeRecord": "Remove {record}",
  "filters.loadingRecords": "Loading choices…",
  "filters.pickValue": "Select value",
  "filters.exportCsv": "Export CSV",
  "filters.exportJson": "Export JSON",

  // The release gate (src/screens/releaseskew.tsx). It renders instead of the
  // app when this bundle and the api come from different releases, so the copy
  // has two readers at once: the colleague who just wants in, and the operator who
  // has to fix it. The first sentence is for the first, the last for the second.
  "release.skewTitle": "App and server versions differ",
  "release.skewBody":
    "The browser app and the server run different releases, so this page is unreliable. Reload for the current version. If this remains, run the same release on every component.",
  "release.skewVersions": "app {app} · server {server}",
  "release.skewReload": "Reload",

  // The queue behind "send later". Every sentence here is addressed to ONE
  // contact: the list is the sender's own, so there is no "a teammate scheduled
  // this" reading to write for.
  //
  // The verb is "withdraw", not "delete" or "cancel". Nothing was transmitted
  // and nothing reaches the timeline, so there is no send to cancel and no
  // record to delete — the rep is taking a message back before it goes, and
  // "withdraw" is the only one of the three that says so.
  "nav.scheduled": "Scheduled messages",
  "sched.sub":
    "Your scheduled messages that have not been sent yet. Only you can see them.",
  "sched.empty": "No scheduled messages yet.",
  "sched.group.held": "Held for your action",
  "sched.group.heldEmpty": "No held messages.",
  "sched.group.waiting": "Scheduled",
  "sched.group.waitingEmpty": "No messages scheduled.",
  "sched.group.closed": "Sent or withdrawn",
  "sched.group.closedEmpty": "Nothing sent or withdrawn yet.",
  "sched.status.scheduled": "Scheduled",
  // "Released" is the wire's word for the step between waiting and sent: the
  // activity and the delivery exist and the provider has not answered yet. To a
  // rep that is a message on its way out, and there is nothing left to do to it.
  "sched.status.released": "Sending",
  "sched.status.sent": "Sent",
  "sched.status.cancelled": "Withdrawn",
  "sched.status.held": "Held",
  "sched.held.consentWithdrawn":
    "A recipient withdrew their consent after you scheduled this. It will not send until you write to them under a purpose they have agreed to.",
  "sched.held.senderInactive":
    "Your seat or mailbox changed after you scheduled this, so it cannot be sent as you.",
  "sched.held.missedWindow":
    "Its send time passed while the system was not running, and it is now too late to send. Reschedule or withdraw it.",
  "sched.held.timerExhausted":
    "The send job ran out of attempts. Reschedule it to retry.",
  "sched.held.sendRefused":
    "A check blocked this message when it was due. Nothing was sent.",
  "sched.inZone": "in {zone}",
  "sched.recipientsUnknown": "No recipient on this message",
  "sched.recipientsMore": "{first} and {count} more",
  "sched.move": "Reschedule",
  "sched.moveTo": "New time for “{subject}”",
  "sched.moveSave": "Reschedule",
  "sched.moveCancel": "Cancel",
  "sched.withdraw": "Withdraw",
  "sched.withdrawTitle": "Withdraw this message?",
  "sched.withdrawBody":
    "“{subject}” will not be sent, and nothing reaches the timeline. Sending it later means writing it again.",
  "sched.withdrawConfirm": "Withdraw message",
  "sched.skewTitle": "List out of date",
  "sched.skew":
    "The message was already sent, withdrawn or moved. Reload the list.",
  "sched.writeFailed": "Change not saved",
  "sched.reload": "Reload",
  // Projects — the body of work a deal is about. It starts during the deal,
  // in the initiative phase, and outlives close-won; this namespace is every
  // word the list, the page and the deal form's picker say about one.
  "nav.projects": "Projects",
  "unit.projects": "projects",
  "companyProjects.title": "Projects",
  "companyProjects.empty":
    "No projects yet. A company appears here once it is on a project as client, partner or subcontractor.",
  "projectCompanies.title": "Companies",
  "projectCompanies.empty":
    "No companies yet. A project can include the client and any partner or subcontractor delivering it.",
  "projectCompanies.attach": "Attach company",
  "projectCompanies.detachTitle": "Remove company from project?",
  "projectCompanies.searchLabel": "Search companies by name",
  "contactProjects.title": "Projects",
  "contactProjects.empty":
    "No projects yet. This contact appears here once added to a project in any role.",
  "projectRole.customer": "Customer",
  "projectRole.partner": "Partner",
  "projectRole.subcontractor": "Subcontractor",
  "contactRole.sponsor": "Sponsor",
  "contactRole.projectLead": "Project lead",
  "contactRole.deliveryLead": "Delivery lead",
  "contactRole.expert": "Subject-matter expert",
  "contactRole.user": "User",
  "projectLinks.new": "New project",
  "projectLinks.attach": "Attach project",
  "projectLinks.move": "Move to another project",
  "projectLinks.detach": "Detach",
  "projectLinks.detachConfirm": "Detach project",
  "projectLinks.detachNamed": "Detach {name}",
  "projectLinks.roleLabel": "As",
  "projectLinks.detachTitle": "Detach this project?",
  "projectLinks.detachBody":
    "{name} is unchanged. Only its link to this record ends; nothing is deleted.",
  "projectLinks.emptyTitle": "No projects yet",
  "projectLinks.searchLabel": "Search projects by name or key",
  "project.name": "Project name",
  "projectHealth.title": "Delivery health",
  "projectHealth.empty": "No health reading yet",
  "projectHealth.emptyDetail":
    "Record how delivery is going so the next reader has the status.",
  "projectHealth.assessedOn": "as of {date}",
  "projectHealth.corrected": "(corrected)",
  "projectHealth.state.onTrack": "On track",
  "projectHealth.state.atRisk": "At risk",
  "projectHealth.state.offTrack": "Off track",
  "projectHealth.record": "Record reading",
  "projectHealth.correct": "Correct reading",
  "projectHealth.correctOne": "Correct the reading of {date}",
  "projectHealth.recordTitle": "Record delivery health",
  "projectHealth.correctTitle": "Correct this reading",
  "projectHealth.stateLabel": "Health",
  "projectHealth.note": "Note",
  "projectHealth.noteOptionalHint": "Optional while the project is on track.",
  "projectHealth.noteRequiredHint":
    "Describe the problem so the next reader has the context.",
  "projectHealth.correctionNote":
    "A correction changes what was recorded, never when. The reading keeps its original date.",
  "projectHealth.saveReading": "Record reading",
  "projectHealth.saveCorrection": "Save correction",
  "project.keyMinted":
    "Each project gets a short key. Put [{key}] in an email subject to file the mail under this project.",
  "project.company": "Company",
  "project.owner": "Owner",
  "project.ownerKeep": "Keep current owner",
  "project.ownerMe": "Me",
  "project.ownerUnassign": "Unassign",
  "project.assignOwner": "Assign to a colleague",
  "project.assignOwnerTitle": "Assign to a colleague",
  "project.assignOwnerSearch": "Search colleagues",
  "project.assignOwnerNoneSelected": "Select a colleague first",
  // The dialog's own confirm verb. The trigger and the title both read "Assign
  // to a colleague"; the button says what pressing it does, and a button
  // repeating the heading it sits under reads as chrome rather than a verb.
  "project.assignOwnerConfirm": "Assign",
  "project.assignOwnerDone": "Assigned to {name}",
  "project.description": "Description",
  "project.targetEnd": "Target end date",
  "project.targetEnd.inDays_one": "in {days} day",
  "project.targetEnd.inDays_other": "in {days} days",
  "project.targetEnd.overdue_one": "{days} day overdue",
  "project.targetEnd.overdue_other": "{days} days overdue",
  "project.new": "New project",
  "project.edit": "Edit project",
  "project.archive": "Archive project",
  "project.archiveConfirm":
    "Archiving removes this project from the active list and frees its key. This cannot be undone here.",
  "project.archivedReadOnly": "This project is archived and takes no changes.",
  "project.notYoursToChange":
    "You cannot edit this project. Ask its owner to share it, or an administrator for edit rights.",
  "project.phaseLabel": "Phase",
  "project.filterPhaseAll": "All phases",
  "project.viewDelivering": "In delivery",
  "project.phase.initiative": "Initiative",
  "project.phase.pursuing": "Pursuing",
  "project.phase.delivering": "Delivering",
  "project.phase.closed": "Closed",
  "project.emptyTitle": "No projects yet",
  "project.emptyBody":
    "A project is the work a deal is about. It starts in the initiative phase during the deal and continues after the deal is won, when delivery is tracked here.",
  "project.emptyKey":
    "Each project gets a short key. Email with the key in brackets in its subject is filed under the project automatically.",
  "project.rollups.empty": "No figures for this project yet.",
  "project.rollups.openValue": "Open deals",
  "project.rollups.wonValue": "Won deals",
  "project.rollups.noneOpen": "None",
  "project.rollups.noneWon": "None yet",
  "project.rollups.openCommitments": "Open commitments",
  "project.rollups.never": "None",
  "project.rollups.activityCount": "Activity",
  "project.rollups.activityFiled": "{count} filed",
  "project.rollups.activityLast": "Last · {date}",
  "project.history.title": "Phase history",
  "project.history.empty": "No phase change recorded yet.",
  "project.history.current": "current",
  "project.history.moved": "{from} → {to}",
  "project.history.born": "Started in {phase}",
  "project.history.bySystem": "System",
  "project.deals.title": "Deals",
  "project.deals.empty":
    "No deal is linked to this project yet. A deal selects its project on its own form.",
  "project.deals.more": "More deals exist than shown. Open Deals to see all.",
  "project.stakeholders.title": "Stakeholders",
  "project.stakeholders.empty":
    "No stakeholders yet. A stakeholder is a contact with a role on this project, such as sponsor, project lead or champion.",
  "project.stakeholders.add": "Add stakeholder",
  "project.stakeholders.addConfirm": "Add",
  "project.stakeholders.addHint":
    "One role per contact. Adding a contact already on this project changes their role to the one selected here.",
  "project.stakeholders.searchLabel": "Search contacts by name",
  "project.stakeholders.removeTitle": "Remove stakeholder?",
  "project.stakeholders.removeConfirm":
    "{name} is no longer a stakeholder on this project. Their activity stays where it is.",
  "project.stakeholders.removeOne": "Remove {name} from project",
  "project.role.sponsor": "Sponsor",
  "project.role.project_lead": "Project lead",
  "project.role.delivery_lead": "Delivery lead",
  "project.role.subject_matter_expert": "Subject-matter expert",
  "project.contracts.title": "Contracts",
  "project.contracts.empty":
    "No contract is filed under this project. A contract names its project when it is recorded.",
  "project.documents.title": "Documents",
  "project.documents.empty":
    "No file is attached to this project. Files attached to its deals stay on the deals.",
  "project.commitments.title": "Open commitments",
  "project.commitments.empty":
    "No open task is filed under this project. Linked tasks appear here, soonest due first.",
  "project.commitments.overdue": "Overdue",
  "project.timeline.empty":
    "Nothing is filed under this project yet. Email with the key in its subject and linked activities appear here.",
  "project.advance.title": "Move to {phase}",
  "project.advance.confirm": "Move",
  "project.advance.close": "Close project",
  "project.advance.body":
    "The move is recorded in the phase history with the reason you give.",
  "project.advance.closeBody":
    "Closing ends the project’s delivery. It can be reopened later, and the reason stays on record.",
  "project.advance.reason": "Reason",
  "project.advance.reasonRequired": "A closed project needs a reason.",
  "deal.project": "Project",
  "deal.projectNew": "New project…",
  "deal.projectWithheld": "Project withheld",
  "deal.projectNeedsCompany":
    "Select the deal’s company first. A project belongs to a company.",
  "deal.projectUnnamed": "Project",
  "deal.startDeliveryTitle": "Start delivery",
  "deal.startDelivery": "Start delivery",
  "deal.startDeliveryFailed": "Delivery did not start",
  "deal.startDeliveryAttached":
    "This deal is attached to {project}, but the project is not in delivery yet. Move it now?",
  "deal.startDeliveryBody":
    "This deal is won and names no project. Attach it to {project} and move the project into delivery?",

  // The Worklist's own words: the ranked queue, its dials, and the phrase
  // for every fact the server sends as a closed vocabulary.
  "worklist.loading": "Loading Worklist…",
  "worklist.queue": "Today",
  "worklist.review": "To review",
  "worklist.more": "Show more",
  "worklist.more.failed": "More items did not load. Retry.",
  "worklist.summary":
    "{urgent} urgent · {due} due · {inPlay} in play · {lower} routine · {total} total",
  "worklist.summary.noMiddle":
    "{urgent} urgent · {due} due · {lower} routine · {total} total",
  "worklist.completeness": "{shown} of {considered} shown",
  "worklist.review.partial":
    "{loaded} of {total} shown. Load more to see the rest.",
  "worklist.completeness.bounded_one":
    "{shown} shown · {sources} source has more",
  "worklist.completeness.bounded_other":
    "{shown} shown · {sources} sources have more",
  "worklist.clear": "Nothing is waiting on you.",
  "worklist.clearFor": "Nothing is waiting on {name}.",
  "worklist.clearOfTasksToday":
    "No tasks due today or overdue. Later tasks are on each record’s Tasks tab.",
  "worklist.clearOfWhatWasRead": "No items in the sources that loaded.",
  "worklist.partialTitle": "Worklist incomplete",
  "worklist.partial": "{sources}.",
  "worklist.overdue": "Overdue",
  "worklist.pair.ask": "Which record should be kept?",
  "worklist.pair.keep": "Keep {name}",
  "worklist.pair.notDuplicate": "Not duplicates",
  "worklist.pair.mergeBlocked":
    "These records cannot be merged: both have live projects, and nothing shows which work belongs where. You can still mark them as not duplicates.",
  "worklist.pair.related": "{count} linked",
  "worklist.pair.failed": "Pair was not resolved. Retry.",
  "worklist.pair.refused":
    "You cannot resolve this pair. It needs edit access to both records, which an administrator or sales lead has.",
  "worklist.pair.alreadySettled":
    "Pair was not resolved: someone may have decided first, or the records changed. Reload the list.",
  "worklist.pair.stewardOnly":
    "Only a user who can edit both records can resolve this: an administrator or a sales lead.",
  "worklist.needsPrep": "Needs prep",
  "worklist.pane.title": "Record details",
  "worklist.pane.openRow": "Show details for {position}, {title}",
  "worklist.pane.loading": "Loading record…",
  "worklist.pane.nothing": "Nothing recorded yet.",
  "worklist.pane.lastInbound": "Last inbound",
  "worklist.pane.lastOutbound": "Last outbound",
  "worklist.pane.never": "Never",
  "worklist.pane.company": "Company",
  "worklist.pane.role": "Role",
  "worklist.band.now": "Now",
  "worklist.dueGroup.tomorrow": "Due tomorrow",
  "worklist.dueGroup.this_week": "Due this week",
  "worklist.dueGroup.later": "Later",
  "worklist.band.build_pipeline": "Prospecting",
  "worklist.band.keep_momentum": "Keep momentum",
  "worklist.band.review": "Review",
  // A band holding nothing, said rather than left out. Each says what is
  // absent, because "nothing here" four times over tells a reader less than
  // one line naming what they are clear of.
  "worklist.bandClear.now": "No urgent items. Remaining work is below.",
  "worklist.bandClear.build_pipeline": "No prospecting work waiting.",
  "worklist.bandClear.keep_momentum": "No agreed work is drifting.",
  "worklist.bandClear.review": "Nothing to review.",
  "worklist.disposition.verb.snooze": "Snooze",
  "worklist.disposition.snoozeForDays_one": "Snooze for {value} day",
  "worklist.disposition.snoozeForDays_other": "Snooze for {value} days",
  "worklist.disposition.snoozeFor": "Snooze duration",
  "worklist.disposition.snoozeUntil.reply": "Until they reply",
  "worklist.disposition.verb.not_mine": "Not mine",
  "worklist.disposition.verb.not_sales": "Not a customer",
  "worklist.disposition.done.snooze": "Back on your list tomorrow.",
  "worklist.disposition.doneSnooze_one": "Back on your list tomorrow.",
  "worklist.disposition.doneSnooze_other": "Back on your list in {value} days.",
  "worklist.disposition.doneSnoozeUntil.reply":
    "Back on your list when they reply.",
  "worklist.disposition.done.not_mine":
    "Removed from your list. The owner still sees it.",
  "worklist.disposition.done.not_sales": "Removed from every list.",
  "worklist.disposition.swipeCancel": "Keep",
  "worklist.disposition.menu": "Remove from list",
  "worklist.disposition.undo": "Undo",
  "worklist.disposition.undoFailed":
    "Undo failed. The message is still off your list.",
  "worklist.disposition.failed": "Item was not removed. Retry.",
  "worklist.scope.label": "Whose work",
  "worklist.scope.mine": "Mine",
  "worklist.scope.unassigned": "Unassigned",
  "worklist.scope.team": "Team",
  "worklist.scope.all": "All",
  "worklist.owner.visibleLabel": "Viewing",
  "worklist.manager.cancel": "Cancel",
  "worklist.owner.mine": "My Worklist",
  "worklist.owner.backToMine": "Back to your Worklist",
  "worklist.manager.reassign": "Reassign",
  "worklist.manager.reassignTo": "Reassign to",
  "worklist.manager.reassignConfirm": "Reassign",
  "worklist.manager.takeOwnership": "Take over",
  "worklist.manager.takeOwnershipAsk":
    "This moves the record from their Worklist to yours.",
  "worklist.manager.takeOwnershipConfirm": "Take over",
  "worklist.manager.tookOwnership": "Taken over",
  "worklist.manager.takeOwnershipFailed":
    "Takeover failed. The record still belongs to them.",
  "worklist.manager.reassigned": "Reassigned",
  "worklist.manager.reassignFailed": "Item was not reassigned. Retry.",
  "worklist.manager.coach": "Add note",
  "worklist.manager.coachTitle": "Note for {name}",
  "worklist.manager.coachTitleUnnamed": "Note",
  "worklist.manager.coachIntro": "A short note that appears in their Worklist.",
  "worklist.manager.coachAbout": "About",
  "worklist.manager.coachConfirm": "Add note",
  "worklist.manager.coached": "Note added to their Worklist",
  "worklist.manager.coachFailed": "Note was not added. Retry.",
  "worklist.manager.coachRefused": "You cannot add a note for {name}.",
  "worklist.manager.note": "Note (optional)",
  "worklist.manager.kind.reply_aging": "Aging reply",
  "worklist.manager.kind.next_step": "Deal next step",
  "worklist.manager.kind.review_backlog": "Review backlog",
  "worklist.manager.kind.general": "Other",
  "worklist.board.title": "Team work needing attention",
  "worklist.exceptions.title": "Team exceptions",
  "worklist.handled.title": "Handled for you",
  "worklist.walk.arrived":
    "{arrived} new since you opened the list. Refresh to add them.",
  "worklist.walk.gone":
    "{gone} of these were handled since you opened the list.",
  "worklist.walk.both":
    "{arrived} new and {gone} already handled since you opened the list.",
  "worklist.walk.title": "List changed",
  "worklist.walk.refresh": "Refresh",
  "worklist.handled.empty": "No actions taken on your behalf today.",
  "worklist.handled.loading": "Loading actions…",
  "worklist.handled.count": "{count} handled",
  "worklist.handled.what": "Action",
  "worklist.handled.about": "Record",
  "worklist.handled.when": "When",
  "worklist.handled.noRecord": "No record",
  "worklist.handled.wayBack": "Undo",
  "worklist.handled.putBackDone": "Already undone",
  "worklist.handled.truncated": "List truncated. More items exist.",
  "worklist.exceptions.empty": "No team exceptions.",
  "worklist.exceptions.loading": "Loading team…",
  "worklist.exceptions.count": "{count} need attention",
  "worklist.exceptions.condition": "Condition",
  "worklist.exceptions.subject": "Record",
  "worklist.exceptions.owner": "Owner",
  "worklist.exceptions.basis": "Basis",
  "worklist.exceptions.intervene": "Action",
  "worklist.exceptions.nobody": "No owner",
  "worklist.exceptions.ownerWithheld": "Hidden",
  "worklist.exceptions.truncated": "List truncated. More items exist.",
  "worklist.exceptions.kind.response_breached": "First reply overdue",
  "worklist.exceptions.kind.revenue_at_risk": "Revenue at risk",
  "worklist.exceptions.kind.unassigned": "Unassigned",
  "worklist.exceptions.kind.repeated_failure": "Repeated failure",
  "worklist.board.loading": "Loading team work…",
  "worklist.board.empty": "No team members assigned yet.",
  "worklist.board.member": "Member",
  "worklist.board.waiting": "Awaiting reply",
  "worklist.board.atRisk": "Deals at risk",
  "worklist.board.overdue": "Overdue",
  "worklist.board.nobody": "Unassigned work",
  "worklist.coaching.title": "Coaching suggestions",
  "worklist.coaching.promises": "{name}: customer commitments due {count}",
  "worklist.coaching.waiting": "{name}: customers waiting {count}",
  "worklist.coaching.overdue": "{name}: overdue tasks {count}",
  "worklist.board.promises": "Commitments due",
  "worklist.board.truncated":
    "Counts are incomplete. Each figure is a minimum.",
  "worklist.readings.label": "Today’s figures",
  "worklist.readings.revenue": "Revenue at risk",
  "worklist.readings.revenue.detail": "Deals drifting today",
  "worklist.readings.revenue.unpriced": "Drifting deals · none priced",
  "worklist.readings.revenue.noFigure": "Not priced",
  "worklist.readings.replies": "Buyer replies",
  "worklist.readings.replies.detail": "Customers waiting",
  "worklist.readings.prospecting": "Prospecting",
  "worklist.readings.prospecting.detail": "Awaiting first reply",
  "worklist.readings.review": "Reviews",
  "worklist.readings.review.detail": "Proposals · record checks",
  "worklist.readings.truncated":
    "Counts are incomplete. Each figure is a minimum.",
  "worklist.hidden.title": "Hidden from the Worklist",
  "worklist.hidden.loading": "Checking hidden items…",
  "worklist.hidden.clear":
    "Nothing is hidden. Every waiting customer reaches a Worklist.",
  "worklist.hidden.truncated":
    "Counts are incomplete. Each figure is a minimum.",
  "worklist.hidden.count": "{count} waiting",
  "worklist.hidden.pastHorizon": "Too old for the Worklist",
  "worklist.hidden.pastHorizon.detail":
    "No one decided this. The sender wrote months ago and got no answer.",
  "worklist.hidden.unlinked": "Not linked to a record",
  "worklist.hidden.unlinked.detail":
    "Usually not sales. Sometimes an unfiled customer.",
  "worklist.hidden.colleagues": "From your company’s domains",
  "worklist.hidden.colleagues.detail":
    "Treated as a colleague. A mistyped company domain can hide a real customer.",
  "worklist.hidden.notSales": "Marked not sales work",
  "worklist.hidden.notSales.detail":
    "Hidden for the whole company. This does not expire.",
  "worklist.hidden.setAside": "Set aside by you",
  "worklist.hidden.setAside.detail":
    "Snoozed or marked not yours. Snoozed items return automatically.",
  "worklist.hidden.shown": "The Worklist shows {count}.",
  "worklist.filter.label": "Work type",
  "worklist.filter.all": "All",
  "worklist.filter.customer_waiting": "Customer waiting",
  "worklist.filter.leads": "Leads",
  "worklist.filter.deals_at_risk": "Deals at risk",
  "worklist.filter.meetings": "Meetings",
  "worklist.filter.tasks": "Tasks",
  "worklist.filter.decisions": "Approvals",
  "worklist.filter.system": "System",
  "worklist.filter.linked.urgent":
    "Showing only urgent items: someone waiting, or a commitment about to break.",
  "worklist.filter.linked.except_decisions":
    "Showing everything except approvals already in your Morning brief.",
  "worklist.filter.linked.changed_since_brief":
    "Showing only changes since last night’s Morning brief.",
  "worklist.filter.linked.clear": "Show full Worklist",
  "worklist.category.customer_waiting": "Customer waiting",
  "worklist.category.leads": "Lead",
  "worklist.signal.closing_soon": "Closing soon",
  "worklist.signal.stalled": "Deal at risk",
  "worklist.signal.opportunity": "Opportunity",
  "worklist.signal.moved": "Recently moved",
  "worklist.category.deals_at_risk": "Deal at risk",
  "worklist.category.meetings": "Meeting",
  "worklist.category.tasks": "Task",
  "worklist.category.decisions": "Approval",
  "worklist.category.system": "System",
  "worklist.because.pinned": "pinned by you",
  "worklist.because.buyer_wrote_last": "buyer wrote last",
  "worklist.because.waiting_days": "waiting",
  "worklist.because.more_one": "+{count} more",
  "worklist.because.more_other": "+{count} more",
  "worklist.because.waiting_days.value_one": "waiting {value} day",
  "worklist.because.waiting_days.value_other": "waiting {value} days",
  "worklist.because.overdue": "overdue",
  "worklist.because.due_today": "due today",
  "worklist.because.closing_soon": "expected to close within 2 weeks",
  "worklist.because.expected_revenue": "open deal depends on this",
  "worklist.because.expected_revenue.value": "worth {value}",
  "worklist.because.material": "above typical open deal value",
  "worklist.because.material.value":
    "worth {value}, above typical open deal value",
  "worklist.because.below_material": "not above typical at-risk deal value",
  "worklist.because.below_material.value":
    "{value}, not above typical at-risk deal value",
  "worklist.because.quiet_days": "gone quiet",
  "worklist.because.quiet_days.value_one": "quiet for {value} day",
  "worklist.because.quiet_days.value_other": "quiet for {value} days",
  "worklist.because.no_champion": "no champion",
  "worklist.because.promised": "committed by you",
  "worklist.because.approved_and_failed": "approved but did not run",
  "worklist.because.blocks_customer_work": "customer waiting on this",
  "worklist.because.routine": "routine cleanup",
  "worklist.because.repeated_failure": "repeated failure",
  "worklist.because.legal_deadline": "legal deadline running",
  "worklist.because.meeting_soon": "starting soon",
  "worklist.because.meeting_unprepared": "nothing prepared",
  "worklist.because.outcome_unrecorded": "no outcome recorded",
  "worklist.because.response_overdue": "reply overdue",
  "worklist.because.response_due_soon": "reply due soon",
  "worklist.because.response_due_soon.value": "reply due by {value}",
  "worklist.because.unassigned": "no owner",
  "worklist.because.stale": "waiting a long time",
  "worklist.because.no_reply_history": "no reply history",
  "worklist.because.asks_nothing": "asks for nothing",
  "worklist.because.addressed_elsewhere": "addressed to a colleague",
  "worklist.above.pin": "Ranked above the next item because you pinned it.",
  "worklist.above.level":
    "Ranked above the next item because it is more urgent.",
  "worklist.above.deadline": "Ranked above the next item by date.",
  "worklist.above.deadline.pair":
    "Ranked above the next item: {mine} compared with {theirs}.",
  "worklist.above.expected_revenue":
    "Ranked above the next item by expected revenue.",
  "worklist.above.expected_revenue.pair":
    "Ranked above the next item: {mine} compared with {theirs}.",
  "worklist.above.waiting_days": "Ranked above the next item by waiting time.",
  "worklist.above.waiting_days.pair":
    "Ranked above the next item: {mine} compared with {theirs}.",
  "worklist.above.relationship":
    "Ranked above the next item by relationship strength.",
  "worklist.above.crowded":
    "Ranked above the next item, which is one of many of its kind.",
  "worklist.verdict.live": "Live",
  "worklist.verdict.drifting": "Drifting",
  "worklist.verdict.blocked": "Blocked",
  "worklist.verdict.cold": "Cold",
  "worklist.verdict.believes": "Assessment",
  "worklist.verdict.rule": "Why this is here",
  "worklist.verdict.asOf": "As of {when}",
  "worklist.consequence.buyer_waits": "If ignored, the buyer keeps waiting.",
  "worklist.consequence.promise_breaks": "If ignored, a commitment is broken.",
  "worklist.consequence.deal_drifts": "If ignored, the deal keeps drifting.",
  "worklist.consequence.deal_slips_past_close":
    "If ignored, the deal slips past the agreed close date.",
  "worklist.consequence.meeting_unprepared":
    "If ignored, you go into the meeting unprepared.",
  "worklist.consequence.task_slips": "If ignored, the task slips.",
  "worklist.consequence.work_blocked": "If ignored, the work stays blocked.",
  "worklist.consequence.customer_never_received":
    "If ignored, the customer never receives it.",
  "worklist.consequence.you_believe_it_happened":
    "If ignored, it still looks done although it never ran.",
  "worklist.consequence.legal_deadline_missed":
    "If ignored, a legal deadline passes.",
  "worklist.consequence.mailbox_blind":
    "If ignored, mail that is not arriving stays missing from this page.",
  "worklist.consequence.data_drifts": "If ignored, the records go out of date.",
  "worklist.untitled.approval": "Approval waiting",
  "worklist.untitled.dedupe_candidate": "Possible duplicate records",
  "worklist.untitled.task": "Untitled task",
  "worklist.untitled.brief_item": "Deal to review",
  "worklist.untitled.conversation_claim": "Commitment you made",
  "worklist.untitled.customer_waiting": "Customer awaiting reply",
  "worklist.untitled.lead_response": "Untitled lead",
  "worklist.untitled.deal_at_risk": "Deal drifting",
  "worklist.untitled.meeting": "Untitled meeting",
  "worklist.untitled.meeting_outcome": "Meeting outcome",
  "worklist.untitled.relationship_decay": "Relationship going quiet",
  "worklist.untitled.failed_approval": "Approved action did not run",
  "worklist.untitled.dsr": "Open privacy request",
  "worklist.untitled.notice_case": "Disclosure owed to contact",
  "worklist.untitled.capture_health": "Mailbox connection needs attention",
  "worklist.untitled.ai_work_health": "AI work needs review",
  "worklist.untitled.bounce": "Email bounced",
  "worklist.untitled.undelivered": "Email not sent",
  "worklist.untitled.automation_run": "Automation rule failed",
  "worklist.untitled.notice": "Notice",
  // A domain question always carries the domain itself as its title, so this
  // fallback should never render. It exists because the map is total over the
  // sources this build knows.
  "worklist.untitled.domain_question": "Unreviewed domain",
  "worklist.untitled.introduction_request":
    "Introduction requested by colleague",
  "worklist.verb.decide": "Decide",
  // The drawer the decision is answered in. The row shows what is being decided
  // and this names the act, so the heading does not repeat the row's sentence.
  "worklist.decision.title": "Your decision",
  "worklist.decision.loading": "Loading proposal…",
  "worklist.decision.unavailable":
    "Proposal did not load. Answer it in Approvals.",
  "worklist.verb.merge": "Merge",
  "worklist.verb.open": "Open",
  "worklist.verb.complete": "Open",
  "worklist.verb.snooze": "Open",
  "worklist.verb.acknowledge": "Acknowledge",
  "worklist.verb.promiseKept": "Done",
  "worklist.verb.promiseSettled": "Marked as kept",
  "worklist.verb.promiseSettleFailed":
    "Commitment was not marked as kept. Retry.",
  // The card's two verbs. "Update" opens the composer on this meeting; the
  // three answers below it are what that composer offers, and "Cancelled" is
  // the one the card writes by itself.
  "worklist.verb.meetingUpdate": "Update",
  "worklist.verb.meetingUpdateTitle": "Meeting",
  "worklist.verb.meetingReading": "Loading meeting…",
  "worklist.verb.meetingWhatHappened": "Meeting notes",
  "worklist.verb.meetingBodyHint":
    "Discussion and next steps. Calendar notes can be edited here.",
  "worklist.verb.meetingHeld": "Held",
  "worklist.verb.meetingNoShow": "No-show",
  "worklist.verb.meetingCanceled": "Canceled",
  "worklist.verb.meetingOutcomeRecorded": "Meeting outcome recorded",
  "worklist.verb.meetingOutcomeFailed": "Outcome was not recorded. Retry.",
  "worklist.verb.retry": "Run again",
  "worklist.verb.retryStarted": "Rerunning rule…",
  "worklist.verb.retryFailed": "Rule was not rerun. Retry.",
  // An undecided domain, answered from the row. Two verbs of equal weight,
  // because the question genuinely has two answers and neither is the
  // product's expectation: a domain one colleague works with is noise to the
  // next, and leading with either would be the machine guessing again.
  "worklist.verb.keep": "Create company",
  "worklist.verb.discard": "Exclude domain",
  "worklist.verb.domainKept": "Company created from domain",
  "worklist.verb.domainKeepFailed": "Company was not created. Retry.",
  // The discard wording says WHOSE mail stops being captured, because the
  // colleague reading it shares this installation with people whose answer may
  // differ — and a sentence that said "excluded" flatly would read as a
  // workspace-wide act it is not.
  "worklist.verb.domainDiscarded":
    "Your mail from this domain is no longer captured.",
  "worklist.verb.domainDiscardFailed": "Domain was not excluded. Retry.",
  "worklist.verb.retryRefusedNotFailed":
    "Nothing to rerun. This run was stopped on purpose, not by an error.",
  "worklist.verb.retryRefusedRepeats":
    "This rule is not allowed to run twice, so a rerun could repeat its actions.",
  "worklist.verb.retryRefusedEventGone":
    "The triggering event no longer exists, so this cannot be rerun. Scheduled rules check again automatically.",
  "worklist.verb.acknowledgeFailed": "Item was not marked as seen. Retry.",
  "worklist.verb.completeFailed": "Task was not completed. Retry.",
  "worklist.verb.pin": "Pin",
  // WHAT A PIN ACTUALLY DOES, because the word says none of it. Each clause
  // is checked against the code and none of them may be written loosely:
  //
  //   - "your own queue" rather than "only you see it": the ORDERING is
  //     reader-scoped (worklist_pin is keyed by reader_id), but the act is
  //     written to the audit log, so an authorised audit reader can see that
  //     you pinned something. "Only you see it" would be false there.
  //   - "while it stays among your most recent" rather than "until you unpin
  //     it": a reader's newest 50 pins are read (maxPinsPerReader,
  //     worklistpin.go) and older ones silently stop taking effect.
  //   - Nothing about urgency, because a pin moves the ORDER and not the
  //     figure — semanticLevelOf keeps the summary honest.
  //
  // A reminder or a timed snooze is not on offer: the table has no expiry.
  "worklist.verb.pinHint":
    "Moves this to the top of your Worklist while among your recent pins. Urgency is unchanged.",
  // The SAME control, one state over, and it had better not say the pin
  // sentence: a button that now removes the pin described as keeping it is the
  // control contradicting itself.
  "worklist.verb.unpinHint":
    "Returns this to its ranked place in your Worklist.",
  "worklist.verb.unpin": "Unpin",
  "worklist.verb.pinFailed": "Item was not pinned. Retry.",
  "worklist.verb.unpinFailed": "Item was not unpinned. Retry.",
  "worklist.verb.completed": "Task completed",
  "worklist.verb.dismiss": "Not now",
  "worklist.verb.dismissed": "Set aside for a month",
  "worklist.verb.dismissUndo": "Undo",
  "worklist.verb.dismissFailed": "Contact was not set aside. Retry.",
  "worklist.verb.dismissUndoFailed": "Contact was not restored. Retry.",
  "worklist.verb.completeUndo": "Undo",
  "worklist.verb.completeUndoFailed": "Task was not reopened. Retry.",
  // The frame states the fact and the source follows it, rather than the
  // source standing as the subject. `sourceName` returns a row TITLE — "A
  // mailbox connection needs attention", "Two records look like the same one"
  // — and fourteen of the twenty-one are already whole clauses, so used as a
  // subject they ran two sentences together: "A mailbox connection needs
  // attention could not be read". Naming the fact first works for every entry
  // and needs no second vocabulary of source nouns.
  "worklist.source.failed": "Source did not load: {source}",
  "worklist.source.withheld": "Source not available to you: {source}",
  "worklist.untitled.generic": "Item needs attention",
  "worklist.batch.likely_automated": "{count} likely automated senders",
  "worklist.batch.company_match": "{count} addresses at companies you know",
  "worklist.batch.uncertain_contact": "{count} addresses to review",
  "worklist.batch.duplicates": "{count} possible duplicates",
  "worklist.batch.held_draft": "Drafts waiting to send: {count}",
  "worklist.untitled.batch": "Routine items to review",
  "worklist.verb.review_batch": "Review",
  "worklist.verb.draft_reply": "Read and reply",
  // Where the composer actually opens, the verb is the ACT rather than the way
  // to it. The two labels are separate keys because the two clicks differ.
  "worklist.verb.draft_reply_now": "Draft reply",
  // A FIRST message rather than an answer to one. Separate keys because the two
  // are different acts: a row saying "reply" over an opening outreach names a
  // conversation that has not happened yet.
  "worklist.verb.draft_email": "Write email",
  "worklist.verb.draft_email_now": "Draft email",
  // A brief, not a message — one key, because the control never opens a
  // composer and so never needs the reply/write "now" split above.
  "worklist.verb.open_meeting_brief": "Prepare for meeting",
  "worklist.deal.closes": "closes {date}",
  "worklist.when.starts": "starts {when}",
  "worklist.when.due": "due {when}",
  "worklist.batch.system_incident": "{cause} failed {count} times",
  "worklist.batch.unnamedCause": "A process",

  "ob.conv.scene.settleEyebrow": "Your decision needed",
  "ob.conv.review.boardSub":
    "Every line says where it came from. Nothing is written until you confirm.",
  "ob.conv.manual.boardTitle": "Enter details manually",
  "ob.conv.scene.writes": "Writes",
  "ob.core.idle": "Core · idle",
  "ob.core.ingest": "Core · reading input",
  "ob.core.working": "Core · working",
  "ob.core.warning": "Core · needs attention",
  "ob.core.error": "Core · stopped",
  "ob.scan.tallyPages": "pages read",
  "ob.scan.tallyFacts": "facts found",
  "ob.scan.tallyUncertain": "left for you",
  // The ticker's page-level finding: a fact field's own name (already the
  // reader's language via factFieldLabelKey) and the value the page gave up.
  // Punctuation only, nothing here for a locale to translate.
  "ob.scan.tickerFact": "{field}: {value}",
  "ob.digest.where": "What Margince knows about the company",
  "ob.digest.written": "{n} of {m} lines written",
  "ob.digest.companyLine": "Company profile from {host}, pages read: {n}",
  "ob.digest.citedCaption": "lines, each citing its page",
  "ob.digest.openCaption": "still open",
  "ob.digest.section.identity": "Identity",
  "ob.digest.section.offer": "What they sell",
  "ob.digest.section.customer": "Who they sell to",
  "ob.digest.section.sales": "How they sell",
  "ob.digest.facts": "Evidence",
  "ob.digest.contacts": "Contacts",
  "ob.digest.sources": "Sources",
  "ob.digest.blank": "not written yet",
  "ob.digest.notWritten": "not recorded",
  "ob.digest.settle": "Decide",
  "ob.digest.deciding": "decision in progress",
  "ob.digest.yours": "entered by you",
  "ob.digest.editLine": "Edit {label}",
  "ob.digest.saveChanges": "Save changes",
  "ob.digest.changed": "Unsaved line changes: {count}",
  "ob.digest.pickFacts": "Choose facts to keep",
  "ob.digest.referenceNote":
    "A later re-read may propose changes to this record. It never overwrites a line a person has edited.",
  "ob.digest.sidebarLabel": "Facts about the company",
  "ob.digest.sidebar.legalName": "Legal name",
  "ob.digest.sidebar.founded": "Founded",
  "ob.digest.sidebar.headquarters": "Headquarters",
  "ob.digest.sidebar.offices": "Offices",
  "ob.digest.sidebar.employees": "Employees",
  "ob.digest.sidebar.certifications": "Certifications",
  "ob.digest.pageKind.home": "Home page",
  "ob.digest.pageKind.impressum": "Legal notice",
  "ob.digest.pageKind.about": "About page",
  "ob.digest.pageKind.team": "Team page",
  "ob.digest.pageKind.services": "Services page",
  "ob.digest.pageKind.products": "Products page",
  "ob.digest.pageKind.contact": "Contact page",
  "ob.digest.pageKind.other": "Page",
  "ob.deck.counter": "{n} of {m}",
  "ob.deck.left": "{n} of {m} left",
  "ob.deck.settled": "Facts added from evidence: {count}",
  "ob.deck.needed": "Needed to continue",
  "ob.deck.optional": "Optional",
  "ob.deck.next": "Next",
  "ob.deck.leaveOut": "Leave out",
  "ob.deck.readWhole": "Read full profile",
  "ob.deck.backToOpen": "Back to open questions",
  "ob.deck.backToRecord": "Back to record",
  "ob.deck.confirm": "Confirm profile",
  "ob.deck.stillNeeded": "Still needed: {fields}",
  "ob.deck.openLeft":
    "Unanswered questions: {count}. The record is saved without them.",
  "ob.conv.invite.pickOne": "Select one of the two options to continue.",
  "ob.conv.voice.speakerPick": "Select a speaker to continue.",
  "ob.deck.clear": "Nothing left to decide. Facts on record: {count}.",
  "ob.deck.eyebrow": "Other facts backed by evidence",
  "ob.deck.title": "Questions for you",
  "ob.stage.flow": "Setup",
  "ob.stop.read": "Read website",
  "firstRun.ai.eyebrow": "No model connected",
  "firstRun.step.model": "Model",
  "firstRun.step.platform": "Platform",
  "firstRun.google.eyebrow": "Model connected · mail not connected",
  "firstRun.platform.title": "What does your company run on?",
  "firstRun.platform.sub":
    "This answer decides how mail reaches Margince and how people sign in. It can be changed later under Settings.",
  "firstRun.platform.legend": "Platform this company runs on",
  "firstRun.platform.google": "Google Workspace",
  "firstRun.platform.googleWhat":
    "Mail, calendar and sign-in through one Google app you own.",
  "firstRun.platform.microsoft": "Microsoft 365",
  "firstRun.platform.microsoftWhat":
    "Mail, calendar and sign-in through one Entra app you own.",
  "firstRun.platform.imap": "IMAP",
  "firstRun.platform.imapWhat":
    "Each mailbox connects with its own IMAP app password. Sign-in uses email and password.",
  "firstRun.platform.redirectTitle": "Register these redirect URIs in the app",
  "firstRun.platform.redirectHint":
    "Add each URI to the app before saving here. Sign-in adds the sign-in button to the login page; Mailbox and Calendar let people connect theirs. A missing URI fails at the provider’s consent screen.",
  "firstRun.google.helpToggle": "Where to find these",
  "firstRun.google.helpStep1":
    "In the Google Cloud console, open a project and go to “APIs & Services”, then “Credentials”. Select “Create credentials”, then “OAuth client ID”, and choose “Web application”.",
  "firstRun.google.helpStep2":
    "Enable the Gmail API and add both the gmail.readonly and gmail.send scopes to the consent screen. They share one consent because Google does not add a scope to an issued refresh token; requesting send later means connecting the mailbox again.",
  "firstRun.google.helpStep3":
    "Under Authorized redirect URIs, add the URIs listed above. Mailbox is required for mail; Calendar and Sign-in enable those features.",
  "firstRun.google.helpStep4":
    "Copy the client ID and client secret into the 2 fields below. The secret is sent once, stored in the key vault and never readable again.",
  "firstRun.google.helpConsole": "Google Cloud credentials console",
  "firstRun.google.helpDocs":
    "Full prerequisites, including Microsoft and IMAP: docs/how-to/connect-a-mailbox.md",
  "firstRun.platform.imapNote":
    "Nothing is configured for the whole installation. Connect your own mailbox now or later; other mailboxes connect in Settings under Connections, each with its own app password.",
  "firstRun.platform.skip": "Not now",
  "firstRun.needed": "Needed to continue",
  "firstRun.stillNeeded": "Still needed: {fields}",
  "firstRun.platform.foot":
    "All answers here can be changed later in Settings.",
  "firstRun.microsoft.note":
    "Register an app in Microsoft Entra with the redirect URIs above, then paste its client ID and secret here. Pin it to your directory: that directory’s mailboxes connect through it and its people sign in with it.",
  "firstRun.microsoft.helpSignIn":
    "The directory puts Microsoft on the login page, so it is required here. To register an app without one (any company can connect a mailbox, no one signs in with Microsoft), use Settings instead.",
  "firstRun.microsoft.tenantHint":
    "The Entra directory your people belong to. Mailboxes connect through it and Microsoft sign-in runs on it.",
  "firstRun.ai.rankedHint":
    "Also lists the 10 highest-scoring models OpenRouter serves now, ranked by {rankedBy}, with provider prices.",
  "firstRun.ai.rankedUnavailable":
    "OpenRouter’s live model list could not be read, so this shows the models in the price sheet.",
  "aiRates.chatLane": "Chat model",
  "aiRates.embedLane": "Embedding model",
  "aiRates.perMTokInOut": "per 1M tokens, in → out",
  "aiRates.perMTok": "per 1M tokens",
  "aiRates.unpriced": "Not priced",
  "aiRates.unpricedDetail": "calls still run",
  "aiRates.unpricedConsequence": "Missing from usage and spend",
  "aiRates.unpricedBasis":
    "To report the cost of these calls, add a rate in Settings under AI.",
  "aiRates.priced": "From {date}",
  "aiRates.proposed": "OpenRouter’s price",
  "aiRates.proposedDetail": "Provider price · not yet approved",
  "aiRates.proposedBasis":
    "Binding it sends the price to Approvals. Usage and spend include it once confirmed.",
  "firstRun.ignite.title": "Model connected",
  "firstRun.ignite.sub":
    "The key is stored and the model responded. This is what changes.",
  "firstRun.ignite.sealed": "Stored in the vault · {vendor}",
  "firstRun.ignite.reaching": "Contacting the model…",
  "firstRun.ignite.canNow": "Can now",
  "firstRun.ignite.cannot": "Cannot",
  "firstRun.ignite.read": "read the company website and report what it found",
  "firstRun.ignite.draft": "draft in the voice you trained",
  "firstRun.ignite.act":
    "send anything or change a record without your approval",
  "firstRun.ignite.carryOn": "Continue",
  "firstRun.ai.foot":
    "Nothing is sent to the provider until you select Continue.",
  "contact.readings.title": "Contact status",
  "deal360.brief": "Deal summary",
  "lead.brief.title": "Lead brief",
  "lead.standing.qualified": "Qualified",
  "lead.standing.qualifiedOn": "Qualified on {at}. This lead is now a contact.",
  "lead.standing.qualifiedUndated": "This lead is now a contact.",
  "lead.standing.merged": "Merged",
  "lead.standing.mergedBecause":
    "Same prospect as another lead. See that lead; this record is kept as history.",
  "lead.standing.closed": "Closed",
  "lead.standing.closedFor": "Closed: {reason}. The record is kept as history.",
  "lead.standing.closedUnreasoned": "Closed. The record is kept as history.",
  "lead.standing.yourMove": "Awaiting your reply",
  "lead.standing.noResponse": "No response to this lead yet.",
  "lead.standing.theirMove": "Awaiting lead",
  "lead.standing.answeredOn": "Answered on {at}. No reply yet.",
  "lead.standing.inMotion": "In motion",
  "lead.standing.engagedBecause":
    "The lead replied, or a meeting is scheduled.",
  "lead.standing.rests.promoted": "Qualified as a contact.",
  "lead.standing.rests.merged": "Merged into another lead.",
  "lead.standing.rests.closed": "Disqualified, no reason recorded.",
  "lead.standing.rests.ladder": "Lead ladder",
  "lead.standing.rests.record": "Lead record",
  "lead.standing.rests.captured": "Captured {at}.",
  "lead.standing.rests.noResponse": "No first response recorded.",
  "lead.standing.rests.engaged": "Engagement captured {at}.",
  "lead.readings.title": "Lead summary",
  "lead.readings.firstResponse": "First response",
  "lead.readings.noClock": "No target set",
  "lead.readings.archived": "Archived",
  "lead.readings.merged": "Merged",
  "lead.readings.mergedInto": "Into another lead",
  "lead.readings.company": "Company",
  "lead.readings.noCompany": "None",
  "lead.readings.scoreManual": "Set manually",
  "lead.readings.owed": "Pending",
  "lead.today.answer": "Answer {name}",
  "lead.today.answerMeta": "First response due",
  "lead.today.nextTask": "Next task",
  "lead.today.reply": "Reply",
  "lead.today.openTasks": "Open tasks",
  "lead.readings.answered": "Answered",
  "lead.standing.dueBy": "No response yet. First response due by {at}.",
  "lead.standing.overdueSince": "No response yet. First response was due {at}.",
  "stageAutomation.title": "Stage automation",
  "stageAutomation.intro":
    "Outcomes of the stage moves Margince proposed. This report changes nothing; it is the evidence for letting a transition move deals automatically.",
  "stageAutomation.pipeline": "Pipeline",
  "stageAutomation.transition": "Transition",
  "stageAutomation.reviewed": "Reviewed",
  "stageAutomation.reviewedHint":
    "Proposals someone decided. Every rate is a share of this number.",
  "stageAutomation.open": "Still open",
  "stageAutomation.expired": "Expired",
  "stageAutomation.expiredHint":
    "No one answered before the window closed. This is not a rejection.",
  "stageAutomation.cleanAcceptance": "Accepted as proposed",
  "stageAutomation.edits": "Accepted after edits",
  "stageAutomation.rejections": "Rejected",
  "stageAutomation.unsafe": "Undone or corrected",
  "stageAutomation.unsafeHint":
    "Moves someone reversed or whose evidence they marked wrong. A move counts once even if both happened.",
  "stageAutomation.observationDays": "Days observed",
  "stageAutomation.observationHint":
    "From the first reviewed proposal to the last. A good rate from one afternoon is not a track record.",
  "stageAutomation.evidenceKinds": "By evidence",
  "stageAutomation.empty":
    "Margince has not proposed a stage move on this pipeline yet.",
  "stageAutomation.noPipelines": "No pipelines to report on yet.",
  "stageAutomation.unreadable": "This report did not load",
  "stageAutomation.readOnly":
    "You can see each transition’s record. Changing a transition requires permission to edit pipelines.",
  "stageAutomation.rules": "Transition rules",
  "stageAutomation.rulesIntro":
    "Turning a transition on does not start moving deals. Margince keeps asking until the record above meets the threshold, then applies moves automatically.",
  "stageAutomation.modeHint":
    "When on and the record qualifies, Margince moves the deal and notifies you afterward.",
  "stageAutomation.notEarnedYet": "Not qualified yet: {why}",
  "stageAutomation.suspended": "Suspended by Margince",
  "stageAutomation.suspendedSince": "Suspended {date}",
  "stageAutomation.resume": "Resume",
  "stageAutomation.resumeTitle": "Resume this transition?",
  "stageAutomation.resumeBody":
    "Margince suspended it (reason: {reason}). Resuming does not skip the threshold: the transition must meet the record above before it moves deals automatically.",
  "stageAutomation.noRules":
    "No transition on this pipeline has a rule yet, so every move is proposed for a person to review.",
  "stageAutomation.undoWindow": "Undo for {hours} h",
  "stageAutomation.rulesLoading": "Loading transition rules…",
  "stageAutomation.saveFailed": "Change not saved",
  "stageAutomation.nothingReviewed": "Proposed, but none reviewed yet.",
  "employment.importLoading": "Loading purchased employment history…",
  "employment.apply": "Link imported companies",
  "employment.status.current": "Current",
  "employment.status.former": "Former",
  "employment.status.unknown": "Status unknown",
  "employment.statusLabel": "Employment status",
  "employment.review": "Review this role before linking it.",
  "employment.matchNeeded": "Company match needed.",
  "employment.resolve": "Match company",
  "employment.dismiss": "Dismiss evidence",
  "employment.research": "Research",
  "employment.researchDeferred": "Waiting for research settings and budget",
  "employment.website": "Confirmed company website",
  "employment.websiteHint":
    "Select an existing company above, or confirm its website to create it.",
  "employment.saveMatch": "Save company link",
  "employment.researchNeedsWebsite": "Website needed",
  "employment.research.queued": "Queued",
  "employment.research.running": "Researching",
  "employment.research.done": "Completed",
  "employment.research.partial": "Partially completed",
  "employment.research.failed": "Failed. Open the company to retry.",
  "employment.research.cancelled": "Canceled",
  "employment.researchQueued": "Research status is shown on the company page.",
  "employment.edit": "Edit employment",
  "employment.start": "Start date",
  "employment.end": "End date",
  "employment.dateHint": "YYYY-MM or YYYY-MM-DD. Leave blank if unknown.",
  "employment.more": "Show more employment",
} as const;

export type MessageKey = keyof typeof en;
