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
type LineSet = Readonly<Record<ActivityState, MessageKey>>;

/**
 * A stem the catalog spells out under `agent.activity.` for every state.
 *
 * Derived from the catalog rather than listed, so the compiler refuses a stem
 * with no copy: `lineSet` builds all six keys from it, and each of the six has
 * to be a MessageKey, which means a stem missing one state fails the build
 * rather than rendering a raw key to a reader.
 */
type LineStem = {
  [K in MessageKey]: K extends `agent.activity.${infer S}.queued` ? S : never;
}[MessageKey];

/**
 * The six lines of one kind, from the one stem they share.
 *
 * One spelling of the state axis, rather than six literal keys per kind: the
 * table is nineteen sets long, and a stem is the only thing that varies
 * between them.
 */
function lineSet(stem: LineStem): LineSet {
  return {
    queued: `agent.activity.${stem}.queued`,
    running: `agent.activity.${stem}.running`,
    stalled: `agent.activity.${stem}.stalled`,
    done: `agent.activity.${stem}.done`,
    degraded: `agent.activity.${stem}.degraded`,
    failed: `agent.activity.${stem}.failed`,
  };
}

const notDisplayed = (reason: string): NotDisplayed => ({
  notDisplayed: reason,
});

const SYSTEM_SWEEP = notDisplayed(
  "background workspace work that belongs to nobody in particular, so it has no personal line to draw",
);

// Two of the three site-read lanes, which are STEPS of a read the third lane
// already narrates.
//
// One website read files one occurrence per lane it runs, because the
// occurrence key is correlation+task and a read's correlation is its site_read
// row id. `site_extract` is the profile lane every human-requested read runs,
// and it is the one drawn: "I'm reading the zenloop.com website" is the whole
// of what a reader wants to know. `site_fact_extract` is that same read's
// page-parallel fact pass, and `site_triage` fires only for a domain-triage read
// nobody asked for (isDomainTriageRequest) — drawing either would list one read
// two or three times over, under two or three sentences, for one afternoon's
// work.
const SITE_READ_STEP = notDisplayed(
  "a step inside one website read that `site_extract` already narrates: a read files one occurrence per lane it runs under its own correlation id, so drawing this lane too would list the same read twice. site_fact_extract is the read's page-parallel fact pass; site_triage runs only for a domain-triage read no human requested, which is workspace-scoped and reaches nobody's feed anyway",
);
/**
 * The line for one (kind, state), by key, or the reason there is none.
 *
 * Each set is BUILT from one stem by `lineSet`, so the orphan guard in
 * i18n.test.ts cannot hold this namespace: it counts a key as rendered when
 * the key starts with a template stem, and `lineSet`'s template vouches for
 * all of `agent.activity.` at once. What holds it instead is the copy-set spec
 * in ai-activity-lines.test.ts, which asserts the catalog's `agent.activity.`
 * keys are EXACTLY the keys these maps name, in both directions, so a retired
 * kind's copy fails there rather than sitting in three catalogs unflagged.
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
  morning_brief: lineSet("morningBrief"),
  overnight_at_risk_sweep: lineSet("riskSweep"),
  document_extract: lineSet("documentExtract"),

  // `queued` is written for all three and reachable by none of them: the router
  // announces a call it is ABOUT to serve, never one waiting, and no carrier
  // owns these tasks. The LineSet type is total over the state axis, so the key
  // exists because the compiler requires it — not because a producer is
  // missing. Saying so here saves the next reader the hunt.
  summarize: lineSet("summarize"),
  draft_reply: lineSet("draftReply"),
  offer_draft: lineSet("offerDraft"),

  // The AI reading up on a company for the person who asked. Each lands its
  // result on the surface that asked, and each is ALSO drawn here, because the
  // orb is the one place a reader looks to learn the AI is at work at all —
  // a card that fills in forty seconds later tells nobody what was happening
  // during the forty seconds.
  growth_fit: lineSet("growthFit"),
  corpus_ask: lineSet("corpusAsk"),
  // The website read behind the organization page's Enrich card, and behind
  // onboarding — where the screen is railless (`onboarding` is in
  // RAIL_LESS_SCREENS, nav.ts), so this copy is only ever seen for the card.
  cold_start: lineSet("coldStart"),
  // The deep website read a person starts from the organization page. Its
  // attribution is a fact about the READ: compose binds the requester as
  // on_behalf_of (deepreadprincipal.go), so a human's read lands in their own
  // feed, while a domain-triage or auto-enrich read names nobody and reaches
  // nobody's rail.
  site_extract: lineSet("siteExtract"),

  // The weekly retrospective's sentence. A per-rep occurrence a rep can see —
  // it runs under their own principal over their own week — so it gets real
  // copy rather than the system-sweep line.
  weekly_review: lineSet("weeklyReview"),
  brief_ranking: SYSTEM_SWEEP,
  capture_classify: SYSTEM_SWEEP,
  capture_confidentiality_verdict: SYSTEM_SWEEP,
  capture_counterparty_verdict: SYSTEM_SWEEP,
  enrich: notDisplayed(
    "it reaches nobody, and it would not be worth showing if it did — recording only the first half is what made this read like a gap somebody should close. Reachability: the one production site is the signature-enrichment pass, which runs under a system principal with no on_behalf_of, so ResolveActor scopes every occurrence to the workspace with a NULL actor_user_id while the personal feed selects on actor_user_id. Worth: it could not be per-person even if it were reachable. The pass mints ONE correlation id for the whole run (api/jobs.yaml capture_enrich, up to 100 candidates in series), and the occurrence key is correlation+task, so every candidate collapses into ONE row — a per-person subject would make that single row flap from one person to the next rather than narrate any of them. What a reader actually wants from this pass is what it FOUND, which is durable and already drawn as evidence-or-omit provenance on the person record. The ticker's own `enrich` key names DIFFERENT work (a provider run on a person, and the organization page's Enrich card, which runs cold_start rather than this task), which is what makes this one easy to mistake for visible",
  ),
  rate_extract: SYSTEM_SWEEP,
  signal_extract: SYSTEM_SWEEP,
  site_fact_extract: SITE_READ_STEP,
  site_triage: SITE_READ_STEP,
  propose_roles: SYSTEM_SWEEP,
  transcript_propose: SYSTEM_SWEEP,
  voice_build: SYSTEM_SWEEP,

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
export const PANEL_HEADING: Readonly<
  Record<"running" | "wentWrong", MessageKey>
> = {
  running: "agent.panel.runningNow",
  wentWrong: "agent.panel.wentWrong",
};

/**
 * The same lines with the subject NAMED, for the kinds whose source sends a
 * name.
 *
 * PARTIAL on purpose, where `ACTIVITY_LINE` is total: a named variant is not
 * something every kind can have. The scheduled runs are about no single record
 * — a morning brief is about the morning — and demanding copy for them would be
 * the boilerplate the notDisplayed branch exists to avoid. Every kind a person
 * asks for about ONE record is here, because that is the line the reader is
 * actually waiting on: "I'm drafting your reply to Anna Berg" is a sentence
 * about their afternoon, and "I'm drafting your reply" is a sentence about
 * software. The name is the source's own snapshot (`subject_label`): the
 * document reader resolves the document's title, and every router-served task
 * carries the name its caller bound (principal.WithWorkSubject).
 *
 * Each entry is a strict alternative to the unnamed line, never a suffix: "I'm
 * reading Q3-offer.pdf" is one sentence in every locale this ships in, and
 * pasting a name onto the end of a translated sentence is how that stops being
 * true.
 */
