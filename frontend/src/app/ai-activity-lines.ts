// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type ActivityKind = components["schemas"]["AiActivityItem"]["kind"];
type ActivityState = components["schemas"]["AiActivityItem"]["state"];

/**
 * A kind the rail deliberately does not narrate, and why.
 *
 * The server reports every AI task this build can run, because a task that
 * reports nothing is AI work the product performed and then denied. What a
 * reader is SHOWN is a different question, and it is this file's to answer —
 * which is why the reason lives in the code rather than in a review comment.
 */
type NotDisplayed = Readonly<{ notDisplayed: string }>;

/** The (state -> message key) table of a kind the rail narrates. */
export type LineSet = Readonly<Record<ActivityState, MessageKey>>;

const notDisplayed = (reason: string): NotDisplayed => ({
  notDisplayed: reason,
});

const WATCHED_BY_THE_ASKER = notDisplayed(
  "the work lands on the surface that asked for it and changes it when it arrives, so a line here would narrate what the reader is already looking at. growth_fit renders the band it returns on the panel that asked. cold_start runs behind TWO product surfaces and the reason has to cover both (it declares four invocation SITES in aitaskregistry.go — a different count, and not the one that decides whether a line can be drawn): during onboarding the screen is deliberately RAILLESS — `onboarding` is a member of RAIL_LESS_SCREENS (nav.ts), which shell.tsx reads to drop the whole chrome — so there is no rail on screen for its line to appear on — and the company page's Enrich card runs it too (cmd/api/modelwiring.go wires WithScrape with the cold_start brain), where a rail DOES exist and the card itself renders the proposal. Naming only the onboarding half would leave the company-page site looking like an oversight. A task-level map cannot separate those arms and does not have to, because neither is drawn where the task runs",
);
const SYSTEM_SWEEP = notDisplayed(
  "background workspace work that belongs to nobody in particular, so it has no personal line to draw",
);

// The site-read lanes, which do NOT belong above however much they look like it.
//
// They are RETIRED names. A read runs all three as model calls and is one
// occurrence of the read, so nothing announces them any more — but the wire can
// still carry them, from rows written before that and from a caller filtering on
// the name, so the rail still has to answer for what it draws. Nothing, as
// before.
//
// Their attribution is why this is a reason of its own rather than the sweep's:
// compose binds the requester as OnBehalfOf when a human asked
// (deepreadprincipal.go), so those occurrences resolve to that contact and land
// in their personal feed — while a read requested by domain triage or the
// auto-enrich sweep names no human and is workspace-scoped like any sweep.
// Calling all three "work that belongs to nobody" is false for the commonest
// case, and a kind-level map cannot say "sometimes personal" in the SYSTEM_SWEEP
// sentence.
const SITE_READ_WATCHED_WHERE_IT_RUNS = notDisplayed(
  "the site-read lanes, which are the individual model calls a website read makes. The read narrates itself: `site_read` below is one occurrence for the whole crawl, announced by the dossier row from the moment it is queued to the moment it settles, so a line per call would tell one reading several times over. These three no longer report at all — the rail registry has them as steps inside the read — and the names survive here because rows written before that are served until the projection's retention window closes over them, and because the same list is what a caller may filter on. Their attribution was never the task's either: a human-requested read carries that contact as on_behalf_of and IS personal to them, while a domain-triage or auto-enrich read names no human and is workspace-scoped, exactly like the sweeps above",
);

/**
 * The line for one (kind, state), by literal key — or the reason there is none.
 *
 * LITERAL, not `t(`agent.activity.${kind}.${state}`)`. The orphan guard in
 * i18n.test.ts counts a key as rendered when it starts with a template STEM,
 * and its regex stops at the first `${` — so an interpolated key would vouch
 * for the whole `agent.activity.` namespace forever, and a retired kind's copy
 * would sit in three catalogs with nothing to flag it.
 *
 * TOTAL over the contract's kinds, and the compiler is what holds it there: a
 * new kind fails the build until somebody either writes its copy in every
 * locale or says, here, why it is not shown. The second branch exists because
 * every AI task now reports — nineteen of them, most narrating work no rep
 * asked to watch — and forcing copy for all of them would have bought 300
 * strings nobody reads and taught the next author that the answer to a new kind
 * is boilerplate.
 *
 * Total over the STATE axis too, wherever a kind is displayed: every state the
 * feed can report is reachable by every kind. `stalled` in particular is
 * DERIVED by the server from a lease the occurrence's own source declared, so
 * it can arrive for anything, and a keyless entry would render nothing for
 * exactly the case the projection exists to show.
 */
