// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The catalogue keys that interpolate a count into ONE message rather than
// choosing between two, as they stood when the census beside them was armed.
//
// THIS REGISTER IS CLOSED. It records what was already here and it only
// shrinks: a key added from now on is a finding, and the way out of a finding
// is a `_one`/`_other` pair through `usePlural`, never a line added here. That
// is the bargain `uniquenessclaims` strikes in the Go gates, and it is the only
// way to arm a census over a population this size without either a flag-day
// rewrite of a hundred and fifty strings or a gate that goes on reporting PASS
// over the whole class it was written for.
//
// TWO LISTS, and they are not interchangeable. Each says something about the
// STRING that a reader can check against the catalogue at a glance, which is
// what makes an entry auditable rather than decorative — a register whose
// reason cannot be checked is the claim the rulebook calls worse than silence.

// The count is attached to no noun at all: `{count} min`, `All {count}`,
// `{count} overdue`. Nothing in the string inflects with the number, so there
// is no plural choice to make and there never will be. These are permanent.
const STANDALONE: readonly string[] = [
  "aiSettings.providers.missing",
  "aiSettings.providers.value",
  "aicalls.badge.retries",
  "analytics.forecastPriced",
  "brief.focus.position",
  "brief.sentence.rest",
  "brief.weekly.scorecard.answeredDetail",
  "brief.weekly.scorecard.regressionsDetail",
  "co.360.threadCount",
  "co.contacts.band.reachable",
  "co.contacts.band.seatsHeld",
  "co.contacts.band.someHidden",
  "co.contacts.band.untried",
  "co.contacts.map.more",
  "co.contacts.map.scope",
  "co.contacts.map.scopePartial",
  "co.deals.lostCount",
  "co.decisions.group",
  "co.decisions.open",
  "co.facts.showAll",
  "co.rail.all",
  "co.recent.minutes",
  "co.strip.convertedAsOf",
  "co.strip.lastTouch.ago",
  "co.strip.openDeals",
  "co.strip.stalled",
  "co.suggest.commitment.openAtLeast",
  "co.suggest.commitment.openCount",
  "co.suggest.commitment.overdueAtLeast",
  "co.suggest.commitment.overdueCount",
  "co.suggest.more",
  "contact.graph.droppedNote",
  "contact.intro.evidenceFrom",
  "contact.intro.evidenceFromYou",
  "contracts.state.title",
  "deals.bulkFailed",
  "deals.bulkSelected",
  "embedreindex.workspacePending",
  "extIngest.refusalCount",
  "jobs.count.dead",
  "jobs.count.retrying",
  "jobs.count.running",
  "jobs.count.waiting",
  "lead.bulkFailed",
  "lead.bulkSelected",
  "license.seats.left",
  "license.seats.over",
  "ob.conv.triage.contactsCount",
  "ob.conv.triage.factsCount",
  "ob.conv.triage.looksSolid",
  "ob.conv.triage.sectionAdvisory",
  "ob.conv.triage.sectionBlocking",
  "ob.conv.triage.sectionMore",
  "ob.conv.triage.sourceCount",
  "ob.conv.voice.dimensionsCount",
  "ob.scan.pagesSkipped",
  "project.rollups.activityFiled",
  "sched.recipientsMore",
  "state.partialCount",
  "table.perPage",
  "table.range",
  "table.rangeLoaded",
  "tagResult.viewAll",
  "tags.more",
  "voice.insights.statSources",
  "voice.insights.statWords",
  "webhooks.deliveries.deadLetterGroup",
  "worklist.exceptions.count",
  "worklist.handled.count",
  "worklist.hidden.count",
  "worklist.hidden.shown",
  "worklist.pair.related",
];

// The noun really does inflect, and the key prints "1 steps" — or will, the
// day a reader reaches it with one. Some are guarded at the call site today
// (`contact.loops.dueInDays` returns a different key at 0 and 1 before it can
// print "in 1 days") and some are not; what they have in common is that NOBODY
// HAS READ THE CALL SITE, and a register entry claiming a guard nobody checked
// would be worth less than no entry at all. So they say only what is certainly
// true. These are the debt. Reading one and converting it deletes its line.
const PENDING: readonly string[] = [
  "board.count",
  "brief.deck.bundleMembers",
  "brief.deck.bundleSummary",
  "brief.digestCommitmentCount",
  "brief.focus.remaining",
  "co.spine.andOthers",
  "co.spine.exchangeCount",
  "compose.carriageAggregate",
  "contact.loops.dueInDays",
  "contact.network.oneSided",
  "contact.network.twoWay",
  "contact.rail.exchanges",
  "contact.strip.days",
  "embedreindex.entitiesPending",
  "filters.matchCompanies",
  "filters.matchContacts",
  "filters.matchDeals",
  "jobs.deadTotal",
  "lead.boardCount",
  "lead.openTaskCount",
  "lead.scoreSources",
  "leadReasons.leadCount",
  "leadSources.leadCount",
  "network.interactions",
  "ob.conv.recap.readTerminal",
  "ob.conv.voice.dimSentenceEvidence",
  "settings.voice.meter",
  "strength.computedFrom",
  "tagAdmin.usage",
  "tagResult.totalVisible",
  "teamweekly.repsUnread",
  "today.silence.days",
  "tools.inventory",
  "voice.insights.next.addWords",
  "voice.insights.statSentence",
];

/** Why one pre-existing single-key count is not yet a finding. */
export const PLURAL_SINGLE_KEY_DEBT: ReadonlyMap<string, string> = new Map([
  ...STANDALONE.map(
    (key) =>
      [key, "the count is attached to no noun; nothing here inflects"] as const,
  ),
  ...PENDING.map(
    (key) =>
      [
        key,
        "the noun inflects and its call site is unread (issue 2964)",
      ] as const,
  ),
]);
