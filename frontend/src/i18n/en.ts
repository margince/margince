// English catalog. Keys are the contract; de.ts must mirror them exactly
// (compile-time via satisfies, runtime via i18n.test.ts). Placeholders use
// {name} and are filled by t(key, params).
export const en = {
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
  "trust.typedByYou": "typed by you",
  "trust.typedByHuman": "typed by a person",
  "trust.typedByBuyer": "typed by a buyer",
  "trust.typedByPrefix": "typed by",
  "trust.sourceUnknown": "source not recorded",
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
  "trust.connectorTag": "via {connector}",
  "trust.dismissed": "Suggestion dismissed.",
  "trust.stagedProposal": "staged proposal",
  "trust.resolvedValue": "resolved value",
  "trust.editValue": "Edit {description}",
  "trust.evidenceFrom": "Evidence from {source}",
  "trust.evidenceLine_one": "line {lines}",
  "trust.evidenceLine_other": "lines {lines}",

  "history.created": "— created —",
  "history.oldValue": "Previous value",
  "history.newValue": "New value",
  "history.cleared": "— cleared —",
  "history.passport": "Agent passport",
  "history.empty": "No changes recorded",
  "history.fieldEmpty":
    "Set on create and never changed — the audit log records no edits. An empty history is honest, not a gap.",
  "history.filterEmpty": "No changes match this filter.",
  "history.clearFilter": "Clear filters",
  "history.allFields": "All fields",
  "history.actorAll": "All",
  "history.actorHuman": "Human",
  "history.actorAgent": "Agent",
  "history.tabChanges": "By change",
  "history.tabFields": "By field",
  "history.undo.action": "Put back",
  "history.undo.redo": "Redo",
  "history.undo.busy": "Putting this change back…",
  "history.undo.confirmTitle": "Put this change back?",
  "history.undo.confirmEdgeBody":
    "This changes the link with {other}. The records stay; only the connection between them changes.",
  "history.undo.confirmBody":
    "{count} fields go back to what they were before this change:",
  "history.undo.versionSkew":
    "The record moved while you were reading it. The history has been re-read — check the change again before putting it back.",
  "history.undo.noBeforeImage":
    "This change did not record what the record held before it, so there is nothing to put back.",
  "history.undo.notReplayable":
    "This kind of change is not replayed backwards.",
  "history.undo.unsupportedRecordType":
    "Changes to this kind of record cannot be put back.",
  "history.undo.superseded":
    "Somebody has changed these fields since. Putting this back would undo their decision as well.",
  "history.undo.behindErasureBoundary":
    "This change is behind an erasure, and what it held has been deleted for good.",
  "history.undo.alreadyUndone": "This change has already been put back.",
  "history.undo.notRestorableByThisPath":
    "These fields are not written through the path a restore uses.",
  "history.undo.recordArchived":
    "The record is archived. Restore the record itself before putting a change back.",
  "history.undo.nullUnwritable":
    "Putting this back would have to clear a field this record cannot clear, so it cannot be put back.",
  "history.undo.notWritableByCaller":
    "You do not have permission to write these fields.",
  "history.undo.edgeRelinkUnsupported":
    "Putting a removed link back isn't supported yet — add it again on this record.",
  "history.reversal.collapsed": "{actor}'s change, undone by {undoer}",
  "history.reversal.collapsedSelf": "{actor} undid their own change",
  "history.reversal.partly": "{actor}'s change, partly undone by {undoer}",
  "history.reversal.partlySelf": "{actor} partly undid their own change",
  "history.reversal.net": "net: unchanged",
  "history.reversal.stillChanged": "still changed",
  "history.reversal.expand": "Show both changes",
  "history.reversal.collapse": "Hide",
  "history.reversal.undoneBy": "undone by {undoer}",
  "history.reversal.unpaired": "undoing an earlier change",
  "history.edge.marker": "Link",
  "history.field.address": "Address",
  "history.field.amount_minor": "Value",
  "history.field.assignee_id": "Assignee",
  "history.field.body": "Notes",
  "history.field.emails": "Email addresses",
  "history.field.phones": "Phone numbers",
  "history.field.meeting_status": "Meeting outcome",
  "history.field.candidate_org_key": "Matched company",
  "history.field.company_name": "Company name",
  "history.field.currency": "Currency",
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
  "history.field.linkedin_url": "LinkedIn URL",
  "history.field.lost_reason": "Lost reason",
  "history.field.name": "Name",
  "history.field.occurred_at": "Occurred",
  "history.field.organization_id": "Company",
  "history.field.owner_id": "Owner",
  "history.field.parent_org_id": "Parent company",
  "history.field.partner_attribution": "Partner attribution",
  "history.field.partner_org_id": "Partner",
  "history.field.project_id": "Project",
  "history.field.relationship_types": "Relationship types",
  "history.field.remind_at": "Reminder",
  "history.field.score": "Score",
  "history.field.score_override_reason": "Score override reason",
  "history.field.size_band": "Size",
  "history.field.social": "Social profiles",
  "history.field.source": "Source",
  "history.field.started_at": "Started",
  "history.field.status": "Status",
  "history.field.subject": "Subject",
  "history.field.target_end_date": "Target end",
  "history.field.title": "Job title",
  "history.field.wait_until": "Waiting until",
  // What an empty stored jsonb array means to a reader of a change row —
  // never a blank, which reads as a value the row failed to show.
  "history.emptyList": "nothing set",

  "confidence.high": "high",
  "confidence.med": "medium",
  "confidence.low": "low",

  "autonomy.auto": "auto-execute",
  "autonomy.confirm": "confirm-first",

  "nav.home": "Brief",
  "nav.contacts": "People",
  "nav.companies": "Companies",
  "nav.leads": "Leads",
  "nav.deals": "Pipeline",
  "nav.today": "Worklist",
  // The decision lane, one at a time: how far through the reader is, and the
  // cleared plate the whole surface is built to reach.
  // The merge decision. Both values survive a merge — choosing a side decides
  // which record stands and which value is shown first — so the copy never says
  // "delete", because nothing is deleted.
  "nav.analytics": "Analytics",
  "nav.ai": "Ask Margince",
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
  "shell.searchEverything": "Search everything…",
  "shell.breadcrumbAria": "Breadcrumb",
  "shell.license.none": "No license",
  "shell.license.refused": "License refused",
  "shell.signOutAria": "Sign out",
  "shell.collapse": "Collapse sidebar",
  "shell.expand": "Expand sidebar",
  "shell.accountAria": "Account",
  "shell.theme": "Theme",
  "shell.more": "More",
  "shell.unknownPage": "Not found",
  "shell.closeMenu": "Close",
  // The sidebar's second level. The control READS one word at every depth; its
  // accessible name says where it leads, and the level of destinations needs a
  // name of its own to be led back to.
  "shell.navBack": "Back",
  "shell.navBackTo": "Back to {name}",
  "shell.navTop": "Destinations",
  // At phone width a section's entries are reached from the page head. The
  // control READS the entry it is on; the name says what pressing it does and
  // keeps that word inside itself (WCAG 2.5.3).
  "shell.sectionSwitch": "{name} — change section",
  "attention.selected": "{n} selected",
  "locale.name.en": "English",
  "locale.name.de": "Deutsch",
  "locale.name.vi": "Tiếng Việt",
  "locale.switchLabel": "Language",

  "screen.pending":
    "Not built yet — this surface arrives with its build ticket.",

  // The composed extension tier (ADR-0069): #/ext/<unit>. The registry is
  // generated per installation, so these two strings are the only part of a
  // unit surface the core catalogs own.
  "ext.notFound":
    "No extension named “{name}” is enabled on this installation.",
  "ext.operations": "Published operations",

  // The reference extension's own screen (#/ext/notes) carries no keys here:
  // a unit that ships a screen ships its copy with it, under
  // extensions/<unit>/frontend/i18n/, namespaced `ext<Unit>.` and merged into
  // the catalogue by gen-composition (see i18n/index.tsx).

  "search.placeholder":
    "Search people, companies, deals, projects, products, activities, leads…",
  "search.prompt": "Type what you are looking for.",
  "search.empty": "No matches for “{q}”.",
  "search.group.person": "People",
  "search.group.organization": "Organizations",
  "search.group.deal": "Deals",
  "search.group.project": "Projects",
  "search.group.product": "Products",
  "search.group.offerTemplate": "Offer templates",
  "search.group.activity": "Activities",
  "search.group.lead": "Leads",
  "search.group.tag": "Tags",
  "search.kind.person": "Person",
  "search.kind.organization": "Organization",
  "search.kind.deal": "Deal",
  "search.kind.project": "Project",
  "search.kind.product": "Product",
  "search.kind.offerTemplate": "Offer template",
  "search.kind.activity": "Activity",
  "search.kind.lead": "Lead",
  "search.kind.tag": "Tag",
  "search.filter.label": "Show only",
  "search.filter.all": "Everything",
  "search.pending": "Searching…",
  "search.tag.carriedBy": "On {count} records",
  "search.tier.mirrored": "from a connected system",
  "search.tier.unverified": "unverified",

  "context.title": "Related evidence",
  "context.empty": "Nothing related yet.",

  "palette.aria": "Command palette",
  "palette.placeholder": "Jump to, or ask anything…",
  "palette.empty": "No matches.",
  "palette.askAi": "Ask AI: \u201c{query}\u201d",
  "palette.typeScreen": "Screen",
  "palette.typeAction": "Action",
  "palette.typeRecord": "Record",
  "palette.seeAll": "See all results for “{query}”",
  "palette.searching": "Searching records…",
  "palette.searchFailed":
    "Records could not be searched just now. The commands above still work.",
  "action.newDeal": "New deal",
  "action.readCompany": "Read a company",
  "action.booking": "Booking page",

  "common.undo": "Undo",
  "common.close": "Close",

  "explain.open": "Explain this number",
  "explain.mayHaveMoved":
    "This link does not say when the number was worked out, so these figures were recalculated just now. If an exchange rate changed in between, they may not add up to the number you clicked.",
  "explain.title": "How this number is built",
  "explain.rate": "rate {rate} on {date}",

  "board.count": "{count} deals",
  "board.weighted": "weighted {value}",
  "board.mixedCurrencies": "several currencies — no single total",
  "dealfiles.hidden": "Hidden from this deal",
  "dealfiles.unhidden": "Shown on this deal again",
  "deal.stalled": "stalled",
  "deal.singleThreaded": "single-threaded",
  "deal.staged": "staged",
  "deal.archived": "archived",
  "record.notShown": "Not shown",
  "record.timelineLoading": "Loading this record’s history…",
  "record.chronologyLoading": "Reading the change history…",
  // The same word the tab strip uses (`tab.timeline`): the heading over the
  // slot and the tab that opens it name one thing, and two words for it read
  // as two things.
  "record.timeline": "History",
  "record.edit": "Edit",
  "record.save": "Save",
  "record.saveDone": "“{name}” saved",
  "record.archiveDone": "“{name}” archived",
  "record.archive": "Archive",
  "record.disqualify": "Disqualify",
  "record.archiveConfirm":
    "Are you sure? This archives the record — there is no undo control.",
  "record.archived": "Archived",
  "record.archivedReadOnly":
    "This company is archived. Restore it to change anything on it.",
  "record.notYoursToChange":
    "This company belongs to someone else. Ask its owner to share it with you if you need to make changes.",
  "record.logActivityRefused":
    "You do not have permission to log activities on this record.",
  "record.share": "Share",
  "record.moreActions": "More actions",
  "record.fullHistory": "Full history",

  "share.title": "Share this record",
  "share.ceiling.pre": "A grant changes who can see ",
  "share.ceiling.recordEmphasis": "exactly this one record",
  "share.ceiling.mid":
    " — nothing else about a person's scope moves. A share is capped at your own access, ",
  "share.ceiling.noWider": "no wider",
  "share.ceiling.post": ".",
  "share.unknownRecord": "This isn't a record that can be shared.",
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
    "Can open and read this record — cannot edit or send.",
  "share.access.writeNote":
    "Can open, edit, and add to this record — not change ownership or sharing.",
  "share.expiry": "Expiry",
  "share.expiry.none": "No expiry (until revoked)",
  "share.expiry.day": "Expires in 24 hours",
  "share.expiry.week": "Expires in 7 days",
  "share.expiry.month": "Expires in 30 days",
  "share.expiryConsequence_one":
    "Access auto-revokes in {days} day. You can revoke it sooner at any time.",
  "share.expiryConsequence_other":
    "Access auto-revokes in {days} days. You can revoke it sooner at any time.",
  "share.expiryConsequenceNone":
    "Access lasts until you revoke it — it will not end on its own.",
  "share.reason": "Reason",
  "share.grant": "Grant access",
  "share.update": "Update access",
  "share.unchanged":
    "Nothing changed. {name} already had {access} access to this record.",
  "share.downgradeTitle": "Reduce access?",
  "share.downgradeBody":
    "{name} has {from} access to this record. Continuing leaves them with {to} access only. Either direction is recorded in the audit trail.",
  "share.downgradeConfirm": "Reduce to {to}",
  "share.seatCeiling":
    "This seat is read-only, so it cannot hold write access to a record. Raise the seat first, or grant read.",
  "share.whoHasAccess": "Explicit shares",
  "share.grantedBy": "granted by",
  "share.revoke": "Revoke",
  "share.revokeConfirm":
    "Revoke this grant? The subject loses this record's access at the next request — there is no undo control.",
  "share.approvalRequired":
    "This share needs approval before it takes effect — it's queued for a decision, not applied yet.",
  "share.teamMembers_one": "Team · {count} member",
  "share.teamMembers_other": "Team · {count} members",
  "share.rosterLoading": "Loading people and teams…",
  "share.rosterErrorUsers":
    "Couldn't load the people list — teams are shown below.",
  "share.rosterErrorTeams":
    "Couldn't load the teams list — people are shown below.",
  "share.rosterErrorBoth": "Couldn't load people or teams.",
  "share.rosterEmpty": "No shareable people or teams found.",

  "edit.versionSkew":
    "This record changed since you opened it — reload and try again.",

  "merge.person": "Merge contact",
  "merge.org": "Merge company",
  "merge.searchPlaceholder": "Search…",
  "merge.pickTarget": "Select the surviving record",
  "merge.confirm": "Merge {source} into {target}? {source} will be archived.",
  "merge.submit": "Merge",

  "tab.overview": "Overview",
  "tab.relationships": "People & companies",
  "tab.partner": "Partner",
  "tab.rollup": "Roll-up",
  "tab.history": "History",

  "rollup.weightedPipeline": "Weighted pipeline",
  "rollup.closedWon": "Closed-won (current quarter)",
  "rollup.activity30d": "Activity (30d)",
  "rollup.accounts": "Aggregated accounts",
  "rollup.excluded": "{count} account(s) not visible to you were excluded",
  "rollup.fxUnavailable":
    "A currency conversion rate is missing — the roll-up cannot be computed.",
  "rollup.computedAt": "Computed at {when}",

  "nav.partners": "Partners",
  "deal.partnerSourced": "via",
  "deal.partnerInfluenced": "helped by",
  "deal.partnerAttribution": "What the partner did",
  "deal.attributionUnset": "Not specified — treated as brought us the deal",
  "deal.attributionSourced": "Brought us this deal (earns commission)",
  "deal.attributionInfluenced":
    "Helped on a deal we already had (no commission)",
  "partnerDeals.panelTitle": "Deals they brought",
  "partnerDeals.panelSub":
    "Deals at other companies that came through this partner",
  "partnerDeals.none": "No deals brought in yet",
  "partnerDeals.column.deal": "Deal",
  "partnerDeals.column.customer": "Customer",
  "partnerDeals.column.attribution": "Their part",
  "partnerDeals.column.amount": "Deal value",
  "partnerDeals.column.status": "Status",
  "commission.panelTitle": "Commission",
  "commission.panelSub": "What this partner has earned on deals they brought",
  "commission.none": "Nothing earned yet",
  "commission.column.deal": "Deal",
  "commission.column.amount": "Earned",
  "commission.column.rate": "Rate",
  "commission.column.basis": "Deal value",
  "commission.column.status": "Status",
  "commission.status.accrued": "Accrued",
  "commission.status.approved": "Approved",
  "commission.status.paid": "Paid",
  "commission.status.void": "Reversed",
  "commission.outstanding": "Still owed",
  "commission.column.actions": "Decision",
  "commission.decide.withheld": "Not yours to decide",
  "commission.decide.approve": "Approve",
  "commission.decide.pay": "Mark as paid",
  "commission.decide.void": "Reverse",
  "commission.decide.approveConfirm":
    "Approving records that this commission is agreed. It does not pay anything — settle the payment in your finance system, then mark it paid here.",
  "commission.decide.payConfirm":
    "Mark this as paid once your finance system has actually paid it. Margince records the fact; it does not move money.",
  "commission.decide.voidConfirm":
    "Reversing writes a cancelling row beside this one. Nothing is deleted, and the original stays readable.",
  "commission.decide.reasonLabel": "Why is it reversed?",
  "commission.decide.reasonRequired":
    "A reversal needs a reason — it is what explains the entry to the partner later.",
  "commission.decide.approved": "Commission approved",
  "commission.decide.paid": "Commission marked as paid",
  "commission.decide.voided": "Commission reversed",
  "commission.decide.settledElsewhere":
    "Paying happens in your finance system. This records what it did.",
  "partner.setup": "Make this a partner",
  "partner.edit": "Edit partner",
  "partner.none": "Not a partner yet",
  "partner.organization": "Organization",
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
  "partner.marginTier.tier2": "Active Collab (20%)",
  "partner.marginTier.tier3": "Partner closed (25%)",
  "partner.stage.research": "Research",
  "partner.stage.identified": "Identified",
  "partner.stage.contacted": "Contacted",
  "partner.stage.inConversation": "In conversation",
  "partner.stage.fitConfirmed": "Fit confirmed",
  "partner.stage.agreementPending": "Agreement pending",
  "partner.stage.active": "Active",
  "partner.stage.activeReferring": "Active — referring",
  "partner.stage.dormant": "Dormant",
  "partner.stage.noFit": "No fit",

  "rel.add": "Add relationship",
  "rel.addStakeholder": "Add stakeholder",
  "rel.dealStakeholders": "Stakeholders",
  "rel.dealStakeholdersEmpty": "No stakeholder is recorded on this deal",
  "rel.kind": "Kind",
  "rel.saveDone": "Relationship saved",
  "rel.role": "Role",
  "rel.startedAt": "Started",
  "rel.endedAt": "Ended",
  "rel.current": "current",
  "rel.endedOn": "until {when}",
  "rel.remove": "Remove",
  "rel.removeConfirm":
    "Are you sure? This removes the relationship — there is no undo control.",
  "rel.empty": "No relationships yet",
  "rel.counterparty": "Linked to",
  "rel.dates": "Dates",
  "rel.pickCounterparty": "Select the other side",
  "rel.addConfirm": "Add a {kind} link to {target}.",
  "rel.kind.employment": "Employment",
  "rel.kind.dealStakeholder": "Deal stakeholder",
  "rel.kind.projectStakeholder": "Project stakeholder",
  "rel.kind.projectCompany": "Company on project",
  "rel.kind.partnerOf": "Partner of",
  "rel.kind.referredBy": "Referred by",
  "rel.kind.coSellWith": "Co-sell with",
  "rel.kind.worksWith": "Works with",

  "common.error": "Couldn't load this view.",
  // What a failure that carries no server problem is allowed to say. A rejected
  // fetch and a bug in our own code both report in wording nobody authored for
  // a reader, so the screen states the fact it can stand behind and stops.
  "common.errorNoCause": "The request failed. No cause reported.",
  "common.assistantUnavailable":
    "The assistant did not answer, so it cannot draft this for you. An administrator can check the model binding under Settings → AI. Nothing here needs it — the details can be entered by hand.",
  "common.gatewayUnavailable":
    "The server did not finish this request in time. It may still be working — wait a moment before trying again, or the same work can run twice.",
  // Every 403 the server codes `permission_denied`, which is two refusals with
  // one name: a role that does not admit the action on this kind of record, and
  // a record the reader holds read-only through a share. Nothing on the wire
  // tells them apart, so the copy names neither and offers both ways out — an
  // admin widens a role, the person who shared a record widens the share. It
  // does not say the record may be missing: a row the reader may not see at all
  // comes back 404, so by the time this is read the record is one they may know
  // about. Two sentences, no dash (VOICE-RULE-5).
  "common.permissionDenied":
    "You do not have permission for this action. Ask an admin, or whoever shared this record with you, to widen your access.",
  // The licensing ceiling, decided before any role is consulted: a read seat is
  // refused every mutating request, so a reader whose role admits the action is
  // refused anyway. Names the SEAT rather than the reader, because the same code
  // answers a read seat's own request, an agent passport acting for one, and a
  // grant that would give a read seat write access. Two sentences, no dash
  // (VOICE-RULE-5).
  "common.seatReadOnly":
    "This seat is read-only, so the request was refused. Ask an operator to raise the seat.",
  "common.retry": "Retry",
  "common.empty": "Nothing here yet.",
  "common.saving": "Saving…",
  "common.loading": "Loading…",
  // A reference whose name READ failed, which is not the same as a record that
  // has no name: the id stays reachable through the title, and the reference
  // never becomes a link, because a name that did not load cannot be trusted as
  // a destination.
  "ref.nameLoadFailed": "Name didn't load",
  "ref.notInRoster": "Currently assigned (no longer in the user list)",

  // A record search that ANSWERED and found nothing. Distinct from a search
  // that has not run: both drew the same empty space before, and a reader
  // could not tell "nobody here" from a field still thinking.
  "picker.noMatch": "No match",

  // The app-level boundary's fallback. It says what happened and what to do
  // next, and nothing about the error itself: a render throw carries our own
  // internals, which the reader can neither read nor act on. Two sentences,
  // no dash (VOICE-RULE-5).
  "app.errorTitle": "This view stopped working.",
  "app.errorBody": "Try it again. If it keeps failing, reload the page.",
  "app.errorRetry": "Try again",

  // The card-level render boundary (design-system/cardboundary.tsx). It says
  // less than the app-level one because it has taken less: the page and its
  // navigation are still there, and only this card is gone.
  "card.errorTitle": "This card stopped working.",
  "card.errorRetry": "Try again",

  // The nine-state honesty vocabulary (design-system/surfacestate.tsx). These
  // words belong to the STATE and to no particular surface, which is why they
  // are keyed `state.*` rather than under any one screen — the same sentence
  // has to read correctly under a deal list and under a retention card. What
  // there is none OF stays the caller's word, passed as `emptyLabel`.
  "state.withheld": "Hidden — your role cannot read this",
  "state.unavailable":
    "Could not be loaded — this may not be the whole picture",
  "state.unsupported":
    "Not available in this mode — the connected system does not hold it",
  "state.failed": "This section did not load.",
  "state.loading": "Loading this section…",
  "state.retry": "Try again",
  "state.stale": "Last known values — not refreshed since",
  "state.staleAsOf": "Last known values, as of {when}",
  "state.partial": "Showing part of the list",
  "state.partialCount": "{count} more not shown",

  "list.headActions": "More actions",
  "list.search": "Search",
  "list.showArchived": "Show archived",
  "list.loadMore": "Load more",
  "list.viewAll": "All",
  "list.viewHot": "Hot",
  "list.overlayReadOnly":
    "Sorting and filters read through HubSpot — open it there",

  // The list surface (design-system/listtable.tsx). The count says "loaded"
  // rather than a total on purpose: paging is a keyset cursor, so the number
  // of rows in hand is the only figure the client can state honestly.
  "table.range": "{first}–{last} of {count} {unit}",
  "table.pagination": "Pages",
  "table.page": "Page {number}",
  "table.prev": "‹ Prev",
  "table.next": "Next ›",
  "table.rowsPerPage": "Rows per page",
  "table.perPage": "{count} per page",
  "table.sortedBy": "sorted by {column}",
  "table.columns": "Columns",
  "table.shownColumns": "Shown columns",
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
  "table.rangeLoaded": "{first}–{last} of {count} {unit} loaded so far",
  "unit.contacts": "people",
  "unit.companies": "companies",
  "unit.deals": "deals",
  "unit.leads": "leads",
  "unit.partners": "partners",
  "unit.products": "products",
  "unit.offerTemplates": "offer templates",
  "table.filter": "Filter",
  "table.filterSearch": "Search attributes",
  "table.addFilter": "Add a filter",
  "table.filterIs": "is",
  "table.filterCondition": "Condition",
  "table.filterMore": "More actions for the {filter} filter",
  "table.deleteFilter": "Delete filter",
  "table.filterValueSearch": "Search {filter} values",
  "table.filterTypeToSearch": "Type to search",
  "table.filterSearching": "Searching…",
  "table.filterSearchFailed": "The search failed. Try again.",
  "table.filterNoMatches": "No matches.",
  "overlay.unavailable":
    "Not available while reading from HubSpot — open it in HubSpot",
  "overlay.chipLabel": "Reading from HubSpot",
  "overlay.chipAria":
    "This installation reads records from a HubSpot mirror instead of native tables. Open Settings → Integrations to manage the connection.",
  "overlay.refused":
    "Not available while reading from HubSpot — the mirror can't serve this write.",
  "overlay.filterUnsupported":
    "This filter or sort isn't available while reading from HubSpot — remove it and try again.",
  "overlay.emptyOwnerHint":
    "An empty list here usually means the owner's HubSpot email doesn't match a user in this organization, not an empty HubSpot portal.",
  "overlay.partialWriteBack":
    "Only the fields HubSpot accepts are written back — anything else here, including custom fields and owner, is not applied at all; HubSpot's current value is kept.",

  "overlay.title": "HubSpot mirror",
  "overlay.sub":
    "Connect the organization's incumbent CRM so records read from its mirror instead of native tables.",
  "overlay.loading": "Loading the incumbent connection…",
  "overlay.notConfigured": "Overlay mode isn't configured in this deployment.",
  "overlay.loadFailed": "Couldn't load the incumbent connection.",
  "overlay.empty":
    "No incumbent is connected. Connect HubSpot to read records from its mirror.",
  "overlay.adminOnly":
    "You do not have permission to change the HubSpot connection.",
  "overlay.region": "Region",
  "overlay.regionEu1": "EU",
  "overlay.connectionLabel": "Connection",
  "overlay.notConnectedYet": "Not connected",
  "overlay.regionUs": "United States",
  "overlay.token": "Private-app token",
  "overlay.tokenHint": "Sealed into the vault; never shown again.",
  "overlay.connect": "Connect HubSpot",
  "overlay.reconnect": "Reconnect",
  "overlay.connectConfirmTitle": "Connect HubSpot for the whole organization?",
  "overlay.reconnectConfirmTitle":
    "Reconnect HubSpot for the whole organization?",
  "overlay.connectConfirmBody":
    "This switches every seat's reads to HubSpot's mirror immediately, and records become read-only wherever the mirror can't serve a write. This affects the whole installation, not just your own session.",
  "overlay.statusActive": "Connected",
  "overlay.statusRevoked": "Revoked",
  "overlay.statusError": "Sync error",
  "overlay.connectedAt": "Connected {at}",
  "overlay.syncTitle": "Mirror sync",
  "overlay.syncLoadFailed": "Couldn't load sync status.",
  "overlay.syncEmpty": "Nothing has synced yet.",
  "overlay.syncStateFresh": "Fresh",
  "overlay.syncStatePending": "Pending sync",
  "overlay.syncStateStale": "Stale",
  "overlay.backfillDone": "Backfill complete",
  "overlay.backfillPending": "Backfill in progress",
  "overlay.lastSynced": "Last synced {at}",
  "overlay.neverSynced": "Never synced",
  "overlay.budgetTitle": "API budget",
  "overlay.budgetLoadFailed": "Couldn't load the budget window.",
  "overlay.budgetHeadroom": "Headroom: {headroom}",
  "overlay.budgetUnmeasured":
    "The call budget cannot be measured right now, so live calls are paused as a precaution. This is not HubSpot quota pressure — the meter itself is not reporting.",
  "overlay.budgetEmpty":
    "The incumbent reported no budget window for this period.",
  "overlay.budgetSources":
    "Force-fresh {forceFresh} · Poller {poller} · Capture {capture}",
  "overlay.budgetSearch": "Search API: {consumed} / {limit} per second",
  "overlay.bandOk": "Healthy",
  "overlay.bandWarn": "Approaching limit",
  "overlay.bandShed": "Shedding load",
  "overlay.reconcile": "Sync now",
  "overlay.reconcileQueued":
    "Sweep queued — the worker picks it up on its next poll (about every 2 minutes).",
  "overlay.disconnect": "Disconnect",
  "overlay.disconnectTitle": "Disconnect HubSpot?",
  "overlay.disconnectBody":
    "This purges the mirrored data and switches the organization back to native records. The audit trail is kept.",

  "overlay.userMap.title": "Mirror user mapping",
  "overlay.userMap.sub":
    "Who each user in this organization is as a {principal} user. This mapping is the whole of their mirror visibility.",
  "overlay.userMap.cost":
    "A user with no mapping sees no mirrored records at all — their lists come back empty.",
  "overlay.userMap.loading": "Loading the user mapping…",
  "overlay.userMap.loadFailed": "Couldn't load the user mapping.",
  "overlay.userMap.adminOnly":
    "You do not have permission to review who is mapped.",
  "overlay.userMap.notOverlay":
    "This organization reads from native tables, so there is nothing to map.",
  "overlay.userMap.notConfigured":
    "Overlay mode isn't configured in this deployment.",
  "overlay.userMap.empty": "This organization has no users to map.",
  "overlay.userMap.view": "Grouping",
  "overlay.userMap.viewByUser": "By user",
  "overlay.userMap.viewByOwner": "By {principal} user",
  "overlay.userMap.principal.hubspot": "HubSpot",
  "overlay.userMap.principal.generic": "connected CRM",
  "overlay.userMap.you": "You",
  "overlay.userMap.matchEmail": "Matched by email",
  "overlay.userMap.matchManual": "Manual override",
  "overlay.userMap.map": "Map",
  "overlay.userMap.change": "Change",
  "overlay.userMap.unmap": "Unmap",
  "overlay.userMap.cancel": "Cancel",
  "overlay.userMap.pickerLabel": "Search {principal} users",
  "overlay.userMap.pickTitle": "Map to a {principal} user",
  "overlay.userMap.truncated":
    "The {principal} directory is longer than this list — someone you can't find here may be past the cut-off.",
  "overlay.userMap.directoryFailed":
    "Couldn't read the {principal} directory, so nobody can be picked right now.",
  "overlay.userMap.notMapped": "Not mapped",
  "overlay.userMap.chip.noEmailMatch": "No email match",
  "overlay.userMap.chip.ambiguousEmail": "Ambiguous email",
  "overlay.userMap.chip.blockedByAdmin": "Unmapped by an admin",
  "overlay.userMap.chip.notYetSynced": "Not synced yet",
  "overlay.userMap.chip.directoryUnavailable": "Reason unknown",
  "overlay.userMap.reason.noEmailMatch":
    "No {principal} user has this email address.",
  "overlay.userMap.reason.ambiguousEmail":
    "Two or more {principal} users share this email address, so no automatic match is safe.",
  "overlay.userMap.reason.blockedByAdmin":
    "An admin unmapped this user, and automatic matching will not map them again.",
  "overlay.userMap.reason.notYetSynced":
    "The {principal} directory hasn't listed this user yet.",
  "overlay.userMap.reason.directoryUnavailable":
    "Couldn't read the whole {principal} directory, so no reason can be derived.",
  "overlay.userMap.staleChip": "No longer in the {principal} directory",
  "overlay.userMap.staleNote":
    "This manual mapping grants no visibility. It is reported, never withdrawn automatically — the decision stays yours.",
  "overlay.userMap.unmapTitle": "Unmap this user?",
  "overlay.userMap.unmapSelfTitle": "Unmap yourself?",
  "overlay.userMap.unmapBody":
    "{user} will stop seeing every mirrored record until they are mapped again.",
  "overlay.userMap.unmapSelfBody":
    "You will stop seeing every mirrored record until you are mapped again. This tab stays reachable, so you can undo it here.",
  "overlay.userMap.sharedSeat": "Shared seat — {count} users",
  "overlay.userMap.ownerEmpty": "Nobody is mapped to a {principal} user yet.",
  "overlay.userMap.unmappedCount_one":
    "1 user is not mapped and isn't shown here — switch to By user to fix that.",
  "overlay.userMap.unmappedCount_other":
    "{count} users are not mapped and aren't shown here — switch to By user to fix that.",
  "overlay.userMap.partialView":
    "This grouping and count cover the users loaded so far. Load more to see the rest.",

  "people.name": "Name",
  "people.email": "Email",
  "list.owner": "Owner",
  "list.unowned": "Unassigned",
  "list.created": "Created",
  "list.lastActivity": "Last activity",
  "list.filterOwnerMe": "My records",
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
  "org.filterLifecycleAll": "Any stage",
  "org.filterRelTypeAll": "Any type",
  "org.filterSizeBandAll": "Any size",
  "person.consent": "Consent",
  "consent.grant": "Grant",
  "consent.withdraw": "Withdraw",
  "consent.doiBySubject":
    "This purpose is confirmed by the contact themselves, through a link mailed to their own address. Use \u201cAsk them to confirm their details\u201d below.",
  "consent.askToConfirm": "Ask them to confirm their details",
  "consent.askToConfirmWhat":
    "Mails this contact a private link to see what you hold about them, correct it, and say whether they want to hear from you. It goes to their own recorded address; you cannot send it anywhere else.",
  "consent.askQueued": "On its way to {address}.",
  "consent.askNotDelivered":
    "The link was created for {address} but this installation sends no mail, so nobody was sent it.",
  "consent.askExpires": "The link works until",
  "consent.noRecord": "no record",
  "consent.noPurposes": "This organization tracks no consent purposes yet.",
  "consent.defaultDeny":
    "Outbound is default-deny per purpose: a send is blocked unless an active, proven grant exists for that purpose. A grant for one purpose never authorizes another.",
  "consent.basis": "Basis: {basis}",
  "consent.proofLog": "Proof log",
  "consent.proofEmpty":
    "No consent decision recorded for this purpose. An empty log is honest, not a gap.",
  "consent.sourceUnknown": "source not recorded",
  "consent.actorHuman": "Human",
  "consent.actorAgent": "Agent",
  "consent.actorSystem": "System",
  "consent.actorConnector": "Connector",
  "consent.actorUnknown": "actor not recorded",
  "consent.purposesUnavailable":
    "Couldn't load the consent purpose catalogue, so which purposes need a double opt-in can't be shown right now.",

  "org.name": "Company",
  "org.description": "What they do",
  "org.website": "Website",
  "org.contactCount": "People",
  "org.openDealCount": "Open deals",
  // Offered only where there is no partner programme yet: the tab that holds
  // the form appears once one exists, so this is how the first one is made.
  // Where the account stands with us, and what it is to us — the two
  // questions the retired classification answered with one value.
  "org.lifecycle": "Account lifecycle",
  "org.relationshipTypes": "Relationship to us",
  "org.sizeBand": "Company size",
  "org.lifecycle.unknown": "Not assessed",
  "org.lifecycle.target": "Target",
  "org.lifecycle.prospect": "Prospect",
  "org.lifecycle.opportunity": "Opportunity",
  "org.lifecycle.customer": "Customer",
  "org.lifecycle.former_customer": "Former customer",
  "org.lifecycle.disqualified": "Disqualified",
  "org.relType.customer": "Customer",
  "org.relType.partner": "Partner",
  "org.relType.supplier": "Supplier",
  "org.relType.investor": "Investor",
  "org.relType.portfolio_company": "Portfolio company",
  "org.relType.competitor": "Competitor",
  "org.relType.other": "Other",
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
  "co.strip.title": "Where this account stands",
  "co.strip.convertedAsOf": "{count} converted, rates from {date}",
  "co.strip.noOpenDeals": "No open deals",
  "co.strip.pipeline": "Open pipeline",
  "co.description.label": "Description",
  "co.description.placeholder": "Add description",
  "co.strip.netInvoiced": "Net invoiced · 12 mo",
  "co.strip.notAssessed": "Not assessed",
  "co.strip.lifetimeOf": "{amount} lifetime",
  // The collapsed slot's label, shown once in place of the strip's money
  // readings when the connection cannot answer any of them. Generic on
  // purpose: it stands for all of them at once.
  "co.strip.finance": "Finance",
  "co.strip.financeUnknown": "—",
  "co.strip.open.deals": "Open deals",
  "co.strip.open.finance": "Open finance",
  "stat.evidence": "Evidence",
  "stat.evidence.rests": "What this rests on",
  "co.strip.fin.notACustomer": "Not a customer yet",
  "co.strip.fin.noConnection": "Connect your accounting",
  "co.strip.fin.unmapped": "Not matched to a customer yet",
  "co.strip.fin.syncing": "Syncing…",
  "co.strip.fin.withheld": "You may not see this account's finance",
  "co.strip.fin.staleFigure": "Last synced a while ago — check the date",
  "co.strip.fin.errorFigure": "Last sync failed — this may not be current",
  "co.strip.fin.nothingBilled": "Nothing invoiced yet",
  "co.strip.fin.error": "Could not be read",
  "co.strip.fin.loading": "Loading…",
  "co.strip.unpriced": "No convertible amount on these deals",
  "co.strip.pricedPartly": "{priced} of {total} deals priced",
  // "Conversation", not "Relationship": the health receipt beside this tile
  // rates a DIMENSION called Relationship (how many people are in contact),
  // and one word carrying two readings let the tile say "One-sided" while the
  // receipt said "Strong" about what read as the same thing. This tile is
  // about the correspondence.
  "co.strip.health": "Conversation",
  "co.strip.healthOneSided": "One-sided",
  "co.strip.healthBalanced": "Balanced",
  "co.strip.replyShare": "{percent}% of the exchange is theirs",
  "co.strip.healthActive": "In conversation",
  "co.strip.lastTouch": "Last touch",
  "co.strip.lastTouch.today": "Today",
  "co.strip.lastTouch.ago": "{count} d",
  "co.strip.lastTouch.theirs": "They wrote last",
  "co.strip.lastTouch.ours": "You wrote last",
  "co.strip.lastTouch.never": "No exchange yet",
  "co.strip.next": "Next",
  "co.strip.next.none": "Nothing scheduled",
  "co.strip.open.history": "Open history",
  "co.strip.open.tasks": "Open tasks",
  "co.360.thread": "What happened",
  "co.360.threadCount": "What happened · {count}",
  "co.360.fullHistory": "Full history",
  "co.strip.healthQuiet": "Gone quiet",
  "co.strip.noInboundEver": "They have never written",
  "co.strip.engagement.never_contacted": "Never contacted",
  "co.strip.engagement.active": "In conversation",
  "co.strip.engagement.waiting_on_them": "Waiting on them",
  "co.strip.engagement.waiting_on_us": "Waiting on us",
  "co.strip.engagement.dormant": "Gone quiet",
  "co.strip.openDeals": "{count} open",
  "co.strip.stalled": "{count} stalled",
  "co.suggest.act.draftReply": "Create draft",
  "co.suggest.act.openDeal": "Open the deal",
  "co.suggest.act.addTask": "Add the next step",
  // A conversation shown as one event says what it IS before what it
  // says: the reader is scanning for an event, not a sentence.
  "timeline.group.thread_other": "{count} messages",
  "timeline.group.thread_one": "{count} message",
  "timeline.group.bulk_other": "sent to {count} people",
  "timeline.group.bulk_one": "sent to {count} person",
  "timeline.group.expand": "Open",
  "timeline.group.collapse": "Close",
  "timeline.group.openThread": "View the whole thread",
  "timeline.group.mayContinue": "may continue earlier",
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
    "Conversations whose content you may not open are left out of a search.",
  "tab.people": "People",
  "tab.deals": "Deals",
  "tab.tasks": "Tasks",
  "tab.timeline": "History",
  "tab.finance": "Finance",
  "tab.network": "Network",
  "tab.documents": "Documents",
  "tab.profile": "Profile",
  "tab.meetings": "Meetings",
  "tab.research": "Data & tools",
  // The brief under the questions it answers, and what kind of claim each
  // sentence makes — a judgment must not read as a stored fact.
  "co.brief.nature.fact": "Fact",
  "co.brief.nature.assessment": "Our read",
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
    "Whether the people here are still in touch — who wrote, how recently, and which side started it.",
  "co.health.means.commercial":
    "Whether the work in flight is moving — the open deals, the stage they sit at, and how long they have stood still.",
  "co.health.means.payment":
    "Whether invoices are settled on time — what is overdue now, and how late this account usually runs.",
  "co.health.rating.atRisk": "At risk",
  "co.health.rating.good": "Good",
  "co.health.rating.strong": "Strong",
  "co.health.payment.overdue": "Money is overdue right now.",
  "co.health.payment.late": "Typically pays {days} days after due.",
  "co.health.payment.onTime": "Pays on time.",
  "co.health.sinceInbound": "They last wrote {days} days ago",
  "org.partnerSetUp": "Set up partner programme",
  "signal.kind.stalled_deal": "Deal stalled",
  "signal.kind.champion_left": "Champion left",
  "signal.kind.reengagement": "Worth re-engaging",
  "signal.kind.buying_intent": "Buying intent",
  "signal.kind.risk": "Risk",
  "signal.kind.other": "Other",
  "signal.kind.contract_ended": "Contract ending",
  "signal.kind.new_opportunity": "New opportunity",
  "signal.kind.commitment_made": "Something was promised",
  "signal.kind.ghosted_thread": "No answer",
  "signal.kind.project_gone_quiet": "Project gone quiet",
  "signal.kind.funding": "Funding",
  "signal.kind.leadership_change": "Leadership change",
  "signal.kind.expansion": "Expansion",
  "signal.kind.product_launch": "Product launch",
  "co.routeIn.band.strong": "in regular contact",
  "co.routeIn.band.some": "some contact",
  "co.routeIn.band.faint": "barely in contact",
  "co.routeIn.band.unknown": "contact on file, no pattern yet",
  "record.profile": "Profile",
  "record.context": "Context",
  "record.restsOn": "What this rests on",
  "record.restsOn.source_one": "source",
  "record.restsOn.source_other": "sources",
  "record.tabs": "Parts of this record",
  "record.panel.details": "Details",
  // The Deal Room aside on a deal. `room.` rather than `dealroom.` for the
  // reason the other abbreviated namespaces give: the surface is named once at
  // the top of the panel and every key under it is read in that context.
  "room.editorial":
    "A document you add is shared straight away, and comments are immediate.",
  "room.readOnly": "You can read this room but not change it.",
  "room.finished": "This room is finished, so what it shared is now a record.",
  "room.card.title": "Deal Room",
  "room.card.people": "{invited} invited · {active} signed in",
  "room.card.lastSeen": "Last seen by a buyer: {when}",
  "room.card.open": "Open the Deal Room",
  "room.create.sub":
    "A space the buyer enters by link to read what you share and discuss it.",
  "room.create.open": "Open a Deal Room",
  "room.create.confirm": "Open",
  "room.create.titleLabel": "Room title",
  "room.create.titleHint":
    "What the buyer sees as the heading. You can change it later.",
  "room.create.defaultTitle": "{deal}",
  "roompage.none":
    "This deal has no Deal Room yet. Open one from the deal page.",
  "roompage.backToDeal": "← Back to the deal",
  "roompage.accessMenu": "Room access",
  "roompage.pause": "Pause",
  "roompage.pauseHint":
    "Buyers keep their links but see a paused page until you resume.",
  "roompage.resume": "Resume",
  "roompage.close": "Close the room",
  "roompage.closeHint": "Buyers keep reading; nothing can be added or said.",
  "roompage.setExpiry": "Set an end date",
  "roompage.setExpiryHint": "Access stops on that day.",
  "roompage.closeTitle": "Close this Deal Room?",
  "roompage.closeBody":
    "Buyers keep reading the room. No document, comment or decision is accepted afterwards. You can still revoke people and issue links.",
  "roompage.expiryLabel": "Access ends on",
  "roompage.expiryHint": "Leave empty for no end date.",
  "roompage.banner.paused":
    "Paused. Buyers see a paused page until you resume.",
  "roompage.banner.closed":
    "Closed. Buyers can still read the room; nothing more is accepted.",
  "roompage.banner.expired": "Expired. Buyer links no longer work.",
  "roompage.banner.archived": "Archived. Nobody can enter this room.",
  "roompage.banner.liveUntil": "Live. Access ends on {when}.",
  "roompage.text.title": "Title and welcome",
  "roompage.text.sub": "What the buyer reads first. They see it straight away.",
  "roompage.text.titleLabel": "Room title",
  "roompage.text.welcomeLabel": "Welcome message",
  "roompage.viewAsBuyer": "View as buyer",
  "roompage.previewArchived": "An archived room has nothing to preview.",
  "access.title": "Access",
  "access.sub": "Who may enter, and what each person may do.",
  "access.invite": "Invite",
  "access.empty": "Nobody has been invited yet.",
  "access.cap.view": "Read only",
  "access.cap.viewHint": "Can read the documents and the conversation.",
  "access.cap.comment": "Read and comment",
  "access.cap.commentHint": "Can also ask questions and reply.",
  "access.state.invited": "invited",
  "access.state.active": "signed in",
  "access.state.revoked": "revoked",
  "access.lastSeen": "last seen {when}",
  "access.downloads": "Downloaded {count} document(s)",
  "access.linkRequested":
    "Asked for a new link {when}. Issue one and send it yourself.",
  "access.rowActions": "Actions for {name}",
  "access.issueLink": "Issue new link",
  "access.changeCapability": "Change what they may do",
  "access.revoke": "Revoke access",
  "access.inviteTitle": "Invite someone to the Deal Room",
  "access.inviteConfirm": "Invite",
  "access.done": "Done",
  "access.save": "Save",
  "access.nameLabel": "Name",
  "access.emailLabel": "Email",
  "access.capabilityLegend": "What may they do?",
  "access.inviteNote":
    "You will get the link to copy. If a mail relay is configured it is also sent to them.",
  "access.issued.title": "Link for {name}",
  "access.issued.mailed": "Sent to {email}. You can also copy it below.",
  "access.issued.notMailed":
    "No mail was sent. Copy the link and send it yourself.",
  "access.issued.linkLabel": "Their link",
  "access.issued.copy": "Copy link",
  "access.issued.copied": "Copied",
  "access.issued.copyFailed": "Could not copy; select the link and copy it.",
  "access.issued.oneTime":
    "Personal, one-time link. It works once, on one device. Each person needs their own invitation.",
  "access.issueLinkTitle": "Issue a new link for {name}",
  "access.issueLinkBody":
    "The link they have now stops working. You will get the new one to copy.",
  "access.revokeTitle": "Revoke access for {name}?",
  "access.neverSignedIn": "never signed in",
  "access.revokeBody":
    "Their session ends now and their link stops working. Their comments stay visible and attributed. Access cannot be restored by them asking for a link.",
  "access.changeCapabilityTitle": "What may {name} do?",
  "persondealrooms.title": "Deal Rooms",
  "persondealrooms.sub": "Rooms this contact can still enter.",
  "persondealrooms.open": "Open",
  "persondealrooms.seatGone":
    "This address no longer holds a seat in that room.",
  "persondealrooms.cut":
    "Only the first rooms are shown; this contact sits in more.",
  "persondealrooms.revokeTitle": "Revoke access to {room}?",
  "room.state.draft": "Draft",
  "room.state.building": "Building",
  "room.state.ready": "Ready",
  "room.state.publishing": "Publishing",
  "room.state.live": "Live",
  "room.state.paused": "Paused",
  "room.state.closed": "Closed",
  "room.state.expired": "Expired",
  "room.state.archived": "Archived",
  "co.pulse.created": "Created {when}",
  // The later of the two directions \u2014 which side wrote last moved to the
  // daily brief's own detail line, so the header states only that the
  // relationship is or is not live.
  "co.pulse.owner": "Owner",
  "co.pulse.sizeBand": "{band} employees",
  "co.pulse.strongestLead": "Way in",
  "co.pulse.strengthTail_one": "\u2014 the only contact here",
  "co.pulse.strengthTail_other": "\u2014 of {count} contacts here",
  "co.pulse.unowned": "Unassigned",
  "co.since.first": "You are opening this account for the first time.",
  "co.partial":
    "Some of this page could not be loaded, so it may not show everything on this account.",
  "evidence.explain": 'Where "{value}" came from',
  "evidence.fullHistory": "Full history",
  "co.section.unavailable":
    "Could not be loaded — this may not be the whole picture",
  "finance.title": "Finance",
  "finance.titleHistorical": "Finance · historical",
  "finance.none": "Nothing recorded.",
  "finance.loading": "Reading the invoices…",
  "finance.syncing":
    "Syncing with your accounting source. Figures appear once the first sweep lands.",
  "finance.noConnection":
    "No financial source connected — connect one to see what this customer has been invoiced and whether they pay on time",
  "finance.unmapped":
    "Connected, but this company is not matched to a customer in the accounting system yet",
  "finance.netInvoiced": "Net invoiced · 12 months",
  "finance.overdue": "Overdue",
  "finance.behaviour": "Payment behaviour",
  "finance.behaviourShape": "Days late per settled invoice, oldest first",
  "finance.shareOfOpen": "{percent}% of everything open",
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
  "finance.status.partiallyPaid": "Part paid",
  "finance.status.paid": "Paid",
  "finance.status.overdue": "Overdue",
  "finance.status.disputed": "Disputed",
  "finance.status.credited": "Credited",
  "finance.status.void": "Void",
  "commercial.closes": "closes {when}",
  "contracts.title": "Contracts",
  "contracts.empty": "No agreements on record",
  "contracts.noneActive": "No agreement is active today",
  "contracts.filter.all": "All",
  "contracts.filter.active": "Active",
  "contracts.status.draft": "Draft",
  "contracts.status.active": "Active",
  "contracts.status.expired": "Expired",
  "contracts.status.cancelled": "Cancelled",
  "contracts.status.superseded": "Superseded",
  "contracts.endsOn": "Ends {when}",
  "contracts.renewsOn": "Renews {when}",
  "contracts.endedPendingStatus": "Term ended \u2014 status change pending",
  "contracts.form.title": "Record an agreement",
  "contracts.form.name": "Title",
  "contracts.form.number": "Contract number",
  "contracts.form.value": "Value",
  "contracts.form.basis": "This value is",
  "contracts.basis.total": "the total for the whole term",
  "contracts.basis.annual": "twelve months of an open-ended agreement",
  "contracts.form.startsOn": "Starts",
  "contracts.form.endsOn": "Ends",
  "contracts.form.endsOnHint": "Leave empty for an open-ended agreement.",
  "contracts.form.renewalOn": "Renews",
  "contracts.form.noticeDays": "Notice period (days)",
  "contracts.form.noticeDaysHint":
    "How much notice a cancellation needs. The renewal warning fires before this deadline, not before the renewal date.",
  "contracts.form.signedOn": "Signed",
  "contracts.form.signedOnHint":
    "Only when a human knows it was signed \u2014 never taken from a deal's close date.",
  "contracts.form.save": "Record agreement",
  "contracts.form.errNoName": "An agreement needs a title.",
  "contracts.form.errTermOrder": "A term cannot end before it starts.",
  "contracts.add": "Add a contract",
  "contracts.rowMenu": "Contract actions",
  "contracts.renew.title": "Renew this agreement",
  "contracts.renew.hint":
    "Creates a new agreement and marks this one superseded. Its own terms — nothing carries over but the counterparty.",
  "contracts.renew.submit": "Renew",
  "contracts.statusChange.title": "Change status",
  "contracts.statusChange.label": "New status",
  "contracts.statusChange.submit": "Change status",
  "contracts.statusChange.errSame": "Already at this status.",
  "contracts.cancel.title": "Record cancellation",
  "contracts.cancel.hint":
    "The customer stays under contract until the effective date — this records notice, not a state change.",
  "contracts.cancel.noticeOn": "Notice given",
  "contracts.cancel.effectiveOn": "Takes effect",
  "contracts.cancel.effectiveOnHint":
    "Not after the term ends, and not before the notice date.",
  "contracts.cancel.submit": "Record cancellation",
  "contracts.cancel.menuLabel": "Cancel agreement",
  "contracts.cancel.errIncomplete": "Both dates are needed.",
  "contracts.cancel.errOrder":
    "Cancellation cannot take effect before notice was given.",
  "contracts.cancel.errTermEnd":
    "Cancellation cannot take effect after the term already ends.",
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
    "\u201c{title}\u201d leaves the lists and the account totals. The record and its history stay, so what was true stays answerable \u2014 nothing is deleted.",
  "contracts.archive.confirm": "Archive",
  "contracts.form.editTitle": "Edit contract",
  "contracts.form.saveEdit": "Save changes",
  "contracts.form.file": "Signed document",
  "contracts.form.fileHint":
    "Drop the signed PDF here, or click to choose one. It is filed against this contract and appears in the account's documents.",
  "contracts.form.fileEmpty": "Drop a file here or click to choose",
  "contracts.form.fileAdd": "Drop another file here or click to choose",
  "contracts.perYear": "{amount} / year",
  "contracts.state.title": "Under contract · {count} active",
  "contracts.state.none": "No contract on record",
  "contracts.state.renewsOn": "Renews {when}",
  "contracts.state.endsOn": "Cancelled — ends {when}",
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
  "co.section.restricted": "Hidden \u2014 your role cannot read this",
  // The same word the tab strip uses (`tab.tasks`): a tab called Tasks
  // opening a card called Next steps reads as a wrong turn.
  "co.next.title": "Tasks",
  "co.next.empty": "No open task on this account.",
  "co.next.overdue": "Overdue",
  "co.next.due": "Due {when}",
  "co.next.undated": "No date",
  "co.facts.pipeline": "Open pipeline",
  "co.facts.inFlight": "In flight",
  "co.facts.reading": "Reading\u2026",
  "co.facts.noDeals": "No open deals",
  "co.facts.unpriced": "Not priced yet",
  "co.facts.nothing": "Nothing",
  "co.facts.deals_one": "1 deal",
  "co.facts.deals_other": "{count} deals",
  "co.facts.projects_one": "1 project",
  "co.facts.projects_other": "{count} projects",
  "co.facts.atLeast": "or more",
  "co.work.title": "What is in flight, and why",
  "co.work.count": "{count} in flight",
  "co.work.countAtLeast": "{count}+ in flight",
  "co.work.deals": "Deals",
  "co.work.noDealsDetail":
    "A deal is where the money and the close date live. Open one when there is something to win.",
  "co.work.noDeals": "No open deals.",
  "co.work.closes": "closes {date}",
  "co.work.stalled":
    "Nothing has been filed against this deal in the last 60 days.",
  "co.work.overdueTask":
    "{who} was supposed to \u2018{title}\u2019 by {date} and has not.",
  "co.work.overdueTaskUnnamed":
    "\u2018{title}\u2019 was due {date} and is still open.",
  "co.work.owesUs": "{who} said: \u2018{body}\u2019",
  "co.work.owesUsUnnamed": "They said: \u2018{body}\u2019",
  "co.work.wasDue": "\u2014 by {date}.",
  "co.work.statusesWithheld":
    "You cannot read this account\u2019s conversations, so the rows above carry no reasons.",
  "co.brief.by.model": "Written by Margince",
  "co.brief.by.deterministic": "Assembled from your records",
  "co.brief.generatedAt": "as of {when}",
  "co.growthFit.title": "What they are worth to you",
  "co.growthFit.unavailable":
    "This assessment could not be read. Nothing about the company has changed.",
  "co.growthFit.assembling":
    "Working out what this account is worth — the first assessment reads the record and takes a moment.",
  "co.growthFit.reassess": "Assess it again",
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
  "co.growthFit.positive": "Argues for",
  "co.growthFit.negative": "Argues against",
  "co.growthFit.whitespace": "Room to sell",
  "co.growthFit.objections": "Likely pushback",
  "co.growthFit.angle": "Suggested approach",
  "co.dossier.title": "What this company is",
  "co.dossier.unavailable":
    "This description could not be read. Nothing about the company has changed.",
  "co.dossier.empty":
    "Nothing has been recorded about this company yet. Read their website, or fill in the profile below.",
  "co.dossier.stale": "Read over a month ago",
  "co.dossier.rewrite": "Write it again",
  "co.dossier.rewriting": "Writing…",
  "co.dossier.section.summary": "In short",
  "co.dossier.section.products_services": "What they sell",
  "co.dossier.section.markets": "Where and to whom",
  "co.dossier.section.buying_center": "Who decides",
  "co.dossier.section.differentiation": "What sets them apart",
  "co.dossier.section.firmographics": "Size, age and registration",
  "co.evidence.unavailable":
    "This receipt could not be read. The record itself is unchanged.",
  "co.evidence.producedBy": "recorded by {who}",
  "co.evidence.retrievedAt": "Read {when}",
  "co.evidence.verifiedAt": "Confirmed by a person {when}",
  "co.evidence.confidence": "The model was {percent}% confident",
  "co.evidence.gaps": "Not recorded: {fields}.",
  "co.evidence.kind.site_read": "Read from their website",
  "co.evidence.kind.connector": "From a connected system",
  "co.evidence.kind.human": "Entered by a person",
  "co.evidence.kind.migration": "Imported",
  "co.evidence.kind.rule": "Derived",
  "co.brief.cite.deal": "deal",
  "co.brief.cite.activity": "activity",
  "co.brief.cite.person": "contact",
  "co.brief.cite.organization": "account",
  "co.brief.cite.fact": "fact",
  "co.brief.cite.profile_field": "profile field",
  // Several sources of one kind that have no screen to open collapse into one
  // counted chip, rather than a run of identical labels.
  "co.brief.cite.deal.many": "{count} deals",
  "co.brief.cite.activity.many": "{count} activities",
  "co.brief.cite.person.many": "{count} contacts",
  "co.brief.cite.organization.many": "{count} accounts",
  "co.brief.cite.fact.many": "{count} facts",
  "co.brief.cite.profile_field.many": "{count} profile fields",
  "approval.kind.advance_deal": "Move a deal forward",
  "approval.kind.close_date_correction": "Correct a close date",
  "approval.kind.deal_follow_up": "Add a follow-up on a deal",
  "approval.kind.promote_lead": "Promote a lead",
  "approval.kind.archive_record": "Archive a record",
  "approval.kind.merge_records": "Merge two records",
  "approval.kind.merge_tags": "Fold one tag into another",
  "approval.kind.update_record": "Update a record",
  "approval.kind.create_record": "Create a record",
  "approval.kind.send_email": "Send an email",
  "approval.kind.held_draft": "Review a drafted email",
  "approval.kind.book_meeting": "Book a meeting",
  "approval.kind.volume_release": "Let an agent continue",
  "approval.kind.coldstart": "Fill in a new account",
  "approval.kind.enrich": "Enrich from the web",
  "approval.kind.deepread": "Read the company site",
  "approval.kind.linkedin_match": "LinkedIn match",
  "approval.kind.site_lead": "Add a person found on the site",
  "approval.kind.capture_counterparty": "Add someone from your mail",
  "approval.kind.org_name_promotion": "Rename an account",
  "approval.kind.vcard_create": "Create a contact from a card",
  "approval.kind.lifecycle_change": "Account stage",
  "approval.kind.transcript_proposal": "Add a next step from a transcript",
  "approval.kind.fx_rate_proposal": "Refresh exchange rates",
  "approval.kind.ai_model_rate_proposal": "Refresh model prices",
  "approval.kind.disqualify_lead": "Disqualify a lead",
  "approval.kind.advance_project_phase": "Move a project to its next phase",
  "approval.kind.assign_owner": "Hand a record to an owner",
  "approval.kind.commit_import": "Commit an import",
  "approval.kind.emit_flow_event": "Record an automation step",
  "approval.kind.relink_activity": "Refile an activity",
  "approval.kind.relink_thread": "Refile a conversation",
  "approval.kind.relink_activities": "Refile several activities",
  "approval.kind.scheduled_send_held": "Release a stopped message",
  "approval.kind.send_account_email": "Send an email to an account",
  "approval.kind.send_message": "Send a message",
  // What a staged proposal's own fields are CALLED. Without these a card falls
  // back to the payload's JSON keys, and a business question reads as a
  // database row.
  "approval.field.basis": "Why",
  "approval.field.because": "Why",
  "approval.field.step": "The step",
  "approval.field.intent": "Why this was drafted",
  "approval.field.evidence_snippet": "What the page said",
  "approval.field.previous_close_date": "Date on it now",
  "approval.field.expected_close_date": "Proposed date",
  "approval.field.due_date": "Due",
  "approval.field.scheduled_at": "Was going out",
  "approval.field.flags": "What is wrong with it",
  "approval.field.closeDateFlag.overdue": "the date has passed",
  "approval.field.closeDateFlag.missing": "no date at all",
  "approval.field.closeDateFlag.unrealistic_soon": "too soon to be real",
  "approval.field.closeDateFlag.unrealistic_stale": "nothing has moved on it",
  "approval.field.name": "Name",
  "approval.field.role": "Role",
  "approval.field.email": "Email",
  "approval.field.domain": "Domain",
  "approval.field.company": "Company",
  "approval.field.title": "Job title",
  "approval.field.phone": "Phone",
  "approval.field.url": "Website",
  "approval.field.address": "Address",
  "approval.field.published_email": "Email on the page",
  "approval.field.connection_name": "On LinkedIn",
  "approval.field.connection_company": "Works at",
  "approval.field.person_name": "Contact here",
  "approval.field.owner": "Owner",
  "approval.field.to": "To",
  "approval.field.currency": "Currency",
  "approval.field.rate": "New rate",
  "approval.field.prior_rate": "Rate now",
  "approval.field.provider": "Provider",
  "approval.field.model": "Model",
  "approval.field.input_per_mtok": "Input, per million tokens",
  "approval.field.output_per_mtok": "Output, per million tokens",
  "approval.field.tool": "What it was doing",
  "approval.field.observed": "Used",
  "approval.field.limit": "Limit",
  "approval.field.allowance": "Asking for",
  "co.assistant.title": "Ask about this account",
  "co.assistant.aiTag": "AI-assisted",
  "co.decisions.open": "Review {count} waiting",
  "co.decisions.title": "Decisions waiting",
  "co.decisions.group": "{count} × {kind}",
  "co.decisions.empty": "Nothing is waiting on a decision here.",
  "co.ask.title": "Ask Margince",
  "co.ask.q.whats_open": "What's open here?",
  "co.ask.q.meeting_prep": "Prep me for a meeting",
  "co.ask.q.whats_changed": "What's changed recently?",
  "co.ask.nothing": "Nothing here that you can see would answer that.",
  "co.ask.failed": "That question could not be answered — try it again.",
  "co.suggest.title": "Margince suggests",
  "co.suggest.kind.no_reply": "No reply",
  "co.suggest.kind.stalled_deal": "Stalled deal",
  "co.suggest.kind.no_next_step": "Nothing scheduled",
  "co.suggest.kind.lifecycle_conflict": "Record disagrees",
  "co.suggest.more": "{count} more not shown here.",
  "co.suggest.basedOn": "What this is based on",
  "co.suggest.dismiss": "Not now",
  "co.suggest.byline": "Margince suggests",
  "co.suggest.dismissFailed":
    "That could not be dismissed — it is still showing for you",
  "co.suggest.viewTasks": "View tasks",
  "co.suggest.commitment.overdueCount": "{count} overdue",
  "co.suggest.commitment.openCount": "{count} open",
  "co.suggest.commitment.overdueAtLeast": "{count}+ overdue",
  "co.suggest.commitment.openAtLeast": "{count}+ open",
  "co.deals.title": "Deals",
  "co.deals.empty": "No open deal on this account.",
  "co.deals.wonLifetime": "Won to date",
  "co.deals.lostCount": "{count} lost",
  "co.deals.noStage": "No stage",
  "co.rail.all": "All {count}",
  "co.rail.add": "Add",
  "co.rail.allUncounted": "All",
  "co.rail.more": "More",
  "co.rail.deals.title": "Active deals",
  "co.rail.deals.empty": "No deals on this account yet.",
  "co.rail.deals.emptyClosedOnly": "Nothing open — only closed history.",
  "co.rail.deals.noCloseDate": "no close date",
  "co.rail.deals.attentionOverdue": "Overdue",
  "co.rail.deals.attentionCommitment": "They owe us",
  "co.rail.people.title": "Their key people",
  "co.rail.people.empty": "No contacts yet. Nobody to write to.",
  "co.rail.people.add": "Add a contact",
  "co.rail.people.inTouch": "Already in touch with them",
  "co.rail.details.all": "All fields",
  "co.commercial.title": "Commercial",
  "co.commercial.lostFigure": "Lost deals",
  "co.commercial.allDeals": "All deals",
  "co.commercial.truncated":
    "This account has more open deals than fit here. Open All deals to see the rest.",
  "linkedinImport.title": "LinkedIn connections",
  "linkedinImport.sub":
    "Import your own export to see who your team already knows",
  "linkedinImport.profileLabel": "Your LinkedIn profile URL",
  "linkedinImport.profilePlaceholder": "https://www.linkedin.com/in/…",
  "linkedinImport.saveProfile": "Save profile",
  "linkedinImport.editProfile": "Edit",
  "linkedinImport.editProfileTitle": "Your LinkedIn profile",
  "linkedinImport.profileNotSet": "Not recorded yet",
  "linkedinImport.connectedNote":
    "Connected. Imported connections are attributed to this profile, so the CRM can say which colleague knows someone rather than that \u201cthe company\u201d does.",
  "linkedinImport.notConnectedNote":
    "Recording your profile URL attributes any connections you import to you by name.",
  "linkedinImport.whichFile":
    "LinkedIn gives you Connections.csv under Settings \u2192 Data privacy \u2192 Get a copy of your data; the archive holds a dozen others, and this is the one. What you upload never becomes contacts: the connections stay out of search, lists and contact pages, and nobody can write to or email them.",
  "linkedinImport.choose": "Choose Connections.csv",
  "linkedinImport.importLabel": "Connections export",
  "linkedinImport.noMatchesYet":
    "No matches yet, which is normal in a new organization: your connections are matched against contacts the CRM knows, and those arrive as your mail is read. This runs again every hour, so matches appear as the CRM fills up.",
  "linkedinImport.working": "Reading your export…",
  "linkedinImport.imported": "Connections imported",
  "linkedinImport.confirmed": "Matched to a contact",
  "linkedinImport.suggested": "Awaiting your confirmation",

  // The review queue and the reach table (ADR-0078 §2.1b).
  "linkedinReach.title": "Where your network reaches",
  "linkedinReach.sub":
    "Accounts on file where you already know somebody, most connections first.",
  "linkedinReach.empty":
    "None of your connections work at an account on file yet.",
  "linkedinReach.allUnresolved":
    "All {unresolved} of your connections work somewhere that is not an account on file yet.",
  "linkedinReach.accountsLabel": "Accounts you reach",
  "linkedinReach.account": "Account",
  "linkedinReach.connections": "You know",
  "linkedinReach.onFile": "Already contacts",
  "linkedinReach.onFileOf": "{onFile} of {total}",
  "linkedinReach.footnote":
    "Showing {shown} of {total} accounts. {unresolved} connections work somewhere that is not an account on file yet.",
  "linkedinImport.skipped": "Rows skipped (no usable name)",
  "co.signals.title": "Margince also spotted",
  "co.signals.emptyDetail":
    "Margince reads meetings, mail and invoices for promises, blockers and risks. It needs at least one of those first.",
  "co.signals.empty": "No open signal on this account.",
  "co.signals.openProject": "Open the project",
  "co.signals.openSource": "Read the announcement",
  "chronology.label": "What to show in the timeline",
  "chronology.activities": "Activities",
  "chronology.changes": "Changes",
  "filter.label": "Narrow this list",
  "chronology.all": "All",
  "chronology.conversations": "Conversations",
  "chronology.conversationsEmpty": "No conversations with them yet.",
  "convo.yourMove": "Your move",
  "convo.waitingOnThem": "Waiting on them",
  "chronology.changesEmpty":
    "No field on this record has been changed since it was created.",
  "chronology.allEmpty": "Nothing has happened on this record yet.",
  "chronology.truncated":
    "Older entries are not shown here — there are more of both kinds than this view can put in order. Pick Activities or Changes to read further back.",
  "chronology.truncatedActivities":
    "There are more activities here than fit. Only the most recent ones are listed.",
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
    "Mail to {addresses} is bouncing. The last delivery there was refused, and no delivery since has got through. Send anyway, or use another address.",
  "compose.threadShare": "Share thread",
  "compose.threadKeepPrivate": "Keep private",
  "compose.threadStillHeld":
    "Still held: {count} other seat(s) on this thread have not shared it.",
  "compose.reason.posture": "Held by your setting",
  "compose.reason.workspaceFloor": "Held by the organization",
  "compose.reason.noRecord": "Held, no record",
  "compose.reason.pendingVerdict": "Held until classified",
  "compose.reason.manual": "Kept private",
  "compose.reason.verdict": "Held by a classification",
  "compose.reason.counterparty": "Held, mail with this party",
  "compose.reason.explicitlyConfidential": "Marked confidential",
  "compose.reason.noCounterparty": "Held, nobody to file it under",
  "compose.audience": "Visibility",
  "compose.audienceTitle": "Who may read this message?",
  "compose.audienceLegend": "Visibility of this one message",
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
  "email.access.team": "Team",
  "email.access.participants": "Participants",
  "email.access.selected": "Selected",
  "email.access.withheld": "Withheld",
  "email.access.sentence.team": "Everyone in the organization can read this.",
  "email.access.sentence.participants":
    "Only the people on this message can read it.",
  "email.access.sentence.selected":
    "Only the people named below can read this.",
  "email.access.sentence.withheld": "This message is not shared with you.",
  "email.access.unnamedMember": "Someone no longer here",
  "email.move.needsReply": "Needs reply",
  "email.move.waitingForThem": "Waiting for them",
  "email.detail.loading": "Opening the message",
  "email.detail.none": "This message",
  "email.detail.attachments_one": "{count} attachment",
  "email.detail.attachments_other": "{count} attachments",
  "email.detail.showQuoted": "Show quoted history",
  "email.detail.close": "Close",
  "email.detail.withheldReason": "This message is not shared with you",
  "email.detail.from": "From",
  "email.detail.to": "To",
  "email.detail.cc": "Cc",
  "email.detail.when": "Sent",
  "email.detail.bccWithheld":
    "Some recipients were blind-copied and are not shown to you",
  "compose.audienceWorkspace": "Everyone in the organization",
  "compose.audienceWorkspaceHint":
    "Anyone who may see the contact reads this message too.",
  "compose.audienceParticipants": "Participants only",
  "compose.audienceParticipantsHint":
    "Only the people on this message read its subject and body. Others see that a message was exchanged that day, nothing more.",
  "compose.audienceSelected": "Named people",
  "compose.audienceSelectedHint":
    "only the people and teams you name, plus anyone already on the message.",
  "compose.audienceMembersLegend": "Who may read it",
  "compose.audienceMembersLoading": "Reading the list of people…",
  "compose.audienceConfirm": "Save visibility",
  "compose.audienceNote":
    "Applies to this message only \u2014 not to the thread and not to the contact.",
  "timeline.textMore": "Read it",
  "timeline.textLess": "Show less",
  "timeline.tailMore": "Show signature and quoted text",
  "timeline.tailLess": "Hide signature and quoted text",
  "co.profileField.display_name": "Company name",
  "co.profileField.offer_summary": "What they sell",
  "co.profileField.icp": "Who they sell to",
  "co.profileField.buying_center": "Who decides there",
  "co.profileField.value_proposition": "What they promise",
  "co.profileField.usp": "How they differentiate",
  "co.profileField.customer_pains": "Pain they solve",
  "co.profileField.desired_outcomes": "Outcome they promise",
  "co.profileField.buying_intents": "What triggers a purchase",
  "co.profileField.common_objections": "Objections they meet",
  "co.profileField.sales_motion": "How they sell",
  "co.profileField.legal_name": "Registered legal name",
  "co.profileField.registered_address": "Registered address",
  "co.profileField.register_vat": "Register / VAT ID",
  "co.profileField.legal_form": "Legal form",
  "co.profileField.register_court": "Register court",
  "co.profileField.register_number": "Register number",
  "co.profileField.industry": "Industry",
  "co.profileField.history": "History",
  "co.narrative.title": "What they do",
  "co.narrative.sub":
    "The account's own story, as its website tells it. Correct anything wrong - a correction sticks, and the next read will not overwrite it.",
  "co.narrative.add": "Add",
  "co.people.engagement": "Engagement",
  "co.people.lastInteraction": "Last exchange",
  "co.people.strength": "Relationship",
  "co.people.neverInTouch": "No exchange yet",
  "co.people.theyWrote": "They wrote",
  "co.people.weWrote": "We wrote",
  "co.people.filter.status": "Engagement",
  "co.people.filter.statusAll": "Any engagement",
  "co.people.band.wayIn": "Best way in",
  "co.people.band.noWayIn": "Nobody has answered",
  "co.people.band.noWayInWhy":
    "Everyone here was written to and nobody replied",
  "co.people.band.showAnswered": "Show who answered",
  "co.people.band.showWaiting": "Show who is waiting",
  "co.people.band.committee": "Buying team",
  "co.people.band.missing": "No {role}",
  "co.people.band.committeeComplete": "Champion and economic buyer named",
  "co.people.band.committeeUnread": "Hidden from you",
  "co.people.band.committeeUnreadWhy":
    "Your role cannot read this account's deals",
  "co.people.band.seatsHeld": "{count} on the team",
  "co.people.band.someHidden": "{count} more you cannot see",
  "co.people.band.coverage": "Coverage",
  "co.people.band.reachable": "{count} answering",
  "co.people.band.untried": "{count} never approached",
  "co.people.band.showUntried": "Show who is untried",
  "co.people.board.nobodyHolds": "Nobody holds this role",
  "co.people.band.noOpenDeal": "No open deal",
  "co.people.band.noOpenDealWhy": "Buying roles are recorded on a deal",
  "co.people.band.committeePartial": "Cannot be judged",
  "co.people.band.showAll": "Show everyone",
  "co.people.board.otherRoles": "Other roles",
  "co.people.band.unavailable": "Could not be read",
  "co.people.band.unavailableWhy":
    "The coverage reading failed; the list below is unaffected",
  "co.people.view": "Committee view",
  "co.people.view.board": "Board",
  "co.people.view.map": "Map",
  "co.people.map.region": "Who can reach whom at this account",
  "co.people.map.bestRoute": "Best route",
  "co.people.map.alternatives": "Alternatives",
  "co.people.map.noRoute": "No route recorded",
  "co.people.map.more": "Show {count} more",
  "co.people.map.clear": "Clear selection",
  "co.people.map.emptyTitle": "No route recorded yet",
  "co.people.map.emptyBody":
    "Assign the buying roles, or import the interactions this account already has.",
  "co.people.map.nothingSelected":
    "Select a person to see the best route into them.",
  "co.people.map.ourSide": "Our side",
  "co.people.map.account": "Account",
  "co.people.map.missing": "{role} missing",
  "co.people.map.awaiting": "awaiting reply",
  "co.people.map.owed": "reply owed",
  "co.people.map.replied": "they replied",
  "co.people.map.never": "never written to",
  "co.people.map.onDeal": "on the deal",
  "co.people.map.routesWithheld": "Who can reach them is hidden from you",
  "co.people.map.assignHint": "Nobody is carrying this deal",
  "co.people.map.scope": "{count} on the buying team · selected deal only.",
  "co.people.map.scopePartial":
    "{count} on the buying team · {hidden} more you cannot see.",
  "co.people.board.readFromMessages": "Read from their messages",
  "co.intro.title": "Ask for an introduction",
  "co.intro.who": "Asking {colleague} to introduce you to {contact}.",
  "co.intro.write": "Write the message",
  "co.intro.writing": "Writing",
  "co.intro.fromTemplate":
    "Written from a template — this installation has no model configured.",
  "co.intro.subject": "Subject",
  "co.intro.body": "Message",
  "co.intro.basedOn": "Based on",
  "co.intro.copy": "Copy",
  "co.intro.copyFailed":
    "This browser would not let the page copy. Select the message and copy it yourself.",
  "co.intro.copied": "Copied",
  "co.intro.openMail": "Open in your mail app",
  "co.map.askIntro": "Ask for an intro",
  "co.people.board.suggest": "Suggest roles",
  "co.people.board.suggesting": "Reading their messages",
  "co.people.board.suggestNoDeal":
    "Roles are recorded on a deal, and this account has no open one.",
  "co.people.board.suggestWrote": "Seated {count} from what they wrote.",
  "co.people.board.suggestUnavailable":
    "Reading roles needs a model, and this installation has none configured.",
  "co.people.board.suggestNothing": "Nothing in their messages says who buys.",
  "co.people.board.suggestRefused":
    "Nothing was clear enough to record. {count} reading(s) were dropped for weak evidence.",
  "co.people.board.confirm": "Confirm",
  "co.people.board.confirming": "Confirming",
  "co.people.board.change": "Change role",
  "co.reach.waiting": "Needs reply",
  "co.reach.answered": "Answered",
  "co.reach.silent": "No reply",
  "co.reach.untried": "Not approached",
  "co.role.champion": "champion",
  "co.role.economic_buyer": "economic buyer",
  "co.role.blocker": "blocker",
  "co.role.influencer": "influencer",
  "co.role.user": "end user",
  "co.evidence.extractedUnconfirmed": "AI extracted · not yet confirmed",
  "co.evidence.previous": "Previous claim",
  "co.evidence.next": "Next claim",
  "co.evidence.title": "Where this came from",
  "co.relationships.title": "Linked people and companies",
  "co.tools.title": "Data & tools",
  "co.prep.withheld":
    "Parts of this account are hidden from you, so this reading is incomplete.",
  "co.read.newActivity_one": "One new item since your last visit.",
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
  "co.vat.markUnchecked": "VAT ID: not checked with the register yet",
  "co.vat.markUnreadable":
    "VAT ID: the check could not be read just now — press to try again",
  "co.vat.numberMoved":
    "The number on this record has changed since this check. Ask the register again to check the new one.",
  "co.vat.verdict": "Register answer",
  "co.vat.number": "Number consulted",
  "co.vat.registeredName": "Registered to",
  "co.vat.registeredAddress": "Registered address",
  "co.vat.checkedAt": "Consulted on",
  "co.vat.receipt": "Consultation number",
  "co.vat.status.valid": "Valid",
  "co.vat.status.invalid": "Not valid",
  "co.vat.noReceipt":
    "None issued. The register issues a receipt only for a check made under your own VAT ID — add yours in the settings and the next check carries proof a tax authority accepts.",
  "co.vat.never":
    "This company's VAT ID has not been consulted. It is checked on its own when the number is read from the company's imprint, and you can ask the register now.",
  "co.vat.askNow": "Check with the register",
  "co.vat.askAgain": "Check again",
  "co.vat.askingBusy": "Asking the register",
  "co.vat.asking":
    "Asking the register — the answer appears here once it replies.",
  "co.tech.title": "Technology",
  "co.tech.sub":
    "What this company publicly runs, read from its DNS records, its certificates and its own homepage.",
  "co.tech.mail": "Mail",
  "co.tech.web": "Website technology",
  "co.tech.services": "Services",
  "co.tech.hosting": "Hosting",
  "co.tech.empty":
    "No technical reading yet. This fills itself in when the company's site is read, and refreshes on its own.",
  "co.tech.laneFailed":
    "{lane} did not answer — what it read last time is unchanged.",
  "co.tech.laneRefused": "The site declined to be read.",
  "co.tech.lane.dns": "DNS",
  "co.tech.lane.certlog": "Certificates",
  "co.tech.lane.homepage": "Homepage",
  "signal.kind.technical_change": "Technology changed",
  "co.factField.quantified_outcome": "Result",
  "co.facts.title": "Facts about this company",
  "co.facts.empty":
    "Nothing on file yet. Read the website, or state what you already know.",
  "co.facts.add": "Add fact",
  "co.facts.addField": "What kind of fact",
  "co.facts.addValue": "What it says",
  "co.facts.addSave": "Save fact",
  "co.facts.addCancel": "Cancel",
  "co.facts.addIncomplete":
    "Pick what kind of fact this is and type what it says.",
  "co.facts.remove": "Remove {value}",
  "co.facts.removeTitle": "Remove this fact?",
  "co.facts.removeConfirm": "Remove",
  "co.facts.removeAsk":
    "{field} is recorded as \u201c{value}\u201d. Removing it says this is not a fact about the company. A later read of their website may state it again.",
  "co.facts.showAll": "Show all {count}",
  "co.facts.showLess": "Show fewer",
  "co.project.new": "New project",
  "co.deal.new": "New deal",
  "co.recent.title": "What happened lately",
  "co.recent.emptyDetail":
    "Once you send an email, log a call or hold a meeting, the exchange appears here, with who did what on each side.",
  "co.recent.empty": "Nothing logged with them yet.",
  "co.recent.kind.email": "Email",
  "co.recent.kind.call": "Call",
  "co.recent.kind.meeting": "Meeting",
  "co.recent.kind.note": "Note",
  "co.recent.kind.task": "Task",
  "co.recent.kind.message": "Message",
  "co.recent.dir.theyWrote": "they wrote",
  "co.recent.dir.weSent": "we sent",
  "co.recent.dir.theyCalled": "they called",
  "co.recent.dir.weCalled": "we called",
  "co.recent.dir.both": "both sides",
  "co.recent.minutes": "{count} min",
  "co.recent.re": "on a deal",
  "co.recent.reNamed": "on {name}",
  "tagAdmin.title": "Tags",
  "tagAdmin.sub":
    "The words this organization files records under. Anyone can apply one; only admin and ops seats add, rename or retire them.",
  "tagAdmin.listLabel": "Vocabulary",
  "tagAdmin.empty":
    "No tags yet. Add the first word this organization files records under.",
  "import.contextTag": "File this batch under a tag",
  "import.contextTagChosen":
    "Records this import creates will be filed under {name}.",
  "import.contextTagChosenUnnamed":
    "Records this import creates will be filed under the tag chosen for this run.",
  "import.contextTagHint":
    "Applied to records this import creates, so the batch stays findable. Records it updates keep the tags they have.",
  "import.contextTagNone": "No tag",
  "tagAdmin.add": "Add tag",
  "tagAdmin.addTitle": "Add a tag",
  "tagAdmin.editTitle": "Edit tag",
  "tagAdmin.nameLabel": "Name",
  "tagAdmin.colorLabel": "Colour",
  "tagAdmin.colorNone": "No colour",
  "tagAdmin.create": "Add",
  "tagAdmin.save": "Save",
  "tagAdmin.edit": "Edit",
  "tagAdmin.merge": "Merge",
  "tagAdmin.archive": "Retire",
  "tagAdmin.restore": "Restore",
  "tagAdmin.usage": "{count} records",
  "tagAdmin.usagePending": "Counting…",
  "tagAdmin.nearMatch":
    "Close to a word this organization already has: {names}. Apply that one instead unless this is a different thing.",
  "tagAdmin.mergeTitle": "Merge {name} into another tag",
  "tagAdmin.mergeIntoLabel": "Keep this tag",
  "tagAdmin.mergeIntoNone": "Choose a tag",
  "tagAdmin.mergeConfirm": "Merge",
  "tagAdmin.mergeWarning":
    "This cannot be undone. Records carrying {name} will carry the other tag instead, and the name is released for anyone to use again.",
  "tagAdmin.mergedTitle": "Merged",
  "tagAdmin.mergedBody":
    "{moved} records moved to the surviving tag. {collapsed} already carried both, so their duplicate was dropped.",
  "tagAdmin.countUsage": "Count records",
  "tagAdmin.noVersion":
    "This tag was read without a version, so it cannot be saved. Reopen the page and try again.",
  "tagAdmin.withheld":
    "You do not have access to this organization's tag vocabulary.",
  "tagAdmin.truncated":
    "This list is shortened. Words past the limit are not shown here and cannot be edited or merged into.",
  "tagAdmin.usageFailed": "Count unavailable",
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
  "tags.panelSub": "Open a tag, or use its menu to manage this assignment",
  "tags.add": "Add tag",
  "tags.more": "+{count} more",
  "tags.showLess": "Show less",
  "tags.options": "Options for {name}",
  "tags.addedBy": "Added by {who} · {when}",
  "tags.addedOn": "Added {when}",
  "tags.visibleWorkspaceWide": "Tag names are visible across the organization.",
  "tags.removeFromRecord": "Remove from this record",
  "tags.withheld": "Hidden — your role cannot read the tag vocabulary",
  "tags.emptyTitle": "No tags yet",
  "tags.emptyBody":
    "Add durable context such as an event, a relationship, or a cohort.",
  "tags.pickerLabel": "Find a tag",
  "tags.alreadyAdded": "Already added",
  "tags.catalogTruncated":
    "This list is shortened, so a word may be missing. Search for it by name before asking for a new one.",
  "tags.noMatch":
    "No tag by that name. An admin or ops seat can add one to the vocabulary.",
  "tagResult.gone":
    "This tag no longer exists. It may have been merged into another.",
  "tagResult.totalVisible": "{count} visible assignments",
  "tagResult.people": "People",
  "tagResult.companies": "Companies",
  "tagResult.deals": "Deals",
  "tagResult.viewAll": "View all {count} {kind}",
  "tagResult.resultsTitle": "Records with this tag",
  "tagResult.nothingCarries":
    "Nothing carries this tag yet. Apply it from any contact, company or deal.",
  "tagResult.loadingRows": "Loading {kind}…",
  "tagResult.noneLeft": "Nothing carries it any more",
  "tagResult.unnamed": "Unnamed",
  "co.timeline.empty": "Nothing logged on this account yet.",
  "co.overlayFallback":
    "This account is served from the connected system of record, so the company view is not assembled here. Open it in that system to see the full picture.",
  "org.domains": "Domains",
  "org.factCategory.company": "Company",
  "org.factCategory.offering": "Offering",
  "org.factCategory.market": "Market",
  "org.factCategory.signal": "Signals",

  "lead.score": "Score",
  "lead.status": "Status",
  "lead.nextTask": "Next step",
  "lead.openTaskCount": "{count} open tasks",
  "lead.noNextTask": "No next step",
  "lead.scoreNoSignals": "No qualifying signals",
  "lead.source": "Source",
  "lead.project": "Project",
  "lead.openLinkedIn": "Open LinkedIn profile",
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
    "Written by a connector — it keeps its own source.",
  "leadSources.title": "Lead sources",
  "leadSources.sub":
    "Where leads come from. Used in the New lead form, as a filter, and by the score.",
  "leadSources.readOnly": "Only an admin or ops seat changes this list.",
  "leadSources.labelFor": "Label of source {key}",
  "leadSources.intentFor": "Intent of {label}",
  "leadSources.intent": "Intent",
  "leadSources.intent.high": "High interest",
  "leadSources.intent.neutral": "Neutral",
  "leadSources.intent.low": "Low interest",
  "leadSources.intentHint":
    "High adds points to the score, Low subtracts; a change applies on each lead's next rescore.",
  "leadSources.leadCount": "{count} leads",
  "leadSources.builtIn": "built-in",
  "leadSources.builtInKept":
    "Built-in sources can be renamed and switched off, not removed.",
  "leadSources.inUse": "{count} leads use this source — switch it off instead.",
  "leadSources.deactivateInstead": "switch off instead",
  "leadSources.activeFor": "{label} is active",
  "leadSources.remove": "Remove",
  "leadSources.removeTitle": "Remove this source?",
  "leadSources.removeBody":
    '"{label}" is not used by any lead and will disappear from the list.',
  "leadSources.newLabel": "New source",
  "leadSources.labelField": "Label",
  "leadSources.addOpen": "New source",
  "leadSources.listLabel": "Sources in the list",
  "leadSources.discovered": "Discovered values",
  "leadSources.newPlaceholder": "Trade show",
  "leadSources.add": "Add source",
  "leadSources.discoveredSub":
    "From integrations and imports — values that live on leads but are not in the list yet. Add one to give it a label and a weight.",
  "leadSources.adopt": "Add to list",
  "leadReasons.title": "Disqualification reasons",
  "leadReasons.sub":
    "What a rep picks when a lead is dropped. The reason shows on the lead and can be filtered on.",
  "leadReasons.labelFor": "Label of reason {label}",
  "leadReasons.leadCount": "{count} leads",
  "leadReasons.inUse":
    "{count} leads carry this reason — switch it off instead.",
  "leadReasons.newLabel": "New reason",
  "leadReasons.listLabel": "Reasons in the list",
  "leadReasons.add": "Add reason",
  "leadReasons.removeTitle": "Remove this reason?",
  "leadReasons.removeBody":
    '"{label}" is not used by any lead and will disappear from the list.',
  "leadHandling.title": "Lead handling",
  "leadHandling.sub": "How this installation treats a new lead.",
  "leadHandling.firstResponse": "First-response target",
  "leadHandling.firstResponseHint":
    "Off by default. On, every open lead carries a reply deadline, the list gains the Overdue view, and overdue leads sort first.",
  "leadHandling.targetMinutes": "Target (minutes)",
  "leadHandling.targetOutOfRange":
    "Enter a whole number of minutes between 15 and 10080 (7 days).",
  "leadHandling.targetHint":
    "How long a lead may wait for its first reply once it is routed (or created). 15 minutes to 7 days.",
  "lead.boardCount": "{count} leads",
  "lead.duplicateFound":
    "A lead with this email or LinkedIn profile already exists.",
  "lead.promote": "Qualify",
  "lead.promoteIneligible": "needs an email address and an open status",
  "lead.filterStatus": "Status",
  "lead.filterStatusAll": "All statuses",
  "lead.filterScore": "Score",
  "lead.filterScoreAll": "Any score",
  "lead.bulkSelected": "{count} selected",
  "lead.bulkOwner": "New owner",
  "lead.bulkOwnerPick": "Pick an owner",
  "lead.bulkAssign": "Assign",
  "lead.bulkDisqualify": "Disqualify",
  "lead.bulkDisqualifyTitle_one": "Disqualify this lead?",
  "lead.bulkDisqualifyTitle_other": "Disqualify {count} leads?",
  "lead.bulkDisqualifyBody":
    "Closed with the reason \u201c{reason}\u201d. Each lead keeps its own record, and there is no one step that puts them back.",
  "lead.bulkFailed": "{count} not applied —",
  "lead.bulkFailedRow": "could not be saved",
  "lead.bulkSelectRow": "Select {name}",
  "lead.unnamed": "Unnamed lead",
  "lead.sla.breached": "Overdue",
  "lead.sla.atRisk": "Due soon",
  "lead.sla.withinTarget": "On time",
  "lead.sla.answeredAt": "First response on {at}",
  "lead.sla.dueBy": "First response due by {at}",
  "lead.sla.overdueSince": "First response was due {at}",
  "lead.filterSla": "Response",
  "lead.filterSlaAll": "Any",
  "list.viewOverdue": "Overdue",
  "lead.filterScoreHot": "80 and up",
  "lead.filterScoreWarm": "60 and up",
  "lead.filterScoreCool": "40 and up",
  "lead.details": "Details",
  "lead.ladder.title": "Where this lead stands",
  "lead.railTitle": "Owner",
  "lead.detailsUnset": "Not set",
  "lead.terminalReadOnly": "This lead is closed and takes no changes.",
  "lead.callNotInOverlay":
    "This lead is a mirror of the system of record, which takes no activity from here \u2014 log the call where the record lives.",
  "lead.boardCountsUnavailable":
    "The Qualified and Disqualified counts could not be read.",
  "lead.boardTerminalRowsUnavailable":
    "These leads could not be read. The count above still stands.",
  "lead.boardTerminalOnly":
    "None of these leads are still open \u2014 they are counted under Qualified and Disqualified.",
  "person.fromLead": "From lead",
  "lead.promotedTitle": "Promoted to a contact",
  "lead.promotedMerged":
    "This lead merged into a contact we already knew — no duplicate was created.",
  "lead.promotedCreated": "This lead became a new contact.",
  "lead.promotedAt": "Promoted",
  "lead.promotedTrigger": "Trigger:",
  "lead.promotedEvidence": "Evidence:",
  "lead.previewPending": "Checking whether we already know this person…",
  "lead.previewCreate": "Promoting will create a new contact.",
  "lead.previewMerge": "Promoting will merge into the existing contact",
  "lead.previewMergeWithheld":
    "Promoting will merge into an existing contact you cannot see.",
  "lead.demote": "Reverse promotion",
  "lead.demoteDialog": "Reverse this promotion?",
  "lead.demoteExplain":
    "The lead returns to the queue as “Working”. A contact the promotion created is archived; a contact it merged into stays as it is. A contact on a live deal cannot be reversed.",
  "lead.demoteReason": "Reason (recorded in the audit trail)",
  "lead.demoteReasonRequired": "Say why first.",
  "lead.demoteConfirm": "Reverse",
  "lead.promotedOutcomePending": "Reading what this promotion did…",
  "lead.promotedOutcomeUnavailable":
    "We cannot show whether this merged or created a contact.",
  "lead.terminalPromoted": "Promoted — this lead is now read-only.",
  "lead.statusNew": "New",
  "lead.statusContacted": "Contacted",
  "lead.statusEngaged": "Engaged",
  "lead.statusPromoted": "Qualified",
  "lead.statusDisqualified": "Disqualified",
  "lead.disqualified": "Disqualified",
  "lead.status.new": "New",
  "lead.status.contacted": "Contacted",
  "lead.status.engaged": "Engaged",
  "lead.explainScore": "Explain this score",
  "lead.scoreOverridden": "Human override: {reason}",
  "lead.machineScore": "Machine score was {score}",
  "lead.overrideScore": "Override score",
  "lead.clearOverride": "Clear override",
  "lead.overrideReason": "Reason",
  "lead.shortfall.lead": "What this score has to work with:",
  "lead.shortfall.engagementMoves":
    "A reply or a meeting is what moves it most.",
  "lead.shortfall.noSource":
    "No source on record — nothing says where this lead came from.",
  "lead.shortfall.sourcePenalised":
    "Came in as “{source}”, which counts against the score.",
  "lead.shortfall.noTitle": "No job title on record.",
  "lead.shortfall.titleNotSenior":
    "“{title}” isn’t one of the senior titles the model looks for.",
  "lead.shortfall.sourceNoIntent":
    "Came in as “{source}”, which carries no buying intent on its own.",
  "lead.scoreNotStoredYet":
    "The breakdown for this score isn’t stored yet — the next update will show it.",
  "lead.scoreLoading": "Working out why…",
  "lead.scoreNoFactors": "Nothing counted toward this score yet.",
  "lead.scoreFactorsFailed": "What counts toward this score could not be read.",
  "lead.scoreFactorsExplainMachine":
    "You set this score by hand. The factors below explain what the model says: {score}.",
  "lead.scoreDecayed": "{base} halving every 14 days",
  "lead.scoreSources": "{count} activities",
  "lead.scoreReconciles": "{raw} adds up, rounds to {rounded}, scored {score}",
  "lead.factor.decision_maker_title": "Decision-maker title",
  "lead.factor.high_intent_source": "Came from a high-intent source",
  "lead.factor.low_intent_source": "Came from a low-intent source",
  "lead.factor.reply": "They replied",
  "lead.factor.meeting_held": "Meeting held",
  "lead.factor.meeting_booked": "Meeting booked",
  "lead.signalsTitle": "What you know about this lead",
  "lead.signalUnset": "Not entered",
  "lead.signalClear": "Withdraw",
  "lead.signalBandPick": "Pick a value",
  "lead.signalMore": "More",
  "lead.signalProvenanceHint":
    "Untouched, an answer is stored as an estimate with no confidence claimed.",
  "lead.signalEvidenceQuality": "How reliable is this?",
  "lead.signalConfidence": "Confidence",
  "lead.signalConfidenceUnstated": "Not stated",
  "lead.signalConfidenceValue": "{value}% confidence",
  "lead.signalRecordedAt": "Recorded {at}",
  "lead.signalSuperseded": "Previously {value}; replaced by {source}",
  "lead.signalAutomaticSource": "an automatic source",
  "lead.signalReason": "How do you know?",
  "lead.signalReasonHint":
    "Optional. Whatever you write is stored with the score.",
  "lead.signalReasonUnstated": "No source given. Entered by hand.",
  "lead.signalSave": "Add to the score",
  "lead.signal.web_traffic": "Web traffic",
  "lead.signal.employees": "Employees",
  "lead.signal.budget_hint": "Budget",
  "lead.signal.ask.web_traffic": "Website traffic?",
  "lead.signal.ask.employees": "Company size?",
  "lead.signal.ask.budget_hint": "Budget?",
  "lead.signal.fact": "Verified",
  "lead.signal.assumption": "Estimated",
  "lead.signal.judgement": "My assessment",
  "lead.signal.web_traffic.low": "Low",
  "lead.signal.web_traffic.medium": "Medium",
  "lead.signal.web_traffic.high": "High",
  "lead.signal.employees.1-10": "1–10",
  "lead.signal.employees.11-50": "11–50",
  "lead.signal.employees.51-200": "51–200",
  "lead.signal.employees.201+": "201+",
  "lead.signal.budget_hint.none": "No budget",
  "lead.signal.budget_hint.unknown": "Unknown",
  "lead.signal.budget_hint.some": "Some budget",
  "lead.signal.budget_hint.confirmed": "Budget confirmed",
  "lead.factor.manual:web_traffic": "Web traffic (your input)",
  "lead.factor.manual:employees": "Employees (your input)",
  "lead.factor.manual:budget_hint": "Budget (your input)",
  "lead.ownerLabel": "Owner",
  "lead.ownerYou": "You",
  "lead.overriddenBadge": "overridden",
  "lead.unassigned": "Unassigned",
  "lead.terminalDisqualified": "Disqualified — this lead is now read-only.",
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
  "lead.trigger.humanQualify": "Human qualified",
  "lead.evidenceNote": "Evidence note (optional)",
  "lead.segregation":
    "Leads are kept apart from Contacts. A lead becomes a contact only when you qualify it.",
  "lead.segregationDismiss": "Got it",
  "list.emptyMine": "You own no {unit}.",
  "list.showAll": "Show all",
  "lead.assignedAway": "{names} assigned to {owner} — no longer in Mine.",
  "lead.viewNew": "New",
  "lead.viewNeedsFollowUp": "Needs follow-up",
  "lead.viewEngaged": "Engaged",
  "lead.ladder": "Lead status",
  "lead.ladder.new": "New — nobody has reached out yet.",
  "lead.ladder.overlay":
    "The mirror does not move a lead's status; change it in the source system.",
  "lead.ladder.automatic": "{label} · set automatically from captured activity",
  "lead.ladder.automaticWith": "{label} · set automatically — {what} on {at}",
  "lead.ladder.byHand": "{label} · set by hand",
  "lead.ladder.theyReplied": "they replied",
  "lead.ladder.meetingBooked": "a meeting was booked",
  "lead.ladder.meetingHeld": "a meeting was held",
  "lead.ladder.qualified": "Qualified — this lead is a contact now.",
  "lead.ladder.qualifiedOn": "Qualified on {at} — this lead is a contact now.",
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
    "Optional. Whole units; the installation's base currency.",
  "lead.qualify.amountInvalid": "Enter a number, or leave it empty.",
  "lead.qualify.amountNoCurrency":
    "Waiting for the installation's base currency — try again in a moment, or leave the amount empty.",
  "lead.qualify.why": "Why",
  "lead.qualify.reasonReplied": "Reason: they replied on {at}.",
  "lead.qualify.reasonMeetingBooked": "Reason: a meeting was booked for {at}.",
  "lead.qualify.reasonMeetingHeld": "Reason: a meeting was held on {at}.",
  "lead.qualify.reasonHuman": "Reason: qualified by you.",
  "lead.qualify.confirm": "Qualify",
  "lead.qualify.confirmWithDeal": "Qualify and open deal",
  "lead.qualify.done": "{name} is now a contact:",
  "lead.disqualify.title": "Disqualify {name}",
  "lead.disqualify.reason": "Reason",
  "lead.disqualify.pickReason": "Pick a reason",
  "lead.disqualify.reasonRequired": "Pick a reason first.",
  "lead.disqualify.note": "Note (optional)",
  "lead.disqualify.confirm": "Disqualify",

  "deals.viewBoard": "Board",
  "deals.viewTable": "Table",
  "deals.amount": "Value",
  "deals.lastSignal": "Last signal",
  "deals.lastSignalNone": "no signal yet",
  "deals.stage": "Stage",
  "deals.close": "Expected close",
  "deals.confirmAdvance": "Move to {stage}?",
  "deals.confirmTerminal":
    "This closes the deal as {status}. Confirm first — nothing happens until you do.",
  "deals.lostReason": "Lost reason",
  "deals.winNoEvidence":
    "This deal has no signed contract attached, so tell us how it was won. The answer is kept on the deal and counted in reports.",
  "deals.winReason": "How was it won?",
  "deals.winReasonPick": "Pick how it was won",
  "deals.winReasonImported": "Imported from another system",
  "deals.winReasonPurchaseOrder": "On a purchase order",
  "deals.winReasonVerbal": "Verbally, in person or by phone",
  "deals.winReasonRenewalByEmail": "Renewed by email",
  "deals.winReasonOther": "Something else",
  "deals.winReasonDetail": "What was it?",
  "deals.confirm": "Confirm",
  "deals.loading": "Reading the deals…",
  "deals.cancel": "Cancel",
  "deals.advanced": "Moved to {stage}",
  "deal.pendingApprovals": "Awaiting your confirmation",
  "deal.edit": "Edit deal",
  "deal.ownerKeep": "Keep current owner",
  "deal.ownerMe": "Assign to me",
  "deal.ownerUnassign": "Unassign",
  "deal.partnerOrg": "via Partner",
  // A reference the reader may not read, on a surface with no room for the
  // mask glyph: a Kanban card's company line, and the one entry a withheld
  // picker offers. Both have to say WHICH thing is withheld, because a card
  // and a form field carry no header to say it for them.
  "deal.companyWithheld": "Company withheld",
  "deal.partnerWithheld": "Partner withheld",
  "deal.forecastCategory": "Forecast category",
  "deal.strip.title": "Where this deal stands",
  "deal.seats.title": "Who is on this deal",
  "deal.seats.empty": "No stakeholder is recorded on this deal",
  "deal.seats.ours": "{count} of ours carry it",
  "deal.committee.title": "The buying committee",
  "deal.committee.empty": "No stakeholder is recorded on this deal",
  "deal.committee.engaged": "Talking",
  "deal.committee.quiet": "No reply",
  "deal.committee.legendEngaged": "Talking with us",
  "deal.committee.legendQuiet": "On the deal, not talking",
  "deal.committee.legendGap": "Missing cover",
  "deal.committee.threads":
    "{engaged} of {total} on the deal are talking to us.",
  "deal.strip.money": "The money",
  "deal.strip.money.offer": "Offer {number} · {status}",
  "deal.strip.money.noOffer": "No offer written yet",
  "deal.strip.close": "The close",
  "deal.strip.close.none": "No date",
  "deal.strip.close.noneDetail": "Nobody has said when this closes",
  "deal.strip.close.inDays": "in {days} days",
  "deal.strip.close.overdue": "{days} days past the date",
  "deal.strip.close.provisional": "provisional, not confirmed by a human",
  "deal.strip.close.waiting": "they asked us to wait until {date}",
  "deal.strip.people": "The people",
  "deal.strip.people.count": "{engaged} of {total} engaged",
  "deal.strip.people.champion": "a champion is named",
  "deal.strip.people.noChampion": "no champion named",
  "deal.strip.people.none": "Nobody",
  "deal.strip.people.noneDetail": "No stakeholder is recorded on this deal",
  "deal.strip.momentum": "The momentum",
  "deal.strip.momentum.detail": "since the last contact",
  "deal.strip.withheld": "Hidden",
  "deal.strip.withheldDetail": "You may not read who is on this deal",
  "deal.forecast.commit": "commit",
  "deal.forecast.bestCase": "best case",
  "deal.forecast.pipeline": "pipeline",
  "deal.forecast.omitted": "left out of the forecast",
  "deal.pulse.yourMove": "It's your move.",
  "deal.pulse.theirMove": "Their move.",
  "deal.pulse.theirMoveWhy": "Nobody here is owed an answer.",
  "deal.pulse.wroteOn": "They wrote last on {date} — {days} days ago.",
  "deal.pulse.wroteUnknown": "They wrote and nobody has answered.",
  "deal.waitUntil": "Wait until",
  "deal.fxBase": "Base {value} · rate {rate} as of {date}",
  "deal.archive": "Archive deal",
  "deal.archiveConfirm":
    "Archiving removes this deal from the active pipeline. This cannot be undone from the UI.",
  "deal.archivedReadOnly": "This deal is archived and takes no changes.",
  "deal.reopen": "Reopen",
  "deal.reopenPick": "Move this deal back to an open stage",
  "deal.reopenConfirm": "Reopen",
  "deal.fcCommit": "Commit",
  "deal.fcBestCase": "Best case",
  "deal.fcPipeline": "Pipeline",
  "deal.fcOmitted": "Omitted",
  "deal.fcSlipped": "Slipped",
  "deal.fcUncategorised": "No category yet",

  "deals.pipeline": "Pipeline",
  "deals.filterStalled": "Stalled only",
  "deals.filterOwnerMe": "My deals",
  // Both reasons say "loaded only" rather than naming the sum alone: with no
  // server aggregate the column's figure is the cards LOADED, and the board
  // pages on demand, so that number grows as the reader presses Load more.
  // Naming only the total would leave the count reading as final.
  "deals.totalsNeedOwnerFilter":
    "Loaded only — filter to My deals for the total",
  "deals.totalsNoTagFilter": "Loaded only — no total while a tag filters",
  "deals.filterPartner": "Partner",
  "deals.filterPartnerAnyOne": "Any partner",
  "deals.filterForecast": "Forecast",
  "deals.filterForecastAll": "Any forecast category",
  "deals.filterPartnerSourced": "Partner-sourced",
  "deals.filterStageAll": "All stages",
  "deals.filterOrgAll": "All companies",
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
    "They leave every list and report, and there is no way to bring one back from here yet.",
  "deals.bulkFailed": "{count} not applied —",
  "deals.bulkFailedRow": "could not be saved",

  "deal.offers": "Offers",
  "deal.newOffer": "New offer",
  "deal.offerNeedsCurrency":
    "Price this deal first — an offer is written in the deal's own currency.",
  "deal.offerNumber": "Offer #",
  "deal.offerRevision": "Rev.",
  "deal.offersEmpty": "No offers yet",

  "offer.revision": "Revision {revision}",
  "offer.backToDeal": "Back to deal",
  "offer.totals": "Totals",
  "offer.net": "Net",
  "offer.tax": "Tax",
  "offer.gross": "Gross",
  "offer.edit": "Edit header",
  "offer.currency": "Currency",
  "offer.buyerOrg": "Buyer organization",
  "offer.buyerOrgConfirm": "Buyer organization: {name}",
  "offer.template": "Template",
  "offer.validUntil": "Valid until",
  "offer.introText": "Intro text",
  "offer.termsText": "Terms text",
  "offer.lines": "Line items",
  "offer.addLine": "Add line",
  "offer.position": "Pos.",
  "offer.description": "Description",
  "offer.unit": "Unit",
  "offer.quantity": "Quantity",
  "offer.unitPrice": "Unit price",
  "offer.discountPct": "Discount %",
  "offer.taxRate": "Tax %",
  "offer.lineTotal": "Line total",
  "offer.unpriced": "unpriced — excluded from total",
  "offer.removeLine": "Remove",
  "offer.pickProduct": "Pick product",
  "offer.pickProductConfirm": "Product: {name}",
  "offer.send": "Send",
  "offer.sendConfirm": "Send this offer to the buyer?",
  "offer.sendBody": "The offer becomes read-only until the buyer responds.",
  "offer.accept": "Accept",
  "offer.acceptConfirm": "Mark this offer as accepted?",
  "offer.acceptBody":
    "The deal's amount and currency will be updated to match this offer.",
  "offer.reject": "Reject",
  "offer.rejectConfirm": "Mark this offer as rejected?",
  "offer.rejectReason": "Reason (optional)",
  "offer.regenerate": "Regenerate revision",
  "offer.aiDisclosureTitle": "AI-assisted disclosure",
  "offer.diffAdded": "{count} line(s) added",
  "offer.diffRemoved": "{count} line(s) removed",
  "offer.diffChanged": "{count} line(s) changed",
  "offer.renderPdf": "Render PDF",
  "offer.viewPdf": "View PDF",
  "offer.pdfUnavailable": "PDF rendering not available on this deployment.",

  // The queue of staged actions a person has to decide. It is a DECISION here
  // and an `approval` on the wire, and those two are the only names it has: it
  // was also being called an inbox, a drafts queue and a staged list, and a
  // reader told four names for one surface has been told none. Copy that has to
  // point at it says what the reader does there ("waiting on you", "wait for
  // your decision") rather than inventing a fifth noun for the place.
  "decision.viaTool": "via {verb}",
  "decision.approveEdited": "Approve edited",
  "decision.reject": "Reject",
  "decision.draftSubject": "Subject",
  "decision.draftBody": "Message",
  "decision.dismiss": "Dismiss",
  "decision.versionSkew":
    "This record changed since it was staged — re-stage it before deciding.",
  "decision.reRead": "Re-read",
  "decision.alreadyDecided": "Already decided — nothing left to do here.",
  "decision.expired": "Expired",
  "decision.expiresIn": "expires in {countdown}",
  "decision.detail": "Approval detail",
  "decision.detailLoading": "Reading this approval…",
  "decision.detailTechnical": "Technical details",
  "decision.detailAsked": "Asked",
  "decision.detailDecided": "Decided",
  // The confirmation after an approval, and the way back to the change it
  // made. The undo lives on the RECORD — the history panel reverses the
  // update this approval wrote — so the verb says where it is going.
  "decision.applied": "Done.",
  "decision.undoOnRecord": "Undo on the record",
  "decision.status.approved": "Approved",
  "decision.status.rejected": "Rejected",
  "decision.status.expired": "Expired",

  "home.pipelineWeighted": "{amount} weighted",
  "home.pipelineCount_one": "{count} open deal",
  "home.pipelineCount_other": "{count} open deals",
  "home.pipelinePartial":
    "{count} deals are not in these figures — your access does not cover them.",
  "home.pipelineUnavailable": "This figure could not be loaded.",
  // The morning brief's own narrative. The "no pass" line is the honest degrade:
  // a run nobody annotated and a night with nothing in it read identically as
  // silence, so the screen says which one this is.
  // The week just gone. No nav entry of its own: Today is the single door to
  // the work that waits on a person, and this is a view of that same work.
  "home.panel.weekly": "Last week",
  "home.weekly.weekOf": "Week of {day}",
  // The week ahead. The frozen review says what happened; this is the only part
  // of that page anybody can still change.
  "plan.title": "Plan next week",
  // The head of the ranked queue, on the page a rep opens first. The same rows
  // the Worklist draws, in the order the server decided.
  // The Brief's opening sentence, composed from the rows the page is showing —
  // never model-written, so it cannot say what the rows contradict.
  "brief.eyebrow": "Your morning",
  "brief.eyebrow.weekly": "Your week",
  "brief.eyebrow.asOf": "{scope} · as of {at}",
  // The Brief's two dials. Which brief, and whose.
  "brief.view.label": "Which brief",
  "brief.view.morning": "Morning",
  "brief.view.weekly": "Weekly",
  "brief.scope.label": "Whose brief",
  "brief.scope.mine": "Mine",
  "brief.scope.team": "Team",
  "brief.sentence.clear": "Nothing is waiting on you this morning.",
  "brief.sentence.one": "First: {lead}",
  "brief.sentence.oneWithCost": "First: {lead} — {consequence}",
  "brief.sentence.many": "First: {lead} Then {rest} more.",
  "brief.sentence.manyWithCost":
    "First: {lead} — {consequence} Then {rest} more.",

  // The weekly Brief's opening sentence, composed from the counts the week was
  // frozen with. Result first, then what carried — the outcome before the debt.
  "brief.week.won": "You closed {count} deals.",
  "brief.week.moved": "You moved {count} deals forward.",
  "brief.week.met": "You held {count} meetings.",
  "brief.week.carryPromises": "{count} promises carried over.",
  "brief.week.carryTasks": "{count} tasks carried over.",
  "brief.week.andCarry": "{result} {carry}",
  "brief.week.quiet": "A quiet week — nothing closed and nothing moved.",

  "brief.feed.title": "Today",
  "brief.feed.sub": "One order, decided once.",
  "brief.feed.loading": "Reading your morning",
  "brief.feed.clear": "Nothing is waiting on you right now.",
  "brief.feed.rest": "{count} more on the worklist",
  "brief.feed.section.respond_now": "Respond now",
  "brief.feed.section.prepare_conversations": "Prepare conversations",
  "brief.feed.section.move_revenue": "Move revenue",
  "brief.feed.section.build_pipeline": "Build pipeline",
  "brief.feed.section.review_and_repair": "Review and repair",

  // A team's week, frozen when it closed. Two weeks compare because neither
  // moves under the comparison.
  "teamweekly.title": "The team's week",
  "teamweekly.weekOf": "{team} · week of {day}",
  "teamweekly.frozen": "Frozen",
  "teamweekly.loading": "Reading the team's week",
  "teamweekly.empty": "Nothing to show for this week.",
  "teamweekly.forbidden":
    "A team's week is a team question, and your access reaches your own rows only.",
  "teamweekly.noSnapshot":
    "No week has closed for this team yet. The first snapshot is written on the Monday after their first full week.",
  "teamweekly.pickTeam": "Choose a team",
  "teamweekly.repsUnread":
    "{count} member(s) could not be read. Every figure here covers {counted}.",
  "teamweekly.ofTotal": "{part} of {whole}",
  "teamweekly.headline.plain":
    "The week ran without a reading that stands out either way.",
  "teamweekly.headline.healthy":
    "{reading} is healthy at {pct}%, against a bar of {bar}%.",
  "teamweekly.headline.weak":
    "{reading} is not, at {pct}% against a bar of {bar}%.",
  "teamweekly.reading.firstResponse": "First response",
  "teamweekly.reading.nextStep": "Meetings with a next step",
  "teamweekly.reading.commitments": "Plan commitments kept",
  "teamweekly.card.firstResponse": "Answered in time",
  "teamweekly.card.firstResponseBasis": "{breached} breached",
  "teamweekly.card.meetings": "Meetings with a next step",
  "teamweekly.card.meetingsBasis": "of the meetings held",
  "teamweekly.card.commitments": "Plan commitments kept",
  "teamweekly.card.commitmentsBasis": "of what was owed",
  "teamweekly.card.won": "Won",
  "teamweekly.card.wonBasis": "{lost} lost",
  "teamweekly.card.wonBasisValue": "{value} won · {lost} lost",
  "teamweekly.card.reps": "Members counted",
  "teamweekly.card.repsBasis": "whose week was read in full",
  "teamweekly.movement.title": "What the week did",
  "teamweekly.movement.won": "Won",
  "teamweekly.movement.lost": "Lost",
  "teamweekly.movement.moved": "Advanced",
  "teamweekly.movement.meetings": "Meetings held",
  "teamweekly.movement.leads": "Leads routed",
  "teamweekly.agenda.title": "Monday agenda",
  "teamweekly.agenda.sub":
    "The week's own items, first to raise at the top. One per member, including the member whose week went well.",
  "teamweekly.agenda.empty":
    "Nobody's week could be read for this team, so there is nothing to take to the meeting.",
  "teamweekly.agenda.summary":
    "{count} to take to Monday, starting with {first}.",
  "teamweekly.agenda.copy": "Copy agenda",
  "teamweekly.agenda.copied": "Copied",
  "teamweekly.agenda.copyFailed":
    "This browser would not hand over the clipboard. Select the list and copy it.",
  "teamweekly.focus.help_requested": "Asked for help",
  "teamweekly.focus.leads_breached": "Leads went unanswered",
  "teamweekly.focus.commitments_missed": "Plan commitments missed",
  "teamweekly.focus.meetings_without_next_step":
    "Meetings left without a next step",
  "teamweekly.focus.strong_week": "Worth copying",
  "teamweekly.focus.quiet_week": "A quiet week",

  "plan.sub": "What you said you would do, and what you need to do it.",
  "plan.loading": "Reading your plan",
  "plan.empty": "Nothing on the plan yet.",
  "plan.none": "You have not planned this week yet.",
  "plan.start": "Plan my week",
  "plan.readOnly":
    "Read-only view \u2014 planning a week and settling a commitment are not yours to do here.",
  "plan.add": "Add commitment",
  "plan.saveRefused_one":
    "One commitment could not be saved. It is still ticked — try again.",
  "plan.saveRefused_other":
    "{count} commitments could not be saved. They are still ticked — try again.",
  "plan.save_one": "Save {count} change",
  "plan.save_other": "Save {count} changes",
  "plan.due": "due {day}",
  "plan.state.open": "Open",
  "plan.state.done": "Done",
  "plan.state.missed": "Missed",
  "plan.state.dropped": "Dropped",
  "plan.help.label": "What do you need from your lead?",
  "plan.help.ask": "Ask for help",
  "plan.help.edit": "Edit request",
  "plan.help.send": "Send",
  "plan.help.cancel": "Cancel",
  "plan.help.asked": "You asked: {text}",
  "plan.help.waiting": "Waiting on your lead.",
  "plan.new.label": "What will you do?",
  "plan.new.due": "By when",
  "plan.new.save": "Add",
  "plan.new.cancel": "Cancel",

  "home.weekly.frozen": "Frozen",
  "home.weekly.written": "written {at}",
  "home.weekly.pickWeek": "Open another week",
  "home.weekly.none":
    "No weekly review yet — the first one is written on the Monday after your first full week.",
  "home.weekly.tasksDelivered": "Tasks delivered",
  "home.weekly.ofDue": "{done} of {due}",
  "home.weekly.dealsWon": "Won",
  "home.weekly.dealsLost": "Lost",
  "home.weekly.dealsMoved": "Moved",
  "home.weekly.decided": "You decided",
  "home.weekly.acceptedRejected": "{accepted} yes · {rejected} no",
  "home.weekly.noNarrative":
    "No summary of this week — Margince did not run a pass over it. The numbers below are still the week's own.",
  "home.weekly.queueWorked": "Morning queue",
  "home.weekly.actedDismissed": "{acted} acted · {dismissed} dismissed",
  "home.weekly.sincePrior": "{delta} vs last week",
  "home.weekly.wonVsPrior": "{value} · {delta} vs prior week",
  "home.weekly.leadsAnswered": "Leads answered in time",
  "home.weekly.ofRouted": "{answered} of {routed}",
  "home.weekly.planCommitmentsKept": "Plan commitments kept",
  "home.weekly.meetingsHeld": "Meetings with a next step",
  "home.weekly.ofMeetings": "{withStep} of {held}",
  "home.weekly.carriedOver": "Carried over",
  "home.weekly.outcome.moved": "moved",
  "home.weekly.outcome.won": "won",
  "home.weekly.outcome.lost": "lost",
  "home.act": "Done",
  "home.dismiss": "Dismiss",
  "home.actedState": "acted",
  "home.dismissedState": "dismissed",
  "home.evidence_other": "{count} evidence rows",
  "home.evidence_one": "{count} evidence row",
  "home.openDeal": "Open deal",
  "home.factorWinnability": "Winnability",
  "home.factorRevenue": "Revenue",
  "home.factorTiming": "Timing",
  "home.factorMomentum": "Momentum",
  "home.factorWarmth": "Warmth",

  "home.digestFor": "digest for {date}",
  "home.digestSynced": "Emails synced",
  "home.digestPeople": "People created",
  "home.digestOrgs": "Companies created",
  "home.digestDedupe": "Duplicates to review",
  "home.digestClassify":
    "Classified overnight: {commitments} commitments · {meetings} meetings · {noise} noise",
  "home.digestProjects": "Projects",
  "home.digestPhaseChanges": "Phase moves",
  "home.digestNewCommitments": "New commitments",
  "home.digestGoneQuiet": "Gone quiet",
  "home.digestPhaseChange": "{from} → {to}",
  "home.digestCommitmentCount": "{count} new open commitments",
  "home.digestQuietDays": "quiet for {days} days",
  "home.glance.morning": "Good morning, {name}.",
  "home.glance.morningAnon": "Good morning.",
  "home.glance.afternoon": "Good afternoon, {name}.",
  "home.glance.afternoonAnon": "Good afternoon.",
  "home.glance.evening": "Good evening, {name}.",
  "home.glance.eveningAnon": "Good evening.",
  "home.glance.night": "Still at it, {name}.",
  "home.glance.nightAnon": "Still at it.",
  "home.glance.introWeekly": "This is the week you just closed.",
  "home.glance.intro": "Here is your day.",
  "home.panel.decisions": "Waiting on you",
  "home.panel.overnight": "Overnight",
  "home.panel.position": "Position",
  "home.panel.schedule": "Today's schedule",
  "home.schedule.clear": "Nothing is booked today.",
  "home.panel.promises": "Promises & tasks",
  "home.promises.clear": "Nothing is open on you.",
  "home.promises.untracked":
    "Promises made in conversation are not tracked yet — only tasks are listed here.",
  "home.panel.watch": "Gone quiet",
  "home.overnight.fixConnector": "Fix the connection",
  "home.watch.clear": "Nothing has gone quiet.",
  "home.readings.label": "Your morning, in five readings",
  "home.readings.truncated":
    "A source was read to its limit, so every figure above is a floor.",
  "home.readings.openLane": "Open these",
  "home.readings.waiting": "Customer waiting",
  "home.readings.waitingBasis": "waiting on an answer",
  "home.readings.meetings": "Meetings ahead",
  "home.readings.meetingsBasis": "on today's calendar",
  "home.readings.needsPrep_one": "1 needs prep",
  "home.readings.needsPrep_other": "{count} need prep",
  "home.readings.prepUnknown": "not all could be checked",
  "home.readings.prepared": "all prepared",
  "home.readings.promises": "Promises due",
  "home.readings.promisesBasis": "promises are not tracked yet",
  "home.readings.leads": "Lead response",
  "home.readings.leadsBasis": "owed a first answer",
  "home.readings.leadsDue": "next due {value}",
  "home.readings.quota": "Quota pace",
  "home.readings.quotaBasis": "no target is set",
  "home.rail": "Context",
  "home.pct": "{pct}%",
  "home.deck.later": "Later",
  "home.deck.showMore": "Show the whole message",
  "home.deck.showLess": "Show less",
  "home.deck.view": "How the queue is shown",
  "home.deck.viewDeck": "Deck",
  "home.deck.viewList": "List",
  "home.deck.keys":
    "Arrows stage a decision: → accept · ← reject · ↑ edit · ↓ later · U undo · Enter sends the staged ones",
  "home.deck.behind_one": "1 more behind",
  "home.deck.behind_other": "{count} more behind",
  "home.deck.staged_one": "1 decision staged",
  "home.deck.staged_other": "{count} decisions staged",
  "home.deck.commit": "Send staged decisions",
  "home.deck.unstage": "Undo the last one",
  "home.deck.clearedTitle": "Deck clear",
  "home.deck.cleared_one": "1 decision sent",
  "home.deck.cleared_other": "{count} decisions sent",
  "home.deck.clearedTime": "at {at}",
  "home.deck.empty": "Nothing is waiting on you.",
  "home.deck.bundleSummary": "One decision · {count} items",
  "home.deck.bundleMembers": "Show the {count} items",
  "home.brief.rank": "Rank",
  "home.brief.composite": "Score",
  // A deal the rep dismissed, come back. The suppression rule holds a dismissed
  // deal out until a linked activity arrives after the mark, so the sentence
  // states that rule rather than guessing: it can only ever name an activity.
  "home.brief.previouslyDismissed": "Flagged {day} — you dismissed it.",
  "home.brief.returnedWith": "It came back with activity on",
  "home.brief.revenueBasis": "Revenue measured against {amount}",
  "home.brief.resurfaces": "Back",
  "home.evidenceNone": "no evidence recorded",
  "home.snooze": "Snooze",
  "home.snoozedState": "snoozed",

  "enrich.toInbox": "Open the Worklist",

  "deepread.title": "Margince can fill this in",
  "deepread.sub":
    "It reads the company's website for the domain, industry, size, locations and likely decision-makers, then suggests a first move. Findings are staged for your review — nothing is written until you accept.",
  "deepread.cta": "Start company research",
  "deepread.starting": "Starting…",
  "deepread.unavailable": "Site reading is not configured on this server.",
  "deepread.statusQueued": "Queued",
  "deepread.statusDeferred": "Waiting for AI budget",
  "deepread.statusRunning": "Reading…",
  "deepread.statusDone": "Done",
  "deepread.statusPartial": "Stopped early",
  "deepread.statusFailed": "Failed",
  "deepread.statusCancelled": "Cancelled",
  "deepread.resumesAt": "Resumes automatically {when}.",
  "deepread.pagesSoFar_one": "{count} page read so far",
  "deepread.pagesSoFar_other": "{count} pages read so far",
  "deepread.stoppedEarly": "Stopped early: {reason}",
  "deepread.stage.crawling": "Reading the site",
  "deepread.stage.extracting": "Reading what it says",
  "deepread.step.done": "done",
  "deepread.step.running": "under way",
  "deepread.step.queued": "waiting",
  "deepread.stopBudget": "model budget",
  "deepread.stopPageCap": "page cap",
  "deepread.stopByteCap": "byte cap",
  "deepread.stopDeadline": "deadline",
  "deepread.factCount_one": "{count} evidenced fact staged",
  "deepread.factCount_other": "{count} evidenced facts staged",
  "deepread.proposals_other": "{count} proposals waiting for your review",
  "deepread.proposals_one": "{count} proposal waiting for your review",
  "deepread.kindHome": "Home",
  "deepread.kindImpressum": "Impressum",
  "deepread.kindAbout": "About",
  "deepread.kindTeam": "Team",
  "deepread.kindServices": "Services",
  "deepread.kindProducts": "Products",
  "deepread.kindContact": "Contact",
  "deepread.kindOther": "Other",

  "transcriptread.title": "Read this transcript",
  "transcriptread.sub":
    "Find the next steps and commitments this conversation states. Nothing is written until you confirm.",
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
  "transcriptread.proposals_other":
    "{count} next steps waiting for your review",
  "transcriptread.proposals_one": "{count} next step waiting for your review",
  "transcriptread.nothingStated":
    "Read in full. This conversation states no next steps.",
  "transcriptread.failedFallback":
    "This transcript could not be read. Nothing was staged.",

  "create.cancel": "Cancel",
  "create.multiselect.required": "Required — select at least one.",
  "create.save": "Create",
  "create.saving": "Creating…",
  "create.contact": "New contact",
  // The fast path beside it: reading a profile in another window and typing
  // what it says. The label names the ACT, not the source, because the same
  // form takes a conference badge and a business card.
  "vcardImport.action": "Import cards",
  "vcardImport.title": "Import address cards",
  "vcardImport.fileLabel": "Address card file",
  "vcardImport.whichFile":
    "A .vcf file, the format every phone and mail client exports contacts as. A card someone handed you is theirs to give, so these are written straight in rather than queued for approval.",
  "vcardImport.choose": "Choose a .vcf file",
  "vcardImport.working": "Reading the cards…",
  "vcardImport.done": "Close",
  "vcardImport.noCards": "That file held no cards.",
  "vcardImport.outcome.created": "Added",
  "vcardImport.outcome.updated": "Filled in the gaps",
  "vcardImport.outcome.needsReview": "Looks like someone you already have",
  "vcardImport.outcome.skipped": "Skipped",
  "create.quickCapture": "Quick capture",
  "create.quickCaptureSaved": "Saved {name}",
  "create.company": "New company",
  "create.lead": "New lead",
  "create.deal": "New deal",
  "create.fullName": "Full name",
  "create.firstName": "First name",
  "create.lastName": "Last name",
  "create.personTitle": "Title",
  "create.email": "Email",
  "create.phone": "Phone",
  "create.linkedin": "LinkedIn",
  "create.linkedinUrl": "LinkedIn URL",
  "create.displayName": "Company name",
  "create.legalName": "Legal name",
  "create.industry": "Industry",
  "create.sizeBand": "Company size",
  "co.address.summary": "Address",
  "co.address.add": "Add an address",
  "create.addressLine1": "Street and number",
  "create.addressLine2": "Address line 2",
  "create.city": "City",
  "create.region": "State / region",
  "create.postalCode": "Postal code",
  "create.country": "Country (ISO-3166, e.g. DE)",
  "create.companyName": "Company",
  "create.dealName": "Deal name",
  "create.amount": "Value",
  "create.currency": "Currency",
  "create.stage": "Stage",
  "create.organization": "Company",
  "create.expectedClose": "Expected close",

  "field.unset": "Not set",
  "field.addEmail": "Add email",
  "field.addPhone": "Add phone",
  "field.addDomain": "Add domain",
  "field.addLegalName": "Add legal name",
  "field.addIndustry": "Add industry",
  "field.addLinkedinUrl": "Add LinkedIn URL",
  "field.addRegisterVat": "Add VAT ID",
  "field.addRegisteredAddress": "Add registered address",
  "field.addFullName": "Add name",
  "field.addTitle": "Add title",
  "field.addAddressLine1": "Add street and number",
  "field.addAddressLine2": "Add address line 2",
  "field.addPostalCode": "Add postal code",
  "field.addCity": "Add city",
  "field.addRegion": "Add state / region",
  "field.addCountry": "Add country code, e.g. DE",
  "field.country": "Country",
  "field.domain": "Domain",
  "field.domainRequired":
    "A domain cannot be cleared here — use the full editor to remove one.",
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
  "field.yes": "Yes",
  "field.no": "No",

  "dedupe.viewExisting": "View existing record",

  "co.spine.earlierMore": "More conversations before this",
  "co.spine.failed": "The thread could not be read.",
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
  "co.spine.lastSpoke": "You last spoke",
  "co.spine.days_one": "{count} day",
  "co.spine.days_other": "{count} days",
  "co.spine.quietSince": "Silence since then",
  "co.spine.neverReplied": "They have never written back",
  "co.spine.singleThreaded": "One contact, and no reply from them",
  "co.spine.overdue": "Past its date",
  "co.spine.expectedClose": "Expected close",
  "co.360.subject": "{name} · 360",
  "co.360.subjectUnnamed": "This account · 360",
  "today.title": "What needs you",
  "co.spine.earlier_other": "{count} earlier conversations",
  "co.spine.earlier_one": "{count} earlier conversation",
  "today.failed":
    "This could not be assembled. The rest of the page still shows what it could read.",
  "today.quiet": "Nothing here needs you today.",
  "task.untitled": "Untitled task",
  "today.withheld":
    "Hidden from you: {sections}. This list is assembled without them.",
  "today.source.moments": "what Margince found",
  "today.source.nextSteps": "open tasks",
  "today.source.nextMeeting": "the calendar",
  "today.source.deals": "deals",
  "today.meeting.prepare": "Prepare meeting",
  "today.source.people": "the contacts",
  "today.source.standing": "whose move it is and the signals",
  "today.source.activities": "what was said",
  "today.silence.days": "no answer in {count} days",
  "today.draft.to": "Draft follow-up to {name}",
  "today.draft.act": "Draft",

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
  "acctCoverage.title": "Who covers this account",
  "acctCoverage.contact": "Contact",
  "acctCoverage.findContact": "Find a contact",
  "acctCoverage.untried": "Untried",
  "acctCoverage.noMatch": "No contact matches that.",
  "acctCoverage.columnCap":
    "Showing {cap} colleagues — deselect one to add another.",
  "acctCoverage.partial":
    "This grid was built from a partial read, so a blank cell may mean the read stopped short rather than that nobody has tried.",
  "acctCoverage.noneButPartial":
    "Nobody connected was returned, but the read was capped — this is not a claim that nobody covers this account.",
  "acctCoverage.noneAtAll":
    "Nobody here has exchanged messages with anyone at this company yet.",
  "docs.title": "Documents",
  "docs.empty": "No documents on this account yet.",
  "docs.noneInCategory": "No documents of that kind on this account.",
  "docs.allOnAgreements":
    "Every document here is filed against an agreement above.",
  "docs.allSuperseded":
    "Only superseded documents are left here. Show them to read the history.",
  "docs.superseded.show": "Show superseded",
  "docs.superseded.hide": "Hide superseded",
  "docs.superseded.hidden_one": "1 superseded document is hidden.",
  "docs.superseded.hidden_other": "{count} superseded documents are hidden.",
  "docs.superseded.shown_one": "1 superseded document is listed below.",
  "docs.superseded.shown_other":
    "{count} superseded documents are listed below.",
  "docs.reading.show": "Read this document",
  "docs.reading.hide": "Hide the reading",

  // Adding one. The "About" wording is doing real work: it decides whether the
  // file becomes evidence in a deal or a paper about the account, and only the
  // first can be read for deal fields — so the hint says so rather than leaving
  // the reader to discover it from a panel that never appears.
  "docs.add.action": "Add a document",
  "docs.add.title": "Add a document",
  "docs.add.about": "About",
  "docs.add.aboutHint":
    "A document on a deal can be read for deal fields; one on the company cannot.",
  "docs.add.thisCompany": "This company",
  "docs.add.aDeal": "A deal",
  "docs.add.dealSearch": "Search this account's deals",
  "docs.add.dealSearchReach":
    "The search covers this account's {deals} newest deals and offers the first {matches} matches. A deal older than those cannot be picked here.",
  "docs.add.category": "Category",
  "docs.add.name": "Title",
  "docs.add.nameHint": "Optional. Left blank, the row shows the filename.",
  "docs.add.file": "File",
  "docs.add.fileHint": "Up to {size}.",
  "docs.add.fileEmpty": "Drop the file here, or click to choose one",
  "docs.add.cancel": "Cancel",
  "docs.add.submit": "Upload",
  "docs.add.uploading": "Uploading…",
  "docs.add.errNoFile": "Choose a file to upload.",
  "docs.add.errNoDeal": "Pick the deal to file this against.",
  "docs.add.errRefused": "You may not add documents to this record.",
  "docs.add.errTooLarge":
    "That file is larger than {size}, which is the most this installation accepts. Choose a smaller one.",
  "docs.add.failedTitle": "The upload did not go through",
  "docs.add.failed": "Nothing was stored. Try again, or choose another file.",
  "docs.add.partialTitle": "Uploaded, but not filed",
  "docs.add.partial":
    "The file is on the record and listed below. Only its category and title were not saved, so it is filed under Other.",

  // The staged-document-reading panel (RD-AC-N-2/-3). Three states that must
  // stay apart in the words as well as in the data: not answered yet, answered
  // and states none of them, could not be read at all.
  "extraction.neverRead": "Nobody has read this file for deal fields yet.",
  "extraction.readIt": "Read this file",
  "extraction.readAgain": "Try reading it again",
  "extraction.starting": "Starting…",
  "extraction.startFailed":
    "That file could not be sent for reading. Nothing was changed.",
  "extraction.loading": "Checking whether this file has been read…",
  "extraction.reading": "Reading this file…",
  "extraction.failed": "This file could not be read.",
  "extraction.groundedNothing":
    "AI read this file and it states none of the deal fields.",
  "extraction.heading_one":
    "AI read this file — {count} field it can ground, staged for your record (accept to persist)",
  "extraction.heading_other":
    "AI read this file — {count} fields it can ground, staged for your record (accept to persist)",
  "extraction.accept_one": "Accept {count} field",
  "extraction.accept_other": "Accept {count} fields",
  "extraction.dismiss": "Dismiss",
  "extraction.dismissed": "Nothing was written. The file stays attached.",
  "extraction.acceptedLabel": "Accepted fields",
  "extraction.acceptedHeading_one":
    "{count} field accepted to the deal — original snippets retained",
  "extraction.acceptedHeading_other":
    "{count} fields accepted to the deal — original snippets retained",
  "extraction.acceptFailed":
    "Those fields were not written. Nothing on the deal changed.",
  "extraction.edit": "Edit",
  "extraction.editValue": "Edit {field}",
  "extraction.omitted.notStated": "omitted (not stated in this file)",
  "extraction.omitted.notConfident":
    "omitted (this file says something, but not clearly enough to accept)",
  "extraction.field.name": "Deal name",
  "extraction.field.amount": "Amount",
  "extraction.field.currency": "Currency",
  "extraction.field.closeDate": "Expected close date",
  "docs.filterLabel": "Documents by kind",
  "docs.category.all": "All",
  "docs.category.contract": "Contract",
  "docs.category.offer": "Offer",
  "docs.category.legal": "Legal",
  "docs.category.email": "Email attachment",
  "docs.category.message": "Message attachment",
  "docs.category.other": "Other",
  "files.title": "Files",
  "files.sub":
    "What you uploaded on this deal, and what arrived with its emails and messages.",
  "files.empty":
    "No files on this deal yet. Upload one, or link an email that carries one.",
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
  "files.hideHidden": "Hide the hidden files",
  "docs.state.draft": "Draft",
  "docs.state.current": "Current",
  "docs.state.final": "Final",
  "docs.state.superseded": "Superseded",
  "log.title": "Log activity",
  "log.addTask": "Add task",
  "log.sub": "a note or task, straight onto this timeline",
  "log.kind": "Type",
  "log.kindNote": "Note",
  "log.kindTask": "Task",
  "log.kindMeeting": "Meeting",
  "log.kindCall": "Call",
  "log.attendee": "Who was there",
  "log.subject": "Subject",
  "log.body": "Details",
  "log.transcriptLabel": "Transcript",
  "log.transcriptHint":
    "Paste it from your meeting tool (Teams, Zoom, Meet…) — speaker labels, if any, are kept.",
  "log.asTranscript": "This text is a transcript",
  "log.transcriptUpload": "Or upload a file",
  "log.transcriptUploadRejected": "Only a .txt file is accepted.",
  "log.transcriptUploadFailed":
    "Could not read that file — try pasting the text instead.",
  "log.dueAt": "Due date",
  "log.date": "Date",
  "log.save": "Log",
  "log.saving": "Logging…",

  "personAccess.title": "Who can see this contact",
  "personAccess.privateToYou":
    "Private to you. Your mailbox created this contact, and nobody else in the organization can see it — not your team, and not an admin.",
  "personAccess.organization":
    "Everyone in the organization can see this contact.",
  "personAccess.share": "Share with the organization",
  "personAccess.published": "The organization can see this contact now.",
  "compose.reply": "Reply",
  "compose.writeEmail": "Write email",
  "compose.relink": "Relink",
  "compose.draftWithAi": "Draft with AI",
  "compose.drafting": "Drafting…",
  "compose.discardDraft": "Discard draft",
  "compose.discardDraftHint":
    "Tells your Voice DNA this draft missed. The generated text is never kept.",
  "compose.aiDisclosureTitle": "AI-assisted draft",
  "compose.aiDisclosureFallback":
    "This draft was produced by AI. Read it and edit it before you send.",
  "compose.voiceVersion": "Built from your corpus · v{n}",
  "compose.voiceDegraded":
    "Your voice profile couldn't be loaded, so this draft is not written in your voice. Draft again, or edit before sending.",
  "compose.provisional": "Provisional voice",
  "compose.provisionalHint":
    "Your Voice DNA is still being built. It already shapes this draft exactly as a finished one would — nothing is held back.",
  "compose.intent": 'Steer the draft (optional), e.g. "polite follow-up"',
  "compose.to": "To",
  "compose.answering": "Replying to “{subject}” · {when}",
  "compose.answeringTo": "Replying to {who} · “{subject}” · {when}",
  "compose.answeringNoSubject": "Replying to the message of {when}",
  "compose.answeringNothing":
    "No earlier message here — this starts a new thread.",
  "compose.cc": "Cc",
  "compose.subject": "Subject",
  "compose.noGroundableRecipient":
    "No contact on this account yet — write the message yourself, or add a contact first",
  "compose.draftTo": "Draft to",
  "compose.draftToUnset": "Choose a contact",
  "compose.relatedTo": "Related to",
  "compose.relatedToNone": "The account in general",
  "compose.project": "Project",
  "compose.projectNone": "No project",
  "compose.scopedToCounted":
    "Scoped to {key} · {inScope} of {total} activities",
  "compose.scopedTo": "Scoped to {key}",
  // A channel reply has no picker: its send carries no filing field, so the
  // conversation's own project is inherited. This is the disclosure that
  // replaces the choice.
  "compose.channelFiling":
    "Will be filed under {project}, with the conversation it answers.",
  "compose.basedOn": "Based on: {inputs}",
  "compose.whyThisDraft": "Why this draft?",
  "compose.body": "Body",
  "compose.bodyHint": "Click into the text to edit it.",
  "calendar.previousMonth": "Previous month",
  "calendar.nextMonth": "Next month",
  "compose.schedulePick": "Pick date and time",
  "compose.scheduleDate": "Date",
  "compose.scheduleTime": "Time",
  "compose.scheduleGoesOut": "Goes out {when}",
  "compose.willGoOut": "Will go out {when}",
  "compose.scheduleAfternoon": "Tomorrow afternoon",
  "compose.rewrite": "Rewrite",
  "compose.rewriteShorter": "Shorter",
  "compose.rewriteShorterAsk": "Say the same thing in fewer words.",
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
  "compose.scheduleNow": "Send now instead",
  "compose.why": "Why are you writing?",
  "compose.whyHint":
    "The record decides what is allowed; this says what you are doing so the answer can be checked against it.",
  "compose.why.requestedFollowup": "They asked me to get in touch",
  "compose.why.activeDeal": "About a deal we are working on",
  "compose.why.quote": "A quote or proposal they asked for",
  "compose.why.service": "Support for something they bought",
  "compose.why.invoice": "About an invoice or a payment",
  "compose.why.contract": "About their contract",
  "compose.why.account": "About their account",
  "compose.why.marketing": "Marketing",
  "sendPermission.refused": "You cannot send this message.",
  "sendPermission.sayWhy": "Say why you may write",
  "sendPermission.unproven":
    "Margince has no record of why you may write to them.",
  "sendPermission.unprovenHint":
    "If you know why — they asked you to, you met, they are a customer — say so and it is recorded against your name.",
  "sendPermission.reason.objected":
    "They asked not to receive marketing. Nobody here can lift that, including an administrator.",
  "sendPermission.reason.withdrawn":
    "They took back their permission. Nobody here can lift that, including an administrator.",
  "sendPermission.reason.restricted":
    "Their data is under a processing restriction. Nobody here can lift that, including an administrator.",
  "sendPermission.reason.bounced":
    "That address does not accept mail. Correcting it is the fix, not an override.",
  "sendPermission.reason.tooMany":
    "They have had as many marketing messages as the rules here allow for now. This clears on its own.",
  "sendPermission.reason.ambiguous":
    "More than one record shares this address, so Margince cannot tell who the message is for. Merging them is the fix.",
  "sendPermission.reason.unconfirmed":
    "They have not confirmed they want to hear from us. Only they can do that.",
  "sendPermission.reason.other":
    "This message cannot be sent, and it is not something a seat here can overrule.",
  "compose.derivedReply":
    "This continues their own message, so it needs no reason from you.",
  "compose.sendLaterLabel": "Send later (optional)",
  "compose.send": "Send",
  "compose.sendConfirmTitle": "Send this email?",
  "compose.threadHeading": "This conversation",
  "compose.continueHeading": "Continue a conversation?",
  "compose.threadLeave": "Choose another",
  "compose.messageCount_one": "{count} message",
  "compose.messageCount_other": "{count} messages",
  "compose.threadContinuing": "The last exchange, which this will continue",
  "compose.threadPending": "Loading the conversation\u2026",
  "compose.sendBody":
    "You are sending this email now. This is an outbound, irreversible action.",
  // A moment picked in the field above turns this dialog into a different
  // promise, so it says a different thing. The three sentences it replaces all
  // claim the send is happening NOW and is irreversible; a scheduled message is
  // neither, and it can be moved or withdrawn until it goes.
  "compose.schedule": "Schedule send",
  "compose.scheduleConfirmTitle": "Schedule this email?",
  // The composer computed that it had scheduled a send and said nothing —
  // it closed the way a SENT message closes it. The confirm dialog above
  // promises a place to move or withdraw the message from; these two are how a
  // rep gets there.
  "compose.scheduledQueued": "Scheduled. It has not gone out yet.",
  "compose.scheduledOpenQueue": "Scheduled messages",
  "compose.scheduleBody":
    "This does not go out now. It waits for the moment you picked, and the consent and mailbox checks run again then. Until it goes you can move it or take it back from Scheduled messages.",
  "compose.sendMessageConfirmTitle": "Send this message?",
  "compose.sendMessageBody":
    "You are sending this message now. This is an outbound, irreversible action.",
  "compose.consentBlockedTitle": "Send blocked — no consent",
  "compose.consentBlocked":
    "A recipient has not granted consent for this purpose, so the send was suppressed (default-deny).",
  "compose.consentGoto": "Review consent",
  "compose.draftUnavailable":
    "AI drafting is unavailable (the model is not configured). You can still write the email yourself.",
  "compose.sendUnavailable":
    "Sending is unavailable (no mailer is configured).",
  "compose.mailboxNotSendCapable":
    "Your mailbox is connected for capture but was never granted permission to send. Reconnect it and approve sending — a mailbox connected before sending existed cannot be upgraded in place.",
  "compose.mailboxNotSendCapableGoto": "Reconnect your mailbox",
  "compose.sharedUnsubscribeToken":
    "A message carrying an unsubscribe link reaches one addressee at a time, because that link is the recipient's own consent record. Send it once per recipient, with no Cc.",
  "compose.multiRecipientWarning":
    "This purpose carries an unsubscribe link, so a send to more than one addressee will be refused. Send it once per recipient, with no Cc.",
  "compose.relinkTitle": "Relink this activity",
  "compose.relinkTarget":
    "Search a person, organization, deal, lead, or project",
  "compose.relinkNoVersion":
    "This activity was read without a version, so a relink cannot say what it is changing. Reopen it and try again.",
  "compose.relinkReplace": "Move instead of also-link",
  "compose.relinkReplaceHint":
    "Replaces the existing link of the same type rather than adding another.",
  "compose.relinkConfirm": "Relink",
  "compose.relinkThread": "Also move the rest of this conversation",
  "compose.relinkThreadHint":
    "Every message in this thread you can edit moves with it, in one step.",
  "compose.emptyRecipients": "Add at least one recipient.",
  "compose.missingSubject": "Give this email a subject.",
  "compose.missingBody": "Write the message before sending it.",
  "compose.missingWhy": "Say why you are writing to them.",
  "compose.removeRecipient": "Remove {recipient}",
  "compose.actionFailed": "The request failed. Please try again.",

  "tasks.complete": "Done",
  "tasks.snooze": "Snooze 1d",
  "tasks.detail": "Task",
  "tasks.detailLoading": "Reading this task…",
  "tasks.isDone": "Completed",
  "tasks.logged": "Logged",

  "analytics.sub":
    "open deals only, each converted into {currency} — unweighted next to weighted",
  "analytics.currency": "Currency",
  "analytics.count": "Deals",
  "analytics.unweighted": "Unweighted",
  "analytics.weighted": "Weighted",
  "analytics.planNote":
    "the executed plan and the rows this number reconciles to",
  "analytics.reportDeals": "Open pipeline by stage",
  "analytics.sections": "Analytics sections",
  "analytics.sectionForecast": "Forecast",
  "analytics.sectionPipeline": "Pipeline",
  "analytics.share.open": "Share view",
  "analytics.share.title": "Share this view",
  "analytics.share.kindLegend": "What the link shows",
  "analytics.share.liveLabel": "Live view",
  "analytics.share.liveHelp":
    "Recomputed each time it is opened, under what the reader may see. The numbers move as the pipeline does.",
  "analytics.share.snapshotLabel": "Frozen state",
  "analytics.share.snapshotHelp":
    "The figures as they stood when the state was taken. They do not change, so the link says which moment it describes.",
  "analytics.share.snapshotUnavailable":
    "No state has been frozen for this period yet.",
  "analytics.share.expiryNote":
    "The link stops working after 30 days. You can close it sooner.",
  "analytics.share.create": "Create link",
  "analytics.share.linkTitle": "Your link",
  "analytics.share.linkWarning":
    "This is the only time the link is shown. Copy it now — it cannot be read back.",
  "analytics.share.leaveWarning":
    "Leaving without copying discards the link. You would have to create another.",
  "analytics.share.copy": "Copy link",
  "analytics.share.copied": "Copied",
  "analytics.share.copyFailed":
    "The link could not be copied. Select it above and copy it by hand.",
  "analytics.share.done": "Done",
  "analytics.frame": "As of {asOf} · {zone}",
  "review.title": "What should be checked before the call?",
  "review.ready": "Ready",
  "review.readyWithExceptions": "Ready, with notes",
  "review.needsReview": "Needs review",
  "review.checksIncomplete": "Checks incomplete",
  "review.allSourcesRead": "Every source was read.",
  "review.sourcesUnread":
    "Not read: {sources}. Findings below cover only what could be checked.",
  "review.notCheckedYet":
    "Nothing has been checked yet — no nightly run has completed for this installation. The readings above stand on the records as they are.",
  "review.nothingToCheck": "Nothing to check.",
  "review.answer": "Answer",
  "review.closePast": "Close date has passed",
  "review.closeUnconfirmed": "Close date not confirmed",
  "review.closePushed": "Close date keeps moving",
  "review.amountVsOffer": "Amount disagrees with the offer",
  "review.amountVsContract": "Amount disagrees with the contract",
  "review.noNextStep": "No next step",
  "review.noEconomicBuyer": "Nobody identified who can sign",
  "review.buyerSilent": "Buyer has gone quiet",
  "review.commitUnpriced": "Committed with no amount",
  "review.unknownCheck": "Something to check",
  "review.sheetTitle": "Answer this check",
  "review.outcomeLegend": "What kind of answer is this?",
  "review.fixedRecord": "I corrected the record",
  "review.addedEvidence": "I added the evidence",
  "review.valueCorrect": "The value is correct",
  "review.notRelevant": "Not relevant to this deal",
  "review.remindLater": "Not now",
  "review.reassign": "Somebody else's to answer",
  "review.hidesUntilExpiry": "Hides this check until it expires.",
  "review.reason": "Why",
  "review.reasonHelp":
    "The next person to see this number is owed the reason it is not flagged.",
  "review.remindAt": "Bring it back on",
  "review.expiresAt": "Stops holding on",
  "review.expiresHelp":
    "At most 90 days: a value that was correct in May is a claim about May.",
  "review.cancel": "Cancel",
  "review.submit": "Save answer",
  "forecast.question": "Where will we land this quarter?",
  "forecast.answerWithCall":
    "The current call is {call}. Evidence supports {evidence}.",
  "forecast.answerNoCall":
    "Nobody has called this period yet. Evidence supports {evidence}.",
  "forecast.partialTitle": "Not every deal is priced",
  "forecast.partial":
    "{priced} of {eligible} deals carry an amount. The rest are real pipeline contributing nothing to the totals above.",
  "forecast.currentCall": "Current call",
  "forecast.evidence": "Supported by evidence",
  "forecast.alreadyWon": "Already won",
  "forecast.updateCall": "Update the current call",
  "forecast.callExplains":
    "A call is what you believe will close. It records your number and changes no deal.",
  "forecast.expectedTotal": "Expected total for this period",
  "forecast.supportingNote": "Supporting note",
  "forecast.cancel": "Cancel",
  "forecast.saveCall": "Save call",
  "analytics.scopeLabel": "Which records these numbers cover",
  "analytics.scopeFixed": "These numbers cover {scope}.",
  "forecast.receipt": "Data and evidence checked",
  "forecast.eligible": "Eligible deals",
  "forecast.priced": "Priced",
  "forecast.confirmed": "Close date confirmed",
  "forecast.fxMissing": "Exchange rate missing",
  "analytics.reportForecast": "Forecast categories",
  "analytics.reportOpenByCompany": "Open deals per company",
  "analytics.forecastBanner":
    "Each tile shows the raw total and, beneath it, the probability-weighted total — rounded per deal, so it always reconciles to Explain This Number.",
  "analytics.company": "Company",
  "analytics.openStageDeals": "Open the deals in {stage}",
  "analytics.openCompanyDeals": "Open this company's deals",
  "analytics.noCompany": "No company",
  "analytics.openDeals": "Open deals",
  "explain.sources": "Source rows",
  "explain.col.record": "Deal",
  "explain.col.stage": "Stage",
  "explain.col.owner": "Owner",
  "explain.col.pipeline": "Pipeline",

  "ai.sub": "bring your own agent — governed by the two-tier contract",
  "ai.tiers": "What an agent may do",
  "ai.tierAutoExecute": "Read & draft run instantly.",
  "ai.tierAutoExecuteDetail":
    "Lookups, summaries, drafts — visible, reversible, logged.",
  "ai.tierConfirmationRequired": "Sensitive changes wait for you.",
  "ai.tierConfirmationRequiredDetail":
    "New custom fields, webhook subscriptions and paid enrichment stage into the inbox first. Most record changes and sends run instantly, within the scopes you granted.",
  "ai.connect": "Connect an agent",
  "ai.connectDetail":
    "Point any MCP-capable agent at your organization and approve the access it asks for. There is nothing to set up first.",
  "ai.paletteHint": "Ask from anywhere with",

  "settings.accountCard": "Your account",
  "unsaved.title": "You have unsaved changes",
  "unsaved.body":
    "Leaving this page now discards what you have typed. Go back to save it first.",
  "unsaved.discard": "Discard changes",
  "settings.addedItem": "“{name}” added",
  "settings.removedItem": "“{name}” removed",
  "settings.removed": "Removed.",
  "settings.saved": "Saved.",
  "settings.signature": "Email signature",
  "settings.signatureSub":
    "Appended below every message you send, above the unsubscribe footer.",
  "settings.signatureLabel": "Your sign-off",
  "settings.signaturePlaceholder": "Marek Janetzke\nGradion · +49 40 123456",
  "settings.signatureHint":
    "Plain text. Leave it empty to send unsigned. The AI never writes a sign-off — this is the one that goes out.",
  "settings.signatureSaving": "Saving…",
  "settings.signatureEdit": "Edit signature",
  "settings.signatureNone": "No sign-off set",
  "settings.signatureCancel": "Cancel",
  "brief.coverage.summary": "Which sources had more",
  "brief.coverage.bounded": "{shown} shown of at least {considered} read",
  "delivery.morningLabel": "Your morning brief",
  "delivery.morningHelp":
    "Whether the day's brief also arrives by email. It is on your Brief page either way.",
  "delivery.weeklyLabel": "Your weekly review",
  "delivery.weeklyHelp": "Whether Monday's review also arrives by email.",
  "delivery.byEmail": "By email",
  "delivery.none": "Not by email",
  "settings.languageHelp": "Lasts for this session.",
  "role.admin": "Admin",
  "role.management": "Management",
  "role.manager": "Team Lead",
  "role.rep": "User",
  "role.readOnly": "Read-only",
  "role.ops": "Ops",
  "inlineChoice.change": "Change {field}",
  "rbac.masked": "Masked value",
  "settings.passports": "Agent passports",
  "settings.passportsSub":
    "An agent acts as you, never above you: every call re-checks your RBAC.",
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
    "Credentials you have minted for scripts and integrations. Connecting an MCP client does not use these — it creates its own connection, listed below.",
  "settings.passportLabel": "Agent name",
  "settings.mint": "Mint passport",
  "settings.minting": "Minting…",
  "settings.mintCancel": "Cancel",
  "settings.mintDone": "Done",
  "settings.mintOpen": "New passport",
  "settings.passportScopes": "What this agent may do",
  "settings.passportScopesHint":
    "Pick at least one. An agent can never do more than you can.",
  "settings.passportScopesRequired":
    "Pick at least one thing this agent may do.",
  // What the scheduled agent is doing for this reader, one line per (kind,
  // state). First person for what Margince did, result first, and never a word
  // that reads as finished on a run that stopped part-way.
  "agent.activity.weeklyReview.queued": "Your week is queued for a summary.",
  "agent.activity.weeklyReview.running": "Summarising your week…",
  "agent.activity.weeklyReview.stalled":
    "The summary of your week is taking longer than expected.",
  "agent.activity.weeklyReview.done": "Your week has a summary.",
  "agent.activity.weeklyReview.degraded":
    "Your week is measured, without a summary — the numbers are all there.",
  "agent.activity.weeklyReview.failed":
    "No summary of your week this time. The numbers are still the week's own.",
  "agent.activity.morningBrief.queued": "Your morning brief is queued.",
  "agent.activity.morningBrief.running":
    "I'm putting your morning brief together.",
  "agent.activity.morningBrief.done": "Your morning brief is ready.",
  "agent.activity.morningBrief.degraded":
    "I got partway through your morning brief and stopped.",
  "agent.activity.morningBrief.failed": "I couldn't finish your morning brief.",
  "agent.activity.morningBrief.stalled":
    "Your morning brief has been running unusually long. It may have stopped.",
  "agent.activity.riskSweep.queued": "The overnight risk sweep is queued.",
  "agent.activity.riskSweep.running": "I'm checking your deals for risk.",
  "agent.activity.riskSweep.done":
    "Done. I checked your deals for risk overnight.",
  "agent.activity.riskSweep.degraded":
    "I got partway through the risk sweep and stopped.",
  "agent.activity.riskSweep.failed":
    "I couldn't finish the overnight risk sweep.",
  "agent.activity.riskSweep.stalled":
    "The risk sweep has been running unusually long. It may have stopped.",
  "agent.activity.documentExtract.queued":
    "Your document is queued to be read.",
  "agent.activity.documentExtract.running": "I'm reading your document.",
  "agent.activity.documentExtract.stalled":
    "Reading your document has taken unusually long. It may have stopped.",
  "agent.activity.documentExtract.done": "I've read your document.",
  "agent.activity.documentExtract.degraded":
    "I got partway through your document and stopped.",
  "agent.activity.documentExtract.failed": "I couldn't read your document.",
  // The same six, with the document NAMED. A rail that says "I'm reading your
  // document" reports that software is busy; one that says "I'm reading
  // Q3-offer.pdf" reports the reader's own afternoon. The name is the source's
  // snapshot of what the product calls that document elsewhere, so these are
  // used only when it sent one — every kind falls back to the pair above.
  "agent.activity.documentExtractNamed.queued": "{name} is queued to be read.",
  "agent.activity.documentExtractNamed.running": "I'm reading {name}.",
  "agent.activity.documentExtractNamed.stalled":
    "Reading {name} has taken unusually long. It may have stopped.",
  "agent.activity.documentExtractNamed.done": "I've read {name}.",
  "agent.activity.documentExtractNamed.degraded":
    "I got partway through {name} and stopped.",
  "agent.activity.documentExtractNamed.failed": "I couldn't read {name}.",
  // The AI work a person ASKS for and then waits on. Same rules as the
  // scheduled lines above — first person, result first, and never a word that
  // reads as finished on a run that stopped part-way.
  //
  // These four were deliberately not narrated until the router could say
  // `running`: reporting them settled-only meant a line that appeared already
  // finished, which tells a waiting reader nothing they did not already know.
  "agent.activity.summarize.queued": "Reading up on this company is queued.",
  "agent.activity.summarize.running":
    "I'm pulling together what I know about this company.",
  "agent.activity.summarize.done": "What I know about this company is ready.",
  "agent.activity.summarize.degraded":
    "I gathered some of what I know about this company and stopped.",
  "agent.activity.summarize.failed":
    "I couldn't finish reading up on this company.",
  "agent.activity.summarize.stalled":
    "Reading up on this company has taken unusually long. It may have stopped.",
  "agent.activity.draftReply.queued": "Your reply is queued to be drafted.",
  "agent.activity.draftReply.running": "I'm drafting your reply.",
  "agent.activity.draftReply.done": "Your draft reply is ready.",
  "agent.activity.draftReply.degraded":
    "I got partway through your reply and stopped.",
  "agent.activity.draftReply.failed": "I couldn't draft your reply.",
  "agent.activity.draftReply.stalled":
    "Drafting your reply has taken unusually long. It may have stopped.",
  "agent.activity.offerDraft.queued": "Your offer is queued to be drafted.",
  "agent.activity.offerDraft.running": "I'm drafting your offer.",
  "agent.activity.offerDraft.done": "Your draft offer is ready.",
  "agent.activity.offerDraft.degraded":
    "I got partway through your offer and stopped.",
  "agent.activity.offerDraft.failed": "I couldn't draft your offer.",
  "agent.activity.offerDraft.stalled":
    "Drafting your offer has taken unusually long. It may have stopped.",
  "agent.panel.runningNow": "Running now",

  "agents.connected": "Connected agents",
  "agents.connectedSub":
    "MCP clients holding their own credential, scoped to what you ticked when you authorized them",
  "agents.noneConnected": "No agent is connected yet.",
  "agents.connectedOn": "connected {date}",
  "agents.disconnect": "Disconnect",
  "agents.disconnectOpen": "Disconnect",
  "agents.disconnectNamed": "Disconnect {client}",
  "agents.disconnected": "disconnected",
  "agents.lapsed": "credential expired",
  "agents.renewing": "renewing",
  "agents.renewsBy": "credential renews by {date}",
  "agents.expiredOn": "credential expired {date}",
  "agents.revokeGrantOpen": "End connection",
  "agents.revokeGrantNamed": "End the connection to {client}",
  "agents.disconnectConfirm":
    "This ends the whole connection, not just one credential: the agent loses access on its next call and cannot renew. Reconnecting means approving access again.",
  "agents.connectHow": "Connect an agent",
  "agents.connectSteps":
    "Run one of these. The client registers itself and brings you back here to choose the access it can have.",
  "agents.connectAntigravityPath":
    "Antigravity has no add command — put that block in ~/.gemini/config/mcp_config.json.",
  "agents.connectorOff": "The MCP connector is off for this installation.",
  "agents.connectorOffDetail":
    "No agent can connect until an operator enables it. Your passports still work as REST credentials.",
  "settings.tokenOnce": "Copy it now — you'll only see this token once.",
  "settings.token": "token",
  "settings.autonomy": "Autonomy tiers",
  "settings.autonomySub": "what runs instantly vs. what waits in the inbox",
  "settings.tierRead": "Read, summarize, draft — runs instantly, fully logged.",
  "settings.tierSend":
    "Send email, book meetings, update a contact or a deal — runs instantly too, if you gave the agent that scope. Your grant is the approval, given once.",
  "settings.tierWait":
    "Enrichment, custom fields, webhooks, merging tags — these wait in your inbox.",
  "settings.tierAdvance":
    "Advance a deal stage — waits only when the move closes the deal as won or lost.",
  "settings.locked": "locked",
  "settings.purposes": "Consent purposes",
  "settings.purposesSub":
    "What this installation asks consent for, and which lawful basis each purpose stands on.",
  "settings.created": "created {date}",
  "settings.expires": "expires {date}",
  "settings.revoked": "revoked",
  "settings.revoke": "Revoke",
  "settings.revokeConfirm":
    "This passport's credential is invalidated immediately — the agent loses access on its next call.",
  "import.withheld":
    "Importing a file is an admin or ops action — this installation still has one, it is not yours to run.",
  "import.title": "Import a file",
  "import.sub":
    "Bring a CSV of prospects or companies into the estate. Nothing is written until you have read what it will do.",
  "import.startLabel": "Import a CSV file",
  "import.start": "Start an import",
  "import.objectLabel": "What the rows are",
  "import.object.lead": "Prospects",
  "import.object.organization": "Companies",
  "import.object.person": "People",
  "import.objectHint.lead":
    "An unworked list lands as leads for a human to qualify before anyone treats them as contacts.",
  "import.objectHint.organization":
    "Companies are matched by the name you map, so a re-upload corrects rather than duplicates.",
  "import.objectHint.person":
    "For people you already deal with. Matched by email, so a re-upload corrects rather than duplicates, and an address already held is left alone.",
  "import.fileLabel": "The CSV to import",
  "import.choose": "Choose a file",
  "import.chooseAnother": "Choose a different file",
  "import.profiled": "Read from the first {rows} rows of the file.",
  // The name the mapping grid announces once it is wider than its box.
  "import.mappingTable": "Column mapping",
  "import.col.column": "Column",
  "import.col.filled": "Filled",
  "import.col.samples": "Values",
  "import.col.destination": "Goes to",
  "import.dontImport": "Don't import",
  "import.noSamples": "empty",
  "import.destinationFor": "Where {column} goes",
  "import.identifiedBy":
    "Rows are identified by {column}, so re-importing this file updates rather than duplicates.",
  "import.needsIdentifier":
    "Map a column to {field}. Without it no row can be recognized on a second upload, or undone.",
  "import.validate": "Check what this will do",
  "import.validating": "Checking…",
  "import.previewTitle": "What this import will do",
  "import.outcomeTitle": "What this import did",
  // Shown when the card read this run back on mount instead of the reader
  // having just caused it: an outcome with no press behind it reads as an
  // import that ran by itself.
  "import.resumedRun":
    "Picked up from earlier: this import ran on {when}. Everything below is still open to you.",
  "import.count.created": "Create",
  "import.count.updated": "Update",
  "import.count.unchanged": "Unchanged",
  "import.count.skipped": "Skipped",
  "import.rowsRead": "{rows} rows read, identified by {column}.",
  "import.linksOffered":
    "{offered} rows name an employer; {unresolved} name a company that is not in the CRM yet.",
  "import.linksApplied": "{applied} of {offered} employer links written.",
  "import.issuesLead":
    "Some rows cannot be imported. They are listed with the line to open in your file.",
  "import.issueLine": "Line {line}:",
  "import.commit_one": "Import 1 row",
  "import.commit_other": "Import {rows} rows",
  "import.importing": "Importing…",
  "import.done": "The import finished.",
  "import.failed":
    "The import stopped after {checkpoint} rows. Resuming continues from there rather than starting again.",
  "import.resume": "Resume the import",
  "import.another": "Import another file",
  "import.undo_one": "Undo this import (1 row)",
  "import.undo_other": "Undo this import ({rows} rows)",
  "import.undoing": "Undoing…",
  "import.undoInterrupted":
    "The undo was interrupted partway through. Continuing picks up where it stopped, not from the start.",
  "import.continueUndo": "Continue the undo",
  "import.undone": "The import was undone.",
  "import.undoReversed_one": "1 row reversed.",
  "import.undoReversed_other": "{rows} rows reversed.",
  "import.undoKeptLead": "Kept — you edited these since the import:",
  "import.undoErroredLead":
    "Could not be reversed — left exactly as they stood:",
  "settings.dangerZone": "Danger zone",
  "settings.dangerZoneSub":
    "Non-production only — irreversible on this installation.",
  "settings.resetDataDesc":
    "Reset this installation to its first-boot state. Domain and configuration data is wiped; the organization and its users are preserved and stay signed in.",
  "settings.resetDataButton": "Reset data",
  "settings.resetDataLabel": "Reset all data",
  "settings.resetDataConfirmButton": "Reset everything",
  "settings.resetDataConfirmTitle": "Reset all data?",
  "settings.resetDataConfirmBody":
    "Type your organization's name to confirm. This cannot be undone.",
  "settings.resetDataConfirmName": "Type this organization name:",
  "settings.resetDataConfirmLabel": "Confirm organization name",
  "settings.resetDataResult":
    "Cleared {tables} tables, {jobs} job rows, {streams} event streams, {keys} cache keys and {objects} stored files.",
  "settings.resetDataDrainWarning":
    "A background job was still running when the reset began. It will fail against the wiped data — harmless, but expect one error in the log.",

  "settings.jobs": "Background jobs",
  "settings.jobsSub": "What the queue is holding, and whose work failed.",
  "jobs.adminOnly":
    "Only an admin can see background-job health. It reports work across the whole installation, so it is not shown more widely.",
  "jobs.empty":
    "Nothing in the background queue — no work waiting, running, retrying or dead.",
  "jobs.workspaceKinds": "This organization",
  "jobs.workspaceEmpty":
    "No background work of any kind for this organization.",
  "jobs.dispatcherKinds": "Fleet dispatchers",
  "jobs.dispatcherSub":
    "Rows that carry no organization: a dispatcher fans work out to every organization and does none of its own. Their counts belong to the installation, not to you.",
  "jobs.dispatcherEmpty":
    "No dispatcher rows. The periodic ticks re-insert them, so an empty list means none is scheduled right now.",
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
  "jobs.deadTitle": "Dead work needs a hand",
  "jobs.deadBody":
    "{count} jobs are discarded or cancelled: that work will not happen without intervention. A discarded job spent every attempt; a cancelled one was stopped deliberately. Read the failures below before re-queueing anything.",
  "jobs.failures": "Recent failures",
  "jobs.failuresSub":
    "Most recent first, capped at 50. A bounded list, not a log.",
  "jobs.failuresEmpty": "No failures recorded.",
  "jobs.state.retryable": "retrying",
  "jobs.state.discarded": "discarded",
  "jobs.state.cancelled": "cancelled",
  "jobs.attempt": "attempt {attempt} of {max} · {when}",
  "jobs.remedy": "What to do: {remedy}",
  "jobs.jobId": "job {id}",
  "jobs.failingSince": "failing since {when}",
  "jobs.reasonVetted":
    "Each reason, class and remedy is the job layer's own wording, never the worker's raw cause. A failure it cannot phrase reports a fixed substitute and carries no class at all. A class invented for text nobody could vet would key your alerts on a guess.",
  "jobs.generatedAt": "Read at {time}",

  "audit.you": "You",
  "audit.system": "System",
  "audit.unknownBuyer": "Deal Room participant",
  "audit.unknownMember": "Unknown member",
  "audit.viaAgent": "via an agent",
  "audit.viaConnector": "via a connector",
  "audit.viaDealRoom": "in the Deal Room",
  "audit.viaNamed": "via {client}",
  "audit.noHumanAuthority": "No human authority recorded",
  "settings.auditSub": "every action, attributed — human, agent, or connector",
  "settings.auditAdminOnly":
    "Only an admin can read the full trail. It records every actor and every record they touched, so it is not shown more widely.",
  "settings.auditFilters": "Filters",
  "settings.auditEntries": "Audit log",
  "settings.auditTrailLabel": "Recorded actions",
  "settings.auditActor": "Actor",
  "settings.auditEntity": "Entity type",
  "settings.auditEntityId": "Entity id",
  "settings.auditAction": "Action",
  "settings.auditFrom": "From",
  "settings.auditTo": "To",
  "settings.auditExpand": "Show change detail",
  "settings.auditRule": "Authorization rule",
  "settings.auditOnBehalf": "on behalf of",
  "settings.privacy": "Privacy inbox",
  "settings.privacySub": "data-subject requests with their statutory deadlines",
  "settings.due": "due {date}",

  "privacy.addPurpose": "Add purpose",
  "privacy.purposesRegistry": "Registered purposes",
  "privacy.purposesReadOnly":
    "Read-only view — only an admin or ops can add a purpose.",
  "privacy.purposeKey": "Key",
  "privacy.purposeLabel": "Label",
  "privacy.purposeDoi": "Requires double opt-in",
  "privacy.purposeCreate": "Create purpose",
  "privacy.purposeAppendOnly":
    "A purpose cannot be renamed or removed once created — the catalogue is append-only. Choose the key carefully.",
  "privacy.facetAll": "All",
  "privacy.inboxAdminOnly":
    "Only an admin can see subject requests. They name the people who asked, so the queue is not shown more widely.",
  "privacy.overdue": "Overdue",
  "privacy.closed":
    "Closed — a closed request never reopens. A new concern is a new request.",
  "privacy.assignee": "Assignee",
  "privacy.assigneeUnassignable":
    "Once set, an assignee cannot be cleared here.",
  "privacy.resolution": "Resolution",
  "privacy.resolutionRequired": "Closing a request needs its answer.",
  "privacy.movedOn":
    "This request moved on — someone else decided it first. Re-read below.",
  "privacy.inProgress": "In progress",
  "privacy.fulfil": "Fulfil",
  "privacy.reject": "Reject",
  "privacy.newRequest": "New request",
  "privacy.queue": "Requests",
  "privacy.kind": "Kind",
  "privacy.person": "Person",
  "privacy.subjectRef": "Subject reference",
  "privacy.dueAt": "Due",
  "privacy.openRequest": "Open request",
  "privacy.erasureNeedsPerson":
    "An erasure request must name a person in this organization — fulfilling it erases that record. A free-text subject cannot be erased.",
  "privacy.accessManual":
    "An access request is fulfilled by hand: record what you sent in the resolution. This system does not assemble or export the data for you.",
  "privacy.fulfilErasureTitle": "Fulfil erasure request",
  "privacy.erasureIrreversible":
    "This permanently erases the person across the whole system — record, captured activity, and derived values. It cannot be undone. The erasure is itself audited.",
  "privacy.typeErase": "Type ERASE to confirm",
  "privacy.erasureConfirm": "Erase + suppress",
  "privacy.legalHold":
    "Blocked — legal hold. This person is inside a statutory retention window, so erasure does not win here (Art. 17(3)(b)). The block applies to every role, including admin — there is no override. The attempt was audited.",

  "restricted.title": "Restricted records",
  "restricted.sub":
    "what a statutory retention obligation is holding after an erasure — which record, why, and until when. The correspondence itself is not shown: it is restricted precisely so it is not read.",
  "restricted.withheld":
    "Only an admin or ops can see which records a statutory obligation is holding. It reads through the same authority as the retention ladder.",
  "restricted.empty":
    "No record is being held — every erasure so far could be completed in full.",
  "restricted.heldLabel": "Records held now",
  "restricted.kind": "Record",
  "restricted.occurred": "Dated",
  "restricted.deals": "Transaction",
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
    "Recorded in the audit trail with your name. This is what makes the decision accountable, so say what you decided and on what basis.",
  "restricted.release.action": "Release",
  "restricted.release.title": "Release this record from the retention floor?",
  "restricted.release.body":
    "Releasing ERASES the record. It does not put it back in use: the erasure request this obligation suspended is still outstanding, so lifting the obligation completes it. This cannot be undone.",
  "restricted.release.confirm": "Release and erase",
  "restricted.pin.action": "Pin a record",
  "restricted.pin.submit": "Pin",
  "restricted.pin.idHint":
    "For correspondence the automatic rule cannot recognise — supplier and purchasing mail qualifies under §257 HGB and has no deal in this product to hang off. The record id is on its audit entry.",
  "restricted.pin.idMalformed":
    "That is not a record id. It looks like 8-4-4-4-12 hexadecimal characters, and the audit entry for the record shows it in full.",
  "restricted.pin.idPlaceholder": "Record id",
  "restricted.pin.title": "Place this record under the retention floor?",
  "restricted.pin.body":
    "The record is held for the statutory window: hidden from every ordinary view, unchangeable, and erased when the window closes. Its identifiers are redacted now.",
  "restricted.pin.confirm": "Pin and hold",
  "retention.title": "Retention",
  "retention.sub":
    "how long each kind of record is kept, and what happens when its window runs out",
  "retention.retainOnly": "Retain-only posture",
  "retention.retainOnlyHelp":
    "While this is on, this installation destroys nothing: no anonymising and no erasing, whatever a policy below says. Archiving still runs — an archived record is kept, not destroyed.",
  "retention.adminOnly": "Only an admin or ops can change retention.",
  "retention.withheld":
    "Only an admin or ops can see the retention ladder. It sets what this installation keeps for everybody, so it is not shown more widely.",
  "retention.addPolicy": "Add policy",
  "retention.create": "Create policy",
  "retention.scope": "Applies to",
  "retention.window": "Window in days",
  "retention.windowDays": "{days} days",
  "retention.windowInvalid": "A window is a whole number of days, at least 1.",
  "retention.action": "Action",
  "retention.actionHint":
    "Archive keeps the record; anonymise and erase destroy data, and are the two the retain-only posture holds back.",
  "retention.lawfulBasis": "Lawful basis",
  "retention.lawfulBasisHint":
    "Optional. The Art. 6 basis this window is argued from, for the auditor reading the row.",
  "retention.enabled": "Enabled",
  "retention.edit": "Edit",
  "retention.save": "Save policy",
  "retention.delete": "Delete policy",
  "retention.deleteTitle": "Delete retention policy?",
  "retention.deleteBody":
    "This drops the rule for {scope} entirely, so nothing in that scope ages out any more. To pause the rule and keep its window, turn Enabled off instead.",
  "retention.duplicateScope":
    "A policy for this scope already exists — each scope carries at most one rule. Edit the existing row instead.",
  "retention.empty":
    "No retention policy yet — nothing in this installation ages out.",
  "retention.effectActing": "Acting nightly",
  "retention.effectSuppressed": "Suppressed by retain-only",
  "retention.effectDisabled": "Disabled",
  "retention.suppressedWhy":
    "Enabled, but the retain-only posture is holding it back: this rule destroys data, so it will not act until the posture is turned off.",
  "retention.disabledWhy":
    "Turned off and kept — its window is preserved, and nothing in this scope ages out while it is off.",
  "retention.actionArchive": "Archive",
  "retention.actionAnonymize": "Anonymise",
  "retention.actionErase": "Erase",
  "retention.scopeLeadUnconverted": "Leads that never converted",
  "retention.scopeActivity": "All captured activity",
  "retention.scopeActivityTranscript": "Call transcripts",
  "retention.scopePersonNoConsentNoDeal": "People with no consent and no deal",
  "retention.scopeDealLost": "Lost deals",
  "retention.scopeDealWon": "Won deals",
  "retention.scopeAiCallPayloadContent": "AI call payloads",

  "settings.pipelines": "Pipelines",
  "settings.pipelinesReadOnly":
    "Read-only view — you may not change pipelines or their stages.",
  "settings.pipelinesSub":
    "The stages a deal moves through, one ladder per pipeline.",
  "pipeline.new": "New pipeline",
  "pipeline.edit": "Edit pipeline",
  "pipeline.name": "Name",
  "pipeline.default": "Default",
  "pipeline.notDefault": "Not default",
  "pipeline.position": "Position",
  "stage.new": "New stage",
  "stage.edit": "Edit stage",
  "stage.name": "Name",
  "stage.semantic": "Semantic",
  "stage.winProb": "Win probability",
  "stage.semOpen": "Open",
  "stage.semWon": "Won",
  "stage.semLost": "Lost",
  "stage.remove": "Remove",
  "stage.removeConfirm": "Remove stage",
  "stage.removeTitle": "Remove this stage?",
  "stage.removeBody":
    "“{name}” leaves the pipeline and the stages after it move up. Past stage changes stay readable. Deals still sitting on it have to move first.",

  "ob.url": "Website",
  "ob.urlScheme": "https://",
  "ob.back": "Back",
  "ob.restoring": "Restoring your setup…",
  "ob.readManual": "Tell me yourself",
  "ob.coreIntroTitle": "First, I need to know your legal company.",
  "ob.coreIntroBody":
    "I need its legal name, address and VAT or register number. Then I learn what you sell and to whom.",
  "ob.coreLegalKicker": "I start with legal identity",
  "ob.corePathLabel": "What I'll learn",
  "ob.corePathLegal": "Legal identity",
  "ob.corePathOffer": "Offer",
  "ob.corePathCustomer": "Customers",
  "ob.coreReadingPage": "I'm reading",
  "ob.coreWebsiteTitle": "Which website should I read?",
  "ob.coreWebsiteBody":
    "I read the legal notice first, then your products, customers and positioning.",
  "ob.corePreparing": "I'm preparing to read {host}",
  "ob.coreLegalReading": "I'm reading the legal identity on {host}",
  "ob.coreLegalReadingBody":
    "I look for the legal notice, address and register or VAT number. Unstated details stay empty.",
  "ob.coreBusinessReading": "I'm learning how the business works",
  "ob.coreBusinessReadingBody":
    "I'm connecting products, customers and positioning to the exact public text that supports them.",
  "ob.coreReady": "I found {count} cited company details",
  "ob.corePartial": "I found {count} useful details, with some gaps",
  "ob.coreReadyBody":
    "Nothing is saved yet. Review the legal identity first, then the offer and customer.",
  "ob.coreDeferredBody": "I'll resume this read automatically.",
  "ob.coreFailedBody":
    "I could not read this site well enough, so I stopped instead of guessing. You can tell me yourself.",
  "ob.coreFindingsTitle": "What I found and can support",
  "ob.coreFindingsBody":
    "Every value carries the public wording behind it. What I cannot verify, I leave empty.",
  "ob.ai.identity": "Hi, I'm Margince",
  "ob.ai.role": "Your company research AI",
  "ob.ai.speaker": "M",
  "ob.ai.speakerName": "Margince",
  "ob.ai.ready": "I'm ready to research",
  "ob.ai.configured": "Configured AI",
  "ob.ai.modelsUsed": "Models used in this task",
  "ob.ai.route": "Task · tier · provider",
  "ob.ai.calls": "AI calls",
  "ob.ai.tokens": "Tokens",
  "ob.ai.latency": "Model latency",
  "ob.ai.estimatedCost": "Estimated provider cost",
  "ob.ai.partialEstimate": "Partial · unpriced usage exists",
  "ob.ai.awaitingModel": "Shown after my first model call",
  "ob.ai.notAvailableYet": "Not available yet",
  "ob.ai.runtimeUnavailable": "Runtime details unavailable",
  // The runtime disclosure is a chip you can open rather than a permanent
  // band: cost is stated WHILE it is being spent, but a reader deciding
  // whether a legal entity is right should not have to read a billing table
  // to get to it.
  "ob.ai.runtimeChip": "What is answering, and what it costs",
  "ob.ai.answeringNow": "What is answering right now",
  "ob.ai.runScope": "This run only. The full log is in Settings → AI.",
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
  "ob.ai.liveArtifact": "Live, reviewable artifact",
  "ob.ai.companyKnowledge": "What I understand about your company",
  "ob.ai.companyKnowledgeBody":
    "Website evidence stays separate from our conversation. You decide what becomes company context.",
  "ob.ai.companyKnowledgeManualBody":
    "Your answers and my suggestions stay editable here. You decide what becomes company context.",
  "ob.ai.askPlaceholder":
    "Ask me about a finding, correct a detail, or tell me what I missed…",
  "ob.ai.send": "Send to Margince",
  "ob.ai.reviewBoundary":
    "I can suggest changes here. I only apply them to your draft when you approve.",
  "ob.ai.confirmBoundary":
    "Nothing becomes company context until you confirm this draft.",
  "ob.ai.confirmCompany": "Confirm and save company",
  "ob.ai.thinking": "I'm checking the dossier and preparing an answer…",
  "ob.ai.suggestedChanges": "Suggested changes to your draft",
  "ob.ai.applyChanges": "Apply to my draft",
  "ob.ai.applied": "Applied to draft",
  "ob.ai.finding_one": "cited finding",
  "ob.ai.finding_other": "cited findings",
  "ob.continueManual": "Tell me instead",
  "ob.readStatus.queued": "I'm getting ready",
  "ob.readStatus.deferred": "I'm waiting for AI budget",
  "ob.readStatus.reading": "I'm reading now",
  "ob.readStatus.ready": "I've finished reading",
  "ob.readStatus.partial": "I've finished with some gaps",
  "ob.readStatus.failed": "I need your help",
  "ob.readStatus.confirmed": "I've saved your choices",
  "ob.readStatus.abandoned": "I've stopped",
  "ob.pagesRead": "pages I've read",
  "ob.legalEntitiesFound": "legal entities I've found",
  "ob.coverageDetails": "What I covered and couldn't read",
  "ob.legalFoundTitle": "Legal entities I found",
  "ob.legalFoundBody":
    "Each block keeps the registered name, address and register or VAT number. Pick yours in the review.",
  "ob.legalEntity": "Legal entity",
  "ob.confirmWebsite":
    "I grounded this in {count} public pages. Edit anything; untouched values keep their evidence.",
  "ob.confirmManual":
    "You told me this directly, so I'll store your answers as human assertions.",
  "ob.legalTitle": "Which legal entity should I use?",
  "ob.legalSub":
    "I found several entities in the legal notice. Choose yours and I'll fill in its details.",
  "ob.factsTitle": "Other facts I found",
  "ob.factsSelected": "{selected} of {total} selected",
  "ob.factsSub":
    "Untick anything that should not become company context — up to 100 facts can be selected.",

  // No step number: the rail decides how many stops a reader gets, so the
  // count belongs to ob.conv.scene.step, which reads it off the rail. A total
  // written into a string here can only ever disagree with the real one.
  "ob.s1.kick": "Confirm",
  "ob.s1.title": "Review what I learned about your company",
  "ob.s1.sub":
    "I filled only what I could support from your website. Please correct anything that is wrong.",
  "ob.s1.urlPlaceholder": "yourcompany.com",
  "ob.s1.identityLabel": "Legal organization",
  "ob.s1.offerLabel": "Products and offer",
  "ob.s1.customerLabel": "Customer",
  "ob.s1.salesLabel": "Positioning and sales context",
  "ob.s1.fieldRequired": "Required.",
  "ob.s1.requiredMissing": "Fill these in before you continue: {fields}",
  "ob.s1.saving": "Saving…",
  "ob.s1.saveFailed": "Couldn't save your company",
  "ob.s1.savedNote":
    "Saved to your organization. Change anything here and continue to save it again.",
  "ob.readGo": "Read my website",
  "ob.urlWillRead": "I'll read {host}",
  "ob.readFromSite": "read from site",
  "ob.failTitle": "I couldn't read enough from this website",

  "ob.manualChapterLegal": "Your legal organization",
  "ob.manualChapterOffer": "Products and offer",
  "ob.manualChapterCustomer": "Ideal customer",
  "ob.manualChapterSales": "How you sell",
  "ob.manualNext": "Next question",
  "ob.manualLater": "Add later",
  "ob.manualReview": "Review my answers",
  "ob.manualRequired": "Required to create a usable company profile",
  "ob.manualOptional": "Optional — leave it empty to add later",
  "ob.manual.display_name": "What name do customers know your company by?",
  "ob.manual.display_nameHint":
    "Use the familiar company or trading name shown throughout Margince.",
  "ob.manual.legal_name": "What is the full registered legal name?",
  "ob.manual.legal_nameHint":
    "Include the legal form, such as GmbH, Ltd, Inc. or AG. Add it later if it does not apply.",
  "ob.manual.registered_address": "What is the registered address?",
  "ob.manual.registered_addressHint":
    "Use the official address from the commercial register or legal notice.",
  "ob.manual.register_vat": "What are the register and VAT/UID numbers?",
  "ob.manual.register_vatHint":
    "Enter the identifiers exactly as issued. Leave this empty when none applies.",
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
  "ob.manual.industryHint":
    "Choose the description your customers would immediately recognize.",
  "ob.manual.history": "Is there useful company history Margince should know?",
  "ob.manual.historyHint":
    "For example founding year, origin or an important change in the business.",
  "ob.manual.offer_summary": "What products or services do you sell?",
  "ob.manual.offer_summaryHint":
    "One or two concrete sentences are enough. This is the commercial explanation Margince will use.",
  "ob.manual.value_proposition": "What outcome does the offer create?",
  "ob.manual.value_propositionHint":
    "Explain the value customers receive, not only the product features.",
  "ob.manual.usp": "What makes customers choose you?",
  "ob.manual.uspHint":
    "Name the strongest meaningful difference from the alternatives.",
  "ob.manual.icp": "Who is your ideal customer?",
  "ob.manual.icpHint":
    "Describe the companies or people that benefit most — size, industry, situation or geography.",
  "ob.manual.buying_center": "Who evaluates, buys or approves the purchase?",
  "ob.manual.buying_centerHint":
    "List the typical roles and who has the final say.",
  "ob.manual.customer_pains": "What problems bring those customers to you?",
  "ob.manual.customer_painsHint":
    "Use the problems customers would describe in their own words.",
  "ob.manual.desired_outcomes": "What are they trying to achieve?",
  "ob.manual.desired_outcomesHint":
    "Describe the practical or business outcomes they care about.",
  "ob.manual.buying_intents": "What usually signals buying interest?",
  "ob.manual.buying_intentsHint":
    "For example a new initiative, hiring pattern, deadline or operational problem.",
  "ob.manual.common_objections": "What objections do you hear most often?",
  "ob.manual.common_objectionsHint":
    "Include the concerns that regularly slow down or stop a purchase.",
  "ob.manual.sales_motion": "How does a typical sale happen?",
  "ob.manual.sales_motionHint":
    "Describe the path from first conversation to decision, including trials or procurement if relevant.",

  "ob.field.display_name": "Company name",
  "ob.field.offer_summary": "What do you sell?",
  "ob.field.icp": "Ideal customer",
  "ob.field.buying_center": "Who buys",
  "ob.field.value_proposition": "Value proposition",
  "ob.field.usp": "What sets you apart",
  "ob.field.customer_pains": "Customer pains",
  "ob.field.desired_outcomes": "Desired outcomes",
  "ob.field.buying_intents": "Buying intents",
  "ob.field.common_objections": "Common objections",
  "ob.field.sales_motion": "Sales motion",
  "ob.field.legal_name": "Registered legal name",
  "ob.field.registered_address": "Registered address",
  "ob.field.register_vat": "Register / VAT ID",
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
    "The name customers actually call you by, not the legal one. It is what shows across Margince.",
  "ob.fieldHint.offer_summary":
    "One or two plain sentences on what you sell, so Margince can explain the business without asking again.",
  "ob.fieldHint.icp":
    "Who benefits most, by size, industry or situation, so outreach can be aimed rather than generic.",
  "ob.fieldHint.buying_center":
    "The roles who evaluate or sign off, so a rep knows who else belongs in the conversation.",
  "ob.fieldHint.value_proposition":
    "The outcome a customer gets, not the product feature, stated plainly enough to stand alone in a pitch.",
  "ob.fieldHint.usp":
    "The one difference that actually moves a decision, not a strength every competitor also claims.",
  "ob.fieldHint.customer_pains":
    "The problem in the customer's own words, the way they would say it before they found you.",
  "ob.fieldHint.desired_outcomes":
    "What the customer is trying to achieve, in business terms rather than product terms.",
  "ob.fieldHint.buying_intents":
    "The signal that usually means a prospect is close to buying, such as a hire or a deadline.",
  "ob.fieldHint.common_objections":
    "The concern that most often slows or stops a deal, so a rep can prepare for it early.",
  "ob.fieldHint.sales_motion":
    "The path from first conversation to signed deal, including any trial or procurement step.",
  "ob.fieldHint.legal_name":
    "The name as it appears on the register, legal form included, since this is what belongs on an invoice.",
  "ob.fieldHint.registered_address":
    "The address printed in the legal notice, not a mailing or showroom address.",
  "ob.fieldHint.register_vat":
    "Both identifiers exactly as issued, since they appear together on invoices and contracts.",
  "ob.fieldHint.legal_form":
    "The form as the register states it, which decides how the company is named on a contract.",
  "ob.fieldHint.register_court":
    "The court named in the legal notice that holds the company's register entry.",
  "ob.fieldHint.register_number":
    "The register entry alone, without the VAT ID, which has its own field above.",
  "ob.fieldHint.industry":
    "The description your own customers would recognize, not an internal classification code.",
  "ob.fieldHint.history":
    "Add it only if it changes how the company should be read, such as a founding year or a major pivot.",

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
  "ob.fieldEg.usp": "Only vendor offering same-day, on-site support.",
  "ob.fieldEg.customer_pains": "We keep running out of stock without noticing.",
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
  "ob.s4.provImap": "Any other mailbox (IMAP/SMTP)",
  "ob.s4.microsoftBtn": "Allow access to my Microsoft",
  "ob.s4.microsoftHint":
    "Reads your mail, and can send from it. You grant both on Microsoft's own screen, and can disconnect any time.",
  "ob.s4.microsoftUnverified":
    'You may see an "unverified app" notice — that\'s this self-hosted install, not a third party.',
  "ob.s4.microsoftFailed": "The Microsoft connection didn't complete.",
  "ob.s4.connectOkTitle": "You're connected",
  "ob.s4.connectOkBody":
    "Your mailbox is linked. Capture begins on the next sync.",
  "ob.s4.connectVerifying": "Confirming the connection…",
  "ob.s4.connectLive": "Live and capturing",
  "ob.s4.connectConfirmFailed": "We couldn't confirm the connection.",
  "ob.s4.connectRetry":
    "Head to Settings → Connections to try connecting again.",
  "ob.s4.connectDenied": "You declined access — nothing was connected.",
  "ob.s4.googleBtn": "Allow access to my Gmail",
  "ob.s4.googleHint":
    "Reads your mail, and can send from it. You grant both on Google's own screen, and can disconnect any time.",
  "ob.s4.googleUnverified":
    "If Google warns about an “unverified app”, choose Advanced → Continue. Google’s screen lists exactly what you are granting.",
  "backfill.title": "Import your mail history",
  "backfill.intro":
    "Choose how far back to import. You'll see the scope and estimated cost before anything runs — and you can skip this entirely.",
  "backfill.windowLabel": "Import window",
  "backfill.window3m": "3 months",
  "backfill.window6m": "6 months",
  "backfill.window12m": "12 months",
  "backfill.window24m": "2 years",
  "backfill.window60m": "5 years",
  "backfill.previewLoading": "Counting your mailbox…",
  "backfill.estimateMessages": "Messages in this window:",
  "backfill.estimateCost": "Estimated AI cost:",
  "backfill.estimateNote":
    "An estimate, not a bill — actual usage is metered and visible as it happens.",
  "backfill.startCta": "Start the import",
  "backfill.starting": "Starting…",
  "backfill.skip": "Skip the history import",
  "backfill.skippedNote":
    "No history imported. New mail is still captured from now on — you can start an import later from Settings.",
  "backfill.loading": "Checking import status…",
  "backfill.statusUnavailable":
    "The import status can't be read right now — capture itself keeps running.",
  "backfill.queuedTitle": "Import queued",
  "backfill.runningTitle": "Importing your mail history",
  "backfill.doneTitle": "History import complete",
  "backfill.errorTitle": "The import hit a problem",
  "backfill.cancelledTitle": "Import cancelled",
  "backfill.progressLabel": "Import progress",
  "backfill.countScanned": "Messages scanned",
  "backfill.statEmails": "Emails captured",
  "backfill.statPeople": "People",
  // The count is domains this run raised a company question for, not
  // companies created — a domain becomes one only if its site says so.
  "backfill.statCompanies": "Companies to check",
  "backfill.errorNote":
    "It will retry on its own; everything captured so far is kept.",
  "backfill.cancel": "Stop the import",
  "backfill.cancelledNote": "Stopped. Everything captured so far is kept.",
  "backfill.unsupportedNote":
    "This mailbox type can't be backfilled — only new mail is captured from now on.",
  "backfill.narrowingNote":
    "A wider window already ran for this mailbox; the import window can only be widened, not narrowed.",
  "backfill.staleUpdated": "Last updated {duration} ago — no recent progress.",

  // The units an installation composed, offered on the settings page that
  // already holds the kind of credential each one is configured with. The two
  // headings differ because the two pages mean different things — one is your
  // own account somewhere, the other is the installation's.
  "extUnits.open": "Open",
  "extUnits.openNamed": "Open the {name} page",
  "extUnits.user.title": "Your other accounts",
  "extUnits.user.sub":
    "Accounts this installation can connect on your behalf. Each one is yours alone — nobody else sees it, and disconnecting it affects only you.",
  "extUnits.workspace.title": "Installation add-ons",
  "extUnits.workspace.sub":
    "Add-ons this installation runs with one shared credential. What you set here applies to everybody.",

  // Connected inboxes (Settings → Connections): the "manage in Settings"
  // surface the onboarding copy promises.
  "connectors.title": "Connected inboxes",
  // The rep's standing overnight authority — one question, asked beside the
  // mailbox connect in onboarding and again in Settings. The danger line names
  // the features that go empty, because "some things stop working" is not
  // something a rep can weigh.
  "overnightGrant.title": "Overnight preparation",
  "overnightGrant.sub":
    "Margince works through your deals while you sleep and has your morning ready when you arrive. It acts as you, sees only what you can see, and you can stop it at any time.",
  "overnightGrant.label": "Let Margince prepare my morning brief overnight",
  "overnightGrant.help":
    "It reads your deals and mail to rank what needs you today, and writes notes back. It cannot send: the permission you give here covers reading and writing only, never sending.",
  "overnightGrant.danger":
    "Without this, your morning brief, your worklist lanes and your weekly review stay empty. These are the screens Margince opens on, so most of the product will look like it is not working.",
  "overnightGrant.saveFailed":
    "Your answer to the overnight question could not be saved. Everything else is connected — set it under Settings → Connections when you are in.",
  "overnightGrant.renew":
    "You said yes, but the authority Margince was working under has expired. Turn this off and on again to renew it — until then your brief is not being prepared.",
  "overnightGrant.renewScope":
    "You said yes, but Margince has learned to do more since, and the authority you gave does not cover the new work. Turn this off and on again to widen it — until then your brief is not being prepared.",
  "aiHealth.title": "Model lanes",
  "aiHealth.sub":
    "Whether each model tier is answering. A lane that stopped and one that is merely cautious look the same everywhere else — captured mail stays held either way.",
  "aiHealth.noCalls": "no model was called in the last {hours} hour(s)",
  "aiHealth.colTier": "Tier",
  "aiHealth.colState": "State",
  "aiHealth.colCalls": "Last {hours}h",
  "aiHealth.colLatency": "Median",
  "aiHealth.colLast": "Last answer",
  "aiHealth.answering": "Answering",
  "aiHealth.notAnswering": "Not answering",
  "aiHealth.callCounts": "{calls} calls, {failures} failed",
  "aiHealth.ms": "{ms} ms",
  "heldThreads.title": "Held back from your team",
  "heldThreads.sub":
    "Threads your mailbox is withholding. Releasing one lets every colleague read it; nobody else can release yours.",
  "heldThreads.empty": "your mailbox is withholding nothing right now",
  "heldThreads.colThread": "Thread",
  "heldThreads.colWhy": "Why it is held",
  "heldThreads.colWhen": "Arrived",
  "heldThreads.colActions": "What you can do",
  "heldThreads.release": "Share with the team",
  "heldThreads.released": "Shared with the team",
  "heldThreads.noSubject": "the message this began with is gone",
  "heldThreads.nothingToShare":
    "There is no message left to share — this thread\u2019s first message was erased, and the hold stays so a later reply does not arrive open.",
  "heldThreads.pending": "Waiting on a verdict",
  "heldThreads.attempts": "asked {count} time(s)",
  "heldThreads.backlogStalled":
    "{count} thread(s) have been asked about repeatedly with no answer. Mail stays withheld while this lasts — nothing is lost, and it clears on its own once the classifier answers again.",
  "heldThreads.heldByOthers":
    "Still held: {count} other mailbox imported this message and has not shared it. A thread opens only when everyone who received it agrees.",
  "heldThreads.kind.legal": "Legal",
  "heldThreads.kind.financialCorporate": "Company finances",
  "heldThreads.kind.personnel": "Personnel",
  "heldThreads.kind.personal": "Personal",
  "heldThreads.kind.securityIncident": "Security incident",
  "heldThreads.kind.explicitlyConfidential": "Marked confidential",
  "senders.title": "Senders",
  "senders.sub":
    "What was decided about each address your mailbox brought in — and your own answer where you gave one. Your senders only; a colleague never sees this list.",
  "senders.emptyTitle": "Nothing decided yet",
  "senders.emptyBody":
    "Once your mailbox has brought in mail, every sender it saw is listed here with what became of them.",
  "senders.colSender": "Sender",
  "senders.colDecision": "Decided",
  "senders.colRecord": "Contact",
  "senders.colActions": "What you can do",
  "senders.recordYes": "Yes",
  "senders.recordNo": "No",
  "senders.byYou": "— you decided",
  "senders.deletesOn": "Oldest message deleted on {date}",
  "senders.markBusiness": "Business",
  "senders.keepOut": "Keep out",
  "senders.withdraw": "Undo",
  "senders.keepOutTitle": "Keep this sender out for good?",
  "senders.keepOutBody":
    "No contact is created, and the mail this sender already brought into your mailbox is destroyed. Mail a colleague also imported stays theirs.",
  "senders.keepOutConfirm": "Keep out and destroy",
  "senders.kind.person": "A person",
  "senders.kind.roleMailbox": "A role mailbox",
  "senders.kind.organizationSender": "An organization",
  "senders.kind.newsletter": "A newsletter",
  "senders.kind.transactional": "An automated tool",
  "senders.kind.spam": "Spam",
  "senders.kind.personal": "Personal",
  "senders.kind.advisor": "An advisor",
  "senders.kind.business": "Business",
  "senders.kind.keptOut": "Kept out",
  "senders.kind.undecided": "Not yet decided",
  "mailSharing.title": "Email sharing",
  "mailSharing.sub":
    "Captured mail is readable by every colleague who can see the contact. On by default — it is what makes the pipeline shared.",
  "mailSharing.label": "Share captured mail with the team",
  "mailSharing.help":
    "Individual messages can be limited afterwards, and addresses or domains excluded up front.",
  "mailSharing.danger":
    "DANGER: Switching off email sharing will make usage of the CRM difficult. New mail will be visible only to the people on each message.",
  "mailSharing.sharedPosture.label": "Allow mailboxes to share on arrival",
  "mailSharing.sharedPosture.help":
    "Lets a colleague put their own mailbox in the shared posture, where a captured message is readable by the team the moment it lands, before anything has judged it. Off by default.",
  "mailSharing.sharedPosture.warning":
    "Reading an employee's mailbox into a shared CRM is what a works-council agreement covers in Germany and Austria. Turning this on says your organization holds one. Margince does not check.",
  "mailSharing.save": "Save",
  "connectors.originLabel": "Address used in emailed links",
  "connectors.originReachable": "Answering",
  "connectors.originUnreachable": "Not answering",
  "connectors.originUnchecked": "Not checked yet",
  "connectors.sub":
    "Mailboxes capturing into your CRM. Disconnect any one when you need to — already-captured records stay.",
  "connectors.loading": "Loading your connections…",
  "connectors.loadFailed": "Couldn't load your connections.",
  "connectors.empty": "No inbox is connected yet.",
  "connectors.provGmail": "Gmail",
  "connectors.provGcal": "Google Calendar",
  "connectors.provGraph": "Outlook",
  "connectors.provGraphCal": "Outlook Calendar",
  "connectors.provImap": "IMAP mailbox",
  "connectors.statusConnected": "Capturing",
  "connectors.statusPending": "Pending — not yet confirmed live",
  "connectors.statusReauth": "Needs reconnect",
  "connectors.statusError": "Sync error",
  "connectors.statusDisconnected": "Disconnected",
  "connectors.cannotSend": "Capturing only — cannot send",
  "connectors.reconnectToSend":
    "Reconnect this mailbox to send from it. A mailbox connected before sending existed cannot be upgraded in place — the provider only grants sending on a fresh connection.",
  "connectors.lastSynced": "Last synced {at}",
  "connectors.neverSynced": "Waiting for the first sync",
  "connectors.nextCheck": "Next check ~{at}",
  "connectors.polled": "Polled on a schedule (no push subscription)",
  "connectors.pushRenewal": "Push renewal by {at}",
  "connectors.notConfigured":
    "Mail capture isn't configured in this deployment.",
  "connectors.reconnect": "Reconnect",
  "connectors.disconnect": "Disconnect",
  "connectors.signatureEnrich.label": "Read contact details from this mailbox",
  "connectors.contextTag.label": "File what this connector brings in under",
  "connectors.contextTag.none": "No tag",
  "connectors.contextTag.hint":
    "An existing tag. Every contact this connector creates from now on is filed under it, so you can ask what came in from this source. Contacts already here keep the tags they have.",
  "connectors.contextTag.archived":
    "{name} has been archived, so nothing is being filed under it. Choose another tag, or none.",
  "connectors.signatureEnrich.followingDefault":
    "Following your organization's setting. Change it here and this mailbox keeps its own answer.",
  "connectors.signatureEnrich.ownAnswer":
    "This mailbox's own answer, kept whatever your organization's setting becomes.",
  "hold.sectionTitle": "Private correspondence",
  "hold.notHeld": "Mail with this contact follows your mailbox setting.",
  "hold.heldByAddress": "You keep mail with this address to the people on it.",
  "hold.heldByDomain": "You keep mail with {domain} to the people on it.",
  "hold.holdAddress": "Keep private",
  "hold.holdDomain": "Keep all of {domain} private",
  "hold.lift": "Lift",
  "hold.liftingWidensNothing":
    "Lifting applies to new mail. What was held stays held.",
  "hold.confirmVerb": "Keep private",
  "hold.confirmTitle": "Keep this correspondence private?",
  "hold.confirmAddressBody":
    "Mail with {address} stays with the people who were on it. It is still captured and still yours to read — colleagues do not see it.",
  "hold.confirmDomainBody":
    "Mail with anyone at {domain}, including subdomains, stays with the people who were on it. It is still captured and still yours to read — colleagues do not see it.",
  "hold.confirmHistoryNote":
    "This covers mail from here on. Mail already captured keeps the visibility it has.",
  "captureNotice.whatHappens":
    "Margince reads this mailbox and files what it finds: the messages, who was on them, and the contacts and companies behind the addresses. Attachments are stored with their message.",
  "captureNotice.whoReads":
    "A new mailbox is held by default. A message stays with the people who were on it until a classifier judges the thread to be ordinary business — only then can colleagues read it. You can set the mailbox to hold everything instead, at any time.",
  "captureNotice.yourControl":
    "You decide per sender and per thread, under Settings → Connections: keep a correspondent out entirely, share a thread with the team, or delete what a sender brought in. Nothing here asks you to agree — this is what happens, so you know it before you connect.",
  "connectors.mailPosture.label": "Who may read mail from this inbox",
  "connectors.mailPosture.classified": "Held until classified",
  "connectors.mailPosture.held": "Always held",
  "connectors.mailPosture.shared": "Shared with the team",
  "connectors.mailPosture.sharedNeedsAdmin":
    "“Shared with the team” needs an admin to allow it for this organization.",
  "connectors.mailPosture.help.classified":
    "A new message is held to the people on it until a classifier judges the thread ordinary. Colleagues see nothing before that.",
  "connectors.mailPosture.help.held":
    "A new message is held to the people on it, whatever any classifier concludes. You share a thread yourself, one at a time.",
  "connectors.mailPosture.help.shared":
    "A new message is readable by colleagues the moment it lands.",
  "connectors.mailPosture.historyTitle": "And the mail already captured?",
  "connectors.mailPosture.historyBody":
    "This answer governs mail captured from here on. Mail already in the CRM keeps the audience it has, unless you narrow it to match.",
  "connectors.mailPosture.historyConfirm": "Change what colleagues may read",
  "connectors.mailPosture.historyApply": "Also narrow mail already captured",
  "connectors.disconnectTitle": "Disconnect this inbox?",
  "connectors.disconnectBody":
    "This will delete the credential we stored for this mailbox. Capture stops immediately; everything already captured stays in your CRM, and reconnecting will ask for permission again.",
  "connectors.disconnectBodyGoogleNote":
    "Google may still list Margince under your account's third-party access — remove it there if you want to revoke it fully.",
  "connectors.disconnectBodyMicrosoftNote":
    "Microsoft may still list Margince among your account's connected apps — remove it there if you want to revoke it fully.",
  "connectors.errRateLimited":
    "The provider is throttling us. Capture is running slower than usual; nothing is lost.",
  "connectors.errUnreachable":
    "We couldn't reach the provider. We'll keep retrying.",
  "connectors.errAuth":
    "The provider rejected our credentials. Reconnect to resume.",
  "connectors.errHistoryGone":
    "The provider's change history expired. The next sync re-anchors from a fresh point.",
  "connectors.errInternal":
    "Something went wrong on our side. We stopped rather than capture partial data.",
  "connectors.errUnknown":
    "Capture hit a problem we can't classify yet. We'll keep retrying.",

  // The OAuth return outcome (Task 2): the callback lands back on
  // #/settings/connections/{outcome} — a dismissible inline note driven by
  // that route segment, never a claim the server hasn't confirmed.
  "connectors.oauthOk": "Connected. Your mailbox is now capturing.",
  "connectors.oauthDenied": "You declined access — nothing was connected.",
  "connectors.oauthError":
    "The connection couldn't be completed — please try again.",
  // Three failures that "try again" would be wrong about, each fixed somewhere
  // else: the provider refused the grant (retrying the same way repeats it),
  // its API is not enabled for this deployment (the vendor's console), and it
  // refused this deployment's own client credentials (the app card in
  // Settings). The last two are an administrator's; no user action clears them.
  "connectors.oauthRejected":
    "The provider declined the connection. Make sure you accept every permission it asks for, then try connecting again.",
  "connectors.oauthMisconfigured":
    "This deployment can't complete that connection yet — the provider's API isn't enabled for it. An administrator needs to enable it; the server log names which API.",
  "connectors.oauthBadClient":
    "The provider refused this installation's app credentials. An administrator should check the client ID and secret under Settings → General; re-connecting will not clear it on its own.",
  "connectors.dismissOutcome": "Dismiss",

  // The "Add a connection" affordance (Task 1): one verb in the card's header
  // opens a dialog listing the providers still addable, each with the sentence
  // it needs. `addOpen` names the act of OPENING that list, and the picks inside
  // it name the act of connecting — so no two buttons on the surface read the
  // same while both are on screen.
  "connectors.addConnection": "Add a connection",
  "connectors.addOpen": "Connect an account",
  "connectors.connect": "Connect",
  "connectors.connectProvider": "Connect {provider}",
  "connectors.rosterLabel": "Mailboxes capturing",
  "connectors.addGmailBrings":
    "The mail you send and receive, from Google. Margince can send from it too.",
  "connectors.addGcalBrings":
    "Your Google calendar. It connects separately from Gmail.",
  "connectors.addGraphBrings":
    "The mail you send and receive on a Microsoft work account. Margince can send from it too.",
  "connectors.addGraphCalBrings":
    "Your Outlook calendar. It connects separately from your Outlook mail.",
  "connectors.addImapBrings":
    "Any other mail host, with an app password. Capture only.",
  "connectors.providerNotConfigured":
    "{provider} isn't configured in this deployment.",

  // The inline IMAP connect form (Task 6): first-connect and reconnect for
  // the one credential provider, done in Settings instead of bouncing to
  // onboarding.
  "connectors.imapModalTitle": "Connect an IMAP mailbox",
  "connectors.imapHost": "IMAP server",
  "connectors.imapPort": "Port",
  "connectors.imapUsername": "Email address",
  "connectors.imapSecret": "App password",
  "connectors.imapMailbox": "Mailbox",
  "connectors.imapMaxMessages": "Messages per sync",
  "connectors.imapSecretHint":
    "Use an app-specific password. We seal it in the credential vault and read your mail on a schedule until you disconnect — disconnecting deletes it.",
  "connectors.imapSubmitCta": "Connect",
  "connectors.imapNeeded": "Needed to connect",
  "connectors.imapStillNeeded": "Still needed: {fields}",
  "connectors.imapLoginRejected":
    "The mailbox rejected these credentials. Check host, email and app password.",
  "connectors.imapUnreachable": "The mail server could not be reached.",

  // The Telegram connector panel (Task 17, design §9.1-§9.2): one bot
  // connects for the whole workspace — no OAuth handshake, a BotFather
  // token submitted through the same inline-form shape the IMAP connector
  // uses. Unlike the mail providers, the connection stays editable in
  // place: replacing the token goes through PATCH, never a disconnect.
  "connectors.provTelegram": "Telegram",
  "connectors.telegramTitle": "Telegram bot",
  "connectors.telegramSub":
    "One bot receives and sends messages for the whole organization.",
  "connectors.telegramNotConfigured":
    "Messaging channels aren't configured in this deployment.",
  "connectors.telegramConnectCta": "Connect a Telegram bot",
  "connectors.telegramRosterLabel": "Bot carrying messages",
  "connectors.telegramEmpty": "No bot is connected yet.",
  "connectors.telegramEditToken": "Replace token",
  "connectors.telegramDisconnectTitle": "Disconnect this bot?",
  "connectors.telegramDisconnectBody":
    "This deletes the stored token and stops checking the bot for new messages. Capture and sending stop immediately; everything already captured stays in your CRM.",
  "connectors.telegramModalTitle": "Connect a Telegram bot",
  "connectors.telegramEditTitle": "Replace the bot token",
  "connectors.telegramBotToken": "Bot token",
  "connectors.telegramBotTokenHint":
    "Paste the token BotFather gave you when you created the bot. We seal it in the credential vault and never show it again.",
  "connectors.telegramSubmitCta": "Connect",
  "connectors.telegramReplaceCta": "Replace token",
  "connectors.telegramConnectedAs": "Connected as @{username}.",

  // The workspace's own consumer-mail list (CAP-PARAM-5): what the shipped
  // baseline missed, and what it got wrong. Admin-curated and shared, because
  // whether a domain can name a company is a fact about the domain.
  "consumerMail.title": "Consumer mail domains",
  "consumerMail.sub":
    "Mail from a consumer mailbox still creates the person — it just never creates a company. Margince ships a list of these providers; add what it missed, or take back a domain it wrongly claimed.",
  "consumerMail.addedTitle": "Added here",
  "consumerMail.addTitle": "Add a domain",
  "consumerMail.domainLabel": "Domain",
  "consumerMail.domainPlaceholder": "provider.example",
  "consumerMail.kindLabel": "What this domain is",
  "consumerMail.kind.extra": "Consumer mail — never a company",
  "consumerMail.kind.never": "A real company — ignore the shipped list",
  "consumerMail.add": "Add",
  // The header verb names the whole act it opens a dialog for; the dialog's own
  // submit is the bare verb, so no two buttons on this surface read the same.
  "consumerMail.addOpen": "Add a domain",
  "consumerMail.remove": "Remove",
  "consumerMail.none": "Nothing added. The shipped list decides every domain.",
  "consumerMail.adminOnly": "You do not have permission to change this list.",
  "consumerMail.addOnly":
    "You can add consumer-mail domains. Overriding the shipped list and removing entries need an admin.",
  "consumerMail.baselineTitle": "Shipped list",
  "consumerMail.baselineCount":
    "Margince ships with {total} known consumer-mail domains.",
  "consumerMail.baselineSearchLabel": "Search the shipped list",
  "consumerMail.baselinePlaceholder": "gmail.com",
  "consumerMail.baselineNone": "No shipped domain matches.",
  "consumerMail.baselineMore":
    "Showing the first {shown} of {matched} matches.",

  "blockedDomains.title": "Refused domains",
  "blockedDomains.sub":
    "Which domains this installation refuses a company, and what decided each one — a model verdict, a heuristic, or a person. Letting a domain back in re-opens the company question rather than merely clearing a flag.",
  "blockedDomains.listTitle": "Decisions on record",
  "blockedDomains.record": "Record a decision",
  "blockedDomains.recordOpen": "Record a decision",
  "blockedDomains.domainLabel": "Domain",
  "blockedDomains.domainPlaceholder": "vendor.example",
  "blockedDomains.admissionLabel": "Decision",
  "blockedDomains.admission.suppressed": "Never a company",
  "blockedDomains.admission.admitted": "Allowed, and kept",
  "blockedDomains.reasonLabel": "Why",
  "blockedDomains.reasonHint":
    "One sentence somebody reviewing this later can act on.",
  "blockedDomains.reasonPlaceholder": "a tool we use, not a customer",
  "blockedDomains.save": "Save decision",
  "blockedDomains.stored": "Stored: {domain} — {admission}.",
  "blockedDomains.adminOnly":
    "Only an admin or ops seat may change a domain decision. The list itself is yours to read.",
  "blockedDomains.none":
    "No domain has been refused a company yet. Bulk-sender verdicts land here on their own, and so does anything you refuse by hand.",
  "blockedDomains.unit": "domain decisions",
  "blockedDomains.openCompany": "the company",
  "blockedDomains.col.domain": "Domain",
  "blockedDomains.col.admission": "Decision",
  "blockedDomains.col.source": "Decided by",
  "blockedDomains.col.reason": "Why",
  "blockedDomains.col.decided": "When",
  "blockedDomains.col.revise": "Change",
  "blockedDomains.source.verdict": "A model verdict",
  "blockedDomains.source.heuristic": "A heuristic",
  "blockedDomains.source.human": "A person",
  "blockedDomains.rowAdmit": "Allow this one",
  "blockedDomains.rowRefuse": "Refuse this one",

  "ob.s4.googleFailed": "The Google connection didn't complete",
  "ob.s4.imapHost": "IMAP host",
  "ob.s4.imapHostPlaceholder": "imap.gmail.com",
  "ob.s4.imapPort": "Port",
  "ob.s4.imapEmail": "Email",
  "ob.s4.imapPassword": "App password",
  "ob.s4.imapMailbox": "Mailbox",
  "ob.s4.imapMax": "How many recent emails",
  "ob.s4.imapHint":
    "Use an app password. We store it encrypted, and disconnecting deletes it.",
  "ob.s4.imapConnect": "Test and connect",
  "ob.s4.connecting": "Connecting securely…",
  "ob.s4.accessToggle": "What access this gives",
  "ob.s4.scope1Lead": "We read — we don't clutter.",
  "ob.s4.scope1Rest":
    "Your mail becomes contacts, companies and activities, captured automatically.",
  "ob.s4.scope2Lead": "Sending is part of this permission.",
  "ob.s4.scope2Rest":
    "Margince can send from this mailbox — when you send, and when you give an agent a passport that allows sending. That grant is your approval, given once. You can withdraw it at any time.",
  "ob.s4.scope3Lead": "Your data stays in your organization.",
  "ob.s4.scope3Rest": "Own-your-data — export or delete everything anytime.",
  "ob.s4.scope4Lead": "Disconnect in one click.",
  "ob.s4.scope4Rest": "The CRM keeps working; it just stops capturing.",
  "ob.s4.capturedTitle": "Mailbox connected",
  "ob.s4.capturedBody":
    "Your CRM is building itself. New mail lands here as the first sweep runs, usually in minutes.",
  "ob.s4.enterCrm": "Continue",
  "ob.s4.connectFailed": "Couldn't connect that mailbox",
  "ob.s4.notNow": "Not now",

  "ob.conv.threadLabel": "Onboarding conversation",
  "ob.conv.read.started": "Reading {host} now. I will tell you what I find.",
  "ob.conv.read.pages": "Pages read so far: {pages}.",
  "ob.conv.read.learnedField": "Learned {field}: {value}",
  "ob.conv.read.extracting":
    "Done crawling. Now extracting what the site says about your business.",
  "ob.conv.read.warning": "Heads up: {warning}",
  "ob.conv.read.failed":
    "I could not read that site. Try another URL, or tell me directly.",
  "ob.conv.read.deferred":
    "The read is paused for now. I will pick it up again automatically.",
  "ob.conv.read.pollFailed":
    "I lost the connection while reading. What I already found is kept.",
  "ob.conv.clarify.entity":
    "The site names more than one legal entity. Which one is this installation for?",
  "ob.conv.company.confirmed":
    "Company profile confirmed. Everything I stored carries its source.",
  "ob.conv.manual.chosen": "I will type it in myself.",
  "ob.conv.voice.skipped": "Skip voice for now.",
  "ob.conv.voice.uploadAdded": "Added {name}.",
  "ob.conv.voice.speakerQuestion":
    "This transcript has several speakers. Which one is you? Only your own words count.",
  "ob.conv.voice.speakerOptionDetail": "words: {words} · turns: {turns}",
  "ob.conv.voice.speakerFoot": "Your choice applies to this file only.",
  "ob.conv.voice.speakerContinue": "Use this speaker",
  "ob.conv.voice.continueSkippedStatus":
    "Skipped for now — add it later in Settings.",
  "ob.conv.voice.continueFailedStatus":
    "Your material is safe — retry now, or continue and pick this up later.",
  "ob.conv.voice.continueDeferredStatus":
    "No action needed here — continue, and it finishes on its own.",
  "ob.conv.voice.composer": "Paste the text you wrote here",
  "ob.conv.voice.dropHint":
    "You can also drop files anywhere in this conversation.",
  "ob.conv.voice.fileSkipped":
    "I cannot read {name}. I take .txt, .md, .vtt, .srt, or .json.",
  "ob.conv.voice.fileEmpty":
    "There are no words in {name}, so nothing was counted.",
  "ob.conv.voice.reactionTranscript":
    "Words kept: {kept} of {total}. Only your turns count; speech sharpens your voice most.",
  "ob.conv.voice.reactionDocument":
    "Words counted: {words}. Every word here is yours, so all of them count.",
  "ob.conv.voice.refusalUnattributed":
    "That looks like a conversation, but I cannot tell which words are yours, so I counted none.",
  "ob.conv.voice.refusalSpeaker":
    "I could not find that speaker in the transcript. Nothing was counted.",
  "ob.conv.voice.refusalUnsupported":
    "I could not parse that file as text or a transcript. Nothing was counted.",
  "ob.conv.voice.ingestFailed": "I could not add that source: {detail}",
  "ob.conv.voice.ingestUnexpected":
    "I could not add that source. Try again in a moment.",
  "ob.conv.voice.pasteAdd": "Yes, add it to my corpus.",
  "ob.conv.voice.pasteDiscard": "No, discard it.",
  "ob.conv.voice.pasteSource": "Pasted text",
  "ob.conv.voice.buildChip": "Build my voice profile",
  "ob.conv.voice.retryBuild": "Try the build again",
  "ob.conv.voice.buildPollFailed":
    "I lost the connection during the build. Your texts are kept; try the build again.",
  "ob.conv.voice.statusBuilding": "Building your voice profile",
  "ob.conv.voice.resultTitle": "Here is your voice, in your own words.",
  "ob.conv.voice.resultLoading": "Loading what the build learned.",
  "ob.conv.voice.resultEmpty":
    "The build finished, but there is nothing to show yet. You can review it in Settings.",
  "ob.conv.voice.candidateNote":
    "This version needs your review before it goes live. You can approve it in Settings.",
  "ob.conv.voice.artifactTitle": "Voice corpus",
  "ob.conv.voice.artifactBody":
    "Only your own words count here. Every number comes from the server after speaker filtering.",
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
  "ob.conv.build.extract": "Finding your signature moves.",
  "ob.conv.build.evaluate": "Testing drafts against held-out samples.",
  "ob.conv.build.activate": "Activating your voice profile.",
  "ob.conv.build.succeeded": "Your voice profile is ready.",
  "ob.conv.build.deferred":
    "The build is queued behind budget. It will run automatically.",
  "ob.conv.build.failed":
    "The build did not finish. Your texts are kept and you can retry anytime.",
  "ob.conv.done": "Setup complete. Your CRM is ready.",
  "ob.conv.clarify.question": "{question}",
  "ob.conv.clarify.optionDetail": "{detail}",
  "ob.conv.clarify.dismiss": "Skip this - I will set it myself",
  "ob.conv.clarify.keepMine": "Keep my value",
  "ob.conv.review.skipped": "You skipped: {fields}. Edit them any time.",
  "ob.conv.clarify.applyFailed":
    "I could not record that choice: {detail} Pick it again.",
  "ob.conv.clarify.applyMissing":
    "The server did not confirm that choice. Pick it again.",
  "ob.conv.loadFailed": "I could not check your setup. Please try again.",
  "ob.conv.retry": "Try again",
  "ob.conv.connect.persistFailed": "I could not record the finish. Try again.",
  "ob.conv.review.title": "Here is everything I found. Correct me.",
  "ob.conv.review.showLess": "Show less",
  "ob.conv.review.continue": "Continue",
  "ob.conv.review.progressLabel": "Required fields completed",
  "ob.conv.review.requiredRemaining_one":
    "{count} field needed before you can continue",
  "ob.conv.review.requiredRemaining_other":
    "{count} fields needed before you can continue",
  "ob.conv.review.requiredDone": "Nothing more needed — you can continue.",
  "ob.conv.review.confirmQuestionOpen":
    "A decision is still open. Answer it to continue.",
  "ob.conv.triage.stateRequired": "required, still empty",
  "ob.conv.triage.stateEmpty": "empty",
  "ob.conv.triage.stateTyped": "typed by you",
  "ob.conv.triage.stateStored": "from your profile",
  // A value the entity census read off the legal notice — the one company the
  // site names, or the candidate the human picked from several. Nothing ever
  // scored it, so the word names WHERE it came from; "chosen by you" would be
  // false on the sole-candidate path, where nobody was asked anything.
  "ob.conv.triage.stateQuoted": "read from your legal notice",
  // Where a value would stand on an empty row. It says only that the row is
  // empty: the same line serves the manual path, where nothing ever read the
  // site, and a read-backed board, where the wire says nothing about why any
  // one field came back missing. Naming a cause here would invent one.
  "ob.conv.triage.emptyHint": "Nothing here yet. Yours to add.",
  "ob.conv.triage.legalNotPublished":
    "Not stated on your legal or imprint page. Yours to add.",
  "ob.conv.triage.legalNotChecked":
    "I did not find a legal or imprint page on your site to check. Yours to add.",
  // Several companies stand on the one imprint. The value IS on the page, so
  // saying it is not stated would be false; what is missing is the human's
  // decision about which company this installation belongs to.
  "ob.conv.triage.legalUnpicked":
    "Your imprint names more than one company. Choose which one is yours and I will fill this in.",
  // The omission notice on an empty row, once a read has actually run: the
  // field is named as withheld rather than left blank, and the reason is only
  // ever one the read can support for THAT field. The read's crawl-wide
  // warnings belong to the coverage card, which states each under its own
  // heading; beside a field they would name a cause the read never gave.
  "ob.conv.triage.omittedLabel": "Omitted, not guessed",
  "ob.conv.triage.omittedField": "{field}: {reason}",
  "ob.conv.triage.mapLabel": "Jump to a section",
  "ob.conv.triage.sectionBlocking": "{count} needed to continue",
  "ob.conv.triage.sectionAdvisory": "{count} worth a check",
  "ob.conv.triage.blockingHead": "Needed to continue",
  "ob.conv.triage.advisoryHead": "Worth a check",
  "ob.conv.triage.sectionSettled": "Nothing outstanding here",
  "ob.conv.triage.sectionMore": "+{count} more",
  "ob.conv.triage.restTitle": "Background, not work",
  "ob.conv.triage.looksSolid": "Looks solid · {count}",
  "ob.conv.triage.companyWebsite": "Website",
  "ob.conv.triage.sourceCount": "{count} src",
  "ob.conv.triage.peopleLabel": "People",
  "ob.conv.triage.peopleCount": "{count} found",
  "ob.conv.triage.peopleEmpty": "No people found on your site.",
  "ob.conv.triage.factsLabel": "Facts",
  "ob.conv.triage.factsCount": "{count} found",
  "ob.rail.tokensUnit": "tok",
  "ob.conv.scene.step": "Step {n} of {m} · {label}",
  "ob.conv.scene.detour": "A quick detour",
  "ob.conv.scene.decisionSub":
    "Your site names several legal entities. The one you pick goes on every quote and invoice.",
  "ob.conv.scene.continue": "Continue",
  "ob.conv.connect.sceneTitle": "Connect your accounts.",
  "ob.conv.connect.sceneSub":
    "I build your contacts, companies and history from what is already in your inbox.",
  "ob.conv.connect.mailboxTitle": "Your mailbox",
  "ob.conv.connect.mailboxHint":
    "Pick one. This is where your contacts, companies and history come from.",
  "ob.conv.connect.networkTitle": "Your network",
  "ob.conv.connect.networkHint":
    "Optional but worth it. Turns who you know into accounts and watches them for triggers.",
  "ob.conv.connect.required": "required",
  "ob.conv.connect.recommended": "recommended",
  // Neither grant carries calendar or contacts — those are their own,
  // separate consent (Settings → Calendar) — and neither carries sign-in:
  // both connect the SAME two things here, mail read and send, so the two
  // lines say the same thing rather than inventing a difference that is not
  // in the grant.
  "ob.conv.connect.gmailBrings": "Mail read and sent via Google",
  "ob.conv.connect.microsoftBrings": "Mail read and sent via Microsoft",
  "ob.conv.connect.imapBrings":
    "Mail from any host, with your email address and an app password",
  "ob.conv.connect.linkedinAuth": "Profile link, read only",
  "ob.conv.connect.scopeGoogle": "OAuth, read and send scopes",
  "ob.conv.connect.scopeMicrosoft": "OAuth, Graph API",
  "ob.conv.connect.scopeImap": "Mail address and password",
  "ob.conv.connect.connectCta": "connect →",
  "ob.conv.connect.connectedCta": "connected",
  "ob.conv.connect.blockedCard":
    "You already picked a mailbox. Disconnect it in Settings to switch.",
  "ob.conv.connect.guaranteesToggle": "What connecting actually does",
  "ob.conv.connect.dialogHeadlineAccess": "{name} access needed",
  "ob.conv.connect.dialogHeadlineImap": "Connect your mail host",
  "ob.conv.connect.appMissingCard":
    "Your organization has not registered its {name} app yet.",
  "ob.conv.connect.appUnusableCard":
    "Your organization's {name} app cannot be opened right now. It needs an admin, not a new app.",
  "ob.conv.connect.unsupportedCard": "This installation does not serve {name}.",
  "ob.conv.connect.appSetupLink": "Set it up in Settings",
  "ob.conv.connect.dialogIntro":
    "{brings}. I read it once to build your contacts and history, then keep it in sync.",
  "ob.conv.connect.dialogClose": "Close",
  "ob.conv.connect.linkedinName": "LinkedIn",
  "ob.conv.connect.linkedinConnected": "Connected",
  "ob.conv.connect.linkedinSkippedNote": "Skipped: add it later in Settings",
  "ob.conv.connect.rosterFailedTitle": "Could not check your mailboxes",
  "ob.conv.connect.rosterFailedBody":
    "Something went wrong loading your connection status. Try again before picking a provider.",
  "ob.conv.voice.sceneTitle": "Teach me how you write.",
  "ob.conv.voice.sceneSub":
    "This CRM drafts every email in your own words, so what goes out sounds like you.",
  "ob.conv.voice.heroBody":
    "It learns your tone, rhythm and phrasing from your own writing, and from nobody else's.",
  "ob.conv.voice.whyToggle": "Why this matters",
  "ob.conv.voice.dropTitle": "Drop your writing here",
  "ob.conv.voice.dropSub":
    "Sent mail works best, because it shows how you write when you want something.",
  "ob.conv.voice.browse": "Browse files",
  "ob.conv.voice.pasteInstead": "Paste text instead",
  "ob.conv.voice.sourcesTitle": "Sources",
  "ob.conv.voice.meterLabel": "Progress toward the {min}-word minimum",
  "ob.conv.voice.meterProgress": "{words} of {min} words",
  "ob.conv.voice.meterReady":
    "{words} words — enough to build. More still sharpens it.",
  "ob.conv.voice.footReady":
    "Training takes about a minute. You will see a sample before anything is saved.",
  "ob.conv.voice.footFloor":
    "{min} words minimum. Below that the model just copies phrasing.",
  "ob.conv.voice.buildingTitle": "Learning your voice",
  "ob.conv.voice.buildingMeta": "{words} words, {sources} sources",
  "ob.conv.voice.resultSub":
    "Read the sample first. If it lands, confirm. If it is off, add more sources and I rebuild.",
  "ob.conv.voice.resultSubNoSample":
    "No sample draft came back for this build. Here is what it learned \u2014 add more of your writing and I will try again.",
  "ob.conv.voice.resultContinue": "That is me",
  "ob.conv.voice.revise": "Not quite me — add more writing",
  "ob.conv.voice.distilling": "Distilling",
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
  "ob.conv.showField": "Show me",
  "ob.conv.review.editDirectly": "Edit fields directly",
  "ob.conv.review.backToDossier": "Back to the dossier",
  "ob.conv.review.proposalFallback":
    "I could not load the prepared mapping. Review what I read directly; every field keeps its source.",
  "ob.conv.review.confirmFailed":
    "I could not save that yet: {detail} Fix it and accept again.",
  "ob.conv.review.confirmVersionSkew":
    "Your review just picked up newer information. Have a look, then press Continue again.",
  "ob.conv.review.confirmVersionSkewStuck":
    "Nothing has changed yet, so Continue would fail again. Have another look, or try in a moment.",
  "ob.conv.review.confirmNotReady":
    "This read has no draft to confirm yet. Check again when it finishes, or start a fresh read.",
  "ob.conv.review.confirmCheckFailed":
    "This read is confirmed, but I could not load the company it created. Try again shortly.",
  "ob.conv.artifact.empty":
    "Nothing read yet. Give me a website and this panel fills with sourced findings.",
  "ob.conv.results.continue": "Continue",
  "ob.conv.recap.back": "Welcome back. Here is where we stand.",
  "ob.conv.recap.company": "Your company profile for {name} is confirmed.",
  "ob.conv.recap.companyUnsaved":
    "Your company details are not saved yet. You can complete them in Settings.",
  "ob.conv.recap.voiceBuilt":
    "Your voice profile is built. Drafts can sound like you.",
  "ob.conv.recap.voiceSkipped":
    "You skipped the voice profile. Drafts use a neutral starter voice.",
  "ob.conv.recap.corpus":
    "Your corpus already holds {words} of your own words.",
  "ob.conv.recap.readTerminal":
    "Welcome back. I finished reading {host}: {count} findings with sources, ready below.",
  "ob.conv.recap.readReading":
    "Welcome back. I am still reading {host}. Pages so far: {pages}.",
  "ob.conv.recap.readFailed":
    "Welcome back. My earlier read of {host} did not finish. Give me a website again, or tell me directly.",
  "ob.conv.recap.readDeferred":
    "Welcome back. My read of {host} is paused for now. Give me a website again, or tell me directly.",
  "ob.conv.connect.skip": "Continue without a mailbox",
  "ob.conv.connect.continue": "Continue",
  "ob.conv.connect.mailboxNeeded":
    "A mailbox is still needed: mail is what gets read and drafted. Connect one above, or continue without one for now.",
  "ob.conv.linkedin.cardBody":
    "Turns your network into accounts and contacts, and flags it when a connection changes jobs.",
  "ob.conv.linkedin.limitsToggle": "What Margince can and cannot see",
  "ob.conv.linkedin.scope1Lead": "Your connection list \u2014",
  "ob.conv.linkedin.scope1Rest":
    "name, position, company and the date you connected.",
  "ob.conv.linkedin.scope2Lead": "Nothing else.",
  "ob.conv.linkedin.scope2Rest":
    "No messages, no posts, no who-viewed-you, no activity.",
  "ob.conv.linkedin.scope3Lead": "Your network stays yours.",
  "ob.conv.linkedin.scope3Rest":
    "It is attributed to you, never to the company, and disconnecting removes it.",
  "ob.conv.linkedin.scope4Lead": "Nobody is contacted.",
  "ob.conv.linkedin.scope4Rest":
    "Connecting sends no invitations and no messages, ever.",
  "ob.conv.linkedin.neverContacts":
    "Your connections never become contacts. They answer one question: who here already knows them?",
  "ob.conv.linkedin.profileLabel": "Your LinkedIn profile URL",
  "ob.conv.linkedin.profilePlaceholder": "https://www.linkedin.com/in/…",
  "ob.conv.linkedin.profileWhy":
    "So the network is attributed to you: “Anna knows them”, never “the company knows them”.",
  "ob.conv.linkedin.authorize": "Authorize with LinkedIn",
  "ob.conv.linkedin.appPending":
    "Our LinkedIn app is still awaiting approval, so nothing syncs yet. Upload Connections.csv in Settings.",
  "ob.conv.linkedin.skip": "Skip LinkedIn for now",
  "ob.conv.linkedin.connected":
    "LinkedIn authorized. Your connections will sync as soon as the app is approved.",
  "ob.conv.linkedin.skipped":
    "Skipped LinkedIn. You can connect it any time in Settings.",

  // The setup rail: five stops, one word each. Long enough to name the step,
  // short enough that five of them fit a column at 10px.
  "ob.rail.read": "Read",
  "ob.rail.confirm": "Confirm",
  "ob.rail.voice": "Voice",
  "ob.rail.connect": "Connect",
  "ob.rail.prefs": "Preferences",

  // The invite: asked once the company is confirmed, before the two steps
  // that are only about the person answering. An administrator who sets the
  // installation up for a team and never works in it finishes here.
  "ob.conv.invite.title": "Will you be working in Margince yourself?",
  "ob.conv.invite.body":
    "The company is set up. Two more steps are about you, and they only make sense if you will be using Margince too.",
  "ob.conv.invite.yes": "Yes, I'll work in Margince",
  "ob.conv.invite.yesBody":
    "Train your voice and connect your inbox and calendar: two short steps, both about you.",
  "ob.conv.invite.no": "No, I'm only setting it up",
  "ob.conv.invite.noBody":
    "Invite the first person who will work here instead, and you're done.",
  "ob.conv.invite.foot":
    "Either way, a voice and accounts can be set up later from Settings.",
  "ob.conv.invite.continue": "Continue",
  "ob.conv.invite.accepted": "Yes, I'll be working in it.",
  "ob.conv.invite.declined": "No, I'm only setting it up.",

  // The team act: the first person who will work here, invited with the
  // same form Settings → People uses.
  "ob.conv.team.title": "Invite the first user.",
  "ob.conv.team.body":
    "Somebody has to be the first person working in Margince. Add them now, or later from Settings → People.",
  "ob.conv.team.invitedLabel": "Invited so far",
  "ob.conv.team.invitedLine": "{name} is invited.",
  "ob.conv.team.skip": "Skip for now",
  "ob.conv.team.finish": "Finish setup",
  "ob.conv.team.done":
    "Setup is complete. Anyone you add can train their voice and connect their accounts from Settings.",
  "ob.conv.prefs.title": "Last, a few preferences.",
  "ob.conv.prefs.body":
    "Everything here is prefilled from what is already recorded, and can be changed later in Settings. Press Done if it all reads right.",
  "ob.conv.prefs.reportingTitle": "How the numbers are reported",
  "ob.conv.prefs.timezoneNeeded": "A reporting timezone is needed.",
  "ob.conv.prefs.autonomyTitle": "What it may change on its own",
  "ob.conv.prefs.autonomyBody":
    "Each kind of change below is proposed to you first. Switch one on and it applies without asking; switch it back any time.",
  "ob.conv.prefs.done": "Done",
  "ob.conv.prefs.persistFailed":
    "I couldn't record that setup is complete. Try again, or leave it and finish from Settings later.",

  // --- the gate: the first screen after sign-in -------------------------
  // One question and nothing else. Nobody should meet the whole tool on their
  // first screen, so the gate names what it will do, what it costs the reader
  // (two minutes), and who decides (they do) — then asks once.
  "ob.gate.title": "Hi {name}, I am the Margince AI.",
  "ob.gate.titleAnonymous": "I am the Margince AI.",
  "ob.gate.sub":
    "I read your site and draft your company profile. You approve before anything is saved. About two minutes.",
  "ob.gate.trustToggle": "How this works",
  "ob.gate.trustBody":
    "I read only public pages. Nothing is saved until you confirm it, and reading your site sends nothing to anyone.",
  "ob.gate.field": "Your website address",
  "ob.gate.placeholder": "yourcompany.com",
  "ob.gate.submit": "Read my site",
  "ob.gate.altPrompt": "No website handy?",
  "ob.gate.altAction": "Enter the details yourself",
  "ob.gate.invalidUrl":
    "That does not look like a web address. Try it as yourcompany.com.",
  // One string for two failures that look identical to the reader: the request
  // to start never landed, or the read started and did not finish. {detail} is
  // the server's own guidance and may be empty, so the sentence has to stand
  // without it.
  "ob.gate.startFailed":
    "I could not read that site. {detail} Try another address, or enter the details yourself.",
  // A deferred read is shelved, not broken: the server will come back to it. So
  // this says what is true and names both doors, without asking the reader to
  // fix anything.
  "ob.gate.readPaused":
    "That read is paused. {detail} It resumes on its own — or give me another address, or type it in.",

  // --- the read theatre -------------------------------------------------
  // Volume made visible. The wire gives no page-count denominator, so every
  // number here is an open count — never "14 of 18", never a bar with a known
  // end, because inventing the total would be inventing data.
  "ob.scan.title": "Reading {host}",
  "ob.scan.sub":
    "Every fact keeps the page it came from, so you can check anything I claim.",
  "ob.scan.doneTitle": "Read {host}",
  "ob.scan.doneSub":
    "{facts} facts and {fields} profile fields, each with the page it came from. Opening your review.",
  "ob.scan.phaseCrawling": "Fetching pages",
  "ob.scan.phaseExtracting": "Working out what you sell",
  "ob.scan.phaseQueued": "Queued, starting shortly",
  "ob.scan.phaseDeferred": "Paused for now",
  "ob.scan.pagesRead": "{pages} pages read",
  "ob.scan.pagesSkipped": "{count} skipped",
  "ob.scan.stillReading": "still reading",
  "ob.scan.pageStripLabel": "Pages read so far",
  "ob.scan.logLabel": "The pages I am walking, newest first",
  "ob.scan.pageFetched": "{url}: read",
  "ob.scan.pageSkipped": "{url}: skipped, {reason}",
  "ob.scan.pageFailed": "{url}: could not be read, {reason}",
  "ob.scan.pageNoReason": "no reason recorded",
  "ob.scan.pageStatusFetched": "read",
  "ob.scan.pageStatusSkipped": "skipped: {reason}",
  "ob.scan.pageStatusFailed": "could not be read: {reason}",
  "ob.scan.skipReason.robots": "the site asked me not to read it",
  "ob.scan.skipReason.offDomain": "it lives on another domain",
  "ob.scan.skipReason.pageCap":
    "I had already read as many pages as one read allows",
  "ob.scan.skipReason.byteCap":
    "this read had already taken in as much text as it allows",
  "ob.scan.skipReason.unreadable": "I could not read the page",
  "ob.scan.transparency": "Transparency",
  "ob.scan.costLine": "{calls} calls · {tokens} tokens · {cost}",
  "ob.scan.costPending": "no model calls billed yet",
  "ob.scan.costUnpriced": " · unpriced usage exists",

  // --- the live panel: what the read covered, and what it left ----------
  "ob.live.stateDone": "done",
  "ob.live.stateNow": "in progress",
  "ob.live.stateWaiting": "waiting",
  "ob.live.review": "Review",
  "ob.live.hide": "Hide",
  "ob.live.countPages": "{read} read · {skipped} skipped",
  "ob.live.cardCoverage": "What I read, and what I skipped",
  "ob.live.coverageWarning": "Warning",
  // A bounded read has to say it was bounded: the page counts beside it
  // otherwise read as the whole site.
  "ob.live.coverageStopped": "Stopped early",
  "ob.live.stoppedPageCap":
    "I reached the page limit for one read, so there is more of your site I did not open.",
  "ob.live.stoppedByteCap":
    "I reached the size limit for one read, so there is more of your site I did not open.",
  "ob.live.stoppedBudget":
    "I reached the budget for one read, so there is more of your site I did not open.",
  "ob.live.stoppedDeadline":
    "I ran out of time for one read, so there is more of your site I did not open.",
  "ob.live.coverageSkipped": "Skipped",
  "ob.live.coverageFailed": "Could not read",
  "ob.live.coverageClean":
    "Every page I tried came back. Nothing was skipped and nothing failed.",

  // --- facts: saving one, and the ceiling on how many -------------------
  "ob.facts.rowSave": "Save the fact: {fact}",
  "ob.facts.capReached":
    "You can save up to {max} facts. Clear one to make room for another.",

  // --- the handoff into the app -----------------------------------------
  "ob.enter.assembling": "Assembling your organization",

  // --- the mailbox backread ---------------------------------------------
  // A separate operation from connecting, and the copy has to keep them
  // separate: connecting grants access, the backread spends budget reading
  // history. Read-only, and it writes nothing until the reader approves.
  "ob.backread.heading": "How far back should I read?",
  "ob.backread.window3m": "3 months — recent context",
  "ob.backread.window6m": "6 months — recommended",
  "ob.backread.window12m": "12 months — full sales cycle",
  "ob.backread.window24m": "2 years — the relationship, not just the deal",
  "ob.backread.window60m": "5 years — everything the mailbox still holds",
  "ob.backread.estimate": "About {messages} messages in that window.",
  "ob.backread.estimateHeuristic":
    "Estimated from the mailbox, not counted yet.",
  "ob.backread.estimateCost": "Roughly {cost} in model calls.",
  "ob.backread.estimateFailed":
    "I could not estimate that window: {detail} You can still start, or pick another.",
  "ob.backread.note":
    "The backread only reads. You see every person and company I found before anything is written.",
  "ob.backread.start": "Connect and read",
  "ob.backread.startFailed":
    "I could not start the backread: {detail} Try again, or continue and start it later in Settings.",
  "ob.backread.running": "Reading your mailbox",
  "ob.backread.runningNote":
    "You can leave this running and keep working. I pick it up where I left off.",
  "ob.backread.queued": "Queued. It starts in a moment.",
  "ob.backread.progress": "{scanned} of about {total} messages",
  "ob.backread.progressNoTotal": "{scanned} messages so far",
  "ob.backread.tallyMessages": "messages read",
  "ob.backread.tallyCaptured": "kept",
  "ob.backread.tallySkipped": "ignored",
  "ob.backread.tallyPeople": "people found",
  "ob.backread.tallyCompanies": "companies found",
  "ob.backread.doneHeading": "Here is what is in there.",
  "ob.backread.doneNote":
    "Nothing is written yet. Everything I found waits for your review in the inbox.",
  "ob.backread.failed":
    "The backread stopped: {detail} Your connection is fine — you can start it again in Settings.",
  "ob.backread.cancelled": "I stopped reading. Nothing was written.",
  "ob.backread.cancelledPartial":
    "I stopped reading. What was already captured stays — it is waiting for you in the inbox.",
  "ob.backread.cancelFailed":
    "I could not stop the read: {detail} Try again — it keeps running meanwhile.",
  "ob.backread.detailUnavailable": "Something unexpected went wrong.",
  "ob.backread.cancel": "Stop reading",
  "ob.backread.explore": "Continue while it reads",
  "ob.backread.skip": "Do not read history now",

  "auth.title": "Margince",
  "auth.checking": "Checking your session…",
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
    "Accounts come from your administrator. There is no self-signup.",
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
  "auth.coreGreeting": "Hi, I’m Margince.",
  "auth.corePurpose": "I’m here to take care of the work around your work.",
  "auth.coreDevelopment": "Development AI",
  "auth.coreModeDevelopment": "offline development path",
  // The shortest label that still names the field (VOICE-RULE-1), pinned by the
  // login spec §7.1/§7.2 (Amendment 4) and reconciling
  // single-organization-auth-concept.md §12, which already drew "Email".
  "auth.email": "Email",
  // A placeholder is an EXAMPLE, never an instruction and never the label
  // again. "Enter your email" in a placeholder is a label that disappears.
  // The address is the login spec §7.2's, and the reserved example domain
  // rather than a plausible one: `company.com` belongs to somebody.
  "auth.emailPlaceholder": "name@example.com",
  "auth.password": "Password",
  "auth.passwordPlaceholder": "Password",
  "auth.passwordHint": "at least 12 characters",
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
  // §7.1 verbatim. The noun is "organization", not "workspace": ADR-0061/A107
  // keeps `workspace` internal and §7.3 removed it from authentication. And the
  // line states that ACCESS is restricted, never that data is safe, encrypted or
  // compliant — VOICE-RULE-7 rules those out here, because they are outcome
  // claims the installation's own configuration can contradict, on the screen a
  // CISO reads on the way in.
  "auth.legalProtected": "Access to this organization is restricted.",
  "auth.legalTerms": "Terms",
  "auth.legalPrivacy": "Privacy",
  "auth.signingIn": "Signing in…",
  "auth.signIn": "Sign in",
  "auth.failed": "That didn't work",
  "auth.errCredentials":
    "We couldn't sign you in. Check your email and password and try again.",
  "auth.errRateLimited":
    "Too many sign-in attempts. Wait a moment and try again.",
  "auth.errUnreachable":
    "Margince couldn't be reached. Check your connection and try again.",
  "auth.retry": "Try again",
  "auth.noticeSignedOut": "You have been signed out.",
  "auth.noticeSessionExpired":
    "Your session expired. Sign in again to continue.",
  "auth.noticeOidcFailed":
    "Sign-in with Google didn't work. If you were invited, open the link in your invitation email to finish setting up your account.",
  "auth.connectionTitle": "Margince couldn't be reached",
  "auth.connectionBody":
    "Check your connection and try again. If the problem persists, the server may be restarting.",
  "auth.unavailableTitle": "Installation not ready",
  "auth.unavailableBody":
    "This Margince installation isn't ready to sign you in. An operator needs to complete or repair the setup.",
  // The first-run claim (ADR-0105). An installation with no configured admin
  // waits to be claimed; this is the only screen that creates an account
  // without one, so it names what it is creating.
  // Changing your own password from account settings. The current password is
  // the authority, not the session.
  // The account whose password an operator chose. Authenticated, and
  // able to do exactly one thing until it has a password of its own.
  "forcedPassword.pageTitle": "Choose your password",
  "forcedPassword.title": "Choose your own password",
  "forcedPassword.body":
    "This account is still using the password your operator set up. Choose one only you know before you continue.",
  "password.title": "Password",
  "password.body": "Change the password you sign in with.",
  "password.current": "Current password",
  "password.next": "New password",
  "password.confirm": "Confirm new password",
  "password.hint": "At least 12 characters.",
  "password.tooShort": "Too short. Use at least 12 characters.",
  "password.mismatch": "These two don't match.",
  "password.signsYouOut":
    "Changing it signs you out everywhere, including here. Sign in again with the new password.",
  "password.changing": "Changing your password…",
  "password.open": "Change password",
  "password.cancel": "Cancel",
  "password.submit": "Save new password",
  "password.done": "Password changed. Sign in again with the new one.",
  // Deliberately says nothing about WHICH field: this is the fallback for a
  // refusal the server did not explain, and naming the current password would
  // send someone hunting a mistake that may not be theirs.
  "password.errorGeneric": "The password couldn't be changed. Try again.",
  "setup.pageTitle": "Set up Margince",
  "setup.title": "Claim this installation",
  "setup.body":
    "This Margince installation has no organization yet. Your operator has a one-time setup token from the token file the server wrote at first start.",
  "setup.token": "Setup token",
  "setup.tokenHint":
    "From the token file the server wrote at first start — the server log names its path, and carries the token itself if that file could not be written.",
  "setup.organization": "Organization name",
  "setup.baseCurrency": "Base currency",
  "setup.baseCurrencyHint":
    "Every amount in the product is converted to this currency. It can be changed in Settings, but only until the first amount converts against it — so it is worth getting right now.",
  "setup.baseCurrencyMalformed":
    "A currency is three letters, like EUR, CHF or USD.",
  "setup.baseLanguage": "Base language",
  "setup.baseLanguageHint":
    "The language AI writes in when the whole team reads what it wrote. Each person still picks their own display language, and replies to customers follow the language of the conversation.",
  "setup.timezone": "Reporting timezone",
  "setup.timezoneHint":
    "IANA zone name. Every reporting period is computed in it — guessed from this browser, so change it if you are not where the team works.",
  "setup.adminName": "Your name",
  "setup.adminEmail": "Your email",
  "setup.adminPassword": "Choose a password",
  "setup.passwordHint": "At least 12 characters.",
  "setup.passwordShort": "Too short. Use at least 12 characters.",
  "setup.rootWarning":
    "This creates the administrator account for the whole installation. It has every permission, including managing everyone else.",
  "setup.claim": "Create the organization",
  "setup.claiming": "Creating…",
  "setup.errorToken":
    "That setup token isn't valid for this installation. Check the token file the server named in its log at first start.",
  "setup.errorAlready":
    "This installation already has an organization. Sign in instead, or ask your operator to reset it.",
  "setup.errorFields":
    "Something in the form needs fixing. Check the fields and try again.",
  "setup.errorServer":
    "Margince couldn't complete the setup. Nothing was created. Try again in a moment; if it keeps failing, check the server log.",
  "setup.errorNetwork":
    "Margince couldn't be reached. Check your connection and try again.",
  "auth.forgotLink": "Forgot password?",
  "auth.forgotTitle": "Reset your password",
  // Two sentences, sentence-cased, with no dash. VOICE-RULE-5 forbids an em or
  // en dash anywhere in user-facing copy, and a lowercase opening mid-surface
  // reads as a fragment rather than as a sentence. Same for auth.resetSub.
  "auth.forgotSub":
    "Enter your email. If it has an account, a reset link is on its way.",
  "auth.sendResetLink": "Send reset link",
  "auth.forgotSentTitle": "Check your inbox",
  "auth.forgotSentBody":
    "If that address has an account, a reset link is on its way. It expires in one hour.",
  "auth.resetTitle": "Choose a new password",
  "auth.resetSub": "Your link is valid. Choose a new password.",
  "auth.newPassword": "New password",
  "auth.setNewPassword": "Set new password",
  "auth.resetFailed": "That reset link is invalid, used, or expired.",
  // The password was refused, not the link — so the link is still good and the
  // user must not be sent to replace it.
  "auth.resetRejectedPassword":
    "That password was refused. Choose a different one and try again.",
  // Neither the link's fault nor the user's: the token is untouched, so retrying
  // the same one is the right advice. Two sentences, no dash (VOICE-RULE-5).
  "auth.resetServerFailed":
    "We couldn't set your password just now. Your link is still valid, so try again in a moment.",
  // Its own key rather than auth.errRateLimited, which says "sign-in attempts":
  // this user is setting a password, not signing in, and copy that names the
  // wrong action reads as the wrong error.
  "auth.resetRateLimited":
    "Too many attempts. Wait a moment, then set your password again.",
  "auth.requestNewLink": "Request a new link",
  "auth.askAdminForNewLink":
    "Ask your administrator for a new set-password link.",
  "auth.resetDoneTitle": "Password updated",
  "auth.resetDoneBody":
    "Your password is changed and every other session is signed out. Sign in with the new password.",
  "auth.backToLogin": "Back to sign in",
  "auth.signOut": "Sign out",

  "client.back": "Back to Margince",
  "client.title": "Margince alongside your inbox",
  "client.sub": "the extension surface — shell-free, record-aware",
  "client.sender": "Sender",
  "client.lookup": "Look up",
  "client.open360": "Open the 360",
  "client.unknown": "Not in your organization yet.",
  "client.unknownDetail":
    "This sender matches no contact you can see. Nothing was fetched from anywhere else.",
  "client.createLead": "Capture as lead",
  "client.isolation": "talks only to YOUR organization",
  "client.attribution": "Every capture is attributed and auditable.",

  "book.title": "Book a meeting",
  "book.sub": "live availability from the connected calendar",
  "book.min15": "15 min",
  "book.min30": "30 min",
  "book.min60": "60 min",
  "book.attendee": "Attendee email",
  "book.welcomeBack": "Recognized: {name}",
  "book.subject": "Meeting via Margince",
  "book.confirmed": "Booked.",
  "book.tellThemYourself":
    "Margince does not send the invite — tell your attendee the time yourself.",
  "book.failed": "Booking didn't go through — nothing was scheduled.",
  "book.publicSub": "pick a slot — no account needed",
  "book.name": "Your name",
  "book.email": "Your email",
  "book.consentWording":
    "I agree that my name and email are stored to arrange and follow up on this meeting.",

  "prefs.title": "Choose what you hear from us",
  "prefs.sub":
    "Each purpose is separate — this isn't all-or-nothing. Transactional messages can't be switched off here, because you need them; everything else is yours to control.",
  "prefs.unsub.title": "Stop receiving these emails?",
  "prefs.unsub.lead":
    "One click stops messages of this kind to your address. Nothing else changes.",
  "prefs.unsub.loading": "Opening your email preferences\u2026",
  "prefs.unsub.afterTitle": "What happens next",
  "prefs.unsub.afterBody":
    "We stop sending you emails of this kind. Security and service messages you need for something you asked for are not affected.",
  "prefs.unsub.confirm": "Unsubscribe from these emails",
  "prefs.unsub.busy": "Recording your choice\u2026",
  "prefs.unsub.seeAll": "See all preferences",
  "prefs.unsub.privacy":
    "No login needed. This personal link only controls your email preferences \u2014 please don't share it.",
  "prefs.unsub.doneTitle": "Unsubscribed",
  "prefs.unsub.doneBody":
    "You won't receive {label} from us again. The change applies immediately.",
  "prefs.unsub.manage": "Manage preferences",
  "prefs.unsub.alreadyOff":
    "These emails were already switched off. Nothing changed.",
  "prefs.unsub.lockedTitle": "These messages can't be switched off",
  "prefs.unsub.lockedBody":
    "They're needed for something you asked for \u2014 a password reset, or a confirmation you requested.",
  "prefs.unsub.retry": "Try again",
  "prefs.unsub.unknownPurpose":
    "This link doesn't name a kind of email we send. Open your preferences to see everything.",
  "prefs.purpose.business_correspondence": "Direct correspondence",
  "prefs.purpose.marketing_email": "Product news",
  "prefs.purpose.transactional": "Security & service messages",
  "prefs.sentVia": "Sent via Margince",
  "prefs.noObjection": "On — you have not objected to these",
  "prefs.optedOut": "Off — you asked us to stop these",
  "prefs.invalidLink":
    "This link is no longer valid. Preference links expire and can be withdrawn — ask for a fresh one from any recent email.",
  "buyer.opening": "Opening your Deal Room…",
  "buyer.deadTitle": "This link no longer works",
  "buyer.deadAskContact": "Ask your contact for a fresh link.",
  "buyer.linkDead":
    "The link you used has already been opened, has lapsed, or was replaced by a newer one. Ask for a fresh link below.",
  "buyer.noLink":
    "Open this page from the link you were sent. If you no longer have it, ask for a fresh one below.",
  "buyer.emailLabel": "Your email address",
  "buyer.emailHint": "The address the invitation was sent to.",
  "buyer.requestLink": "Send me a new link",
  "buyer.linkRequested":
    "If that address was invited, a new link is on its way.",
  "buyer.pausedTitle": "Access is paused",
  "buyer.pausedBody":
    "{steward} has paused this room for now. Your link stays valid; you will be able to continue once they resume it.",
  "buyer.expiredTitle": "Access has ended",
  "buyer.expiredBody":
    "Access to this room has lapsed. Contact {steward}, or ask for a fresh link below.",
  "buyer.eyebrow": "Deal Room",
  "buyer.contact": "Your contact: {steward}.",
  "buyer.closed": "This room is closed; what it shared is a record now.",
  "buyer.previewBanner":
    "You are previewing this room as a buyer would see it. You can read everything and change nothing.",
  "buyer.previewReadOnly":
    "A preview cannot write. Close this tab to return to the Deal Room page.",
  "buyer.closedNote": "This room is now read-only.",
  "buyer.stewardUnknown": "your contact",
  "buyer.signOut": "Sign out",
  "room.docs.title": "Documents",
  "room.docs.sub":
    "What the buyer can read, with the conversation about each document under it.",
  "room.docs.empty": "No documents in the room yet.",
  "room.docs.fileLabel": "File from this deal",
  "room.docs.fileHint":
    "Anything in the deal's Files area can go in: uploads and the files its emails carried.",
  "room.docs.pickFile": "Pick a file",
  "room.docs.noFiles": "The deal's Files area is empty",
  "room.docs.groupLabel": "Group",
  "room.docs.add": "Add to room",
  "room.docs.remove": "Remove {title} from the room",
  "room.docs.group.commercial": "Commercial",
  "room.docs.group.legal": "Legal",
  "room.docs.group.security_privacy": "Security & Privacy",
  "room.docs.group.delivery_operations": "Delivery & Operations",
  "buyer.docs.title": "Documents",
  "buyer.docs.sub":
    "What has been shared with you, with the conversation about each document under it.",
  "buyer.docs.empty": "No documents yet.",
  "buyer.docs.download": "Download {title}",
  "buyer.docs.downloadFailed":
    "The download did not start. Try again, or ask your contact.",
  "buyer.docs.downloadShort": "Download",
  "buyer.poweredBy": "Powered by",
  "buyer.poweredByMargince": "Powered by Margince",
  "threads.roomTitle": "The room as a whole",
  "threads.roomSub": "Anything not about one document.",
  "threads.aboutThis_other": "{count} threads about this document",
  "threads.aboutThis_one": "{count} thread about this document",
  "threads.askAbout": "Ask about this document",
  "threads.cancel": "Cancel",
  "threads.empty": "Nothing said yet.",
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
  "deal360.blocker": "What is holding this up",
  "deal360.buyer": "What the buyer wants",
  "deal360.verdict.live": "Live",
  "deal360.verdict.drifting": "Drifting",
  "deal360.verdict.blocked": "Blocked",
  "deal360.verdict.cold": "Cold",
  "dealmail.title": "Email",
  "dealmail.sub.reply": "They wrote and nobody has answered yet.",
  "dealmail.sub.fresh": "Write to the people on this deal.",
  "dealmail.reply": "Draft the reply",
  "dealmail.send": "Send an email",
  "recordmail.title": "Email",
  "recordmail.sub.reply": "An answer is owed.",
  "recordmail.sub.fresh": "Write to them.",
  "recordmail.reply": "Draft the reply",
  "recordmail.send": "Write email",
  "deal360.rewrite": "Write it again",
  "deal360.readFull": "Read the full briefing",
  "deal360.createTask": "Add this task",
  "deal360.openBrief": "Open the meeting brief",
  "deal360.unreadable":
    "This briefing could not be read. Reload the page, or write it again.",
  "prefs.rateLimited":
    "Too many attempts from here just now. Wait a minute and reload.",
  "prefs.subscribed": "On — you asked for these",
  "prefs.alwaysOn": "always on",
  // The public confirm-your-details page. Margince speaks in first person here,
  // as it does in onboarding: short flat sentences, says what it will and will
  // not do, no em dashes.
  "confirm.title": "Your details",
  "confirm.intro":
    "I am Margince, the AI that runs this CRM. Here is everything we have recorded about you. You can correct any of it, or ask us to remove it.",
  "confirm.card.title": "What we have",
  "confirm.field.fullName": "Name",
  "confirm.field.title": "Job title",
  "confirm.field.email": "Email",
  "confirm.field.phone": "Phone",
  "confirm.field.company": "Company",
  "confirm.field.none": "Not recorded",
  "confirm.marketing.title": "May we stay in touch?",
  "confirm.marketing.ask":
    "News from time to time, roughly once a month. You decide, and I will hold to it.",
  "confirm.marketing.yes": "Yes, keep me posted",
  "confirm.marketing.no": "No thanks, just keep my details correct",
  "confirm.provenance.title": "Where we got your details",
  "confirm.provenance.empty": "Nothing recorded about where these came from.",
  "confirm.provenance.line": "{field}: from {source}, recorded {date}",
  "confirm.erasure.ask": "Remove my details",
  "confirm.erasure.staged": "Removal requested. Confirm below to send it.",
  "confirm.submit": "Confirm",
  "confirm.done.title": "Thank you",
  "confirm.done.body":
    "I have recorded your answer. Anything you changed goes to a person here to apply, and this link is now used up.",
  "confirm.invalidLink":
    "This link is no longer valid. It may have been used already, or it may have expired.",
  "prefs.lockedWhy": "Needed for something you asked for, so it stays on.",
  "prefs.confirmationNeededWhy":
    "To switch this on, use the confirmation link in our email. You can switch it off here at any time.",
  "prefs.notSaved": "Not saved yet.",
  "prefs.savePending": "Pending: {changes}.",
  "prefs.saveProof":
    "We record the exact wording you saw and a timestamp as proof — then it applies to every future send.",
  "prefs.save": "Save preferences",
  "prefs.discard": "Discard",
  "prefs.partialSave":
    "Something went wrong part-way. Some of your choices may have been saved — we've reloaded your current settings so you can see exactly where you stand.",
  "prefs.wording.business_correspondence":
    "“Send me replies and direct messages about our conversations.”",
  "prefs.wording.transactional":
    "“Send me what I need for something I asked for.”",
  "prefs.wordingGeneric": '"Send me {label}."',
  "prefs.wording.marketing_email":
    '"Send me product updates & occasional marketing email."',
  "prefs.wording.events": '"Send me event & webinar invitations."',
  "prefs.unsubscribeAll": "Stop everything I can switch off",
  "prefs.unsubscribeAllHint":
    "This switches off every row above that has a checkbox you can use. Rows marked ALWAYS ON stay on — you need them for things you asked for.",
  "prefs.oneClickDone":
    "Done — you're off our marketing email. It takes effect immediately across every campaign.",
  "prefs.oneClickAlreadyOff": "Nothing to do — these were already off.",
  "prefs.undo": "Undo — keep receiving marketing",
  "prefs.undoExplicit":
    "Re-subscribing is an explicit opt-in — we won't silently turn it back on. Save below to record your consent, or discard.",

  "auto.tier.runs": "runs",
  "auto.tier.approval": "approval",
  "auto.sub":
    'A rule marked "runs" acts on its own. One marked "approval" stages into the approval inbox.',
  "auto.readOnly":
    "Read-only view — you do not have permission to change automations.",
  "auto.catalog": "Starter library",
  "auto.catalogSub": "the closed set of automation types",
  "auto.instances": "Configured automations",
  "auto.use": "Use template",
  "auto.name": "Name",
  "auto.create": "Create",
  "auto.createdPaused": "Created paused — nothing runs until you enable it.",
  "auto.delete": "Delete",
  "auto.statusEnabled": "enabled",
  "auto.statusPaused": "paused",
  "auto.dateField.placeholder": "Select date field",
  "auto.dateField.needsObject":
    "Choose an object first to list its date fields.",
  "auto.dateField.empty": "This object has no active date fields yet.",
  "auto.dateField.loadError":
    "Couldn't load this object's date fields. Try again.",
  "auto.enabledFor": "{name} is enabled",
  "auto.rowActions": "Actions for {name}",
  "auto.withheld":
    "The configured automations are hidden — your role cannot read them.",
  "auto.deleteTitle": "Delete this automation?",
  "auto.deleteBody":
    "“{name}” and its settings are removed for good. To stop it firing without losing the rule, turn it off instead.",

  "auto.runs.open": "Runs",
  "auto.runs.title": "Run history",
  "auto.runs.filterAll": "All",
  "auto.runs.filterFired": "Fired",
  "auto.runs.filterFailed": "Failed",
  "auto.runs.filterBlocked": "Blocked",
  "auto.runs.filterSkipped": "Skipped",
  "auto.runs.filterQueued": "Queued for approval",
  "auto.runs.empty": "This automation hasn't fired yet.",
  "auto.runs.emptyFiltered": "No runs with this outcome.",
  "auto.runs.needsApproval": "needs approval",
  "auto.runs.why": "Why",
  "auto.runs.target": "Target",
  "auto.runs.result": "Result",
  "auto.runs.reason": "Reason",
  "auto.runs.outcomeFired": "fired",
  "auto.runs.outcomeFailed": "failed",
  "auto.runs.outcomeBlocked": "blocked",
  "auto.runs.outcomeSkipped": "skipped",
  "auto.runs.outcomeQueued": "queued",

  "auto.preview.open": "Preview",
  "auto.preview.title": "Dry-run blast radius",
  "auto.preview.window": "Window",
  "auto.preview.window7": "7d",
  "auto.preview.window30": "30d",
  "auto.preview.window90": "90d",
  "auto.preview.matchesNow": "Matches now: {n}",
  "auto.preview.wouldFire": "Would fire: ~{n} / {days}d",
  "auto.preview.notComputable": "Trailing estimate not computable",
  "auto.preview.hidden": "{n} hidden — no access",
  "auto.preview.explainer":
    "A read-only dry run — no records are changed and nothing is sent.",

  "strength.title": "Relationship strength",
  "strength.score": "Score {score}/100",
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
  "strength.inout": "{in} in · {out} out (90d)",
  "strength.computedFrom": "Computed from {count} activities",

  // The relationship-graph cards (ADR-0078). The colleague bands are PO-F-3b's
  // own vocabulary and deliberately differ from the workspace-wide card's:
  // the two measure different things and must not read as comparable.
  "network.title": "Who here knows them",
  "network.empty": "Nobody here has recorded contact with this person yet.",
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
  "coverage.risk.single_threaded_ours": "Carried by one colleague",
  "coverage.risk.coverage_gap": "No engaged champion",
  "coverage.risk.champion_left": "Champion has left",
  "coverage.risk.stakeholder_left": "Stakeholder has left",
  "coverage.risk.going_cold": "Going cold",

  "cf.title": "Custom fields",
  "cf.formSection": "Custom fields",
  "cf.subtitle":
    "Add a simple typed field to an object you already have — at runtime, no developer, no deploy. New objects and relationships still go through code.",
  "cf.object": "Object",
  "cf.obj.deal": "Deal",
  "cf.obj.organization": "Company",
  "cf.obj.person": "Contact",
  "cf.obj.lead": "Lead",
  "cf.listLabel": "Fields on {object}",
  "cf.col.field": "Field",
  "cf.col.type": "Type",
  "cf.col.addedBy": "Added by",
  "cf.addedByYou": "You",
  "cf.addedByAdmin": "Admin",
  "cf.empty.deal":
    "No custom fields on Deal yet. Add one if you track something we didn't ship.",
  "cf.empty.organization":
    "No custom fields on Company yet. Add one if you track something we didn't ship.",
  "cf.empty.person":
    "No custom fields on Contact yet. Core fields cover the contact record; add one if you track more.",
  "cf.empty.lead":
    "No custom fields on Lead yet. A field you add here also appears once a lead is promoted to a contact.",
  "cf.type.text": "Text",
  "cf.type.number": "Number",
  "cf.type.date": "Date",
  "cf.type.currency": "Currency",
  "cf.type.picklist": "Picklist",
  "cf.type.boolean": "Yes / No",
  "cf.builder.addTo": "Add a field to {object}",
  "cf.builder.open": "Add a field",
  "cf.builder.noCode": "no code",
  "cf.builder.intro":
    "A new field is a real column on the existing table — it filters, reports, exports, and is in the API like any core field. It is not a new object.",
  "cf.label": "Label",
  "cf.apiKey": "API key",
  "cf.apiKeyHint":
    "Auto-derived, immutable once live. Prefixed cf_ so it never collides with a core field.",
  "cf.typeLabel": "Type",
  "cf.currencyCode": "Currency code",
  "cf.currencyHint":
    "Three-letter ISO-4217 code (e.g. EUR, USD). Money is stored to the cent.",
  "cf.options": "Options",
  "cf.addOption": "Add option",
  "cf.removeOption": "Remove option",
  "cf.optionPlaceholder": "Option label",
  "cf.lastOptionBlocked": "A picklist needs at least one option",
  "cf.gate.title": "Adding a field is gated.",
  "cf.gate.body":
    "On confirm it becomes a live column on every {object} — on the 360, in search & filters, lists, export, and the API. The add is written to the audit trail.",
  "cf.refuse.title":
    "That looks like a new object or relationship, not a field.",
  "cf.refuse.body":
    "This builder adds simple fields to existing records only. A new object, a link between objects, or a calculated roll-up is a structural change — it ships as a reviewed change to Margince in a new version, done by people, not by the product editing its own code.",
  "cf.refuse.route":
    "Route it through the development path — your own engineers, an implementation partner, or Margince services.",
  "cf.confirm": "Confirm & add field",
  "cf.writing": "writing…",
  "cf.added": 'Field "{label}" added — live on 360, filters, export & API',
  "cf.edit": "Edit label",
  "cf.archive": "Archive field",
  "cf.archived":
    '"{label}" archived — hidden from new records, retained in audit & history (reversible)',
  "cf.renamePrompt": "New label",
  "cf.renamed": 'Renamed to "{label}"',
  "cf.audit.title": "Recent field changes",
  "cf.audit.empty": "No custom-field changes yet.",
  "cf.audit.footer":
    "Every add / edit / archive is recorded permanently in the audit log.",
  "cf.noPermission":
    "You have read-only access to custom fields — adding, editing and archiving are not yours to do here.",
  "cf.retired": "Retired",
  // The settings level, in the order the sidebar prints it. "General" rather
  // than "Organization" for the first org entry: the group heading above it
  // already says that word, and a row repeating its own heading names nothing.
  // The same reason keeps the possessive off "Agents" — the group is "You".
  //
  // "Connections" and "Integrations" are the same distinction the two groups
  // are: the mailbox and the network a PERSON connected, against the outside
  // systems the INSTALLATION is wired to. One row carried both before, which
  // is why it had to be ungated to keep a rep's own mailbox reachable.
  "settings.tab.account": "Account",
  "settings.tab.voice": "Writing voice",
  "settings.tab.agents": "Agents",
  "settings.tab.connections": "Connections",
  "settings.tab.general": "General",
  "settings.tab.users": "Users & teams",
  "settings.tab.extensions": "Extensions",
  "settings.tab.integrations": "Integrations",
  "settings.tab.capture": "Capture",
  "settings.tab.data-model": "Data model",
  "settings.tab.ai": "AI",
  "settings.tab.knowledge": "Knowledge",
  "corpusAsk.title": "Ask your documents",
  "corpusAsk.sub":
    "A question in your own words, answered only from one set of documents this organization filed. What the set does not cover is refused rather than guessed at, and every sentence carries the passage it rests on.",
  "corpusAsk.whichSet": "Which set",
  "corpusAsk.question": "Your question",
  "corpusAsk.submit": "Ask",
  "corpusAsk.byModel": "Written from the passages below",
  "corpusAsk.atLine": "line {line}, column {column}",
  "corpusAsk.byPassages": "The passages themselves — nobody wrote a summary",
  "corpusAsk.notReady":
    "This set is not finished being read yet — {embedded} of {total} passages are searchable. Nothing is wrong with your question; try again shortly.",
  "corpusAsk.retrievalUnavailable":
    "Nothing was searched: this installation has no search index configured, so the documents could not be looked at. That is a setup matter rather than anything about your question.",
  "corpusAsk.unreviewed":
    "The search found these passages nearest to your question. Nothing has read them, so nothing has judged whether they answer it.",
  "corpusAsk.notCovered.title": "Not covered by this set",
  "corpusAsk.notCovered.body":
    "{name} was searched in full and holds nothing close enough to answer this. It covers:",
  "knowledge.title": "Document sets",
  "knowledge.sub":
    "Bodies of text this organization can be asked questions of. An answer comes only from what is filed here, and a question they do not cover is refused rather than guessed at.",
  "knowledge.withheld": "Which document sets exist is not yours to see.",
  "knowledge.coverage":
    "{documents} documents · {embedded} of {total} passages searchable",
  "knowledge.reindexing":
    "This set is being re-read after a change to how text is indexed. Asking it will say it is not ready until that finishes; nothing has been lost.",
  "knowledge.showDocuments": "Show documents",
  "knowledge.hideDocuments": "Hide documents",
  "knowledge.documents": "Documents",
  "knowledge.noDocuments": "Nothing filed here yet.",
  "knowledge.archive": "Archive set",
  "knowledge.archiveConfirm.title": "Archive this document set?",
  "knowledge.archiveConfirm.body":
    "The set and everything filed in it stop being searchable. Nothing is destroyed.",
  "knowledge.deleteDocument": "Delete",
  "knowledge.deleteConfirm.title": "Delete this document?",
  "knowledge.deleteConfirm.body":
    "The file, the text taken from it and the search index built on it are destroyed. This cannot be undone.",
  "knowledge.ingest.queued": "Waiting to be read",
  "knowledge.ingest.running": "Being read",
  "knowledge.ingest.done": "Searchable",
  "knowledge.ingest.failed": "Could not be read",
  "knowledge.upload.label": "Add a document",
  "knowledge.upload.hint":
    "Plain text, Markdown, CSV or JSON. There is no reader for PDFs or Word files here, and one would be refused rather than filed empty.",
  "knowledge.upload.empty": "Drop a text file here, or choose one",
  "knowledge.upload.submit_other": "Add {count} documents",
  "knowledge.upload.refused": "{filename} was not added: {message}",
  "knowledge.upload.submit_one": "Add document",
  "knowledge.new.title": "New document set",
  "knowledge.new.name": "Name",
  "knowledge.new.topic": "What this set covers",
  "knowledge.new.topicHint":
    "Write a sentence, not a label. It is quoted back to whoever asks a question this set does not cover, so it is read at their least patient moment.",
  "knowledge.new.submit": "Create set",
  "settings.tab.privacy": "Privacy & audit",
  "settings.tab.capture-activity": "Capture activity",
  "captureActivity.title": "Capture activity",
  "captureActivity.sub":
    "What the last 24 hours of your mail turned into. The senders you keep out are above.",
  "captureActivity.scope.label": "Whose activity",
  "captureActivity.outcomes": "Outcomes",
  "captureActivity.messages": "Messages",
  "captureActivity.scope.mine": "Mine",
  "captureActivity.scope.workspace": "Shared channels",
  "captureActivity.scopeNote":
    "Counted from the point a connector hands a message to this CRM. What a connector filtered on its own side — a chat reaction, a mail rule — is not included. Covers messages; lead capture is not shown here.",
  "captureActivity.filtered":
    "Showing {shown} of {total} {outcome} in this window.",
  "captureActivity.openTrace": "See every step this message went through",
  "captureActivity.emptyFiltered":
    "none of the loaded rows match — load more to reach the rest of the window",
  "captureActivity.loadMore": "Load more",
  "captureActivity.empty": "no capture activity in the last 24 hours",
  "captureActivity.payloadsOff":
    "This installation does not record who sent a message or what it was about, so the rows below name the decision only.",
  "captureActivity.contentNone": "no sender recorded",
  "captureActivity.outcome.captured": "Captured",
  "captureActivity.outcome.internal": "Dropped as internal",
  "captureActivity.outcome.suppressed": "No contact created",
  "captureActivity.outcome.deferred": "Waiting on a verdict",
  "captureActivity.outcome.fault": "Derivation failed",
  "captureActivity.reason.internal_only": "every party was on your own domains",
  "captureActivity.reason.deferral_capped":
    "the open-question limit was reached, so no verdict is coming",
  "captureActivity.reason.noise_prior":
    "a previous verdict judged this sender noise, so it will be archived",
  "captureActivity.reason.decided_prior":
    "this sender was already decided, so no contact will be created",
  "captureActivity.reason.no_granting_human":
    "the connection named no member to act for",
  "captureActivity.reason.invisible_incumbent":
    "it matched a record outside what you can see",
  "captureActivity.reason.derivation_failed":
    "the contact step failed; the message itself is unaffected",
  "captureActivity.reason.no_counterparty": "no sender this CRM could record",
  "captureActivity.reason.role_mailbox":
    "a shared mailbox, not a person — kept, but no contact created",
  "captureActivity.reason.private_thread":
    "a private conversation — kept for you, but no contact created",
  "captureActivity.reason.transactional_infra":
    "the sender is mail infrastructure, not a company you work with",
  "captureActivity.reason.transactional_prefix":
    "the sender looks like an automated mailer, not a person",
  "captureActivity.outcome.deferred_capped": "Not queued",
  "captureActivity.outcome.deferred_sent": "Sent for a verdict",
  "captureActivity.resolution.pending": "still waiting",
  "captureActivity.resolution.unsure": "sent to the review queue",
  "captureActivity.resolution.real": "judged a real contact",
  "captureActivity.resolution.noise": "judged noise",
  "captureActivity.resolution.rejected": "declined by a human",
  "captureActivity.resolution.suppressed": "suppressed",
  "pipeline.title": "How this message was handled",
  "pipeline.sub":
    "Every step of the capture pipeline, in the order this message met them.",
  "pipeline.payloadsOff":
    "No sender or subject is stored for any step: this deployment turned payload capture off.",
  "pipeline.transport": "Carried by",
  "pipeline.unavailable": "this message's pipeline steps could not be read",
  "pipeline.status.done": "Done",
  "pipeline.status.skipped": "Skipped",
  "pipeline.status.pending": "Waiting",
  "pipeline.status.failed": "Failed",
  "pipeline.status.not_applicable": "Did not apply",
  "pipeline.status.unknown": "Cannot tell",
  "pipeline.reason.record_not_available":
    "this step's record is no longer kept, or is not yours to read — once the record is gone the two cannot be told apart",
  "pipeline.status.not_reported": "Not reported here",
  "pipeline.subject.message": "about this message",
  "pipeline.subject.sender": "about the sender, not this message alone",
  "pipeline.subject.domain": "about the sender's domain",
  "pipeline.subject.thread": "about the whole conversation",
  "pipeline.stage.connector_filter": "Connector filtering",
  "pipeline.stage.ingress_gate": "Admission check",
  "pipeline.stage.erasure_check": "Erasure check",
  "pipeline.stage.internal_drop": "Internal-only check",
  "pipeline.stage.activity_write": "Saved to the timeline",
  "pipeline.stage.tier_ladder": "Contact decision",
  "pipeline.stage.person_create": "Contact created",
  "pipeline.stage.verdict": "Sender verdict",
  "pipeline.stage.company_triage": "Company check",
  "pipeline.stage.attention_label": "Attention label",
  "pipeline.stage.material_events": "Conversation reading",
  "pipeline.stage.claim_extraction": "Commitments and open loops",
  "pipeline.reason.internal_only": "every party was on your own domains",
  "pipeline.reason.invisible_incumbent":
    "it matched a record outside what you can see",
  "pipeline.reason.transactional_infra":
    "the sender is mail infrastructure, not a company you work with",
  "pipeline.reason.transactional_prefix":
    "the sender looks like an automated mailer, not a person",
  "pipeline.reason.deferral_capped":
    "the open-question limit was reached, so no verdict is coming",
  "pipeline.reason.noise_prior": "a previous verdict judged this sender noise",
  "pipeline.reason.decided_prior": "this sender was already decided",
  "pipeline.reason.no_counterparty": "no sender this CRM could record",
  "pipeline.reason.role_mailbox":
    "a shared mailbox, not a person — kept, but no contact created",
  "pipeline.reason.private_thread":
    "a private conversation — kept for you, but no contact created",
  "pipeline.reason.no_granting_human":
    "the connection named no member to act for",
  "pipeline.reason.derivation_failed":
    "the contact step failed; the message itself is unaffected",
  "pipeline.reason.not_linked_yet": "no contact is linked to this message yet",
  "pipeline.reason.no_contact_intended":
    "the contact decision concluded that none was to be made",
  "pipeline.reason.awaiting_verdict":
    "the sender is still waiting on a verdict",
  "pipeline.reason.verdict_reached":
    "a verdict has been reached for this sender",
  "pipeline.reason.no_open_question":
    "there was no open question about this sender",
  "pipeline.reason.transport_not_read":
    "this step reads email only, and the message arrived over another transport",
  "pipeline.reason.sender_undecided":
    "the sender is still waiting on a verdict, so the message is held back",
  "pipeline.reason.archived": "the message is archived",
  "pipeline.reason.not_connector_captured":
    "the message was not captured by a connector",
  "pipeline.reason.awaiting_batch":
    "it is eligible and waiting for the next batch",
  "pipeline.reason.labelled": "the message was labelled",
  "pipeline.reason.not_comparable":
    "what a connector filters on its own side is not counted here — the numbers mean different things per connector",
  "pipeline.reason.connector_side_defect":
    "admission failures are a fault of the connection, not of one message",
  "pipeline.reason.would_restore_erased":
    "reporting this would restore data an erasure removed",
  "pipeline.reason.no_writer_yet": "this step does not exist yet",
  "pipeline.reason.not_reported_yet":
    "this step runs, but is not reported here yet",
  "settings.tab.maintenance": "Maintenance",
  "settings.tab.license": "License",
  "license.card.title": "License and seats",
  "license.state.licensed": "Licensed",
  "license.state.uncapped": "Licensed, no seat limit",
  "license.state.unlicensed": "No license configured",
  "license.state.refused": "License refused",
  "license.absent.title": "This installation has no license",
  "license.absent.body":
    "Everything keeps working and nothing is capped. Configure a license token in the deployment when you want seats counted against a grant.",
  "license.refused.title": "This installation's license was refused",
  "license.refused.body":
    "The token in the deployment was presented and rejected. Everything keeps working, uncapped, until it is replaced — check the token and the installation's clock.",
  "license.seats.title": "Seats",
  "license.seats.used": "Seats in use",
  "license.seats.granted": "Seats granted",
  "license.seats.uncapped": "No limit",
  "license.meter.label": "{used} of {granted} seats in use",
  "license.over.title": "You are over your seat entitlement",
  "license.over.body":
    "{used} seats are in use and the license grants {granted}. Nobody loses access and no seat is taken away — but no new member can be invited until you are back inside the entitlement. Deactivate a member, or raise the entitlement.",
  "license.holder.title": "Licensed to",
  "license.holder.org": "Organization",
  "license.holder.contact": "Contact",
  "license.holder.installation": "Installation",
  "license.holder.validUntil": "Valid until",
  "license.holder.expiredOn": "Expired on",
  "license.holder.id": "License id",
  "license.grace.title": "This license expired",
  "license.grace.body":
    "The license expired on {expiry}. It still works, for a limited period. Renew it to keep the installation in service.",
  "license.renewal.title": "This license needs a renewal",
  "license.renewal.body":
    "The license expires on {expiry}. Nothing changes before that date.",
  "license.counting":
    "Full seats that are neither deactivated nor suspended, agents included. Read-only seats are unlimited and never counted. This is the count a new member is admitted against.",
  "settings.group.you": "You",
  "settings.group.admin": "Admin settings",
  "settings.rates.fxTitle": "Currency rates",
  "settings.rates.fxIntro":
    "Exchange rates that convert foreign-currency amounts to your base currency. New rates take effect today or later; past rates are never changed.",
  "settings.rates.fxWithheld":
    "Only an admin or ops can see the currency rates. They are the conversion every roll-up in the installation is built on, so they are not shown more widely.",
  "settings.rates.modelWithheld":
    "Only an admin or ops can see what each model costs. The prices are operator information, so they are not shown more widely.",
  "settings.rates.readOnly":
    "Read-only view — you do not have permission to change rates.",
  "settings.rates.fxTableLabel": "Rates in force",
  "settings.rates.fxAdd": "Set rate",
  "settings.rates.fxEmpty": "No currency rates yet.",
  "settings.rates.fxModalTitle": "Set a currency rate",
  "settings.rates.rateToBase": "Rate (to base currency)",
  "settings.rates.modelTitle": "AI model costs",
  "settings.rates.modelIntro":
    "Per-model prices in USD per 1M tokens, used to estimate AI spend. Transparency only — prices never change how models are routed.",
  "settings.rates.modelTableLabel": "Prices in force",
  "settings.rates.modelAdd": "Add model rate",
  "settings.rates.modelEmpty": "No model rates yet.",
  "settings.rates.modelModalTitle": "Set a model price",
  "settings.rates.setRate": "Save",
  "settings.rates.refresh": "Refresh from sources",
  "settings.rates.refreshEnqueued":
    "Refresh requested — any proposed changes will appear in the inbox.",
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
    "Your personal writing voice. It shapes drafts made for you, stays private to you, and only learns from sources you add.",
  "settings.voice.readOnly":
    "Read-only view — you do not have permission to change your Voice DNA.",
  "settings.voice.emptyBody":
    "Add a few things you wrote and build your Voice DNA from them. It takes about a minute.",
  "settings.voice.status.collecting": "Collecting",
  "settings.voice.status.ready": "Ready",
  "settings.voice.status.stale": "Needs a rebuild",
  "settings.voice.bandThin": "thin",
  "settings.voice.bandGood": "good",
  "settings.voice.bandRich": "rich",
  "settings.voice.bandSharp": "sharp",
  "settings.voice.version": "version {n}",
  "settings.voice.derivedLabel": "Your derived voice",
  "settings.voice.derivedEmpty":
    "Not built yet — add samples and build to see your derived voice.",
  "settings.voice.personalityLabel": "Your preferences",
  "settings.voice.personalityPlaceholder":
    "Notes on how you want to sound — kept exactly as you write them; the model never overwrites this.",
  "settings.voice.savePreferences": "Save preferences",
  "settings.voice.corpusLabel": "Writing samples",
  "settings.voice.corpusRowLabel": "In your corpus now",
  "settings.voice.meter": "{count} of {target} words",
  "settings.voice.register.email": "email",
  "settings.voice.register.social": "social",
  "settings.voice.register.long_form": "long-form",
  "settings.voice.register.spoken": "spoken",
  "settings.voice.register.general": "general",
  "settings.voice.bandDrop":
    "Removing this drops your voice from {from} to {to}. Confirm by activating remove again.",
  "voice.insights.avoidLabel": "What your voice avoids",
  "voice.insights.voiceScore": "voice match {pct}%",
  "voice.insights.next.addTranscript":
    "Add a call or meeting transcript \u2014 spoken words are your highest-signal source.",
  "voice.insights.next.addEmail":
    "Add sent emails \u2014 they are the primary source for how you write at work.",
  "voice.insights.next.addWords":
    "Add roughly {count} more words to reach the sharp band.",
  "voice.insights.next.atTarget":
    "Your corpus is at target; keep it fresh by adding recent writing occasionally.",
  "voice.status.active": "active",
  "voice.status.candidate": "awaiting review",
  "voice.status.superseded": "superseded",
  "voice.status.rejected": "rejected",
  "voice.classification.routine": "routine change",
  "voice.classification.material": "material change",
  "voice.outcome.autoActivated": "activated automatically",
  "voice.outcome.reviewRequired": "review required",
  "voice.outcome.manuallyActivated": "activated by you",
  "voice.outcome.rejected": "rejected",
  "voice.outcome.rollback": "restored",
  "voice.history.versionRow": "v{n} \u00b7",
  "voice.history.loadMore": "Show older entries",
  "voice.insights.provenance": "Built from your corpus \u00b7 v{n}",
  "voice.insights.statWords": "Words: {count}",
  "voice.insights.statSources": "Sources: {count}",
  "voice.insights.statSentence": "\u2248{count} words per sentence",
  "voice.insights.thinkingLabel": "How you think",
  "voice.insights.movesLabel": "Your signature moves \u2014 in your own words",
  "voice.insights.samplesLabel": "Sample drafts in your voice",
  "voice.insights.draftOnly": "draft only \u2014 never sent",
  "voice.insights.disclosure":
    "AI-assisted drafts; every send stays a human decision.",
  "voice.insights.nextBestLabel": "To make it better:",
  "voice.candidate.title":
    "A new voice version (v{n}) is ready — read it before you use it.",
  "voice.candidate.whatItIs":
    "This is what the build learned from your samples. It is not in use yet: nothing is drafted in this voice until you choose it.",
  "voice.candidate.reviewLabel": "What this version says about how you write",
  "voice.candidate.concernsLabel":
    "Why it is waiting for you rather than going live on its own",
  "voice.candidate.applyHint":
    "If it reads like you, use it. If it does not, keep your current voice and add more of your own writing — the next build learns from what is in your corpus.",
  "voice.candidate.reason.malformed":
    "The check that scores a new voice could not read some of the sample drafts, so its score is based on fewer samples than usual.",
  "voice.candidate.reason.lowScore":
    "Sample drafts written in this voice scored {score} against your own writing, under the {floor} this installation requires to activate a voice on its own.",
  "voice.candidate.reason.hardFailures":
    "{n} phrases this voice is supposed to avoid survived into the sample drafts.",
  "voice.candidate.reason.rulesRemoved":
    "{n} rules about what to avoid were dropped compared with your previous version.",
  "voice.candidate.apply": "Use this version",
  "voice.candidate.reject": "Keep my current voice",
  "voice.history.label": "Versions and learning",
  "voice.history.empty": "No versions yet \u2014 build your voice first.",
  "voice.history.deltasLabel": "What changed",
  "voice.history.deltasEmpty":
    "Nothing to compare yet \u2014 a change appears here from your second build on.",
  "voice.history.deltaRow": "v{from} \u2192 v{to}",
  "voice.history.learning":
    "Learning continuously \u2014 drafts served: {drafted} \u00b7 edited before sending: {edited} \u00b7 rejected: {rejected}.",
  "voice.history.rollback": "Restore version {n}",
  "settings.voice.corpusEmpty": "No samples yet.",
  "settings.voice.excluded": "excluded",
  "settings.voice.removeSource": "Remove sample",
  "settings.voice.addSource": "Add writing samples",
  "settings.voice.addFirstLabel": "Your first writing sample",
  "settings.voice.dropHint":
    "Drop files here or choose them. .txt, .md, .vtt, .srt or .json, several at once is fine.",
  "settings.voice.dropEmpty":
    "Drop your writing here, or click to choose files",
  "settings.voice.whyToggle": "Why this matters",
  "settings.voice.whyBody":
    "Margince drafts emails for you in your own words, so what goes out sounds like you. It learns your tone, rhythm and phrasing from your own writing, and from nobody else's. Your samples stay private to you.",
  "settings.voice.worksTitle": "What works best",
  "settings.voice.worksEmails":
    "Sent emails, saved as .txt or .md. They show how you write when you want something.",
  "settings.voice.worksDocs":
    "Proposals, posts and anything else you wrote yourself.",
  "settings.voice.worksTranscripts":
    "Call or meeting transcripts (.vtt, .srt, .json or a text export). I ask which speaker is you and keep only your own turns.",
  "settings.voice.worksNot":
    "Leave out what others wrote and drafts an AI made for you. They would teach it someone else's voice.",
  "settings.voice.floorNote":
    "{min} words minimum for a first build. Below that the model just copies phrasing.",
  "settings.voice.floorLabel": "Progress towards the first build ({min} words)",
  "settings.voice.floorProgress": "{words} of {min} words to a first build",
  "settings.voice.speakerQuestion":
    "“{name}” is a conversation. Which speaker is you?",
  "settings.voice.speakerWhy":
    "Only your own turns are kept. Everyone else's words are dropped.",
  "settings.voice.speakerDetail": "{words} words, {turns} turns",
  "settings.voice.speakerConfirm": "That one is me",
  "settings.voice.speakerDismiss": "Skip this file",
  "settings.voice.noticeKept":
    "{name}: kept {kept} of {total} words. Only your turns count.",
  "settings.voice.noticeAdded": "{name}: {words} words added.",
  "settings.voice.noticeSkippedType":
    "{name} was skipped — only text files can be read.",
  "settings.voice.noticeSkippedEmpty": "{name} was skipped — it has no text.",
  "settings.voice.noticeDismissed":
    "{name} was skipped — nothing in it could be attributed to you.",
  "settings.voice.noticeAskQueueFull":
    "{name} was not added — answer the speaker questions above first, then add it again.",
  "settings.voice.noticeFailed": "{name} could not be added: {detail}",
  "settings.voice.noticeUnexpected": "{name} could not be added.",
  "settings.voice.refusalUnattributed":
    "{name} is a conversation, and none of it could be attributed to you — so none of it was added.",
  "settings.voice.refusalSpeaker":
    "That speaker was not found in {name}, so nothing was added.",
  "settings.voice.refusalUnsupported":
    "{name} is not a format the corpus can read.",
  "settings.voice.buildsTitle": "Builds",
  "settings.voice.buildRowLabel": "Build from your samples",
  "settings.voice.building": "Building…",
  "settings.voice.buildRunning":
    "Building your voice now — this takes about a minute. You can leave this page; the build keeps running.",
  "settings.voice.rebuild": "Rebuild Voice DNA",
  "settings.voice.buildFirst": "Build my Voice DNA",
  "settings.voice.buildNeedsWords":
    "About {n} more words and I can build your first voice. Below that there is not enough of your writing to learn anything honest from.",
  "settings.voice.buildProvisional":
    "Enough to build from. About {n} more words gives the build a fuller picture of how you write.",
  "settings.voice.buildStatus.succeeded": "Voice DNA updated.",
  "settings.voice.buildStatus.failed": "The build didn't finish — try again.",
  "settings.voice.buildStatus.deferred":
    "Queued — it'll finish shortly and update automatically.",
  "settings.voice.buildStatus.pending":
    "Still building — this can take a moment; it'll update here when it's done.",
  "extAccess.title": "Extensions & access",
  "extAccess.sub":
    "What each composed extension unit brought into this installation, and which role may use it. Admin-only.",
  "extAccess.adminOnly": "Extension access is available to admins only.",
  "extAccess.readOnly":
    "Your seat reads this page. Changing a grant needs a full seat.",
  "extAccess.empty": "No extension units are composed into this installation.",
  "extAccess.version": "Version {version}",
  "extAccess.openUnit": "Open the {name} page",
  "extAccess.noPage":
    "{name} is composed into the API, but this build of the app has no page for it — the app is probably older than the server.",
  "extAccess.brings.heading": "What this unit brings",
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
  "extAccess.matrixCaption": "Who may do what with {object}",
  "extAccess.cell": "Allow {role} to {action} {object}",
  "extAccess.versionSkew":
    "Someone else changed this role while you were looking at it, so your change was not applied. The grants above are now the current ones — make the change again if you still want it.",
  "extAccess.systemRole": "Built-in role",
  "extAccess.nobodyReads":
    "No role holds read on {object}, so every member sees an empty screen where this extension should be. Grant read to at least one role below.",
  "users.empty": "No users yet.",
  "users.adminOnly": "Managing users is available to admins only.",
  "users.inviteTitle": "Invite a user",
  "users.teamsLabel": "Teams",
  "users.noTeamsYet": "No teams yet.",
  "users.teamMembersLabel": "Who is in this team",
  "users.teamMembersAdminOnly": "Membership is visible to admins only.",
  "users.teamNobodyToAdd": "No users to add yet.",
  "users.teamsTitle": "Teams",
  "users.teamsSub":
    "Named groups you can share records with. Membership alone still grants no access for most roles — the exception is Team Lead: adding one to a team gives them that team's records to read and work, without a share being arranged.",
  "users.teamsAdminOnly": "Managing teams is available to admins only.",
  "users.deactivated": "{name} deactivated",
  "users.reactivated": "{name} reactivated",
  "users.roleSaved": "Role changed for {name}",
  "users.teamArchived": "Team “{name}” archived",
  "users.teamRestored": "Team “{name}” restored",
  "users.archiveTeam": "Archive team {name}",
  "users.newTeamLabel": "New team",
  "users.newTeamOpen": "New team",
  "users.teamNameLabel": "Team name",
  "users.newTeamPlaceholder": "e.g. DACH Sales",
  "users.createTeam": "Create team",
  "users.access.title": "What this user sees",
  "users.access.identity":
    "Reads every person, company, lead and deal in the organization.",
  "users.access.writesAll": "Edits every record.",
  "users.access.writesTeam":
    "Edits their own records and those of the teams {teams}.",
  "users.access.writesTeamNone":
    "Edits only their own records — not on any team yet.",
  "users.access.writesOwn": "Edits only their own records.",
  "users.access.none": "no access",
  "users.access.read": "read",
  "users.access.write": "write",
  "users.access.delete": "delete",
  "users.access.object.person": "People",
  "users.access.object.organization": "Companies",
  "users.access.object.lead": "Leads",
  "users.access.object.deal": "Deals",
  "users.access.object.project": "Projects",
  "users.access.mask": "{field} is withheld {when}.",
  "users.access.maskAlways": "always",
  "users.access.maskOutside": "on records they may not edit",
  "users.inviteSub":
    "Add someone to this installation and pick the role they start with.",
  "users.membersTitle": "Users",
  "users.membersSub":
    "Everyone who holds a seat here, deactivated accounts included.",
  "users.memberCount_one": "{count} user",
  "users.memberCount_other": "{count} users",
  "users.teamMemberCount_one": "{count} member",
  "users.teamMemberCount_other": "{count} members",
  "users.emailLabel": "New user's email",
  "users.nameLabel": "New user's full name",
  "users.emailPlaceholder": "name@company.com",
  "users.namePlaceholder": "Full name",
  "users.deactivateConfirmTitle": "Deactivate {name}?",
  "users.deactivateConfirmBody":
    "They'll be signed out everywhere and their agent passports revoked immediately. You can reactivate them later, but they'll need to sign in again.",
  "users.deactivateAgentConfirmBody":
    "This is the organization's agent identity. It signs in nowhere and no person loses access. Scheduled extension jobs keep running: each one acts as the job it is, and captures under the authority of the member whose connection produced the record.",
  "users.agentSeat": "Agent",
  "users.agentSeatRole": "Acts under a passport, not a role",
  "users.roleLabel": "Role for the new user",
  "users.inviteOpen": "Invite a user",
  "users.invite": "Invite",
  "users.setRole": "Set role…",
  "users.setRoleFor": "Set role for {name}",
  "users.rowActions": "Actions for {name}",
  "users.rolesHeld": "Holds {roles}. Choosing one replaces them all",
  "users.deactivate": "Deactivate",
  "users.reactivate": "Reactivate",
  "users.status.active": "Active",
  "users.status.invited": "Invited",
  "users.status.deactivated": "Deactivated",
  "users.status.suspended": "Suspended",
  "users.link.action": "Get set-password link",
  "users.link.title": "Set-password link for {name}",
  "users.link.pending": "Creating the link…",
  // Two sentences, no dash (VOICE-RULE-5).
  "users.link.body":
    "Send this link to the member over a channel you trust. It works once and is shown only now. Close this and you can create a new one from their row.",
  "users.link.urlLabel": "Set-password link",
  "users.link.copy": "Copy link",
  "users.link.copied": "Copied",
  "users.link.copyFailed":
    "Could not copy automatically. Select the link and copy it.",
  "users.link.expires": "Expires {when}.",
  "users.link.failed":
    "The member was created, but the link could not be. They cannot sign in until you send them one.",
  "users.link.offline":
    "Could not reach the server. Check your connection and try again.",
  "users.link.retry": "Try again",
  "users.link.done": "Done",
  "settings.companyTitle": "What Margince knows about your company",
  "settings.companyReadOnly":
    "Read-only view — changing the company profile needs an organization write.",
  "settings.companySub":
    "Keep the shared business context behind drafting, offers, search, and governed agents accurate. Every statement stays tied to who supplied it and where it came from.",
  "settings.companyTrust":
    "Confirmed knowledge only — website text never becomes instructions.",
  "settings.companyConfirmed": "confirmed statements",
  "settings.companyMark": "Company logo",
  "settings.companyMarkPresent":
    "Shown here and as the main brand at the top of the sidebar.",
  "settings.companyMarkNone":
    "No logo yet, so the initials stand in. A website read can fill this in, or add one here.",
  "settings.companyMarkAdd": "Add a logo",
  "settings.companyMarkReplace": "Replace",
  "settings.companyMarkRemove": "Remove",
  "settings.companyMarkPick": "Company logo",
  "settings.companyMarkHint":
    "Best results: SVG or a transparent PNG around 800 × 240 px (up to 4:1), under 5 MB. JPEG, GIF, WebP and ICO also work. Your logo keeps its proportions.",
  "settings.companyMarkEmpty": "Drop your logo here, or choose a file",
  "settings.companyWebsite": "Public company website",
  "settings.companyWebsiteHint":
    "The public site every website read starts from.",
  "settings.companySourceTitle": "Where we read it from",
  "settings.companyRefreshRow": "Read the website again",
  "settings.companyRefreshHint":
    "We fetch your public pages and propose changes. Nothing reaches the profile until you review and apply them.",
  "settings.companyEdit": "Edit",
  "settings.companyEditField": "Edit {field}",
  "settings.companyWebsiteRequired": "Add a company website before refreshing.",
  "settings.companyRefresh": "Refresh from website",
  "settings.companyEssentials": "The three essentials",
  "settings.companyPositioning": "Positioning, buyers, and sales motion",
  "settings.companyIdentity": "Identity and legal details",
  "settings.companySave": "Save company context",
  "settings.companySaved": "Saved",
  "settings.companyRefreshUnreadable":
    "We lost track of this website read. Start the refresh again.",
  "settings.companyRefreshStale":
    "The website proposal changed. Review the refreshed comparison before applying it.",
  "settings.companyRefreshReview": "Website comparison",
  "settings.companyRefreshReady": "Review what changed",
  "settings.companyRefreshReading": "Reading and grounding your site…",
  "settings.companyCoverage": "page coverage",
  "settings.companyResolveAll":
    "Choose an outcome for every human-held conflict.",
  "settings.companyApplyRefresh": "Apply selected changes",
  "settings.companySelectChange": "Select the {field} change",
  "settings.companyClass.new": "New",
  "settings.companyClass.machine_change": "Website changed",
  "settings.companyClass.human_conflict": "Needs your decision",
  "settings.companyClass.unchanged": "Unchanged",
  "settings.companyResolution.keep_current": "Keep current",
  "settings.companyResolution.accept_proposal": "Accept website",
  "settings.companyResolution.useValueFor": "Value to keep for {field}",
  "settings.companyResolution.use_value": "Use my edited value",
  "settings.companyManualKicker": "Private, manual setup",
  "settings.companyManualTitle": "Tell Margince the essentials",
  "settings.companyManualSub":
    "Website reading is not enabled for this rollout. These three answers are enough to create useful company context, with no model call and no external request.",
  "settings.companyCreateWorkspace": "Create company context",
  "product.title": "Products",
  "product.settingsSub": "Rate-card entries that offer lines snapshot from.",
  "product.readOnly": "Read-only view — you may not change products.",
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
  "product.active": "Active",
  "product.activeFilter": "Active only",
  "product.activeFilterAll": "All",
  "product.inactive": "Inactive",
  "product.archived": "Archived",

  "template.title": "Offer templates",
  "template.settingsSub": "Branded DE/EN PDF layouts for offers.",
  "template.readOnly": "Read-only view — you may not change offer templates.",
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
    "The governed surface a passport can call — same inventory an MCP client sees.",
  "tools.egress": "reaches out",
  "tools.scopeAll": "All passports",
  "tools.inventory": "All {count} tools",
  "tools.scopeLabel": "Scope to a passport",
  "tools.scopedTo": "Reachable by {label}",
  "tools.unreachable": "scope not granted",

  "aiusage.title": "AI usage & budget",
  "aiusage.withheld":
    "Only an operator can see what the AI runtime spent. The figures cover the whole installation, so they are not shown more widely.",
  "aiusage.sub":
    "Your own bill, made visible — per task and tier, token-denominated.",
  "aiusage.budget": "{spent} of {budget} tokens · {pct}%",
  "aiusage.budgetMeter": "Monthly token budget used",
  "aiusage.band.normal": "normal",
  "aiusage.band.degraded": "economy mode",
  "aiusage.band.queued": "budget reached — background AI queued",
  "aiusage.band.unknown": "unknown budget state",
  "aiusage.col.task": "Task",
  "aiusage.col.tier": "Tier",
  "aiusage.col.calls": "Calls",
  "aiusage.col.cached": "Cached",
  "aiusage.col.tokensIn": "Tokens in",
  "aiusage.col.tokensOut": "Tokens out",
  "aiusage.col.cost": "Est. cost",
  "aiusage.costNote": "Costs are estimates at configured rates.",
  "aiusage.monthLabel": "Month",
  "aiusage.spendLabel": "Spend by task",
  "aiusage.days.show": "Show days",
  "aiusage.empty": "No AI calls in this window.",
  "aiusage.prevMonth": "Previous month",
  "aiusage.nextMonth": "Next month",

  "aibanner.degraded": "AI running in economy mode.",
  "aibanner.queued": "AI budget reached — background AI is queued.",
  "aibanner.unknown": "AI budget status is not recognized.",
  "aibanner.link": "View usage",
  "aibanner.dismiss": "Dismiss",

  "aicalls.title": "AI call trace",
  "aicalls.withheld":
    "Only an operator can read the per-call trace. It records every model call the installation made, so it is not shown more widely.",
  "aiHealth.withheld":
    "Only an operator can read whether the model lanes are answering. It is the installation's own wiring, not a fact about your work.",
  "aicalls.sub":
    "Every model call — routing identity, tokens, retries, captured payload.",
  "aicalls.col.detail": "Detail",
  "aicalls.expandCall": "Show the attempt trail for {task} at {when}",
  "aicalls.col.when": "When",
  "aicalls.col.task": "Task",
  "aicalls.col.model": "Model",
  "aicalls.col.tokens": "Tokens",
  "aicalls.col.latency": "Latency",
  "aicalls.ms": "{value} ms",
  "aicalls.badge.cacheHit": "cache hit",
  "aicalls.badge.degraded": "degraded",
  "aicalls.badge.retries": "retry ×{count}",
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
    "Payload capture is off — set ai.capture_payloads: true in margince.yaml to record request/response content.",
  "aicalls.payload.none": "No payload captured for this call.",

  "aiexport.button": "Export as cert scenario",
  "aiexport.title": "Export run as certification scenario",
  "aiexport.nameLabel": "Scenario name",
  "aiexport.checklist":
    "Secrets were stripped at capture. Personal data was NOT — review and remove PII, then replace sanitized_by before committing this file to the corpus.",
  "aiexport.copy": "Copy YAML",
  "aiexport.copied": "Copied",
  "aiexport.download": "Download .yaml",
  "aiexport.copyFailed": "Copy failed — use the preview or download instead.",
  "aiexport.close": "Close",
  "aiexport.previewLabel": "Scenario preview",
  "aiexport.responseLabel": "Model response",

  "countdown.daysHours": "{days}d {hours}h",
  "countdown.hoursMinutes": "{hours}h {minutes}m",
  "countdown.minutesSeconds": "{minutes}m {seconds}s",
  "countdown.expired": "Expired",

  "installationSettings.orgTitle": "Installation",
  "installationSettings.orgSub":
    "What this installation is called, and the zone every reporting period is computed in.",
  "installationSettings.currencyTitle": "Currency",
  "installationSettings.currencySub":
    "The one currency every roll-up converts amounts to.",
  "installationSettings.name": "Organization name",
  "installationSettings.nameHint":
    "Shown wherever the product names your organization.",
  "installationSettings.timezone": "Reporting timezone",
  "installationSettings.timezoneHint":
    "IANA zone name (for example Europe/Berlin). Your organization's own clock: report period boundaries are computed in it, and every record date — close dates, invoice days, timeline headings — is shown in it, so a date reads the same for the whole team. Separate from your own display timezone.",
  "installationSettings.fiscalYearStart": "Financial year starts",
  "installationSettings.fiscalYearStartHint":
    "The month your business year begins. Reports group by this year and quarter — a year that does not start in January is labelled with both calendar years it spans, like FY2026/27. Changing it re-labels every report at once, and a saved report view filtered on a period will then ask for different months.",
  "installationSettings.baseCurrency": "Base currency",
  "installationSettings.baseCurrencyHint":
    "ISO-4217 code every amount converts to for roll-ups. Changeable until the first amount converts against it.",
  "installationSettings.baseCurrencyLocked":
    "Locked: amounts have already converted against this currency, so changing it would re-mean every roll-up built on them.",
  "installationSettings.baseLanguage": "Base language",
  "installationSettings.baseLanguageHint":
    "The language AI writes in when the whole team reads what it wrote. Your own display language is separate, and replies to customers still follow the language of the conversation.",
  "installationSettings.readOnly":
    "Only an admin or ops can change these settings.",
  "installationSettings.edit": "Edit",
  "installationSettings.editField": "Edit {field}",
  "installationSettings.save": "Save",
  "signInMethods.title": "Sign-in methods",
  "signInMethods.sub":
    "Which ways people may sign in here. The list is what this deployment holds credentials for, so an admin can turn one off but cannot add one.",
  "signInMethods.password": "Email and password",
  "signInMethods.passwordAlways":
    "Always available. Every account can be reached this way, which is what makes the others safe to switch off.",
  "signInMethods.passwordReason":
    "Password sign-in cannot be turned off. It is the method that keeps an installation enterable.",
  "signInMethods.providerHint":
    "Offer this provider on the login screen. Turning it off stops sign-ins already in progress, and existing sessions are unaffected.",
  "signInMethods.noneConfigured":
    "This deployment has no external provider configured, so there is nothing to offer besides a password.",
  "oauthApp.google.title": "Google app",
  "oauthApp.google.sub":
    "Mailboxes are connected, and people sign in with Google, through a Google OAuth app you own. Your organization’s own credentials are used rather than ours.",
  "oauthApp.google.absent":
    "No app is available from any source. Gmail and Calendar cannot be connected, and Google sign-in cannot be offered.",
  "oauthApp.google.redirectSub":
    "Register every URI below on the OAuth client in the Google console. A missing one fails at the consent screen with redirect_uri_mismatch, which does not say which URI was wrong.",
  "oauthApp.google.clientIdPlaceholder":
    "000000000000-xxxx.apps.googleusercontent.com",
  "oauthApp.google.removeConfirmTitle": "Remove the Google app?",
  "oauthApp.google.removeConfirmBody":
    "The client secret cannot be read back, so removing it means re-entering both halves from the Google console. Gmail and Calendar connections are made through this app. Microsoft and IMAP mailboxes are not affected. First-run setup will ask for one again.",
  "oauthApp.microsoft.title": "Microsoft app",
  "oauthApp.microsoft.sub":
    "Outlook mailboxes and calendars are connected, and people sign in with Microsoft, through an Entra app registration you own. Your organization’s own credentials are used rather than ours.",
  "oauthApp.microsoft.absent":
    "No app is available from any source. Outlook mail and calendar cannot be connected, and Microsoft sign-in cannot be offered.",
  "oauthApp.microsoft.redirectSub":
    "Register every URI below under Authentication on the Entra app registration, as a Web platform. A missing one fails at the consent screen with AADSTS50011, which does not say which URI was wrong.",
  "oauthApp.microsoft.clientIdPlaceholder":
    "00000000-0000-0000-0000-000000000000",
  "oauthApp.microsoft.removeConfirmTitle": "Remove the Microsoft app?",
  "oauthApp.microsoft.removeConfirmBody":
    "The client secret cannot be read back, so removing it means re-entering both halves from the Entra portal. Outlook mail and calendar connections are made through this app. Google and IMAP mailboxes are not affected. First-run setup will ask for one again.",
  "oauthApp.configured": "In use: {clientId}",
  "oauthApp.fromEnvironment":
    "In use from this deployment’s configuration: {clientId}. Storing an app here replaces it for as long as one is stored.",
  "oauthApp.pinnedToDirectory": "Pinned to directory {tenant}.",
  "oauthApp.replaceHint":
    "Entering a new pair replaces the stored one. Connections already made keep working until they are reconnected.",
  "oauthApp.store": "Store app",
  "oauthApp.replace": "Replace app",
  "oauthApp.remove": "Remove app",
  "oauthApp.redirectCopied": "Copied",
  "oauthApp.redirectCopy": "Copy {purpose} URI",
  "oauthApp.redirect.mailbox_connect": "Mailbox",
  "oauthApp.redirect.calendar_connect": "Calendar",
  "oauthApp.redirect.sign_in": "Sign-in",
  "oauthApp.redirectTitle": "Authorized redirect URIs",
  "oauthApp.clientId": "Client ID",
  "oauthApp.clientSecret": "Client secret",
  "oauthApp.tenant": "Directory (tenant) ID",
  "oauthApp.tenantHint":
    "Optional. Pins the app to one Entra directory: only its members may connect a mailbox, and Microsoft sign-in runs on it. Leave it empty to let any organization connect; sign-in then waits for the server to name your directories.",
  "oauthApp.tenantPlaceholder": "00000000-0000-0000-0000-000000000000",
  "firstRun.continue": "Continue",
  "firstRun.ai.title": "Choose a model provider",
  "firstRun.ai.sub":
    "Margince has no AI of its own. It thinks through your vendor account, and you can change any of this later under Settings → AI.",
  "firstRun.ai.provider": "Provider",
  "firstRun.ai.key": "API key",
  "firstRun.ai.keyHint": "Sealed in the key vault, never shown again.",
  "firstRun.ai.chatModel": "Model",
  "firstRun.ai.modelHint":
    "A starting point. Any model your provider serves will do.",
  "firstRun.ai.embedModel": "Embedding model",
  // Which vendor this installation's text is sent to. Admin/ops only, on both
  // verbs â see the ai_routing RBAC object.
  "aiSettings.sub": "Where this installation's text goes, and what it costs.",
  "aiSettings.tabs": "Which part of the AI settings is open",
  "aiSettings.tab.routing": "Routing",
  "aiSettings.tab.providers": "Providers",
  "aiSettings.tab.automations": "Automations",
  "aiSettings.tab.usage": "Usage",
  "aiSettings.tab.logs": "Logs",
  "aiSettings.withheld": "Not yours to see",
  "aiSettings.unread": "Could not be read",
  "aiSettings.pending": "Reading…",
  "aiSettings.spend.label": "Spend this month",
  "aiSettings.spend.value": "{spent} of {budget} tokens",
  "aiSettings.spend.estimated": "≈ {amount} estimated",
  "aiSettings.providers.label": "Providers",
  "aiSettings.providers.value": "{count} keyed",
  "aiSettings.providers.missing": "{count} bound with no key",
  "aiSettings.providers.lastCall": "last call {elapsed}",
  "aiSettings.discardTitle": "Leave the routing edits behind?",
  "aiSettings.discardBody":
    "The bindings you changed have not been saved. Leaving this tab discards them.",
  "aiSettings.discard": "Discard",
  "elapsed.justNow": "just now",
  "elapsed.minutes": "{minutes} min ago",
  "elapsed.hours": "{hours} h ago",
  "elapsed.days": "{days} d ago",
  "aiRouting.lane.local_small": "Bulk classifying, on your own hardware",
  "aiRouting.lane.cheap_cloud": "Everyday work — enrichment, summaries, triage",
  "aiRouting.lane.premium": "Anything a customer will read",
  "aiRouting.lane.frontier": "The hardest reasoning, used sparingly",
  "aiRouting.lane.local_large": "Heavier work that must not leave your hosts",
  "aiRouting.lane.embeddings": "Search and retrieval across your records",
  "aiRouting.lanes.title": "Routing lanes",
  "aiRouting.lanes.sub":
    "Cheapest first. A task picks a lane; the lane picks the model.",
  "aiRouting.priceSheet": "Price sheet",
  "aiRouting.provider.label": "Provider",
  "aiRouting.change": "Change",
  "aiRouting.done": "Done",
  "aiRouting.noKey": "no key",
  "aiRouting.unpriced": "unpriced",
  "aiRouting.effect":
    "Saved bindings reach every process within a minute, without a restart.",
  "aiProviderKeys.title": "Model provider keys",
  "aiProviderKeys.sub":
    "The credentials this installation calls each model vendor with. A key is sealed in the key vault and never shown again — replace it if you need to change it.",
  "aiProviderKeys.keyless": "no key needed",
  "aiProviderKeys.field": "API key",
  "aiProviderKeys.save": "Save key",
  "aiProviderKeys.adminOnly":
    "Only an admin or ops can change a provider credential.",
  "aiProviderKeys.configured": "configured",
  "aiProviderKeys.absent": "not set",
  "aiProviderKeys.configuredHint":
    "Sealed in the key vault. It cannot be read back — paste a new one to replace it. It may also arrive as {envVar}.",
  "aiProviderKeys.absentHint":
    "This vendor has no credential, so a model bound to it cannot be called. It may also arrive as {envVar}.",
  "aiProviderKeys.addPlaceholder": "Paste the API key",
  "aiProviderKeys.replacePlaceholder": "Paste a new key to replace",
  "aiProviderKeys.add": "Add",
  "aiProviderKeys.replace": "Replace",
  "aiProviderKeys.removeConfirmTitle": "Remove the {provider} key?",
  "aiProviderKeys.removeConfirmBody":
    "The credential is deleted from the key vault and cannot be recovered — it is never readable, so there is no copy to restore. Every AI lane bound to this vendor stops until a new key is pasted in.",
  "aiProviderKeys.withheld":
    "Only an operator who may change the model binding can see which vendors hold a key.",
  "aiProviderKeys.remove": "Remove",
  "aiRouting.withheld":
    "Only an operator who may change the model binding can see which models this installation uses.",
  "aiRouting.title": "Model routing",
  "aiRouting.sheetAsOf":
    "Model lists are the price sheet as of {date}. Any newer id your provider serves works too — type it.",
  "aiRouting.sheetUnknown":
    "Model lists come from the price sheet, which is not yours to read. Any id your provider serves works — type it.",
  "aiRouting.sub":
    "Which model serves each tier. Changes take effect without a restart, and every process picks them up within a minute.",
  "aiRouting.unbound":
    "This installation has no models bound, so its AI features are off. A deployment declares its first binding under seeds.ai_routing in margince.yaml.",
  "aiRouting.profile.card": "Deployment profile",
  "aiRouting.profile.label": "Location",
  "aiRouting.profile.help":
    "Where inference runs. Sovereign means zero egress: only models on your own hosts, refused at save time rather than at the first call.",
  "aiRouting.profile.eu_hosted": "EU-hosted",
  "aiRouting.profile.sovereign": "Sovereign (no egress)",
  "aiRouting.profile.cloud_frontier": "Cloud frontier",
  "aiRouting.dimensions.label": "Vector width",
  "aiRouting.dimensions.help":
    "Leave blank for the provider's default. A value outside 1 to 2000 is refused.",
  "aiRouting.baseUrl.placeholder": "https://openrouter.ai/api",
  "aiRouting.baseUrl.label": "Host",
  "aiRouting.baseUrl.help":
    "The vendor's host root, with no version segment. The adapter adds /v1. Required for openai_compatible, which has no default of its own.",
  "aiRouting.models.noKey":
    "Showing the price sheet only — this vendor holds no key, so it cannot be asked what it serves. Any id it serves still works: type it.",
  "aiRouting.models.noEndpoint":
    "Showing the price sheet only — fill in the host above and this vendor can be asked what it serves. Any id it serves still works: type it.",
  "aiRouting.models.profileForbids":
    "Showing the price sheet only — this deployment profile does not permit reaching this vendor.",
  "aiRouting.models.notPublished":
    "Showing the price sheet only — this vendor publishes no model list.",
  "aiRouting.models.unreachable":
    "Showing the price sheet only — this vendor did not answer. Any id it serves still works: type it.",
  "aiRouting.model.label": "Model",
  "aiRouting.model.help":
    "The models listed are the ones this installation can price, per million tokens in → out. Any other id your provider serves works too — type it.",
  "aiRouting.save": "Save routing",
  "aiRouting.saving": "Saving the binding…",
  "aiRouting.saved": "Routing saved. Every process is now serving it.",
  "aiRouting.adminOnly": "Only an admin or ops can change model routing.",
  "autonomy.title": "What answers itself",
  "autonomy.sub":
    "Small corrections you have been confirming by hand. Switch one on and it applies as soon as it comes up, with the change and an Undo waiting on your day.",
  "autonomy.noneDecidedYet":
    "You have not decided any of these yet. What reaches this list depends on the records you own and the work your team routes to you, so a seat with neither stays empty. The switches still decide what happens when something appears.",
  "autonomy.noRecord": "You have not decided one of these yet.",
  "autonomy.record":
    "So far: {clean} approved as proposed, {edited} after an edit, {rejected} turned down.",
  "autonomy.kind.close_date_correction.label": "Close dates",
  "autonomy.kind.close_date_correction.help":
    "A deal's close date moved by what was said on a call or written in a mail.",
  "autonomy.kind.org_name_promotion.label": "Company names",
  "autonomy.kind.org_name_promotion.help":
    "A company recorded under its domain takes the name its own website gives.",
  "autonomy.kind.lifecycle_change.label": "Lifecycle stages",
  "autonomy.kind.lifecycle_change.help":
    "A company moves stage on what has happened with it. This one can also change who sees the account and which automations run.",
  "captureSettings.title": "Enrichment",
  "captureSettings.sub":
    "How captured companies and contacts are enriched after they are created.",
  "captureSettings.autoEnrich.label": "Auto-enrich captured companies",
  "captureSettings.autoEnrich.help":
    "When on, each new company created from captured mail gets an automatic web dossier — its site is read and its profile filled in. Runs under a daily limit.",
  "captureSettings.signatureEnrich.label": "Read contact details from mail",
  "captureSettings.signatureEnrich.help":
    "When on, Margince reads what a contact states under their own name in mail they sent you — in a signature, and on a business card attached to it. A title, a phone number, an address, a company. It happens within minutes of the mail arriving. Nothing is inferred: a detail the mail does not state is not written. This is the organization's default; a mailbox that set its own switch keeps it.",
  "captureSettings.adminOnly": "Only an admin or ops can change this.",

  "ownDomains.companyTitle": "Company domains",
  "captureExclusions.title": "Keep out of capture",
  "captureExclusions.sub":
    "Addresses and domains whose messages never enter the CRM. Your own rules bind only the mailboxes you connected; the organization's rules bind everyone.",
  "captureExclusions.notRetroactive":
    "Takes effect from the next message. Messages already captured stay.",
  "captureExclusions.current": "Rules in effect",
  "captureExclusions.empty": "No exclusions.",
  "ownerIdentities.title": "Your other addresses",
  "ownerIdentities.sub":
    "Addresses that are also you: a send-as alias, a private domain you read, an address you forward from. Mail between your own addresses is not correspondence with anybody, so it is not captured and never becomes a contact.",
  "ownerIdentities.add": "Add address",
  "ownerIdentities.addLabel": "Declare another address as your own",
  "ownerIdentities.addDescription":
    "Yours alone. A colleague never sees what you list here.",
  "ownerIdentities.current": "Declared",
  "ownerIdentities.notRetroactive":
    "Applies from the next message on. Mail already captured stays, and a contact already made from an alias stays until you merge or remove it.",
  "ownerIdentities.empty": "You have declared no other addresses.",
  "ownerIdentities.remove": "Withdraw this address",
  "ownerIdentities.added": "Address added.",
  "ownerIdentities.confirm": "Add",
  "ownerIdentities.kindLabel": "What are you declaring?",
  "ownerIdentities.kind.address": "One address",
  "ownerIdentities.learned.deliveredTo":
    "Found automatically — mail addressed here delivers into your connected mailbox. Remove it if that is not you.",
  "ownerIdentities.learned.provider":
    "Reported by your mail provider as one of your own addresses. Remove it if that is not you.",
  "ownerIdentities.kind.domain": "A whole domain",
  "ownerIdentities.valueLabel": "Address or domain",
  "ownerIdentities.addressPlaceholder": "you@example.com",
  "ownerIdentities.domainPlaceholder": "example.com",
  "captureExclusions.scope.user": "Only me",
  "captureExclusions.scope.workspace": "Whole organization",
  "captureExclusions.kind.address": "Address",
  "captureExclusions.kind.domain": "Domain",
  "captureExclusions.scopeLabel": "Applies to",
  "captureExclusions.kindLabel": "Kind",
  "captureExclusions.addLabel": "Exclude an address or a domain",
  "captureExclusions.placeholder.address": "name@example.com",
  "captureExclusions.placeholder.domain": "example.com",
  "captureExclusions.add": "Exclude",
  "captureExclusions.addOpen": "New exclusion",
  "captureExclusions.remove": "Capture {value} again",
  "ownDomains.title": "Own email domains",
  "ownDomains.sub":
    "The domains that belong to this company. When colleagues write to each other, that message is not stored. Not even for you.",
  "ownDomains.curatedTitle": "Managed here",
  "ownDomains.irreversible":
    "Adding a domain takes effect from the next message. Removing it later resumes capture from that point on. Mail skipped while it was registered is never offered again by any mailbox. Mail already captured stays.",
  "ownDomains.fromCompany": "From the company profile. Change them there:",
  "ownDomains.openCompany": "Open the company profile",
  "ownDomains.empty":
    "No further domains registered. Add one if your company also writes from another domain.",
  "ownDomains.confirmed": "confirmed",
  "ownDomains.candidate": "seen on a connected mailbox, not confirmed yet",
  "ownDomains.add": "Add",
  "ownDomains.addOpen": "Add a domain",
  "ownDomains.addLabel": "Add an own domain",
  "ownDomains.placeholder": "example.com",
  "ownDomains.remove": "Remove {domain}",

  "webhooks.title": "Webhooks",
  "webhooks.readOnly":
    "Read-only view — only an admin or ops can change subscriptions.",
  "webhooks.sub":
    "Outbound subscriptions that receive signed HTTP POSTs for chosen events.",
  "webhooks.new": "New subscription",
  "webhooks.notConfigured":
    "Outbound webhooks are not enabled on this deployment — a signing key must be configured first.",
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
    "Archiving stops all delivery for this subscription. This can't be undone.",
  "webhooks.rotate": "Rotate secret",
  "webhooks.rotateConfirm.title": "Rotate signing secret?",
  "webhooks.rotateConfirm.body":
    "Confirming invalidates the current secret immediately and then reveals the new secret once. Copy it and update your receiver as soon as rotation completes.",
  "webhooks.secret.title": "Signing secret",
  "webhooks.secret.warning":
    "This secret is shown once and can't be retrieved again. Store it now — deliveries are signed with it.",
  "webhooks.secret.copy": "Copy",
  "webhooks.secret.copied": "Copied",
  "webhooks.secret.copyFailed":
    "Couldn't copy automatically — select and copy the secret manually.",
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
  "webhooks.deliveries.column.resolved": "Resolved / next retry",
  "webhooks.deliveries.status.pending": "Pending",
  "webhooks.deliveries.status.delivered": "Delivered",
  "webhooks.deliveries.status.retrying": "Retrying",
  "webhooks.deliveries.status.dead_lettered": "Dead-lettered",
  "webhooks.deliveries.status.visibility_revoked":
    "Stopped — no longer visible",
  "webhooks.deliveries.replay": "Replay",
  "webhooks.deliveries.replayConfirm.title": "Replay this delivery?",
  "webhooks.deliveries.replayConfirm.body":
    "Re-attempts delivery now, signed with the current secret and a fresh timestamp. It doesn't wait for the next scheduled retry.",
  "reindexbanner.needed": "Reindex needed",
  "reindexbanner.link": "Review in settings",

  "embedreindex.title": "Search index",
  "embedreindex.sub":
    "The embedding store's reindex status — admin/ops only, including viewing it.",
  "embedreindex.withheld":
    "Only an admin or ops can see the search index. Rebuilding it spends tokens for the whole installation, so its status is not shown more widely.",
  "embedreindex.statusLabel": "Index status",
  "embedreindex.reindexLabel": "Reindex what changed",
  "embedreindex.reindexHelp":
    "Re-embeds only the records whose text changed since the last pass.",
  "embedreindex.rebuildLabel": "Rebuild the whole index",
  "embedreindex.rebuildHelp":
    "Re-embeds every record from scratch. Use it when a run is stuck or the embedding model changed.",
  "embedreindex.statusIdle": "Up to date",
  "embedreindex.statusNeeded": "Reindex needed",
  "embedreindex.statusReembedding": "Reindexing…",
  "embedreindex.lastProgress": "Last progress {duration} ago",
  "embedreindex.entitiesPending": "{count} entities pending",
  "embedreindex.workspacePending": "{count} pending",
  "embedreindex.reviewCta": "Review & reindex",
  "embedreindex.rebuildCta": "Rebuild index",
  "embedreindex.confirmTitle": "Start the reindex",
  "embedreindex.rebuildTitle": "Rebuild the search index",
  "embedreindex.confirmCta": "Start reindex",
  "embedreindex.rebuildConfirmCta": "Rebuild now",
  "embedreindex.previewLoading": "Estimating scope…",
  "embedreindex.estimateEntities": "Entities to (re)embed:",
  "embedreindex.estimateTokens": "Estimated AI tokens:",
  "embedreindex.estimateCost": "Estimated cost:",
  "embedreindex.estimateQualityHeuristic":
    "Heuristic estimate — a cold work-shape floor, not observed spend.",
  "embedreindex.utilizationTitle": "Budget impact",
  "embedreindex.impact.normal": "normal",
  "embedreindex.impact.degraded": "would enter economy mode",
  "embedreindex.impact.queued": "would be queued",

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
    "spends enrichment credits — each purchase still asks you first",
  "consent.ceiling":
    "Never more than your own permissions. You can disconnect any time in Settings → Agents.",
  "consent.pickOne": "Pick at least one, or deny.",
  "consent.offline":
    "It will stay connected without asking again, renewing access until you revoke it.",
  "consent.approve": "Authorize",
  "consent.deny": "Deny access",
  "consent.reentering": "Reconnecting…",
  "consent.backToApp": "Back to Margince",
  "consent.staleTitle": "This request has expired",
  // No {client}: this card renders without the consent-request fetch, so the
  // client's name is not available to name here.
  "consent.staleBody":
    "The connection request is no longer valid. Go back to the app you were connecting and start again — reloading this page will not help.",
  "consent.invalidTitle": "This connection request could not be completed",
  "consent.invalidBody":
    "This installation will not authorize the request as it stands — the app may no longer be registered here. Go back to the app you were connecting and start again.",
  "person.thin.title": "What we know so far",
  "person.thin.known":
    "We have {what} for {name}, but nobody here has a recorded exchange with them yet.",
  "person.thin.remediation.capture":
    "Connect the mailbox that writes to them, and this page fills itself in — every field with the source it came from.",
  "person.thin.remediation.employer":
    "Add their employer and Margince can read that company's site for their role.",
  "person.thin.logFirst": "Log the first interaction",
  "person.enriched.title": "What Margince read",
  "person.enriched.sub":
    "Each value with the text it was read from. Correct one and the correction stands.",
  "person.enriched.field.title": "Title",
  "person.enriched.field.phone": "Phone",
  "person.enriched.field.role": "Role",
  "person.enriched.field.linkedin": "LinkedIn",
  "person.enriched.field.org_name": "Company",
  "person.enriched.field.address": "Address",
  "person.enriched.field.website": "Website",
  "person.enriched.readFrom": "Read from {source} on {when}",
  "person.enriched.undo": "Undo",
  "person.enriched.replaced": "Replaced “{was}”, which was older.",
  "person.enriched.correctedByYou": "Corrected by you",
  "person.enriched.confirmed": "Confirmed",
  "person.enriched.correct": "Correct",
  "person.enriched.confirm": "That is right",
  "person.enriched.save": "Save the correction",
  "person.enriched.cancel": "Cancel",
  "person.graph.loading": "Reading the network around this contact…",
  "person.graph.routeDirect": "{name} already corresponds with them.",
  "person.graph.routeVia":
    "{name} corresponds with {through} at the same company.",
  "person.graph.noRoute":
    "Nobody here corresponds with them or with anyone at their company yet.",
  "person.graph.noDirect": "Nobody here has corresponded with them.",
  // Names the column beside the graph, for a reader who lands in it from the
  // landmark list rather than by scrolling to it.
  "person.graph.sideColumn": "Introductions and moments",
  "person.graph.recordWorksWith": "Record: works with {name}",
  "person.graph.noEdge": "No recorded correspondence with {name}.",
  "person.graph.withColleague": "with {name}",
  "person.graph.withContact": "with this contact",
  "person.graph.counts":
    "{total} interactions in 90 days · {inbound} in, {outbound} out",
  "person.graph.untitledMessage": "Untitled",
  "person.graph.countsOnly":
    "Counts only — the messages themselves stay on the timeline.",
  "person.intro.routesTitle": "Ways in",
  "person.graph.droppedNote": "{count} more not shown.",
  "person.graph.withheldDirect": "Some colleagues are not shown.",
  "person.graph.withheldAccount":
    "Some contacts at this company are not shown.",
  "person.intro.askFirstName": "Ask {name} for an intro",
  "person.intro.leadEyebrow": "Recommended route",
  "person.intro.leadRouteBadge": "Strong route",
  "person.intro.heroDirect": "knows them directly",
  "person.intro.heroIndirect": "reaches them through {through}",
  "person.intro.factReciprocal": "Reciprocal",
  "person.intro.factOneSided": "One-sided",
  "person.intro.factDirect": "Direct relationship",
  "person.intro.factIndirect": "Through a colleague",
  "person.intro.factReceipts": "{count} visible receipts",
  "person.intro.stripPath": "Best path",
  "person.intro.stripDirect": "Direct relationship",
  "person.intro.stripVia": "Through {through}",
  "person.intro.stripNoPath": "Nobody here reaches them yet",
  "person.intro.stripWhyNow": "Why now",
  "person.intro.stripWhyNowSub": "The most recent change on this relationship",
  "person.intro.stripNoMoment": "Nothing new",
  "person.intro.stripNoMomentSub": "No recent change on this relationship",
  "person.intro.stripHandoff": "Request status",
  "person.intro.handoffNotStarted": "Not started",
  "person.intro.handoffNotStartedSub": "No introduction has been asked for",
  "person.intro.handoffOwner": "{name} owns the next move",
  "person.intro.ownerColleague": "your colleague",
  "person.intro.ownerYou": "you",
  "person.intro.ownerNobody": "nobody",
  "person.intro.relayTitle": "Introduction status",
  "person.intro.relaySubOpen": "Where the handoff has got to.",
  "person.intro.relaySubNone": "No request is open.",
  "person.intro.stepRoute": "Choose route",
  "person.intro.stepRoutePick": "pick who to ask",
  "person.intro.stepRequest": "Request",
  "person.intro.stepNotSent": "not sent",
  "person.intro.stepAwaitingAnswer": "waiting on your colleague",
  "person.intro.stepIntroduction": "Introduction",
  "person.intro.stepNameDrop": "Name used",
  "person.intro.stepWaiting": "waiting",
  "person.intro.stepRecorded": "recorded",
  "person.intro.stepReply": "Reply",
  "person.intro.stepObserved": "observed from activity",
  "person.intro.stepDone": "Done",
  "person.intro.stepCurrent": "Now",
  "person.intro.stepPending": "Later",
  "person.intro.laneOurs": "Our team",
  "person.intro.laneTheirs": "Their company",
  "person.intro.lanePeers": "Who they talk to",
  "person.intro.laneTarget": "Target",
  "person.intro.useThisRoute": "Use this route",
  "person.intro.mapRegion": "Who can reach this contact, and through whom",
  "person.intro.edgeDirect": "{name} corresponds with them directly",
  "person.intro.edgeAccount": "works with {name}",
  "person.intro.routesSub":
    "Best first. Pick the one you can actually use — the second is here because the first is not always available.",
  "person.intro.best": "Best",
  "person.intro.alternative": "Alternative",
  "person.intro.evidenceTwoWay_one":
    "{total} two-way exchange in 90 days · {when}",
  "person.intro.evidenceTwoWay_other":
    "{total} two-way exchanges in 90 days · {when}",
  "person.intro.evidenceOneSided_one":
    "{total} interaction in 90 days, one-sided · {when}",
  "person.intro.evidenceOneSided_other":
    "{total} interactions in 90 days, one-sided · {when}",
  "person.intro.whenToday": "last contact today",
  "person.intro.whenYesterday": "last contact yesterday",
  "person.intro.whenDays": "last contact {days} days ago",
  "person.intro.whenNever": "no recent contact",
  "person.intro.askTitle": "Ask for an introduction to {name}",
  "person.intro.cancel": "Cancel",
  "person.intro.askAction": "Request introduction",
  "person.intro.askFailed": "The ask could not be recorded.",
  "person.intro.reasonLabel": "Why you are asking",
  "person.intro.reasonHint":
    "Your colleague reads this, not the contact. Say what makes the introduction worth making.",
  "person.intro.valueLabel": "What is in it for them",
  "person.intro.valueHint":
    "The reason the contact would want this conversation.",
  "person.intro.noteLabel": "Note your colleague can forward",
  "person.intro.noteHint":
    "The only part the contact reads. Write it so it can be pasted as it stands.",
  "person.intro.nameDropAsk": "Ask permission to mention their name",
  "person.intro.fallbackLegend": "If they say no",
  "person.intro.fallbackNone": "Nothing further",
  "person.intro.fallbackNoneHelp":
    "The ask closes and you decide what to do next.",
  "person.intro.fallbackNameDrop": "Ask to use their name instead",
  "person.intro.fallbackNameDropHelp":
    "You would reach out yourself, mentioning them.",
  "person.intro.fallbackNextRoute": "Try the next route",
  "person.intro.fallbackNextRouteHelp":
    "Move on to the next colleague on the list.",
  "person.intro.decideTitle": "An introduction to {name}",
  "person.intro.decideLegend": "Your answer",
  "person.intro.decideAction": "Record answer",
  "person.intro.decideFailed": "The answer could not be recorded.",
  "person.intro.decideReasonLabel": "Anything to add",
  "person.intro.decideReasonHint": "Your colleague sees this as you write it.",
  "person.intro.noteByModel": "Drafted by Margince",
  "person.intro.nameDropRequested":
    "They also asked whether they may mention your name.",
  "person.intro.answerAccept": "I will introduce you",
  "person.intro.answerAcceptHelp": "You make the introduction yourself.",
  "person.intro.answerNameDrop": "You may use my name",
  "person.intro.answerNameDropHelp":
    "They reach out themselves and mention you. This is not an introduction, and nothing records it as one.",
  "person.intro.answerSuggest": "Ask someone else",
  "person.intro.answerSuggestHelp": "Name the colleague better placed to help.",
  "person.intro.answerDecline": "Not this time",
  "person.intro.answerDeclineHelp": "The ask closes. Say why if it helps.",
  "person.intro.asksTitle": "Introductions",
  "person.intro.asksSub": "The asks you are part of, newest first.",
  "person.intro.answerAction": "Answer",
  "person.intro.completeIntroducedAction": "Mark introduced",
  "person.intro.completeNameDroppedAction": "Mark name used",
  "person.intro.completeFailed": "The outcome could not be recorded.",
  "person.intro.withdrawAction": "Withdraw",
  "person.intro.withdrawFailed": "The ask could not be withdrawn.",
  "person.intro.stateRequested": "Waiting on your colleague",
  "person.intro.stateAccepted": "They will introduce you",
  "person.intro.stateNameDropApproved": "You may use their name",
  "person.intro.stateSuggestOther": "They suggested someone else",
  "person.intro.stateDeclined": "Declined",
  "person.intro.stateIntroduced": "Introduced",
  "person.intro.stateNameDropped": "Name used",
  "person.intro.stateReplied": "They replied",
  "person.intro.stateExpired": "No answer in time",
  "person.intro.stateCancelled": "Withdrawn",
  "person.intro.alreadyRequested": "Already asked",
  "person.intro.declined": "Declined before",
  "person.intro.unavailable": "Not available",
  "person.network.momentsTitle": "What changed lately",
  "person.network.momentsSub":
    "Movements in this relationship, from the messages themselves.",
  "person.network.noMoments": "Nothing has moved in this relationship lately.",
  "person.change.repliedAfterGap": "They replied after {days} quiet days.",
  "person.change.wentQuiet": "Nothing has happened for {days} days.",
  "person.change.warmed": "The relationship moved from {from} to {to}.",
  "person.change.cooled": "The relationship fell from {from} to {to}.",
  "person.band.none": "no contact",
  "person.band.weak": "weak",
  "person.band.moderate": "moderate",
  "person.band.strong": "strong",
  "person.pulse.title": "Relationship",
  "person.pulse.warmestIs": "{name} has the warmest relationship here.",
  "person.pulse.nobodyYet":
    "Nobody here has a recorded exchange with them yet.",
  "person.pulse.lastInbound": "They last wrote",
  "person.pulse.lastOutbound": "We last wrote",
  "person.pulse.neverInbound": "never",
  "person.pulse.neverOutbound": "never",
  "person.pulse.why": "How this is computed",
  "person.pulse.arithmetic":
    "Score {score}/100 = 100 x recency {recency} x frequency {frequency} x reciprocity {reciprocity}. Computed at read from captured cadence, never stored.",
  "person.identity.title": "Identity",
  "person.identity.emailDead": "Bounces — mail to this address is not arriving",
  "person.identity.email": "Email",
  "person.identity.phone": "Phone",
  "person.identity.currentRole": "Current role",
  "person.identity.buyingRole": "Buying role",
  "person.career.title": "Former roles",
  "person.consent.title": "Outbound guard",
  "person.consent.allowed": "Allowed: {purposes}",
  "person.consent.noneGranted":
    "No purpose is granted, so outbound stays blocked.",
  "person.consent.blocked": "Blocked: {purposes}",
  "person.network.title": "Who here knows them",
  "person.network.twoWay": "{count} two-way exchanges in 90 days",
  "person.network.oneSided": "{count} interactions in 90 days, one-sided",
  "person.network.replied": "replied {when}",

  // The person record page V2 (ADR-0096). The strip, rail and card words are
  // split by SLOT rather than by sentence, because the same word means
  // different things in different slots: "Never" under a direction is an
  // absence of correspondence, "None" under a meeting is an absence of a
  // booking, and German renders them differently.
  "person.page.loading": "Loading…",
  "person.page.notOpened": "This contact could not be opened.",
  "person.page.buyingRole": "Buying role",
  "person.page.owner": "Owner",
  "person.page.ownerUnassigned": "Unassigned",
  "person.page.linkedin": "LinkedIn",
  // Beside the editable address, not instead of it: the row holds a value to
  // correct AND a place to go, and the verb names the second so neither reads
  // as the other.
  "person.page.openProfile": "Open profile",
  // The rail's own details grid: the contact's own fields, at a glance above
  // the six relationship sections below it.
  "person.rail.detailsTitle": "Details",
  "person.rail.archivedReadOnly":
    "This contact is archived. Restore them to change anything here.",
  // Fired when an employment row's version could not be read back before a
  // write — the row is not saved unpinned, so the reader is told to reload
  // rather than left to think the edit landed.
  "person.rail.employmentVersionUnresolved":
    "This row's current version could not be read back to save against. Reload and try again.",
  // The employers section: every employment edge this person holds, current
  // one first — a person can work at more than one company at once.
  "person.rail.employmentTitle": "Companies",
  "person.rail.noEmployment": "No employment on record.",
  "person.rail.addEmployment": "Add company",
  "person.rail.employer": "Employer",
  "person.rail.allOrgsConnected":
    "Every match is already connected to this person.",
  "person.rail.isCurrentEmployer": "This is their current employer",
  "person.rail.markEnded": "Mark as ended",
  "person.rail.removeEmploymentTitle": "Remove this company connection?",
  "person.rail.removeEmploymentBody":
    "The link to {org} and the history hanging off it disappear, and this cannot be undone. {org} itself stays. If they simply left, mark it ended instead.",
  "person.timeline.empty": "Nothing has been logged with them yet.",
  "person.deals.empty": "They are not recorded on any deal.",
  "person.deals.untitled": "Untitled deal",
  "person.deals.noStage": "No stage yet",
  "person.meetings.next": "Next meeting",
  "person.meetings.past": "Meetings so far",
  "person.meetings.noneBooked": "Nothing is booked with them.",
  "person.meetings.noneLogged": "No meeting with them has been logged.",
  "person.meetings.untitled": "Untitled meeting",
  "person.meetings.participants": "In the room",
  "person.documents.empty": "No file has been filed against this contact.",
  "person.research.empty": "Nothing has been researched about them yet.",
  "person.research.fields": "Enrichment evidence",
  "person.research.fieldsEmpty": "No enriched field carries evidence yet.",
  "person.research.capturedBy": "Captured by",
  "person.action.email": "Email",
  // The lead verb when the record leaves the transport open: either the
  // composer will ask which way to send, or there is no way to send at all.
  "person.action.write": "Write",
  // The lead verb when a chat channel is the ONLY way to reach them. The
  // provider is named by the transport directory, so an extension unit this
  // build has never heard of still reads as itself.
  "person.action.messageOn": "Message on {transport}",
  // Why the lead verb is refused, in two sentences that are never merged: no
  // way to reach them, and consent that says not to.
  "person.action.noTransport": "No address, and no conversation to reply to.",
  "person.action.consentRefused":
    "No purpose currently permits writing to them.",
  "person.action.call": "Call",
  "person.action.meetings": "See meetings",
  "person.action.addTask": "Add task",
  "person.action.research": "Research",

  "person.strip.lastInbound": "Last inbound",
  "person.strip.lastOutbound": "Last outbound",
  "person.strip.reciprocity": "Reciprocity",
  "person.strip.inOut": "{inbound} in · {outbound} out",
  "person.strip.nextMeeting": "Next meeting",
  "person.strip.never": "Never",
  "person.strip.today": "Today",
  "person.strip.yesterday": "Yesterday",
  "person.strip.days": "{count} days",
  "person.strip.noOpenDeal": "No open deal",
  "person.strip.noMeeting": "None",
  "person.consent.allowedWord": "Allowed",
  "person.consent.blockedWord": "Blocked",
  "person.consent.unknownWord": "Unknown",

  "person.moment.rule.meeting_prep": "Meeting soon",
  "person.moment.rule.re_engaged": "They came back",
  "person.moment.rule.job_change": "They moved on",
  "person.moment.rule.overdue_promise": "Promise overdue",
  "person.moment.rule.gone_quiet": "Gone quiet",
  "person.moment.rule.open_promise": "You owe them",
  "person.moment.rule.role_change": "Role changed",
  "person.moment.rule.public_signal": "Said in public",
  "person.moment.rule.missing_next_step": "Nothing scheduled",
  "person.moment.rule.thin_relationship": "One thread only",
  "person.moment.rule.nothing_needed": "Nothing needed",
  "person.moment.evidence.activity": "From an exchange",
  "person.moment.evidence.task": "From a task",
  "person.moment.evidence.relationship_change": "From a change on the record",
  "person.today.source_one": "{count} source",
  "person.today.source_other": "{count} sources",
  "person.today.updated": "Updated {when}",
  "person.today.freshToday": "today",
  "person.today.freshYesterday": "yesterday",
  "person.today.freshDaysAgo": "{count} days ago",

  "person.brief.title": "Relationship brief",
  "person.brief.reading": "Reading the relationship…",
  "person.brief.empty":
    "Nothing has been captured yet that this brief could be written from.",
  "person.brief.sourceActivity": "Conversation",
  "person.brief.sourceDeal": "Deal notes",

  "person.matters.title": "What matters to {name}",
  "person.matters.priorities": "Priorities",
  "person.matters.objections": "Objections",
  "person.matters.successCriteria": "Success criteria",
  "person.matters.absent": "Nothing captured yet",

  "person.commercial.title": "Open deal & buying role",
  "person.commercial.withheld":
    "You do not have access to this person's deals.",
  "person.commercial.noDeal": "No open deal.",
  "person.commercial.closes": "closes {date}",
  "person.commercial.committee": "Buying committee",
  "person.commercial.openDeal": "Open deal",

  "person.loops.title": "Commitments & open loops",
  "person.loops.empty":
    "Nothing has been promised or asked in the captured conversations.",
  "person.loops.ours": "You",
  "person.loops.question": "Open question",
  "person.loops.overdue_one": "overdue {count} day",
  "person.loops.overdue_other": "overdue {count} days",
  "person.loops.overdueUnderDay": "overdue by less than a day",
  "person.loops.due": "due {when}",
  "person.loops.dueToday": "today",
  "person.loops.dueTomorrow": "tomorrow",
  "person.loops.dueInDays": "in {count} days",
  "person.loops.waiting": "waiting",
  "person.loops.open": "open",
  "person.loops.atLeast": "at least {count}",

  "person.memory.title": "Conversation memory",
  "person.memory.empty": "Nothing captured on this channel yet.",
  "person.memory.all": "All",
  "person.memory.email": "Email",
  "person.memory.meetings": "Meetings",
  "person.memory.calls": "Calls",
  "person.memory.notes": "Notes",
  "person.memory.channelEmail": "Email",
  "person.memory.channelMeeting": "Meeting",
  "person.memory.channelCall": "Call",
  "person.memory.channelNote": "Note",
  "person.memory.channelMessage": "Message",
  "person.memory.channelTask": "Task",
  "person.memory.replied": "Replied",
  "person.memory.unanswered": "Unanswered",

  "person.rail.reviewFirst": "Review first",
  "person.rail.blocked": "Blocked",
  "person.rail.pulseTitle": "Relationship pulse",
  "person.rail.explain": "Explain",
  "person.rail.direction": "Direction",
  "person.rail.twoWay": "Two-way",
  "person.rail.oneSided": "One-sided",
  "person.rail.lastReply": "Last reply",
  "person.rail.coverage": "Coverage",
  "person.rail.colleagues_one": "{count} colleague",
  "person.rail.colleagues_other": "{count} colleagues",
  "person.rail.trend": "Trend",
  "person.rail.noInbound": "No inbound",
  "person.rail.cooling": "Cooling",
  "person.rail.warming": "Warming",
  "person.rail.overall": "Overall",
  "person.rail.thin": "Thin",
  "person.rail.atRisk": "At risk",
  "person.rail.strong": "Strong",
  "person.rail.whoKnows": "Who knows {name}",
  "person.rail.nobodyYet": "Nobody here has corresponded with them yet.",
  "person.rail.exchanges": "{count} exchanges",
  "person.rail.signals": "Signals & risks",
  "person.rail.noSignals": "Nothing stands out on this relationship.",
  "person.rail.noReplyDays": "No reply for {count} days",
  "person.rail.repliedDaysAgo_one": "Replied {count} day ago",
  "person.rail.repliedDaysAgo_other": "Replied {count} days ago",
  "person.rail.singleThreaded": "Single-threaded on this deal",
  "person.rail.noMeetingBooked": "No next meeting booked",
  "person.rail.consentTitle": "Consent & channels",
  "person.rail.email": "Email",
  "person.rail.phone": "Phone",
  "person.rail.noEmailAddress": "No address on file",
  "person.rail.noPhoneNumber": "No number on file",
  "person.rail.channelNotDeliverable": "Not deliverable",
  "person.rail.recentActivity": "Recent activity",
  "person.rail.nothingCaptured": "Nothing captured yet.",
  "person.rail.viewAllActivity": "View all activity",
  "person.drawer.close": "Close",
  "person.composer.title": "Draft follow-up · {name}",
  "person.composer.to": "To",
  "person.composer.transport": "How to send",
  "person.composer.transportEmail": "Email",
  "person.composer.toConversation": "Continues your {transport} conversation",
  "person.composer.subject": "Subject",
  "person.composer.bcc": "Bcc",
  "person.composer.bccPlaceholder":
    "One address per line — they receive the message and no other recipient sees them",
  "person.composer.body": "Message",
  "richtext.bold": "Bold",
  "richtext.italic": "Italic",
  "richtext.bulletList": "Bulleted list",
  "richtext.numberList": "Numbered list",
  "richtext.link": "Link",
  "richtext.linkPrompt": "Web address for this link (leave empty to remove it)",
  "person.composer.drafting": "Writing a draft…",
  "person.composer.why": "Why this draft",
  "person.composer.consentUnknown":
    "No consent decision is recorded for this channel.",
  "person.composer.sendNote":
    "Pressing send delivers this message from your own mailbox.",
  "person.composer.purpose": "Consent purpose",
  "person.composer.blockedLead":
    "This message cannot go out under this purpose.",
  "person.composer.blockedRewrite":
    "A message sent under another purpose has to BE that kind of message — relabelling this one does not make it so.",
  "person.composer.blockedRecordConsent":
    "If you have a basis for writing, record the consent decision on their contact record.",
  "person.composer.consentPickPurpose":
    "Choose what this message is for — consent is decided per purpose.",
  "person.composer.intent": "What should it be about?",
  "person.composer.intentHint":
    "Optional — e.g. ask for a date in the first week of September",
  "person.composer.draftWithAi": "Draft with AI",
  "person.composer.intentAgenda": "propose an agenda for the upcoming meeting",
  "person.composer.intentReply": "reply to their last message",
  "person.composer.intentCommitment": "deliver what we promised them",
  "person.composer.intentFollowUp": "follow up — it has gone quiet",
  "person.composer.send": "Send",
  "person.composer.sending": "Sending…",
  "person.composer.sent": "Sent",
  "person.composer.aiDisclosure": "AI-assisted draft · review every word",
  "person.research.title": "Deep research · {name}",
  "person.research.publicOnly": "Public sources only",
  "person.research.running": "Reading public sources…",
  // Deep research and bought contact data are two capabilities, and this
  // sentence sits directly under the bought data on the same drawer. It names
  // the RESEARCH provider for that reason: "data provider" is the licensed
  // contact-data vocabulary (provider.profile.*), and using it here told a
  // reader nothing was connected while eight purchased claims sat above it.
  "person.research.notConnected":
    "No research provider is connected, so no public source has been read for them. That is separate from any bought contact data above — Margince never researches a person on its own authority, and deep research needs a licensed provider that carries the lawful basis for it.",
  "person.research.staged":
    "Research is staged. Nothing changes {name}'s record until you review and save.",
  "person.research.stats": "{sources} sources read · {claims} cited claims",
  "person.research.dismiss": "Dismiss",
  "person.research.discard": "Discard",
  "person.research.save": "Review & save {count} claims",
  "person.research.evidenceOrOmit":
    "AI-assisted · evidence-or-omit · public information only",
  "person.meeting.title": "Meeting brief",
  "person.meeting.brief": "Brief me",
  "person.meeting.empty": "There is nothing recorded for this meeting yet.",
  "person.meeting.loading": "Assembling the brief…",
  "person.meeting.assembledNow": "Assembled just now, from the latest data",
  "person.meeting.header": "At a glance",
  "person.meeting.what_changed": "Since you last spoke",
  "person.meeting.goal": "Goal for this meeting",
  "person.meeting.attendees": "Attendees",
  "person.meeting.commitments": "Open commitments",
  "person.meeting.deal_state": "Where the deal stands",
  "person.meeting.risks": "Risks and watch-outs",
  "person.meeting.talking_points": "Suggested talking points",
  "person.meeting.company_context": "When you last met",
  "person.meeting.objective": "The outcome to earn",
  "person.meeting.openWith": "Open with",
  "person.meeting.arc": "The account arc",
  "person.meeting.arcSub": "Only the moments that change today's conversation.",
  "person.meeting.close": "Close the meeting",
  "person.meeting.advance.minimum": "Minimum advance",
  "person.meeting.advance.best": "Best advance",
  "person.meeting.advance.fallback": "Fallback",
  "person.meeting.unknowns": "What the record does not show",
  "person.meeting.likelyAsks": "What they are likely to ask",
  "person.meeting.beReady": "Be ready for this",
  "person.meeting.say": "Say",
  "person.meeting.show": "Show",
  "person.meeting.avoid": "Avoid",
  "person.meeting.scenarios": "If the meeting goes another way",
  "person.meeting.relevance.high": "Likely",
  "person.meeting.relevance.medium": "Possible",
  "person.meeting.relevance.low": "Less likely",
  "person.meeting.coach.title": "Coach the rep on one thing",
  "person.meeting.coach.eyebrow": "Manager view",
  "person.meeting.coach.listenFor": "Listen for",
  "person.meeting.coach.watchFor": "Watch for",
  "person.meeting.coach.interveneIf": "Step in only if",
  "person.meeting.coach.paths": "How this meeting can go",
  "person.meeting.background": "Background and sources",
  "person.meeting.omittedSource": "Not in this brief",
  "person.meeting.preparedFor": "Prepared for {name}",
  "person.meeting.preparedForAt": "Prepared for {name} · {org}",

  "today.source.suggestions": "the advice",

  // The licensed data provider (ADR-0101). Two surfaces share this
  // vocabulary — the Settings card and the person page — so a state reads
  // the same wherever it appears.
  "provider.title": "Contact data",
  "provider.readOnly":
    "Read-only view — connecting a provider spends money, so it is an admin or ops action.",
  "provider.sub":
    "Buy verified contact details for the people in your CRM. You pay the provider in credits; what you spend here is shown below.",
  "provider.notConfigured":
    "No data provider is available in this installation. Nothing is being bought and nothing can be.",
  "provider.status.connected": "Connected",
  "provider.status.disconnected": "Not connected",
  "provider.status.validating": "Checking the key…",
  "provider.status.invalidCredentials": "The key was refused",
  "provider.status.insufficientCredits": "Out of credits",
  "provider.status.rateLimited": "Rate limited",
  "provider.status.providerError": "The provider is having trouble",
  "provider.connect": "Connect",
  "provider.reconnect": "Replace the key",
  "provider.apiKey": "API key",
  "provider.apiKeyHint":
    "Sealed as soon as it is verified. It is never shown again, and never leaves this installation except to the provider.",
  "provider.apiKeyStored": "Replace the API key",
  "provider.apiKeyReplaceHint":
    "A key is stored and in use. It cannot be shown again, so this box stays empty — paste a new one only if you are replacing it.",
  "provider.apiKeyReplacePlaceholder":
    "Paste a new key to replace the stored one",
  "provider.connectConfirm.title": "Connect this data provider?",
  "provider.connectConfirm.body":
    "The key is checked against the provider before anything is saved. Once connected, enriching a contact spends your credits.",
  "provider.disconnect": "Disconnect",
  "provider.disconnectConfirm.title": "Disconnect the provider?",
  "provider.disconnectConfirm.body":
    "New lookups stop immediately and the key is destroyed. Data already bought stays on your records — disconnecting is not deleting.",
  "provider.deleteData": "Delete bought data",
  "provider.deleteDataConfirm.title":
    "Delete everything bought from this provider?",
  "provider.deleteDataConfirm.body":
    "Every value this provider supplied is removed from every contact. What you spent stays in your records; the data does not. This cannot be undone.",
  "provider.deleteDataConfirm.typed": "Type the provider's name to confirm",
  "provider.automaticLookup": "Look up contacts automatically",
  "provider.automaticLookupHint":
    "Every contact is looked up once for whatever the connection selects that the provider charges nothing for — typically the professional profile link, the current role and employer, and the work history. Email addresses and mobile numbers are never bought this way: those cost credits and stay a decision you make per contact.",
  "provider.automaticLookupJurisdiction":
    "Switch this off if your contacts fall under a law that forbids trading personal data, Vietnam's among them. The button on each contact still works, which keeps the decision with the person making it.",
  "provider.buyable": "Allow buying {category}",
  "provider.buyableHint_one":
    "Switching this on buys nothing. It puts a button on each contact, priced at {credits} credit, so somebody can buy this detail for one person at a time.",
  "provider.buyableHint_other":
    "Switching this on buys nothing. It puts a button on each contact, priced at {credits} credits, so somebody can buy this detail for one person at a time.",
  "provider.buyableNeeds":
    "The provider looks for this only alongside the {prerequisite}, so it cannot be bought on its own. Allow that one first.",
  "provider.backlog": "Still to look up",
  "provider.backlogRemaining_one": "{count} contact",
  "provider.backlogRemaining_other": "{count} contacts",
  "provider.backlogWorking":
    "Contacts that were already here when the provider was connected are being looked up a few at a time.",
  "provider.backlogPaused":
    "Nothing is being looked up right now: automatic lookups are off, the day's limit is spent, or the provider is not usable.",
  "provider.credits": "Credits left with the provider",
  "provider.credits.none": "The provider has not told us a balance yet.",
  "provider.credits.notConnected":
    "Connect a key to see what credit you have with the provider.",
  "provider.constraints": "Limits in force",
  "provider.spend": "What we have used",
  "provider.spend.hint":
    "Our own record of what enrichment consumed. Not the provider's invoice — the same credits can be spent through their app, so the two figures differ legitimately.",
  "provider.spend.thisMonth": "This month",
  "provider.spend.month": "Month",
  "provider.spend.pool": "Pool",
  "provider.spend.chargedHead": "Credits",
  // "Held" rather than "unknown": the column holds credits whose outcome the
  // provider never reported, which is what a human reconciles against the
  // invoice. Kept out of the Credits column on purpose.
  "provider.spend.heldHead": "Held",
  "provider.spend.runsHead": "Lookups",
  "provider.spend.none": "Nothing has been bought yet.",

  // The person page's section. The three "nothing here" states are three
  // different sentences on purpose: only one of them is something the
  // reader can act on.
  "provider.profile.title": "Bought contact data",
  "provider.profile.notConnected":
    "No data provider is connected, so nothing has been bought.",
  "provider.profile.notEligible":
    "This contact is not eligible — they have objected, or the record is archived.",
  "provider.profile.nothingToLookUp":
    "There is nothing to look this contact up by. Add their LinkedIn URL, or the company they work for, and the lookup can run.",
  "provider.profile.neverRun": "Nobody has looked this contact up yet.",
  "provider.profile.queued": "Queued",
  "provider.profile.inProgress": "Looking them up…",
  "provider.profile.working": "Asking {provider}. This takes up to a minute.",
  "provider.profile.landing": "Answer received. Putting it on the record.",
  "provider.profile.completed": "Found",
  "provider.profile.noMatch": "The provider had nothing for this contact.",
  "provider.profile.stale":
    "Bought earlier. The provider is no longer connected, so this cannot be refreshed.",
  "provider.profile.invalidCredentials":
    "The provider refused our key, so this lookup could not run.",
  "provider.profile.insufficientCredits":
    "Not bought: the credit budget for this month is spent.",
  "provider.profile.rateLimited":
    "Not bought: the provider asked us to slow down.",
  "provider.profile.providerError":
    "The last lookup did not get through. Try again, or check the provider's card in Settings if it keeps happening.",
  "provider.profile.submissionUnknown":
    "We never learned how this lookup ended. It may have been charged for.",
  "provider.profile.claimsUnwritten":
    "Paid for, but the details never reached this record. Nobody has to hunt for them — this is the gap.",
  "provider.profile.enrichNow": "Look this contact up · free",
  "provider.profile.recheck": "Check again · free",
  "provider.profile.lookingUp": "Asking the provider. This takes a moment.",
  "provider.profile.emptyTitle": "Nothing bought for this contact yet",
  "provider.profile.emptyBody":
    "A lookup asks {provider} about this contact, for whichever details this connection is set to buy. It spends {provider} credits, and what comes back sits here beside the record rather than overwriting anything a colleague typed.",
  "provider.profile.emails": "Email addresses",
  "provider.profile.emailType.provider": "{type}, as the provider labelled it",
  "provider.profile.emailType.requested":
    "{type}, because that is what we asked for",
  "provider.profile.mobiles": "Mobile numbers",
  "provider.profile.confidence": "{percent}% confidence",
  "provider.profile.linkedin": "LinkedIn",
  "provider.profile.employment": "Current role",
  "provider.profile.jobHistory": "Earlier roles",
  "provider.profile.location": "Location",
  "provider.profile.departments": "Departments",
  "provider.profile.seniorities": "Seniority",
  "provider.profile.notRequested": "Never asked for: {categories}.",
  // The receipt. Without it a lookup that returned one detail out of six read
  // exactly like one that returned all six, and nothing on the page said when
  // the answer arrived.
  // The price rides the button because the decision IS the spend.
  "provider.profile.buy_one": "Buy {category} · {credits} credit",
  "provider.profile.buy_other": "Buy {category} · {credits} credits",
  "provider.profile.buyRebuys":
    "The price includes the {categories} again: the provider will not look for this without it, and it charges for whatever it sends back.",
  "provider.freeTier.hint":
    "LinkedIn profile, current role and work history cost no credits. Leave this on: every new contact gets them without anybody deciding.",
  "provider.pricedTier.hint":
    "Never bought automatically. Somebody presses a button on one contact, and the price is on the button.",
  "provider.profile.receiptAt": "Looked up {at}.",
  "provider.profile.receipt":
    "Looked up {at} · asked for {asked} details, got {answered} back.",
  "provider.profile.noAnswer": "Asked for, none found: {categories}.",
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
  "filters.joinAll": "ALL \u00b7 AND",
  "filters.joinAny": "ANY \u00b7 OR",
  "filters.joinLabel": "How this group joins its clauses",
  "filters.removeGroup": "Remove group",
  "filters.addGroup": "Add group",
  "filters.addClause": "Add clause",
  "filters.emptyGroup":
    "No clauses yet \u2014 an empty group matches nothing, so add one.",
  "filters.field": "Field",
  "filters.choosePlaceholder": "Choose a field",
  "filters.customBadge": "custom field",
  "filters.operator": "Operator",
  "filters.value": "Value",
  "filters.values": "Values",
  "filters.addValue": "Add a value",
  "filters.removeClause": "Remove the {field} clause",
  "filters.existsLabel": "Whether the field has a value",
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
  "filters.title": "Filters & views",
  "filters.subtitle":
    "Build a filter, watch what it selects, and save it as a view.",
  "filters.objectLabel": "Which records to filter",
  "filters.tab.contacts": "People",
  "filters.tab.companies": "Companies",
  "filters.tab.deals": "Deals",
  "filters.builderTitle": "Filter",
  "filters.dynamic": "Dynamic \u2014 recomputes on every event",
  "filters.matchContacts": "{count} contacts match",
  "filters.matchCompanies": "{count} companies match",
  "filters.matchDeals": "{count} deals match",
  "filters.noFilterYet": "Add a clause to see what it selects",
  // The count when the server was asked and did not answer. It must not fall
  // back to noFilterYet: a reader looking at a finished clause would read a
  // refusal as their own unfinished work. Three words, because this sits in a
  // header row beside two buttons; the reason and the retry go in the results
  // card below, which is the only row wide enough for a sentence.
  "filters.countUnavailable": "Count unavailable",
  "filters.loadingVocabulary": "Loading the fields you can filter on\u2026",
  "filters.noFields": "No filterable fields for this record type.",
  "filters.resultsTitle": "Matching records",
  "filters.resultsCaption":
    "The first page of matches — enough to check the filter, not the whole selection.",
  "filters.noMatches": "No records match this filter.",
  "filters.loadView": "Load a saved filter",
  "filters.pickRecord": "Choose one",
  "filters.loadingRecords": "Loading choices…",
  "filters.pickValue": "Choose a value",
  "filters.exportCsv": "Export CSV",
  "filters.exportJson": "Export JSON",

  // The release gate (src/screens/releaseskew.tsx). It renders instead of the
  // app when this bundle and the api come from different releases, so the copy
  // has two readers at once: the person who just wants in, and the operator who
  // has to fix it. The first sentence is for the first, the last for the second.
  "release.skewTitle": "This installation is part-way through an update",
  "release.skewBody":
    "The app in your browser and the server behind it come from different releases, so nothing here works reliably. Reload to get the current version. If this message stays, tell whoever operates this installation: every part of it has to run the same release.",
  "release.skewVersions": "app {app} · server {server}",
  "release.skewReload": "Reload",

  // The queue behind "send later". Every sentence here is addressed to ONE
  // person: the list is the sender's own, so there is no "a teammate scheduled
  // this" reading to write for.
  //
  // The verb is "withdraw", not "delete" or "cancel". Nothing was transmitted
  // and nothing reaches the timeline, so there is no send to cancel and no
  // record to delete — the rep is taking a message back before it goes, and
  // "withdraw" is the only one of the three that says so.
  "nav.scheduled": "Scheduled messages",
  "sched.sub":
    "Messages you have written that have not gone out yet. Only you can see them.",
  "sched.empty": "You have not scheduled a message yet.",
  "sched.group.held": "Stopped, waiting on you",
  "sched.group.heldEmpty": "Nothing has been stopped.",
  "sched.group.waiting": "Waiting to send",
  "sched.group.waitingEmpty": "Nothing is waiting to send.",
  "sched.group.closed": "No longer waiting",
  "sched.group.closedEmpty": "Nothing has gone out or been withdrawn yet.",
  "sched.status.scheduled": "Waiting",
  // "Released" is the wire's word for the step between waiting and sent: the
  // activity and the delivery exist and the provider has not answered yet. To a
  // rep that is a message on its way out, and there is nothing left to do to it.
  "sched.status.released": "Going out",
  "sched.status.sent": "Sent",
  "sched.status.cancelled": "Withdrawn",
  "sched.status.held": "Stopped",
  "sched.held.consentWithdrawn":
    "A recipient withdrew their consent after you scheduled this. It will not send until you write to them under a purpose they have agreed to.",
  "sched.held.senderInactive":
    "Your seat or your mailbox changed after you scheduled this, so it cannot be sent as you.",
  "sched.held.missedWindow":
    "Its moment passed while nothing was running, and it is now too late to be the message you wrote. Move it or withdraw it.",
  "sched.held.timerExhausted":
    "The job that wakes this message ran out of attempts. Move it to a new moment to try again.",
  "sched.held.sendRefused":
    "A check refused this message when it came due. Nothing was sent.",
  "sched.inZone": "in {zone}",
  "sched.recipientsUnknown": "No recipient on this message",
  "sched.recipientsMore": "{first} and {count} more",
  "sched.move": "Change moment",
  "sched.moveTo": "New moment for “{subject}”",
  "sched.moveSave": "Move it",
  "sched.moveCancel": "Leave it",
  "sched.withdraw": "Withdraw",
  "sched.withdrawTitle": "Withdraw this message?",
  "sched.withdrawBody":
    "“{subject}” will not be sent, and nothing will reach the timeline. Writing it again means composing it from scratch.",
  "sched.withdrawConfirm": "Withdraw it",
  "sched.skew":
    "This list is out of date: the message you acted on had already gone, been withdrawn, or been moved somewhere else. Read the list again.",
  "sched.reload": "Read it again",
  // Projects — the body of work a deal is about. It starts during the deal,
  // in the initiative phase, and outlives close-won; this namespace is every
  // word the list, the page and the deal form's picker say about one.
  "nav.projects": "Projects",
  "unit.projects": "projects",
  "companyProjects.title": "Projects",
  "companyProjects.empty":
    "A project is the body of work a deal is about. This company appears here once it is on one — as the client, a partner, or a subcontractor.",
  "projectCompanies.title": "Companies",
  "projectCompanies.empty":
    "A project is work several companies do together — the client, and any partner or subcontractor delivering it.",
  "projectCompanies.attach": "Attach company",
  "projectCompanies.detachTitle": "Take this company off?",
  "projectCompanies.searchLabel": "Search companies by name",
  "personProjects.title": "Projects",
  "personProjects.empty":
    "This contact appears here once they are on a delivery — as a sponsor, a contact, or whoever else is working it.",
  "projectRole.customer": "Customer",
  "projectRole.partner": "Partner",
  "projectRole.subcontractor": "Subcontractor",
  "personRole.sponsor": "Sponsor",
  "personRole.projectLead": "Project lead",
  "personRole.deliveryLead": "Delivery lead",
  "personRole.expert": "Subject-matter expert",
  "personRole.user": "User",
  "projectLinks.new": "New project",
  "projectLinks.attach": "Attach project",
  "projectLinks.move": "Move to another project",
  "projectLinks.detach": "Detach",
  "projectLinks.detachConfirm": "Detach it",
  "projectLinks.detachNamed": "Detach {name}",
  "projectLinks.roleLabel": "As",
  "projectLinks.detachTitle": "Detach this project?",
  "projectLinks.detachBody":
    "{name} stays as it is. Only its link to this record ends — nothing is deleted.",
  "projectLinks.emptyTitle": "No projects yet",
  "projectLinks.searchLabel": "Search projects by name or key",
  "project.name": "Project name",
  "project.keyMinted":
    "Margince gives each project a short key. Write [{key}] in an email subject and the mail is filed under this project.",
  "project.company": "Company",
  "project.owner": "Owner",
  "project.ownerKeep": "Keep current owner",
  "project.ownerMe": "Me",
  "project.ownerUnassign": "Unassign",
  "project.assignOwner": "Assign to a colleague",
  "project.assignOwnerTitle": "Assign to a colleague",
  "project.assignOwnerSearch": "Search colleagues",
  "project.assignOwnerNoneSelected": "Pick a colleague first",
  // The dialog's own confirm verb. The trigger and the title both read "Assign
  // to a colleague"; the button says what pressing it does, and a button
  // repeating the heading it sits under reads as chrome rather than a verb.
  "project.assignOwnerConfirm": "Assign",
  "project.assignOwnerDone": "Assigned to {name}",
  "project.description": "Description",
  "project.targetEnd": "Target end date",
  "project.targetEndShort": "target {date}",
  "project.new": "New project",
  "project.edit": "Edit project",
  "project.archive": "Archive project",
  "project.archiveConfirm":
    "Archiving removes this project from the live list and frees its key. This cannot be undone from the UI.",
  "project.archivedReadOnly": "This project is archived and takes no changes.",
  "project.notYoursToChange":
    "You cannot change this project. Ask its owner to share it with you, or your administrator for the right to edit it.",
  "project.phaseLabel": "Phase",
  "project.filterPhaseAll": "All phases",
  "project.viewDelivering": "In delivery",
  "project.phase.initiative": "Initiative",
  "project.phase.pursuing": "Pursuing",
  "project.phase.delivering": "Delivering",
  "project.phase.closed": "Closed",
  "project.emptyTitle": "No projects yet",
  "project.emptyBody":
    "A project is the body of work a deal is about. It starts during the deal, in the initiative phase, and outlives close-won: once the deal is won, delivery is tracked here.",
  "project.emptyKey":
    "Every project gets a short key. Any email whose subject carries it in brackets is filed under that project automatically.",
  "project.rollups.empty": "No figures for this project yet.",
  "project.rollups.openValue": "Open deal value",
  "project.rollups.wonValue": "Won deal value",
  "project.rollups.openDeals": "Open the deals",
  "project.rollups.openCommitmentsList": "Open what is owed",
  "project.rollups.openCommitments": "Open commitments",
  "project.rollups.lastActivity": "Last activity",
  "project.rollups.never": "nothing yet",
  "project.rollups.activityCount": "Activities",
  "project.history.title": "Phase history",
  "project.history.empty": "No phase change recorded yet.",
  "project.history.current": "current",
  "project.history.moved": "{from} → {to}",
  "project.history.born": "Started in {phase}",
  "project.history.bySystem": "System",
  "project.deals.title": "Deals",
  "project.deals.empty":
    "No deal names this project yet. A deal picks its project on its own form.",
  "project.deals.more": "More deals than shown here — open the pipeline.",
  "project.stakeholders.title": "Stakeholders",
  "project.stakeholders.empty":
    "Nobody is seated on this project yet. A stakeholder is a person with a role here — a sponsor, a project lead, a champion.",
  "project.stakeholders.add": "Add stakeholder",
  "project.stakeholders.addConfirm": "Add",
  "project.stakeholders.addHint":
    "One seat per person. Naming somebody already on this project moves them to the role you pick here.",
  "project.stakeholders.searchLabel": "Search people by name",
  "project.stakeholders.removeTitle": "Take this person off the project?",
  "project.stakeholders.removeConfirm":
    "{name} stops being a stakeholder on this project. Their activity stays where it is.",
  "project.stakeholders.removeOne": "Take {name} off the project",
  "project.role.sponsor": "Sponsor",
  "project.role.project_lead": "Project lead",
  "project.role.delivery_lead": "Delivery lead",
  "project.role.subject_matter_expert": "Subject-matter expert",
  "project.contracts.title": "Contracts",
  "project.contracts.empty":
    "No agreement is filed under this project. A contract names its project when it is recorded.",
  "project.documents.title": "Documents",
  "project.documents.empty":
    "No file is attached to this project. Files attached to its deals stay on the deals.",
  "project.commitments.title": "Open commitments",
  "project.commitments.empty":
    "No open task is filed under this project. Tasks linked to it land here, soonest due first.",
  "project.commitments.overdue": "overdue",
  "project.timeline.empty":
    "Nothing is filed under this project yet. Mail carrying the key in its subject, and activities linked to it, land here.",
  "project.advance.title": "Move to {phase}",
  "project.advance.confirm": "Move",
  "project.advance.close": "Close project",
  "project.advance.body":
    "The move is recorded in the phase history with the reason you give.",
  "project.advance.closeBody":
    "Closing ends the project's delivery. It can be reopened later, and the reason stays on record.",
  "project.advance.reason": "Reason",
  "project.advance.reasonRequired": "A closed project needs a reason.",
  "deal.project": "Project",
  "deal.projectNew": "New project…",
  "deal.projectWithheld": "Project withheld",
  "deal.projectNeedsCompany":
    "Choose the deal's company first — a project is started on a company.",
  "deal.projectUnnamed": "Project",
  "deal.startDeliveryTitle": "Start delivery",
  "deal.startDelivery": "Start delivery",
  "deal.startDeliveryAttached":
    "This deal is attached to {project}, but the project is not in delivery yet. Move it now?",
  "deal.startDeliveryBody":
    "This deal is won and names no project. Attach it to {project} and move the project into delivery?",

  // The Worklist's own words: the ranked queue, its dials, and the phrase
  // for every fact the server sends as a closed vocabulary.
  "worklist.loading": "Reading your day…",
  "worklist.queue": "Today",
  "worklist.review": "To review",
  "worklist.more": "Show more",
  "worklist.more.failed": "Could not load more. Try again.",
  "worklist.summary":
    "{urgent} urgent · {due} due · {inPlay} in play · {lower} routine — {total} in all",
  "worklist.summary.noMiddle":
    "{urgent} urgent · {due} due · {lower} routine — {total} in all",
  "worklist.completeness": "{shown} of {considered} shown",
  "worklist.review.partial":
    "{loaded} of {total} shown — page the day to reach the rest",
  "worklist.completeness.bounded":
    "{shown} shown · {sources} sources have more",
  "worklist.clear": "Nothing is waiting on you.",
  "worklist.clearOfWhatWasRead":
    "Nothing is waiting among the sources that answered.",
  "worklist.partial": "{sources}, so this is not the whole day.",
  "worklist.overdue": "Overdue",
  "worklist.pair.ask": "Which record should survive?",
  "worklist.pair.keep": "Keep {name}",
  "worklist.pair.notDuplicate": "Not the same",
  "worklist.pair.related": "{count} linked",
  "worklist.pair.failed": "Could not settle the pair. Try again.",
  "worklist.pair.refused":
    "You cannot settle this pair. Both records have to be yours to change, so an admin or a sales lead can settle it.",
  "worklist.pair.alreadySettled":
    "This pair could not be settled the way you asked — somebody may have decided it first, or the records changed under you. Reload the list to see where it stands.",
  "worklist.pair.stewardOnly":
    "Only somebody who can change both records can settle this — an admin or a sales lead.",
  "worklist.needsPrep": "Needs prep",
  "worklist.pane.title": "About this record",
  "worklist.pane.openRow": "Show what {position}, {title}, is about",
  "worklist.pane.loading": "Reading the record…",
  "worklist.pane.nothing": "Nothing recorded yet.",
  "worklist.pane.lastInbound": "They last wrote",
  "worklist.pane.lastOutbound": "We last wrote",
  "worklist.pane.never": "Never",
  "worklist.band.now": "Now",
  "worklist.band.build_pipeline": "Build pipeline",
  "worklist.band.keep_momentum": "Keep momentum",
  "worklist.band.review": "Review",
  // A band holding nothing, said rather than left out. Each says what is
  // absent, because "nothing here" four times over tells a reader less than
  // one line naming what they are clear of.
  "worklist.bandClear.now": "Nothing needs you today.",
  "worklist.bandClear.build_pipeline": "No new pipeline work waiting.",
  "worklist.bandClear.keep_momentum": "Nothing agreed is drifting.",
  "worklist.bandClear.review": "Nothing to review.",
  "worklist.disposition.verb.snooze": "Snooze",
  "worklist.disposition.snoozeFor": "For how long",
  "worklist.disposition.snoozeDays_one": "{value} day",
  "worklist.disposition.snoozeDays_other": "{value} days",
  "worklist.disposition.verb.not_mine": "Not mine",
  "worklist.disposition.verb.not_sales": "Not a customer",
  "worklist.disposition.done.snooze": "Back on your list tomorrow.",
  "worklist.disposition.doneSnooze_one": "Back on your list tomorrow.",
  "worklist.disposition.doneSnooze_other": "Back on your list in {value} days.",
  "worklist.disposition.done.not_mine":
    "Off your list. Whoever owns it still sees it.",
  "worklist.disposition.done.not_sales": "Off everyone's list.",
  "worklist.disposition.swipeCancel": "Keep it",
  "worklist.disposition.menu": "Take off the list",
  "worklist.disposition.undo": "Undo",
  "worklist.disposition.undoFailed":
    "That could not be undone. The message is still off your list.",
  "worklist.disposition.failed": "That could not be put down.",
  "worklist.scope.label": "Whose work",
  "worklist.scope.mine": "Mine",
  "worklist.scope.unassigned": "Unassigned",
  "worklist.scope.team": "My team",
  "worklist.scope.all": "All",
  "worklist.owner.label": "Whose queue",
  "worklist.manager.cancel": "Cancel",
  "worklist.owner.mine": "My own day",
  "worklist.owner.backToMine": "Back to my own day",
  "worklist.manager.reassign": "Reassign",
  "worklist.manager.reassignTo": "Hand it to",
  "worklist.manager.reassignConfirm": "Hand it over",
  "worklist.manager.takeOwnership": "Take this on",
  "worklist.manager.takeOwnershipAsk":
    "This moves the record out of their queue and into yours.",
  "worklist.manager.takeOwnershipConfirm": "Take it on",
  "worklist.manager.tookOwnership": "It is yours now.",
  "worklist.manager.takeOwnershipFailed":
    "That could not be handed over. It is still theirs.",
  "worklist.manager.reassigned": "Handed over.",
  "worklist.manager.reassignFailed": "That could not be handed over.",
  "worklist.manager.coach": "Leave a note",
  "worklist.manager.coachAbout": "About",
  "worklist.manager.coachConfirm": "Leave the note",
  "worklist.manager.coached": "Your note is on their queue.",
  "worklist.manager.coachFailed": "That note could not be left.",
  "worklist.manager.note": "Your note (optional)",
  "worklist.manager.kind.reply_aging": "An aging reply",
  "worklist.manager.kind.next_step": "A deal's next step",
  "worklist.manager.kind.review_backlog": "Review work",
  "worklist.manager.kind.general": "Something else",
  "worklist.board.title": "How my team is doing",
  "worklist.exceptions.title": "What needs me",
  "worklist.handled.title": "Handled for you",
  "worklist.walk.arrived":
    "{arrived} more since you started. They wait for a refresh so this list holds still.",
  "worklist.walk.gone":
    "{gone} of these have been dealt with since you started.",
  "worklist.walk.both":
    "{arrived} more since you started, and {gone} already dealt with.",
  "worklist.walk.refresh": "Refresh",
  "worklist.handled.empty": "Nothing was done on your behalf today.",
  "worklist.handled.loading": "Reading what was done",
  "worklist.handled.what": "What happened",
  "worklist.handled.about": "About",
  "worklist.handled.when": "When",
  "worklist.handled.noRecord": "No record named",
  "worklist.handled.truncated":
    "More than this. The list stops at what one sitting can hold.",
  "worklist.exceptions.empty": "Nothing on the team needs you right now.",
  "worklist.exceptions.loading": "Reading the team",
  "worklist.exceptions.condition": "What",
  "worklist.exceptions.subject": "About",
  "worklist.exceptions.owner": "Who answers",
  "worklist.exceptions.basis": "Judged against",
  "worklist.exceptions.intervene": "Intervention",
  "worklist.exceptions.nobody": "Nobody yet",
  "worklist.exceptions.ownerWithheld": "Not shown to you",
  "worklist.exceptions.truncated":
    "More than this. The list stops at what one sitting can hold.",
  "worklist.exceptions.kind.response_breached": "A first reply is late",
  "worklist.exceptions.kind.revenue_at_risk": "Revenue at risk",
  "worklist.exceptions.kind.unassigned": "Nobody has taken it",
  "worklist.exceptions.kind.repeated_failure": "The same thing keeps failing",
  "worklist.board.loading": "Reading your team's work…",
  "worklist.board.empty": "Nobody has been put on a team with you yet.",
  "worklist.board.member": "Who",
  "worklist.board.waiting": "Waiting on a reply",
  "worklist.board.atRisk": "Deals at risk",
  "worklist.board.overdue": "Past due",
  "worklist.board.nobody": "Nobody yet",
  "worklist.board.truncated":
    "There is more work than this could count. These are floors, not totals.",
  "worklist.readings.label": "What today is worth",
  "worklist.readings.revenue": "Revenue at risk",
  "worklist.readings.revenue.detail": "Across the deals drifting today",
  "worklist.readings.revenue.unpriced": "No deal at risk could be priced",
  "worklist.readings.openLane": "Open this lane",
  "worklist.readings.replies": "Buyer replies",
  "worklist.readings.replies.detail": "Customers waiting on an answer",
  "worklist.readings.prospecting": "Prospecting",
  "worklist.readings.prospecting.detail": "New business owed a first reply",
  "worklist.readings.review": "Review",
  "worklist.readings.review.detail": "Routine work queued behind a decision",
  "worklist.readings.truncated":
    "There is more work than this could count. These are floors, not totals.",
  "worklist.hidden.title": "What the queue is not showing",
  "worklist.hidden.loading": "Checking what is held back…",
  "worklist.hidden.clear":
    "Nothing is being held back. Every waiting customer reaches somebody’s queue.",
  "worklist.hidden.truncated":
    "There is more work than this could count. These are floors, not totals.",
  "worklist.hidden.count": "{count} waiting",
  "worklist.hidden.pastHorizon": "Too old for the queue",
  "worklist.hidden.pastHorizon.detail":
    "Nobody decided this. They wrote months ago and were never answered.",
  "worklist.hidden.unlinked": "Attached to no record",
  "worklist.hidden.unlinked.detail":
    "Usually not sales. Sometimes a customer nobody managed to file.",
  "worklist.hidden.colleagues": "From one of our own domains",
  "worklist.hidden.colleagues.detail":
    "A colleague, not a customer. A mistyped domain hides a real one.",
  "worklist.hidden.notSales": "Judged not sales work",
  "worklist.hidden.notSales.detail":
    "Hidden from the whole organization, and it does not lift.",
  "worklist.hidden.setAside": "Set aside by you",
  "worklist.hidden.setAside.detail":
    "Snoozed or marked not yours. A snooze comes back on its own.",
  "worklist.hidden.shown": "The queue itself carries {count}.",
  "worklist.filter.label": "Kind of work",
  "worklist.filter.all": "All",
  "worklist.filter.customer_waiting": "Customer waiting",
  "worklist.filter.leads": "Leads",
  "worklist.filter.deals_at_risk": "Deals at risk",
  "worklist.filter.meetings": "Meetings",
  "worklist.filter.tasks": "Tasks",
  "worklist.filter.decisions": "Decisions",
  "worklist.filter.system": "System",
  "worklist.category.customer_waiting": "Customer waiting",
  "worklist.category.leads": "Lead",
  "worklist.category.deals_at_risk": "Deal at risk",
  "worklist.category.meetings": "Meeting",
  "worklist.category.tasks": "Task",
  "worklist.category.decisions": "Decision",
  "worklist.category.system": "System",
  "worklist.because.pinned": "You pinned this",
  "worklist.because.buyer_wrote_last": "They wrote last",
  "worklist.because.waiting_days": "waiting",
  "worklist.because.more_one": "+{count} more reason",
  "worklist.because.more_other": "+{count} more reasons",
  "worklist.because.waiting_days.value_one": "waiting {value} day",
  "worklist.because.waiting_days.value_other": "waiting {value} days",
  "worklist.because.overdue": "overdue",
  "worklist.because.due_today": "due today",
  "worklist.because.closing_soon": "has a close date",
  "worklist.because.expected_revenue": "an open deal rests on this",
  "worklist.because.expected_revenue.value": "worth {value}",
  "worklist.because.material": "above the typical open deal",
  "worklist.because.material.value":
    "worth {value}, above the typical open deal",
  "worklist.because.below_material": "below the typical open deal",
  "worklist.because.below_material.value":
    "worth {value}, below the typical open deal",
  "worklist.because.quiet_days": "gone quiet",
  "worklist.because.quiet_days.value_one": "quiet for {value} day",
  "worklist.because.quiet_days.value_other": "quiet for {value} days",
  "worklist.because.no_champion": "no champion",
  "worklist.because.promised": "you promised this",
  "worklist.because.approved_and_failed": "you approved it and it did not run",
  "worklist.because.blocks_customer_work": "a customer is held up",
  "worklist.because.routine": "routine tidying",
  "worklist.because.repeated_failure": "the same thing keeps failing",
  "worklist.because.legal_deadline": "a legal deadline is running",
  "worklist.because.meeting_soon": "starting shortly",
  "worklist.because.meeting_unprepared": "nothing prepared",
  "worklist.because.response_overdue": "reply overdue",
  "worklist.because.response_due_soon": "reply due soon",
  "worklist.because.response_due_soon.value": "reply due by {value}",
  "worklist.because.unassigned": "nobody owns it",
  "worklist.because.stale": "waiting a long time",
  "worklist.because.no_reply_history": "no reply history",
  "worklist.because.asks_nothing": "asks nothing of us",
  "worklist.above.pin": "Above the next because you pinned it.",
  "worklist.above.level":
    "Above the next because it is a more pressing kind of work.",
  "worklist.above.deadline": "Above the next on its date.",
  "worklist.above.deadline.pair": "Above the next: {mine} against {theirs}.",
  "worklist.above.expected_revenue": "Above the next on expected revenue.",
  "worklist.above.expected_revenue.pair":
    "Above the next: {mine} against {theirs}.",
  "worklist.above.waiting_days": "Above the next on how long it has waited.",
  "worklist.above.waiting_days.pair":
    "Above the next: {mine} against {theirs}.",
  "worklist.above.relationship":
    "Above the next on how close the relationship is.",
  "worklist.above.crowded":
    "Above the next because that one is one of many of its kind.",
  "worklist.verdict.live": "Live",
  "worklist.verdict.drifting": "Drifting",
  "worklist.verdict.blocked": "Blocked",
  "worklist.verdict.cold": "Cold",
  "worklist.verdict.believes": "Margince believes",
  "worklist.verdict.rule": "Why it is here",
  "worklist.verdict.asOf": "Read {when}",
  "worklist.consequence.buyer_waits": "If you do nothing, they keep waiting.",
  "worklist.consequence.promise_breaks": "If you do nothing, a promise breaks.",
  "worklist.consequence.deal_drifts":
    "If you do nothing, the deal keeps drifting.",
  "worklist.consequence.deal_slips_past_close":
    "If you do nothing, it slips past the date the customer agreed.",
  "worklist.consequence.meeting_unprepared":
    "If you do nothing, you walk in unprepared.",
  "worklist.consequence.task_slips": "If you do nothing, it slips.",
  "worklist.consequence.work_blocked": "If you do nothing, work stays blocked.",
  "worklist.consequence.customer_never_received":
    "If you do nothing, the customer never gets it.",
  "worklist.consequence.you_believe_it_happened":
    "If you do nothing, you go on believing it happened.",
  "worklist.consequence.legal_deadline_missed":
    "If you do nothing, a legal deadline passes.",
  "worklist.consequence.mailbox_blind":
    "If you do nothing, this page keeps missing what is not arriving.",
  "worklist.consequence.data_drifts": "If you do nothing, the records drift.",
  "worklist.untitled.approval": "A decision is waiting",
  "worklist.untitled.dedupe_candidate": "Two records look like the same one",
  "worklist.untitled.task": "A task",
  "worklist.untitled.brief_item": "The night picked this out",
  "worklist.untitled.conversation_claim": "A promise you made",
  "worklist.untitled.customer_waiting": "Someone is waiting for a reply",
  "worklist.untitled.lead_response": "A lead",
  "worklist.untitled.deal_at_risk": "A deal is drifting",
  "worklist.untitled.meeting": "A meeting",
  "worklist.untitled.relationship_decay": "A relationship is going quiet",
  "worklist.untitled.failed_approval": "Something you approved did not run",
  "worklist.untitled.dsr": "An open privacy request",
  "worklist.untitled.sync_health": "The CRM sync needs attention",
  "worklist.sync.class.contacts": "contacts",
  "worklist.sync.class.companies": "companies",
  "worklist.sync.class.deals": "deals",
  "worklist.sync.class.leads": "prospects",
  "worklist.sync.class.calls": "calls",
  "worklist.sync.class.meetings": "meetings",
  "worklist.sync.class.emails": "emails",
  "worklist.sync.class.notes": "notes",
  "worklist.sync.class.tasks": "tasks",
  "worklist.sync.error.rate_limited":
    "the other system is limiting how often we may ask",
  "worklist.sync.error.unreachable": "the other system cannot be reached",
  "worklist.sync.error.auth": "the connection needs signing in again",
  "worklist.sync.error.history_gone":
    "the other system no longer holds that history",
  "worklist.sync.error.internal": "something on our side went wrong",
  "worklist.sync.band.warn":
    "Close to the read budget, so some reads may be served from the copy.",
  "worklist.sync.band.shed":
    "Over the read budget: reads are being served from the copy rather than live.",
  "worklist.sync.failing": "Not syncing — {reason}.",
  "worklist.sync.objects_stale": "Out of date here: {classes}.",
  "worklist.sync.backfill_incomplete": "Still importing: {classes}.",
  "worklist.sync.records_overwritten":
    "Changed here and overwritten by the other system: {classes}.",
  "worklist.untitled.capture_health": "A mailbox connection needs attention",
  "worklist.untitled.ai_work_health": "AI work needs a look",
  "worklist.untitled.bounce": "An email did not arrive",
  "worklist.untitled.undelivered": "An email was never sent",
  "worklist.untitled.automation_run": "A rule did not do its work",
  "worklist.untitled.notice": "A notice for you",
  "worklist.untitled.introduction_request":
    "A colleague asked you for an introduction",
  "worklist.verb.decide": "Decide",
  // The drawer the decision is answered in. The row shows what is being decided
  // and this names the act, so the heading does not repeat the row's sentence.
  "worklist.decision.title": "Your decision",
  "worklist.decision.loading": "Fetching what is being proposed…",
  "worklist.decision.unavailable":
    "This proposal could not be read. Open the approvals queue to answer it.",
  "worklist.verb.merge": "Merge",
  "worklist.verb.open": "Open",
  "worklist.verb.complete": "Open",
  "worklist.verb.snooze": "Open",
  "worklist.verb.acknowledge": "Got it",
  "worklist.verb.acknowledgeFailed": "That could not be marked as seen.",
  "worklist.verb.completeFailed": "That task could not be completed.",
  "worklist.verb.pin": "Pin",
  "worklist.verb.unpin": "Unpin",
  "worklist.verb.pinFailed": "That row could not be pinned.",
  "worklist.verb.unpinFailed": "That row could not be unpinned.",
  "worklist.verb.completed": "Task done.",
  "worklist.verb.dismiss": "Not now",
  "worklist.verb.dismissed": "Set aside for a month.",
  "worklist.verb.dismissUndo": "Undo",
  "worklist.verb.dismissFailed": "That contact could not be set aside.",
  "worklist.verb.dismissUndoFailed": "That contact could not be put back.",
  "worklist.verb.completeUndo": "Undo",
  "worklist.verb.completeUndoFailed": "That task could not be reopened.",
  // The frame states the fact and the source follows it, rather than the
  // source standing as the subject. `sourceName` returns a row TITLE — "A
  // mailbox connection needs attention", "Two records look like the same one"
  // — and fourteen of the twenty-one are already whole clauses, so used as a
  // subject they ran two sentences together: "A mailbox connection needs
  // attention could not be read". Naming the fact first works for every entry
  // and needs no second vocabulary of source nouns.
  "worklist.source.failed": "A source could not be read: {source}",
  "worklist.source.withheld": "A source is hidden from your account: {source}",
  "worklist.untitled.generic": "Something needs you",
  "worklist.batch.likely_automated": "{count} likely automated senders",
  "worklist.batch.company_match": "{count} addresses at companies you know",
  "worklist.batch.uncertain_contact": "{count} addresses to decide on",
  "worklist.batch.duplicates": "{count} possible duplicate records",
  "worklist.batch.held_draft": "{count} drafts waiting to be sent",
  "worklist.untitled.batch": "A group of routine decisions",
  "worklist.verb.review_batch": "Review",
  "worklist.verb.draft_reply": "Open to reply",
  // Where the composer actually opens, the verb is the ACT rather than the way
  // to it. The two labels are separate keys because the two clicks differ.
  "worklist.verb.draft_reply_now": "Draft the reply",
  // A FIRST message rather than an answer to one. Separate keys because the two
  // are different acts: a row saying "reply" over an opening outreach names a
  // conversation that has not happened yet.
  "worklist.verb.draft_email": "Open to write",
  "worklist.verb.draft_email_now": "Draft the email",
  "worklist.deal.closes": "closes {date}",
  "worklist.when.starts": "starts {when}",
  "worklist.when.due": "due {when}",
  "worklist.batch.system_incident": "{cause} failed {count} times",
  "worklist.batch.unnamedCause": "Something",

  "ob.conv.scene.settleEyebrow": "It stopped on something only you can settle",
  "ob.conv.review.boardSub":
    "Every line says where it came from. Nothing is written until you confirm.",
  "ob.conv.manual.boardTitle": "Fill it in yourself.",
  "ob.conv.scene.writes": "Writes",
  "ob.core.idle": "core · at rest",
  "ob.core.ingest": "core · taking it in",
  "ob.core.working": "core · working it out",
  "ob.core.warning": "core · something needs a look",
  "ob.core.error": "core · stopped",
  "ob.scan.tallyPages": "pages read",
  "ob.scan.tallyFacts": "facts found",
  "ob.scan.tallyUncertain": "it will not guess at",
  // The ticker's page-level finding: a fact field's own name (already the
  // reader's language via factFieldLabelKey) and the value the page gave up.
  // Punctuation only, nothing here for a locale to translate.
  "ob.scan.tickerFact": "{field}: {value}",
  "ob.digest.where": "What Margince knows about you",
  "ob.digest.written": "{n} of {m} lines written",
  "ob.digest.companyLine": "Company profile, written from {n} pages of {host}",
  "ob.digest.citedCaption": "lines, each citing its page",
  "ob.digest.openCaption": "still open",
  "ob.digest.section.identity": "Identity",
  "ob.digest.section.offer": "What they sell",
  "ob.digest.section.customer": "Who they sell to",
  "ob.digest.section.sales": "How they write",
  "ob.digest.facts": "Proof",
  "ob.digest.people": "People",
  "ob.digest.sources": "References",
  "ob.digest.blank": "not written yet",
  "ob.digest.notWritten": "not written down",
  "ob.digest.settle": "Settle it",
  "ob.digest.deciding": "you are deciding this now",
  "ob.digest.yours": "yours",
  "ob.digest.editLine": "Edit {label}",
  "ob.digest.saveChanges": "Save changes",
  "ob.digest.changed": "{count} lines changed, not saved yet",
  "ob.digest.pickFacts": "Choose the facts to keep",
  "ob.digest.referenceNote":
    "A later re-read may propose changes to this record. It will never overwrite a line a person has already touched.",
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
  "ob.deck.settled": "{count} facts went in on evidence, without you",
  "ob.deck.needed": "Needed to continue",
  "ob.deck.optional": "Worth a look",
  "ob.deck.next": "Next",
  "ob.deck.leaveOut": "Leave it out",
  "ob.deck.readWhole": "Read the whole profile",
  "ob.deck.backToOpen": "Back to the open ones",
  "ob.deck.backToRecord": "Back to the record",
  "ob.deck.confirm": "Confirm the profile",
  "ob.deck.stillNeeded": "Still needed: {fields}",
  "ob.deck.openLeft":
    "Questions left unanswered: {count}. The record is saved without them.",
  "ob.conv.invite.pickOne": "Pick one of the two to continue.",
  "ob.conv.voice.speakerPick": "Pick a speaker to continue.",
  "ob.deck.clear": "Nothing left to settle. {count} facts are on the record.",
  "ob.deck.eyebrow": "Everything else went in on evidence",
  "ob.deck.title": "It will not guess at these.",
  "ob.stage.flow": "Setup",
  "ob.stop.read": "Read the site",
  "firstRun.ai.eyebrow": "Nothing here can think yet",
  "firstRun.step.model": "The model",
  "firstRun.step.platform": "Your platform",
  "firstRun.google.eyebrow": "It thinks. It cannot reach anyone yet",
  "firstRun.platform.title": "What does your organization run on?",
  "firstRun.platform.sub":
    "One answer decides how mail reaches Margince and how people sign in. You can change it later under Settings.",
  "firstRun.platform.legend": "The platform this organization runs on",
  "firstRun.platform.google": "Google Workspace",
  "firstRun.platform.googleWhat":
    "Mail, calendar and sign-in through one Google app you own.",
  "firstRun.platform.microsoft": "Microsoft 365",
  "firstRun.platform.microsoftWhat":
    "Mail, calendar and sign-in through one Entra app you own.",
  "firstRun.platform.imap": "IMAP",
  "firstRun.platform.imapWhat":
    "Each mailbox connects with its own IMAP app-password. Sign-in is email and password.",
  "firstRun.platform.redirectTitle": "Register these redirect URIs on the app",
  "firstRun.platform.redirectHint":
    "Copy each one into the app before you save it here. Sign-in is what puts the sign-in button on the login page for everyone you invite; Mailbox and Calendar are what let people connect theirs. A missing one fails at the vendor's consent screen, not here.",
  "firstRun.google.helpToggle": "Where do I get these?",
  "firstRun.google.helpStep1":
    "In the Google Cloud console, open a project and go to APIs & Services → Credentials → Create credentials → OAuth client ID, and choose Web application.",
  "firstRun.google.helpStep2":
    "Enable the Gmail API, and put both the gmail.readonly and gmail.send scopes on the consent screen. They ride one consent on purpose: Google will not add a scope to a refresh token it already issued, so asking for send later means connecting the mailbox twice.",
  "firstRun.google.helpStep3":
    "Under Authorized redirect URIs, add the ones listed above. Mailbox is the one you need for mail; Calendar and Sign-in are what the other two do.",
  "firstRun.google.helpStep4":
    "Copy the client ID and client secret Google shows you into the two fields below. The secret is sent once and sealed in the key vault; it is never readable again, here or anywhere.",
  "firstRun.google.helpConsole": "Google Cloud credentials console",
  "firstRun.google.helpDocs":
    "The full prerequisites, Microsoft and IMAP included: docs/how-to/connect-a-mailbox.md",
  "firstRun.platform.imapNote":
    "Nothing is set up for the whole installation. Connect your own mailbox now, or later; every other mailbox is connected under Settings → Integrations, with its own app-password.",
  "firstRun.platform.skip": "Not now",
  "firstRun.needed": "Needed to continue",
  "firstRun.stillNeeded": "Still needed: {fields}",
  "firstRun.platform.foot":
    "Whatever you answer here can be changed later under Settings → Admin.",
  "firstRun.microsoft.note":
    "Register an app in Microsoft Entra with the redirect URIs above, then paste its client id and secret here. Pin it to your directory: that is whose mailboxes connect through it, and whose people sign in with it.",
  "firstRun.microsoft.helpSignIn":
    "The directory is what puts Microsoft on the login page, so it is asked for here rather than left to chance. To register an app without one — any organization may connect a mailbox, and nobody signs in with Microsoft — use Settings instead.",
  "firstRun.microsoft.tenantHint":
    "The Entra directory your people are in. Mailboxes connect through it, and it is the directory Microsoft sign-in runs on.",
  "firstRun.ai.rankedHint":
    "Also listing the ten highest-scoring models OpenRouter serves right now, ranked by {rankedBy}, with the vendor's own prices.",
  "firstRun.ai.rankedUnavailable":
    "OpenRouter's live model list could not be read just now, so this offers what your price sheet holds.",
  "aiRates.chatLane": "What it thinks with",
  "aiRates.embedLane": "What it remembers with",
  "aiRates.perMTokInOut": "per million tokens, in → out",
  "aiRates.perMTok": "per million tokens",
  "aiRates.unpriced": "No price on file",
  "aiRates.unpricedDetail":
    "It will still serve calls. They report as unpriced, so they are missing from usage and spend until someone adds a rate under Settings → AI.",
  "aiRates.priced": "Priced from {date}",
  "aiRates.proposed": "OpenRouter's price",
  "aiRates.proposedDetail":
    "Read from the vendor just now, not from your price sheet. Bind it and it goes to your approvals inbox, so usage and spend can price it once you confirm.",
  "firstRun.ignite.title": "It has a pulse.",
  "firstRun.ignite.sub":
    "The key is sealed and the model answered. Here is what that changes.",
  "firstRun.ignite.sealed": "sealed in the vault · {vendor}",
  "firstRun.ignite.reaching": "reaching the model for the first time…",
  "firstRun.ignite.canNow": "can now",
  "firstRun.ignite.cannot": "cannot",
  "firstRun.ignite.read": "read your website and tell you what it found",
  "firstRun.ignite.draft": "draft in a voice you taught it",
  "firstRun.ignite.act": "send anything, or change a record, unless you say so",
  "firstRun.ignite.carryOn": "Carry on",
  "firstRun.ai.foot":
    "Nothing is sent to your vendor until you press Continue.",
  "person.readings.title": "Where this contact stands",
  "person.readings.move": "Whose move",
  "person.readings.yourMove": "Yours",
  "person.readings.theirMove": "Theirs",
  "person.readings.quiet": "Gone quiet",
  "person.readings.neverSpoke": "Never spoken",
  "person.readings.lastFromThem": "last from them: {when}",
  "person.readings.neverReplied": "nothing from them yet",
  "person.readings.promises": "Open promises",
  "person.readings.nothingOwed": "nothing owed",
  "person.readings.onTime": "none late yet",
  "person.readings.deal": "Deals they decide",
  "person.readings.openDeals": "Open deals",
  "person.readings.openMeetings": "Open meetings",
  "deal360.brief": "What this deal is",
  "deal.strip.openHistory": "See the ledger",
  "deal.strip.lastTouch": "Last touch",
  "lead.standing.qualified": "Qualified",
  "lead.standing.qualifiedOn": "Qualified on {at}. This lead is a contact now.",
  "lead.standing.qualifiedUndated": "This lead is a contact now.",
  "lead.standing.closed": "Closed",
  "lead.standing.closedFor": "Closed: {reason}. The record stays as the trail.",
  "lead.standing.closedUnreasoned": "Closed. The record stays as the trail.",
  "lead.standing.yourMove": "Your move",
  "lead.standing.noResponse": "Nobody has answered this lead yet.",
  "lead.standing.theirMove": "Their move",
  "lead.standing.answeredOn": "We answered on {at}. Nothing has come back yet.",
  "lead.standing.inMotion": "In motion",
  "lead.standing.engagedBecause":
    "They answered, or a meeting is on the calendar.",
  "lead.standing.rests.promoted": "Promoted to a contact.",
  "lead.standing.rests.closed": "Disqualified, no reason recorded.",
  "lead.standing.rests.ladder": "Lead ladder",
  "lead.standing.rests.record": "Lead record",
  "lead.standing.rests.captured": "Captured {at}.",
  "lead.standing.rests.noResponse": "No first response recorded.",
  "lead.standing.rests.engaged": "Engagement captured {at}.",
  "lead.readings.openStatus": "Open leads in this status",
  "lead.readings.title": "Where this lead stands",
  "lead.readings.firstResponse": "First response",
  "lead.readings.noClock": "no response target set",
  "lead.readings.owed": "Still owed",
  "lead.today.answer": "Answer {name}",
  "lead.today.answerMeta": "First response owed",
  "lead.today.nextTask": "Next task",
  "lead.readings.answered": "Answered",
  "lead.standing.dueBy":
    "Nobody has answered yet. The first response is due by {at}.",
  "lead.standing.overdueSince":
    "Nobody has answered yet. The first response was due {at}.",
} as const;

export type MessageKey = keyof typeof en;