export const NAMED_LINE: Readonly<Partial<Record<ActivityKind, LineSet>>> = {
  document_extract: lineSet("documentExtractNamed"),
  summarize: lineSet("summarizeNamed"),
  draft_reply: lineSet("draftReplyNamed"),
  offer_draft: lineSet("offerDraftNamed"),
  growth_fit: lineSet("growthFitNamed"),
  corpus_ask: lineSet("corpusAskNamed"),
  cold_start: lineSet("coldStartNamed"),
  site_extract: lineSet("siteExtractNamed"),
};

/**
 * Why a run stopped, and what the reader can do about it — one sentence per
 * value of the router's closed `degrade_reason` vocabulary.
 *
 * `degrade_reason` is server-authored operator vocabulary: a sentinel like
 * `provider_quota`, never a provider's own message. It reaches the reader ONLY
 * through this table, in their locale, as a cause paired with a repair — "the
 * provider's quota is used up; top it up or bind another model under Settings"
 * — because a stopped run with no reason is a fault the reader can only
 * shrug at, and a raw token is a fault they cannot read. A value with no entry
 * draws nothing, which is how a carrier's own prose (the scheduled runner
 * writes full sentences here, in English) and any vocabulary this build has not
 * heard of stay off the reader's screen rather than appearing verbatim.
 *
 * The keys are the sentinels `classifyError` and the attempt reasons the router
 * writes (ai/callstore.go, ai/logicalcall.go). PARTIAL over strings by
 * construction — the wire types the field as free text — so this is the one
 * map here the compiler cannot hold total; its test holds every key to real
 * copy instead.
 */
export const REASON_LINE: Readonly<Record<string, MessageKey>> = {
  budget_deferred: "agent.activity.reason.budgetDeferred",
  budget_degrade: "agent.activity.reason.budgetDegrade",
  budget_unavailable: "agent.activity.reason.budgetUnavailable",
  metering_failed: "agent.activity.reason.meteringFailed",
  request_failed: "agent.activity.reason.requestFailed",
  schema_invalid: "agent.activity.reason.schemaInvalid",
  provider_quota: "agent.activity.reason.providerQuota",
  provider_throttled: "agent.activity.reason.providerThrottled",
  provider_refused: "agent.activity.reason.providerRefused",
  provider_error: "agent.activity.reason.providerError",
};

