// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MarginceCoreState } from "../design-system/margince-core";
import type { MessageKey } from "../i18n/en";
import type { Screen } from "../lib/router";

/**
 * The agent section's own tables.
 *
 * Everything the section reports about the installation is read from the API
 * (`agentrail.tsx`): approvals waiting, which sources are unreachable, the model
 * the last call actually ran on, the account's own suggestions. What the
 * readings are SAID in lives in the message catalogs under `agent.*`. Here are
 * the state vocabulary, the rotation's order, and the cache-key maps.
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
  // First: the connections read failed, so nobody knows which sources the
  // agent can reach. Every other reading is about a world it may not see.
  // A dropped read is not a fault, so the orb stays idle. But the rotation
  // may not leave this line out because there is other news.
  "sources",
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
 * Present tense and the reader's own verb, with `{name}` for the record:
 * "Enriching zenloop". A write nothing can name yet says the plain phrase.
 * A mutation with no `mutationKey` says nothing, so settings and admin writes
 * stay out of a line about sales work.
 */
export const WROTE: Readonly<
  Record<string, [named: MessageKey, plain: MessageKey]>
> = {
  "activity-log": [
    "agent.ticker.writeNamed.activityLog",
    "agent.ticker.write.activityLog",
  ],
  "company-edit": [
    "agent.ticker.writeNamed.companyEdit",
    "agent.ticker.write.companyEdit",
  ],
  "company-new": [
    "agent.ticker.writeNamed.companyNew",
    "agent.ticker.write.companyNew",
  ],
  "contact-edit": [
    "agent.ticker.writeNamed.contactEdit",
    "agent.ticker.write.contactEdit",
  ],
  "contact-new": [
    "agent.ticker.writeNamed.contactNew",
    "agent.ticker.write.contactNew",
  ],
  "deal-edit": [
    "agent.ticker.writeNamed.dealEdit",
    "agent.ticker.write.dealEdit",
  ],
  "deal-new": ["agent.ticker.writeNamed.dealNew", "agent.ticker.write.dealNew"],
  dedupe: ["agent.ticker.writeNamed.dedupe", "agent.ticker.write.dedupe"],
  // SENDING, not drafting. `email` is the send, and the AI draft carries
  // `email-draft` so the rail narrates it alone and one action never has two
  // vocabularies.
  email: ["agent.ticker.writeNamed.email", "agent.ticker.write.email"],
  enrich: ["agent.ticker.writeNamed.enrich", "agent.ticker.write.enrich"],
  ingest: ["agent.ticker.writeNamed.ingest", "agent.ticker.write.ingest"],
  "lead-edit": [
    "agent.ticker.writeNamed.leadEdit",
    "agent.ticker.write.leadEdit",
  ],
  "site-read": [
    "agent.ticker.writeNamed.siteRead",
    "agent.ticker.write.siteRead",
  ],
  "task-new": ["agent.ticker.writeNamed.taskNew", "agent.ticker.write.taskNew"],
};

/**
 * The named line, per cache key, with `{name}` where the record's name goes.
 *
 * "Loading zenloop" is a status about the reader's own afternoon, and "Loading
 * company" is a status about software. Only the keys that carry a single
 * record's id are here, because those are the only ones there is a name to put
 * in. The name is never fetched for this (`agentrail-ticker.ts`).
 */
export const NAMED: Readonly<Record<string, MessageKey>> = {
  deal: "agent.ticker.readNamed.deal",
  lead: "agent.ticker.readNamed.lead",
  company: "agent.ticker.readNamed.company",
  company360: "agent.ticker.readNamed.company360",
  contact: "agent.ticker.readNamed.contact",
  contact360: "agent.ticker.readNamed.contact360",
  contactBrief: "agent.ticker.readNamed.contactBrief",
};

/**
 * What a read in flight is CALLED, keyed by the first segment of its cache key.
 *
 * The words a salesperson uses about their own day, not the words the cache
 * uses about itself. A key with no entry here produces no line. Half of what a
 * session fetches is plumbing, and narrating it would bury the real events.
 */
export const SAID: Readonly<Record<string, MessageKey>> = {
  activities: "agent.ticker.read.activities",
  approvals: "agent.ticker.read.approvals",
  "ai-calls": "agent.ticker.read.aiCalls",
  "ai-usage": "agent.ticker.read.aiUsage",
  companies: "agent.ticker.read.companies",
  company: "agent.ticker.read.company",
  company360: "agent.ticker.read.company360",
  connectors: "agent.ticker.read.connectors",
  deal: "agent.ticker.read.deal",
  "deal-offers": "agent.ticker.read.dealOffers",
  deals: "agent.ticker.read.deals",
  dsrs: "agent.ticker.read.dsrs",
  lead: "agent.ticker.read.lead",
  leads: "agent.ticker.read.leads",
  contacts: "agent.ticker.read.contacts",
  contact: "agent.ticker.read.contact",
  contact360: "agent.ticker.read.contact360",
  contactBrief: "agent.ticker.read.contactBrief",
  pipelines: "agent.ticker.read.pipelines",
  // The brief is written on every open. This line covers the second before the
  // rail's next poll names the meeting.
  meetingBrief: "agent.ticker.read.meetingBrief",
  "record-history": "agent.ticker.read.recordHistory",
  tasks: "agent.ticker.read.tasks",
  teams: "agent.ticker.read.teams",
};
