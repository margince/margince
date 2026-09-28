// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MarginceCoreState } from "../design-system/margince-core";
import type { MessageKey } from "../i18n/en";
import type { Screen } from "./router";

/**
 * The agent section's own tables.
 *
 * Everything the section reports about the installation is read from the API
 * (`agentrail.tsx`): approvals waiting, which sources are unreachable, the model
 * the last call actually ran on, the account's own suggestions. What the
 * readings are SAID in lives in the message catalogs under `agent.*` — it used
 * to be an English-only table here, which is how a German reader met "Across
 * the workspace" over a translated sentence. What is left here is the tables
 * that are not copy: the state vocabulary, the rotation's order, and the two
 * maps from a cache key to the words for the work behind it.
 */

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
 * the duplicates have their queue, and the licence is stated by the shell
 * banner, the agent panel's pill and the settings card that repairs it.
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
 * What a WRITE is called, keyed by the first segment of its `mutationKey`.
 *
 * Present tense and the reader's own verb, with `%s` where the record's name
 * goes: what a salesperson wants to see is "Enriching zenloop", because that is
 * the sentence that says the tool did the thing they did not want to do. A write
 * whose key carries no id, or whose record nothing can name yet, falls back to
 * the same phrase with the record kind in place of the name.
 *
 * A mutation with no `mutationKey` says nothing at all. That is deliberate: the
 * settings screens, the admin surfaces and the consent flows all write too, and a
 * status line that narrated those would bury the handful of events that are
 * actually the agent doing sales work.
 */
export const WROTE: Readonly<Record<string, [named: string, plain: string]>> = {
  "activity-log": ["Logging activity on %s", "Logging activity"],
  "company-edit": ["Editing %s", "Editing company"],
  "company-new": ["Creating %s", "Creating company"],
  "contact-edit": ["Editing %s", "Editing contact"],
  "contact-new": ["Creating %s", "Creating contact"],
  "deal-edit": ["Editing %s deal", "Editing deal"],
  "deal-new": ["Creating deal for %s", "Creating deal"],
  dedupe: ["Resolving duplicate of %s", "Resolving duplicate"],
  // SENDING, not drafting. The two are told apart by their mutation key rather
  // than by this table: `email` is the send, and the AI draft carries
  // `email-draft` so the rail can own it alone. They used to share one key,
  // which put one action into two vocabularies at once — the bar saying
  // "Writing to Anna" from here while the panel said "I'm drafting your reply."
  // from the server's feed. Deleting this entry fixed that and broke something
  // else: a send is a real write a rep waits on and the rail knows nothing
  // about it, so it went silent. Splitting the key is what serves both.
  email: ["Sending email to %s", "Sending email"],
  enrich: ["Enriching %s", "Enriching contact"],
  ingest: ["Importing %s", "Importing data"],
  "lead-edit": ["Editing %s", "Editing lead"],
  "site-read": ["Reading %s website", "Reading website"],
  "task-new": ["Adding task for %s", "Adding task"],
};

/**
 * The named line, per cache key, with `%s` where the record's name goes.
 *
 * This is the version of the line that does the work Lars asked the surface to
 * do: "Loading zenloop" is a status about the reader's own afternoon, and
 * "Loading company" is a status about software. Only the keys that carry a
 * single record's id are here, because those are the only ones there is a name
 * to put in.
 *
 * The name is never fetched for this (`agentrail-ticker.ts`): it is used when the
 * tab already knows it, and `SAID` below is what shows when it does not.
 */
export const NAMED: Readonly<Record<string, string>> = {
  deal: "Loading %s deal",
  lead: "Loading %s",
  company: "Loading %s",
  company360: "Loading overview of %s",
  contact: "Loading %s",
  contact360: "Loading overview of %s",
  contactBrief: "Summarizing %s",
};

/**
 * What a read in flight is CALLED, keyed by the first segment of its cache key.
 *
 * The line under the orb names one thing at a time (`agentrail-ticker.ts`), and
 * this is the vocabulary it names them in: the words a salesperson uses about
 * their own day, not the words the cache uses about itself. "Loading company
 * overview" is a status; "fetching company360" is a key.
 *
 * A key with no entry here produces NO LINE. That is the point of a table rather
 * than a fallback that opens up the key: half of what a session fetches is
 * plumbing (the session, the feature flags, the custom-field catalog), and a
 * status line that narrated those would bury the two or three events a reader
 * actually cares about.
 */
export const SAID: Readonly<Record<string, string>> = {
  activities: "Loading activities",
  approvals: "Loading approvals",
  "ai-calls": "Loading AI call log",
  "ai-usage": "Loading AI usage",
  companies: "Loading companies",
  company: "Loading company",
  company360: "Loading company overview",
  connectors: "Loading connectors",
  deal: "Loading deal",
  "deal-offers": "Loading deal offers",
  deals: "Loading pipeline",
  dsrs: "Loading privacy requests",
  lead: "Loading lead",
  leads: "Loading leads",
  contacts: "Loading contacts",
  contact: "Loading contact",
  contact360: "Loading contact overview",
  contactBrief: "Summarizing contact",
  pipelines: "Loading pipelines",
  // The brief is written on every open, from the reader's own records, and the
  // rail's own line follows on its next poll: this is the sentence for the
  // second before the feed can name the meeting.
  meetingBrief: "Preparing meeting brief",
  "record-history": "Loading record history",
  tasks: "Loading tasks",
  teams: "Loading teams",
};
