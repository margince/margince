// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { consequenceText, itemTitle } from "./worklist.copy";
import type { Worklist, WorklistItem } from "./worklist.queries";

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
// The overnight narrative is agent-written and IS tagged, one line further
// down, where TodayNarrative draws it.

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
}>;

/** How many rows the sentence is allowed to name. */
const NAMED = 1;

/**
 * What the DECISIONS DECK above already answers, and this page therefore does
 * not count twice.
 *
 * ONE spelling for the whole Brief. The sentence names the lead row and the
 * section below draws it, so a rule kept in two places would let the sentence
 * open with a decision the section deliberately did not show — which is the
 * duplication that rule exists to stop, reappearing one element higher.
 */
const DECK_ANSWERS = "approval";

/** The rows this page is answerable for, in the server's order. */
export function waitingRows(
  day: Worklist | undefined,
): readonly WorklistItem[] {
  // The optional chain reaches the FIELD, not just the payload: an answer that
  // carried no queue must draw nothing rather than throw.
  return day?.queue?.filter((item) => item.source !== DECK_ANSWERS) ?? [];
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
): BriefSentence | null {
  if (!day?.queue) {
    return null;
  }
  const waiting = waitingRows(day);
  if (waiting.length === 0) {
    return { key: "brief.sentence.clear", values: {} };
  }
  const lead = waiting[0];
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
  const values: Record<string, string> = {
    lead: itemTitle(lead, t, locale),
    rest: String(waiting.length - NAMED),
  };
  const consequence = consequenceText(lead, t);
  if (consequence) {
    values.consequence = consequence;
  }
  if (waiting.length === NAMED) {
    return {
      key: consequence ? "brief.sentence.oneWithCost" : "brief.sentence.one",
      values,
    };
  }
  return {
    key: consequence ? "brief.sentence.manyWithCost" : "brief.sentence.many",
    values,
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