/**
 * The reader's sentence for why a run stopped, or null when the run stopped
 * for no reason this build can put into words.
 *
 * `Object.hasOwn`, not a bare lookup: the value comes off the wire, and
 * `constructor` would otherwise answer from the prototype with a function.
 */
export function reasonFor(
  item: Readonly<{ degrade_reason?: string | null }>,
  t: (key: MessageKey) => string,
): string | null {
  const reason = item.degrade_reason ?? "";
  if (reason === "" || !Object.hasOwn(REASON_LINE, reason)) {
    return null;
  }
  return t(REASON_LINE[reason]);
}

/**
 * What to say about one item, or nothing at all.
 *
 * The existence check is not optional and `t()` cannot do it: translate() falls
 * back to THE KEY STRING, so a missing entry would put
 * `agent.activity.foo.running` in front of a reader.
 *
 * There are three ways to draw nothing, and they are different facts: the kind
 * is one this build has never heard of, the kind is one it deliberately does
 * not narrate, or the state has no line under a kind it does narrate. All three
 * answer null, because the reader's screen is the same either way — but they
 * are distinguished HERE rather than collapsed into a lookup miss, so a kind
 * that silently lost its copy cannot hide among the ones that never had any.
 *
 * It takes RAW strings rather than the contract's unions, and that widening is
 * the point: the map is total over the contract, so the only way to reach the
 * first case is a value the contract does not carry — which is exactly what an
 * older tab gets from a newer server that has added a kind or a state. Typed
 * narrowly, that case could only be written with a cast, and a test that casts
 * is asserting against its own escape hatch instead of against the function.
 */
export function lineFor(
  item: Readonly<{
    kind: string;
    state: string;
    subject_label?: string | null;
  }>,
  t: (key: MessageKey, params?: Record<string, string>) => string,
): string | null {
  if (!isActivityKind(item.kind)) {
    return null;
  }
  const entry = ACTIVITY_LINE[item.kind];
  if ("notDisplayed" in entry) {
    return null;
  }
  // The named line first, and ONLY when both halves are present: a kind that
  // has named copy and an occurrence that carried a name. Either missing falls
  // through to the unnamed sentence, which is why an absent label is an
  // ordinary case here rather than a gap to report.
  const name = item.subject_label ?? "";
  if (name !== "") {
    const named: Readonly<Partial<Record<string, MessageKey>>> =
      NAMED_LINE[item.kind] ?? {};
    const namedKey = named[item.state];
    if (namedKey !== undefined) {
      return t(namedKey, { name });
    }
  }
  const byState: Readonly<Partial<Record<string, MessageKey>>> = entry;
  const key = byState[item.state];
  return key === undefined ? null : t(key);
}

/**
 * Whether a raw string is a kind this build knows.
 *
 * An own-key check rather than a cast, so the narrowing is something the
 * runtime actually did: a newer server's kind is not in the map, and saying so
 * is the whole answer lineFor gives for it.
 */
function isActivityKind(kind: string): kind is ActivityKind {
  return Object.hasOwn(ACTIVITY_LINE, kind);
}

/**
 * The kinds this rail draws, as the server's `kinds` filter takes them.
 *
 * DERIVED from ACTIVITY_LINE rather than listed, and that is the whole point:
 * the server reports every AI task, and `recent` is bounded at ten. A client
 * that asked for everything and drew three kinds would be handed the newest ten
 * of twenty-three — ten it renders nothing for — and the rail would go blank on
 * the day a rep used the composer a lot, while the projection was right the
 * whole time. Naming the kinds moves the bound inside this list; deriving it
 * means the list cannot fall out of step with the copy that decides it.
 */
export function displayedKinds(): ActivityKind[] {
  return displayedLines().map(([kind]) => kind);
}

/**
 * The narrated kinds paired with their line tables.
 *
 * The narrowing happens HERE, once, where `"notDisplayed" in entry` is a check
 * the compiler performs rather than an assertion somebody makes: a caller that
 * looked the entry up again would be holding `LineSet | NotDisplayed` and would
 * need a cast to say what it already knows. Returning the pair means nobody
 * downstream has to.
 *
 * `Object.entries` widens the key to `string`, so the entry list is rebuilt from
 * the map's own keys rather than trusting that widening back — `ACTIVITY_LINE`
 * is a total `Record<ActivityKind, …>`, so its keys ARE the kinds.
 */
export function displayedLines(): [ActivityKind, LineSet][] {
  const kinds = Object.keys(ACTIVITY_LINE) as ActivityKind[];
  return kinds.flatMap((kind) => {
    const entry = ACTIVITY_LINE[kind];
    return "notDisplayed" in entry
      ? []
      : [[kind, entry] as [ActivityKind, LineSet]];
  });
}
