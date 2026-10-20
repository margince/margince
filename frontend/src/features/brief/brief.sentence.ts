// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatDayMonth, formatTimeOfDay } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import type { Locale } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isUnprepared, itemTitle, rowHref } from "../worklist/worklist.copy";
import type { Worklist, WorklistItem } from "../worklist/worklist.queries";
import { scheduledMeetings } from "./brief.facts";
import { laneUnread } from "./brief.readings.honesty";

// The Brief's opening sentence, composed from the rows the page is showing.
//
// DETERMINISTIC, AND THAT IS THE POINT. No model writes this. It is built from
// the same queue the section below it draws, using the same copy helpers those
// rows use — so the sentence cannot say something the rows contradict, and it
// needs no pass to have run before the page can speak.
//
// It is also why the sentence carries NO agent tag: indigo means "Margince
// decided this" everywhere in the product, and marking a client-composed
// sentence that way would be a claim about authorship that is simply false.

/**
 * What the sentence says, as a key and the parts to fill it with.
 *
 * A key rather than a built string: this composes in three languages, and a
 * function that concatenated clauses would produce German with English word
 * order. The translator gets a whole sentence with named holes.
 */
export type BriefSentence = Readonly<{
  key: MessageKey;
  values: Readonly<Record<string, string>>;
  /**
   * Where the NAMED lead's own record is, so the words the sentence opens with
   * are the way into the row they describe. Undefined where the row carries no
   * destination — the same `rowHref` the feed's rows are linked by, so the
   * sentence and the row below it cannot send a reader to two places.
   */
  leadHref?: string;
  /** Whom the lead is for, drawn as a chip where the row names a contact. */
  who?: SentenceRecord;
  /** The next meeting, drawn as a chip in the meeting clause. */
  meeting?: SentenceRecord;
}>;

/**
 * What a chip opens under the sentence: where its words came from. A message
 * is cited, never quoted, so its audience rule travels with it.
 */
export type ChipSource =
  | Readonly<{
      kind: "email";
      subject: string | null | undefined;
      occurredAt: string;
      withheld: boolean;
    }>
  | Readonly<{ kind: "note"; from: string; text: string }>;

/** A record the sentence names as a chip, and what pressing it shows. */
export type SentenceRecord = Readonly<{
  id: string;
  label: string;
  source?: ChipSource;
}>;

/** One run of a sentence template: its own words, or a hole to fill. */
export type SentencePart =
  | Readonly<{ kind: "text"; text: string }>
  | Readonly<{ kind: "slot"; name: string }>;

/**
 * A template split into its words and its holes, in order.
 *
 * The Brief's opening sentence puts a LINK in two of its holes, and a string
 * with the holes already filled has nowhere to put one. So the sentence is
 * translated with its holes intact and cut here, which keeps the translator's
 * word order: the lead can open the German sentence and close the Vietnamese
 * one, and neither is assembled from clauses this file joined.
 *
 * The hole pattern is `translate`'s own (`{name}`), and it has to stay that way
 * — a second spelling here would fill holes the translator never wrote.
 */
export function sentenceParts(template: string): readonly SentencePart[] {
  const parts: SentencePart[] = [];
  let at = 0;
  for (const hole of template.matchAll(/\{(\w+)\}/g)) {
    if (hole.index > at) {
      parts.push({ kind: "text", text: template.slice(at, hole.index) });
    }
    parts.push({ kind: "slot", name: hole[1] ?? "" });
    at = hole.index + hole[0].length;
  }
  if (at < template.length) {
    parts.push({ kind: "text", text: template.slice(at) });
  }
  return parts;
}

/** How many rows the sentence is allowed to name. */
const NAMED = 1;

/** The agenda includes approvals, using the worklist's existing review action. */
export function waitingRows(
  day: Worklist | undefined,
): readonly WorklistItem[] {
  return day?.focus?.items ?? [];
}

/**
 * The day in one sentence, or null when there is nothing true to say.
 *
 * Null on an unread day, deliberately. A page that said "nothing needs you"
 * over a read that never landed would send a rep away believing their morning
 * was clear — the same distinction the section below keeps between an empty
 * queue and a failed read.
 */