export const ACTIVITY_LINE: Readonly<
  Record<
    ActivityKind,
    Readonly<Record<ActivityState, MessageKey>> | NotDisplayed
  >
> = {
  morning_brief: {
    queued: "agent.activity.morningBrief.queued",
    running: "agent.activity.morningBrief.running",
    stalled: "agent.activity.morningBrief.stalled",
    done: "agent.activity.morningBrief.done",
    degraded: "agent.activity.morningBrief.degraded",
    failed: "agent.activity.morningBrief.failed",
  },
  overnight_at_risk_sweep: {
    queued: "agent.activity.riskSweep.queued",
    running: "agent.activity.riskSweep.running",
    stalled: "agent.activity.riskSweep.stalled",
    done: "agent.activity.riskSweep.done",
    degraded: "agent.activity.riskSweep.degraded",
    failed: "agent.activity.riskSweep.failed",
  },
  document_extract: {
    queued: "agent.activity.documentExtract.queued",
    running: "agent.activity.documentExtract.running",
    stalled: "agent.activity.documentExtract.stalled",
    done: "agent.activity.documentExtract.done",
    degraded: "agent.activity.documentExtract.degraded",
    failed: "agent.activity.documentExtract.failed",
  },
  // A company's website read end to end, from the button on its page. The
  // other carrier a contact presses and then waits on: the dossier row can say
  // queued and running, which is what lets the orb enter `ingest` for the
  // whole crawl rather than flicker on each settled model call.
  site_read: {
    queued: "agent.activity.siteRead.queued",
    running: "agent.activity.siteRead.running",
    stalled: "agent.activity.siteRead.stalled",
    done: "agent.activity.siteRead.done",
    degraded: "agent.activity.siteRead.degraded",
    failed: "agent.activity.siteRead.failed",
  },

  // `queued` is written for all three and reachable by none of them: the router
  // announces a call it is ABOUT to serve, never one waiting, and no carrier
  // owns these tasks. The LineSet type is total over the state axis, so the key
  // exists because the compiler requires it — not because a producer is
  // missing. Saying so here saves the next reader the hunt.
  summarize: {
    queued: "agent.activity.summarize.queued",
    running: "agent.activity.summarize.running",
    stalled: "agent.activity.summarize.stalled",
    done: "agent.activity.summarize.done",
    degraded: "agent.activity.summarize.degraded",
    failed: "agent.activity.summarize.failed",
  },
  // Narrated although the account page draws the read live too, for the
  // reason the site read is NOT: the page's pending row is where a reader
  // who stayed sees it, and the rail is where the reader who opened three
  // accounts and moved on finds out which one is ready. The scan is that
  // reader's own work item — filed under them, named for the account.
  account_scan: {
    queued: "agent.activity.accountScan.queued",
    running: "agent.activity.accountScan.running",
    stalled: "agent.activity.accountScan.stalled",
    done: "agent.activity.accountScan.done",
    degraded: "agent.activity.accountScan.degraded",
    failed: "agent.activity.accountScan.failed",
  },
  draft_reply: {
    queued: "agent.activity.draftReply.queued",
    running: "agent.activity.draftReply.running",
    stalled: "agent.activity.draftReply.stalled",
    done: "agent.activity.draftReply.done",
    degraded: "agent.activity.draftReply.degraded",
    failed: "agent.activity.draftReply.failed",
  },
  offer_draft: {
    queued: "agent.activity.offerDraft.queued",
    running: "agent.activity.offerDraft.running",
    stalled: "agent.activity.offerDraft.stalled",
    done: "agent.activity.offerDraft.done",
    degraded: "agent.activity.offerDraft.degraded",
    failed: "agent.activity.offerDraft.failed",
  },

  growth_fit: WATCHED_BY_THE_ASKER,
  cold_start: WATCHED_BY_THE_ASKER,
  corpus_ask: WATCHED_BY_THE_ASKER,

  // The weekly retrospective's sentence. A per-rep occurrence a rep can see —
  // it runs under their own principal over their own week — so it gets real
  // copy rather than the system-sweep line.
  weekly_review: {
    queued: "agent.activity.weeklyReview.queued",
    running: "agent.activity.weeklyReview.running",
    stalled: "agent.activity.weeklyReview.stalled",
    done: "agent.activity.weeklyReview.done",
    degraded: "agent.activity.weeklyReview.degraded",
    failed: "agent.activity.weeklyReview.failed",
  },
  // Same ground as weekly_review above: the rep's own week under their own
  // principal, so it says what it is doing rather than the system-sweep line.
  weekly_learnings: {
    queued: "agent.activity.weeklyLearnings.queued",
    running: "agent.activity.weeklyLearnings.running",
    stalled: "agent.activity.weeklyLearnings.stalled",
    done: "agent.activity.weeklyLearnings.done",
    degraded: "agent.activity.weeklyLearnings.degraded",
    failed: "agent.activity.weeklyLearnings.failed",
  },
  brief_ranking: SYSTEM_SWEEP,
  capture_classify: SYSTEM_SWEEP,
  owed_verdict: SYSTEM_SWEEP,
  capture_confidentiality_verdict: SYSTEM_SWEEP,
  capture_counterparty_verdict: SYSTEM_SWEEP,
  enrich: notDisplayed(
    "it reaches nobody, and it would not be worth showing if it did — recording only the first half is what made this read like a gap somebody should close. Reachability: the one production site is the signature-enrichment pass, which runs under a system principal with no on_behalf_of, so ResolveActor scopes every occurrence to the workspace with a NULL actor_user_id while the personal feed selects on actor_user_id. Worth: it could not be per-contact even if it were reachable. The pass mints ONE correlation id for the whole run (api/jobs.yaml capture_enrich, up to 100 candidates in series), and the occurrence key is correlation+task, so every candidate collapses into ONE row — a per-contact subject would make that single row flap from one contact to the next rather than narrate any of them. What a reader actually wants from this pass is what it FOUND, which is durable and already drawn as evidence-or-omit provenance on the contact record. The ticker's own `enrich` key names DIFFERENT work (a provider run on a contact, and the company page's Enrich card, which runs cold_start rather than this task), which is what makes this one easy to mistake for visible",
  ),
  rate_extract: SYSTEM_SWEEP,
  signal_extract: SYSTEM_SWEEP,
  stage_evidence_extract: SYSTEM_SWEEP,
  site_extract: SITE_READ_WATCHED_WHERE_IT_RUNS,
  site_fact_extract: SITE_READ_WATCHED_WHERE_IT_RUNS,
  site_triage: SITE_READ_WATCHED_WHERE_IT_RUNS,
  propose_roles: SYSTEM_SWEEP,
  // Reading a meeting transcript for the next steps in it. A rep presses the
  // button and waits, and the run row can say queued and running — so this is
  // narrated rather than swept, and it is not SYSTEM_SWEEP's "belongs to nobody
  // in particular": the row records who asked.
  //
  // `degraded` is written and unreachable: the run's four statuses are the
  // projection's live and terminal pair exactly, with no partial among them. A
  // transcript that states no next steps settles `done` — a correct answer, not
  // a degraded one. `stalled` IS reachable, derived by the projection from the
  // lease rather than announced.
  transcript_propose: {
    queued: "agent.activity.transcriptRead.queued",
    running: "agent.activity.transcriptRead.running",
    stalled: "agent.activity.transcriptRead.stalled",
    done: "agent.activity.transcriptRead.done",
    degraded: "agent.activity.transcriptRead.degraded",
    failed: "agent.activity.transcriptRead.failed",
  },
  // Learning the reader's OWN writing voice, which they press a button and
  // wait for. Narrated rather than swept: the build row records who asked, and
  // it can say queued and running now that it carries its own occurrence.
  //
  // No named line, and nothing to name: a build is about the reader's own
  // voice, so the sentence has no second party in it.
  voice_build: {
    queued: "agent.activity.voiceBuild.queued",
    running: "agent.activity.voiceBuild.running",
    stalled: "agent.activity.voiceBuild.stalled",
    done: "agent.activity.voiceBuild.done",
    degraded: "agent.activity.voiceBuild.degraded",
    failed: "agent.activity.voiceBuild.failed",
  },

  cert_judge: notDisplayed(
    "the certification lane grading this build's own answers — an operator's measurement, not a rep's work",
  ),
  deal_health: notDisplayed(
    "declared in api/ai-tasks.yaml and not built: no site runs it, so nothing reports it yet",
  ),
  nl_search: notDisplayed(
    "declared in api/ai-tasks.yaml and not built: no site runs it, so nothing reports it yet",
  ),
  transcript: notDisplayed(
    "declared in api/ai-tasks.yaml and not built: no site runs it, so nothing reports it yet",
  ),
};

