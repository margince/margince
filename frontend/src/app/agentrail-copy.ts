// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MarginceCoreState } from "../design-system/margince-core";
import type { MessageKey } from "../i18n/en";
import type { Screen } from "./router";

/**
 * The agent section's own copy.
 *
 * Everything the section reports about the installation is read from the API
 * (`agentrail.tsx`): approvals waiting, which sources are unreachable, the model
 * the last call actually ran on, the account's own suggestions. What is left
 * here is the words those readings are said in.
 */

export const LABELS = {
  /** The month's estimated spend, and it says estimated by saying "so far":
   *  the server prices on read, so the figure moves as rates change. */
  spend: "Cost this month",
  /** The same fact in the width a collapsed rail has: the figure leads and
   *  this names it. One word rather than the panel's sentence, because the
   *  rail is 235px wide and the figure must not be what gets shortened. */
  spendScope: "this month",
  /** Under the figure in the panel head, where the label is the second line. */
  thisMonth: "this month",
  /** The panel's own name. It is portalled to the body, so it is not inside the
   *  region that would otherwise have named it. */
  panel: "Margince agent detail",
  acrossWorkspace: "Across the workspace",
  runtime: "Runtime",
  recap: "What it has done",
  justNow: "just now",
  fullLog: "Full log",
  /** The recap when the feed answered and this contact's day holds nothing yet.
   *  It is bounded to what SETTLED today, so an empty list is a quiet morning
   *  rather than an agent that has never run. */
  nothingToday: "nothing has finished today",
  model: "model",
  sources: "sources",
  tools: "tools",
  approvals: "Decisions waiting",
  offline: "offline",
  idle: "Idle",
  reading: "Loading",
  working: "Working",
  unreachable: "Cannot reach Margince",
  /** A broken run whose kind this build writes no sentence for. The feed is
   *  asked only for the kinds the rail narrates, so these two are the words for
   *  a server that answered with more than it was asked, never the daily case. */
  runFailed: "A run failed",
  runStopped: "A run stopped early",
  waiting: "waiting for you",
  cannotReach: "Cannot reach",
  reconnect: "Reconnect",
  configure: "Set up",
  noModel: "No AI model is configured",
  devModel: "development (offline fake)",
  devLine: "Running on the offline model",
  duplicatesRow: "Duplicate pairs open",
  expand: "Expand the agent panel",
  collapse: "Collapse the agent panel",
  region: "Margince agent",
  unreadable: "not readable on this seat",
  noCallsYet: "nothing has run yet",
  /** The resting line when every read came back with nothing to report. */
  allClear: "Nothing needs you",
  /** The shape of the month's spend, for the reader who cannot see the line. */
  spendShape: "What the agent has cost, day by day",
  /** The panel's one setting: whether the window's own margins light while the
   *  agent works. Named for what a reader SEES rather than for the surface that
   *  draws it — nobody outside this tree calls it the edge. */
  edgeLight: "Screen edge light",
  nothingPriced: "nothing priced yet",
  runningOn: "running on",
} as const;

/**
 * The whole vocabulary the Core is drawn in, in lifecycle order.
 *
 * Written out rather than derived from what the section happens to reach today:
 * it is the list a state has to be ADDED to, and `agentrail.test.tsx` reads it to
 * hold every state to a tone rule of its own in the stylesheet.
 */
export const VOCABULARY: readonly MarginceCoreState[] = [
  "idle",
  "ingest",
  "working",
  "warning",
  "error",
];

/** The states whose line describes work still running. */
export const RUNNING: ReadonlySet<MarginceCoreState> = new Set([
  "ingest",
  "working",
]);

/**
 * The READINGS the section rotates through while the agent is at rest.
 *
 * All three are the AGENT's own facts, and that is the whole selection rule.
 * This rotation used to carry six, and the other three were about the
 * installation rather than the agent: the duplicate queue, the month's spend,
 * the licence. Six subjects taking turns in one slot at the edge of every screen
 * is why a reader could not say what the line was for. What went is not lost,
 * it moved to where it is acted on: the spend has its own figure in the block,
 * the duplicates have their queue, and the licence is stated on the card in
 * settings that can actually repair it.
 *
 * Nothing here is invented activity. A resting surface that narrated fake work
 * would teach a reader that none of the line means anything, which is exactly
 * the credit this surface cannot afford to spend.
 *
 * Each entry is a function of what was read, and one that has nothing to report
 * returns null and is skipped, so the rotation is only ever as long as the facts
 * are. `finished` is the one that can contribute SEVERAL lines: a day settles
 * more than one run, and the newest of them being the only one ever said is what
 * left this surface announcing one summary from breakfast until the evening.
 */