export function briefSentence(
  day: Worklist | undefined,
  t: (key: MessageKey, values?: Record<string, string>) => string,
  locale: Locale,
  zone: string = viewerZone(),
): BriefSentence | null {
  if (!day?.focus) {
    return null;
  }
  const waiting = waitingRows(day);
  const partial = Boolean(day.readings?.more_available || laneUnread(day));
  const [lead] = waiting;
  if (lead === undefined && partial) return null;
  if (lead === undefined) {
    return { key: "brief.sentence.clear", values: {} };
  }
  // A ROW TITLE IS A CLAUSE, NOT A NOUN PHRASE. `itemTitle` returns whole
  // sentences — "Die Nacht hat das herausgesucht", "43 likely automated
  // senders" — so a template that embedded it ("Start with {lead}.") produced
  // "Fang mit Die Nacht hat das herausgesucht an", which is not German. The
  // title therefore stands as its own sentence and the template says what is
  // true AROUND it.
  //
  // Still the row's own words through the same helpers the row prints them
  // with: a second phrasing here would be a second answer to "what is this row
  // about", and the two drift the first time either moves.
  const who = whoOf(lead, locale, zone);
  // The chip already names whom the lead is for, so the title leaves out the
  // record name it would otherwise carry beside it.
  const titled =
    who && lead.subject?.label === who.label
      ? { ...lead, subject: undefined }
      : lead;
  const values: Record<string, string> = {
    lead: itemTitle(titled, t, locale),
    rest: String(waiting.length - NAMED),
  };
  // The row's OWN destination, through the helper the feed's rows are linked
  // by. A second rule for where the lead goes would let the sentence open a
  // record the row under it does not.
  const leadHref = rowHref(lead);
  const single = waiting.length === NAMED || partial;
  if (who) {
    values.who = who.label;
    return {
      key: single ? "brief.sentence.oneFor" : "brief.sentence.manyFor",
      values,
      leadHref,
      who,
    };
  }
  return {
    key: single ? "brief.sentence.one" : "brief.sentence.many",
    values,
    leadHref,
  };
}

/**
 * The contact a lead is for, citing the message they last sent.
 *
 * It is the contact the server put on the row, the one the card under the
 * sentence names. So the chip and the card cannot name two contacts.
 */
function whoOf(
  lead: WorklistItem,
  locale: Locale,
  zone: string,
): SentenceRecord | undefined {
  const contact = lead.contact;
  if (!contact?.label) return undefined;
  const email = lead.email_summary;
  const source: ChipSource | undefined = email?.occurred_at
    ? {
        kind: "email",
        subject: email.subject,
        occurredAt: formatDayMonth(email.occurred_at, locale, zone),
        withheld: email.display_status === "withheld",
      }
    : undefined;
  return { id: contact.id, label: contact.label, source };
}

/**
 * The clause that names the next meeting still ahead today, or null.
 *
 * Read off the same meeting rows the schedule draws (`scheduledMeetings`), so
 * the sentence cannot name a meeting the Today list does not show.
 */
export function meetingSentence(
  day: Worklist | undefined,
  now: Date,
  t: (key: MessageKey, values?: Record<string, string>) => string,
  locale: Locale,
  zone: string = viewerZone(),
): BriefSentence | null {
  // The queue is in rank order, so the next meeting is the earliest start.
  const next = scheduledMeetings(day)
    .filter((item) => item.due_at && Date.parse(item.due_at) >= now.getTime())
    .sort((a, b) => Date.parse(a.due_at ?? "") - Date.parse(b.due_at ?? ""))[0];
  if (!next?.due_at) return null;
  const label = itemTitle(next, t, locale);
  const time = formatTimeOfDay(next.due_at, locale, zone);
  const text = [
    next.contact?.label,
    isUnprepared(next) ? t("worklist.needsPrep") : undefined,
  ]
    .filter(Boolean)
    .join(" · ");
  return {
    key: "brief.sentence.meeting",
    values: { meeting: label, time },
    meeting: {
      id: next.id,
      label,
      source: text ? { kind: "note", from: time, text } : undefined,
    },
  };
}

/**
 * The row the page opens with.
 *
 * Exported so a test can prove the sentence names the SAME lead the section
 * below draws first — the one property that makes this composition honest
 * rather than decorative.
 */
export function leadOf(day: Worklist | undefined): WorklistItem | undefined {
  return waitingRows(day)[0];
}