/**
 * The panel's own headings, which name no single run.
 *
 * They live beside the lines rather than in them: the map above is the (kind,
 * state) table its test holds to exactly, and a heading in it would be a key no
 * state could reach.
 */
export const PANEL_HEADING: Readonly<Record<"running", MessageKey>> = {
  running: "agent.panel.runningNow",
};

/**
 * The same lines with the subject NAMED, for the kinds whose source sends a
 * name.
 *
 * PARTIAL on purpose, where `ACTIVITY_LINE` is total: a named variant is not
 * something every kind can have. Most occurrences are about no single record,
 * and the ones that are only carry a name when their emitter sends
 * `subject_label` — the document reading names its document, the website
 * read names its company, and the summary sites name the company, contact or
 * meeting they were asked about. A total map here would demand copy for
 * sentences no server can produce, which is the boilerplate the notDisplayed
 * branch exists to avoid.
 *
 * Each entry is a strict alternative to the unnamed line, never a suffix: "I'm
 * reading Q3-offer.pdf" is one sentence in every locale this ships in, and
 * pasting a name onto the end of a translated sentence is how that stops being
 * true.
 */
export const NAMED_LINE: Readonly<Partial<Record<ActivityKind, LineSet>>> = {
  account_scan: {
    queued: "agent.activity.accountScanNamed.queued",
    running: "agent.activity.accountScanNamed.running",
    stalled: "agent.activity.accountScanNamed.stalled",
    done: "agent.activity.accountScanNamed.done",
    degraded: "agent.activity.accountScanNamed.degraded",
    failed: "agent.activity.accountScanNamed.failed",
  },
  document_extract: {
    queued: "agent.activity.documentExtractNamed.queued",
    running: "agent.activity.documentExtractNamed.running",
    stalled: "agent.activity.documentExtractNamed.stalled",
    done: "agent.activity.documentExtractNamed.done",
    degraded: "agent.activity.documentExtractNamed.degraded",
    failed: "agent.activity.documentExtractNamed.failed",
  },
  // One table for three kinds of record, because the sentence is the same
  // shape for each: "what I know about Acme", "about Ana Roth", "about the
  // cutover review". The label is what the product calls the record elsewhere,
  // sent by the site that assembled the summary's input.
  summarize: {
    queued: "agent.activity.summarizeNamed.queued",
    running: "agent.activity.summarizeNamed.running",
    stalled: "agent.activity.summarizeNamed.stalled",
    done: "agent.activity.summarizeNamed.done",
    degraded: "agent.activity.summarizeNamed.degraded",
    failed: "agent.activity.summarizeNamed.failed",
  },
  // The company's name, as the source knew it when the read was queued: the
  // dossier is about one company, and the label is that record's own
  // display name, already on the page the read was started from.
  site_read: {
    queued: "agent.activity.siteReadNamed.queued",
    running: "agent.activity.siteReadNamed.running",
    stalled: "agent.activity.siteReadNamed.stalled",
    done: "agent.activity.siteReadNamed.done",
    degraded: "agent.activity.siteReadNamed.degraded",
    failed: "agent.activity.siteReadNamed.failed",
  },
};