export const IDLE_ORDER = [
  "waiting",
  // Second: what the scheduled runner finished while nobody was looking is news
  // rather than a task, so it does not push the queue a contact has to answer
  // down the rotation.
  "finished",
  // Last, and standing rather than daily: an installation on the development
  // path answers every call with an invention, and a reader who does not know
  // that is being misled by a product that looks like it works.
  "model",
] as const;

export type IdleKind = (typeof IDLE_ORDER)[number];

/** One thing that is true of the product, and where it is true of. */
export type Tip = Readonly<{
  /** The sentence, in the reader's own locale. */
  key: MessageKey;
  /**
   * The destination the sentence's `{name}` slot names, drawn as the way there.
   * Null for a tip about no place in particular — and a tip is SUPPRESSED on
   * the screen it names, because telling somebody to go where they already are
   * is the surface admitting it is not reading the room.
   */
  to: Readonly<{ screen: Screen; labelKey: MessageKey }> | null;
}>;

/**
 * What the section says between readings, in the order it says them.
 *
 * NOT readings, and the distinction is what keeps this list honest. Every line
 * in `IDLE_ORDER` is something the installation was asked and answered. Every
 * line here is something that is true of the PRODUCT, whoever is looking and
 * whenever they look — so none of them claims the agent did anything, and the
 * rule that this surface never invents activity is untouched.
 *
 * They exist because the readings run out, and the rotation was built as though
 * they would not. An installation with a clean queue, a bound model and a quiet
 * afternoon has exactly one true reading — "Nothing needs you" — and one that
 * ran a summary this morning has that summary and nothing else, all day. A line
 * at the edge of every screen that has said the same sentence since breakfast is
 * not a status light any more; it is furniture, and a reader stops seeing it
 * long before they stop believing it.
 *
 * ONE of these shows per pass through the readings, and the next pass shows the
 * next (`agentrail-resting.ts`). That cadence is the point: a tip is the thing
 * the rail says once it has run out of news, never a sixth voice competing with
 * the queue a contact has to answer.
 *
 * The bar for a line being here is that it is CHECKABLE. Each one is about a
 * part of the product a reader can go and find, so a tip that stops being true
 * is a tip somebody notices — which is what stops this list from drifting into
 * the cheerful nothing the readings are so careful not to be.
 */
export const TIPS: readonly Tip[] = [
  // What Home is FOR, which is the thing a new reader most often has not
  // worked out: the decisions, the tasks and the duplicate pairs are lanes
  // inside it rather than three queues to go looking for.
  {
    key: "agent.tip.day",
    to: { screen: "home", labelKey: "nav.brief" },
  },
  // The palette is the ask surface, and nothing in the chrome says so: the
  // hint lives on one screen (`ai.paletteHint`), which is the screen a reader
  // reaches by already knowing.
  { key: "agent.tip.ask", to: null },
  // The panel this block opens. A reader who has never opened it has no idea
  // the agent keeps a log of its own, and the recap is the answer to the
  // question the orb raises.
  { key: "agent.tip.recap", to: null },
  // The lit margins. Ambient light nobody explained is ambient light somebody
  // assumes is a fault, and the setting that turns it off is inside the panel
  // above.
  { key: "agent.tip.edge", to: null },
];

/**
 * The named line, per cache key, with `%s` where the record's name goes.
 *
 * This is the version of the line that does the work Lars asked the surface to
 * do: "Reading zenloop" is a sentence about the reader's own afternoon, and
 * "Reading a company" is a sentence about software. Only the keys that carry a
 * single record's id are here, because those are the only ones there is a name
 * to put in.
 *
 * The name is never fetched for this (`agentrail-ticker.ts`): it is used when the
 * tab already knows it, and `SAID` below is what shows when it does not.
 */
/**
 * What a WRITE is called, keyed by the first segment of its `mutationKey`.
 *
 * Present tense and the reader's own verb, with `%s` where the record's name
 * goes: what a salesperson wants to see is "Enriching zenloop", because that is
 * the sentence that says the tool did the thing they did not want to do. A write
 * whose key carries no id, or whose record nothing can name yet, falls back to
 * the same phrase with the article in place of the name.
 *
 * A mutation with no `mutationKey` says nothing at all. That is deliberate: the
 * settings screens, the admin surfaces and the consent flows all write too, and a
 * status line that narrated those would bury the handful of events that are
 * actually the agent doing sales work.
 */
export const WROTE: Readonly<Record<string, [named: string, plain: string]>> = {
  "activity-log": ["Logging an activity on %s", "Logging an activity"],
  "company-edit": ["Editing %s", "Editing a company"],
  "company-new": ["Creating %s", "Creating a company"],
  "contact-edit": ["Editing %s", "Editing a contact"],
  "contact-new": ["Creating %s", "Creating a contact"],
  "deal-edit": ["Editing the %s deal", "Editing a deal"],
  "deal-new": ["Creating a deal on %s", "Creating a deal"],
  dedupe: ["Deciding a duplicate of %s", "Deciding a duplicate"],
  // SENDING, not drafting. The two are told apart by their mutation key rather
  // than by this table: `email` is the send, and the AI draft carries
  // `email-draft` so the rail can own it alone. They used to share one key,
  // which put one action into two vocabularies at once — the bar saying
  // "Writing to Anna" from here while the panel said "I'm drafting your reply."
  // from the server's feed. Deleting this entry fixed that and broke something
  // else: a send is a real write a rep waits on and the rail knows nothing
  // about it, so it went silent. Splitting the key is what serves both.
  email: ["Writing to %s", "Writing an email"],
  enrich: ["Enriching %s", "Enriching a contact"],
  ingest: ["Ingesting %s", "Ingesting"],
  "lead-edit": ["Editing %s", "Editing a lead"],
  "site-read": ["Reading the %s website", "Reading a website"],
  "task-new": ["Adding a task on %s", "Adding a task"],
};

export const NAMED: Readonly<Record<string, string>> = {
  deal: "Reading the %s deal",
  lead: "Reading %s",
  company: "Reading %s",
  company360: "Reading everything about %s",
  contact: "Reading %s",
  contact360: "Reading everything about %s",
  contactBrief: "Summarising %s",
};

/**
 * What a read in flight is CALLED, keyed by the first segment of its cache key.
 *
 * The line under the orb names one thing at a time (`agentrail-ticker.ts`), and
 * this is the vocabulary it names them in: the words a salesperson uses about
 * their own day, not the words the cache uses about itself. "Reading this
 * company" is a sentence; "fetching company360" is a key.
 *
 * A key with no entry here produces NO LINE. That is the point of a table rather
 * than a fallback that opens up the key: half of what a session fetches is
 * plumbing (the session, the feature flags, the custom-field catalog), and a
 * status line that narrated those would bury the two or three events a reader
 * actually cares about.
 */
export const SAID: Readonly<Record<string, string>> = {
  activities: "Reading the activity trail",
  approvals: "Checking what needs you",
  "ai-calls": "Reading its own log",
  "ai-usage": "Adding up what it spent",
  companies: "Reading companies",
  company: "Reading a company",
  company360: "Reading everything about this company",
  connectors: "Checking its sources",
  deal: "Reading a deal",
  "deal-offers": "Reading the offers on a deal",
  deals: "Reading the pipeline",
  dsrs: "Checking privacy requests",
  lead: "Reading a lead",
  leads: "Reading leads",
  contacts: "Reading contacts",
  contact: "Reading a contact",
  contact360: "Reading everything about this contact",
  contactBrief: "Summarising a contact",
  pipelines: "Reading the pipeline",
  // The brief is written on every open, from the reader's own records, and the
  // rail's own line follows on its next poll: this is the sentence for the
  // second before the feed can name the meeting.
  meetingBrief: "Preparing a meeting brief",
  "record-history": "Reading what changed",
  tasks: "Reading your tasks",
  teams: "Reading the team",
};
