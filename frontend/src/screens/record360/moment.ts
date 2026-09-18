// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// THE MOMENT, as every record page READS it: the word for the rule that
// fired, whether the moment belongs in the day's work at all, and whether
// what it rests on says anything the moment has not already said.
//
// Vocabulary, not a row. The contact page and the account brief draw the
// moment with one component (`FoundMove`), and these are the questions both
// of them have to answer the same way — the second page used to reach into
// the first for them, the borrowing that leaves two pages free to drift on
// what "Nothing needed" says or on which evidence a card repeats.

import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";

type ContactMoment = components["schemas"]["ContactMoment"];

// The rule that fired, in one word over the sentence it produced.
//
// The server picks exactly one rule from a fixed ladder, so the word is the
// server's judgement rather than a page's reading of a headline. Named
// rather than left implicit: "Gone quiet" and "Promise overdue" lead to
// different moves, and a reader who sees only the sentence has to infer which
// kind of thing they are looking at.
export const MOMENT_RULE_LABEL = {
  meeting_prep: "contact.moment.rule.meeting_prep",
  re_engaged: "contact.moment.rule.re_engaged",
  job_change: "contact.moment.rule.job_change",
  overdue_promise: "contact.moment.rule.overdue_promise",
  gone_quiet: "contact.moment.rule.gone_quiet",
  open_promise: "contact.moment.rule.open_promise",
  public_signal: "contact.moment.rule.public_signal",
  missing_next_step: "contact.moment.rule.missing_next_step",
  thin_relationship: "contact.moment.rule.thin_relationship",
  nothing_needed: "contact.moment.rule.nothing_needed",
} as const satisfies Record<ContactMoment["rule"], MessageKey>;

/**
 * Whether the moment is a ROW in the day's work, given what else is in the
 * list beside it.
 *
 * Every moment is, except the quiet one — and the quiet one only when there is
 * nothing else. "Nothing needs you today" drawn above a row that asks for
 * something is the panel disagreeing with itself inside one glance, and it is
 * the panel the reader stops believing: the row below it is checkable and the
 * verdict above it is not. The account brief reached that state routinely,
 * because its card fires on owed promises alone while the suggestions under it
 * fire on unanswered mail, a stalled deal, an account with nothing scheduled.
 *
 * Nothing is lost by dropping it. The row goes only where the list has
 * something in it, and what is in the list IS the answer to what needs the
 * reader — the card was not carrying an answer there, it was carrying a wrong
 * one. Where the list is empty the card stands, reason and verb included, and
 * a panel handed no rows at all still has its own one sentence.
 */
export function momentIsARow(
  moment: ContactMoment,
  othersInTheList: boolean,
): boolean {
  return moment.rule !== "nothing_needed" || !othersInTheList;
}

/**
 * Whether the records the moment rests on name anything its headline does not.
 *
 * A promise's headline IS the task's subject, so the single chip under it
 * captions "what this is based on" over the sentence the reader has just
 * finished reading — the claim again in smaller type rather than the working
 * a reader can check. A verbatim snippet is the other case: those are the
 * words the moment was read out of, which no headline carries, so an item
 * carrying one always stands.
 */
export function basisAddsARecord(moment: ContactMoment): boolean {
  const [only, ...rest] = moment.evidence;
  if (!only) {
    return false;
  }
  return (
    rest.length > 0 || Boolean(only.snippet) || !namesIt(moment.headline, only)
  );
}

// The headline names the record when it already contains the record's label.
// Containment rather than equality, and in this direction: the server writes
// the headline FROM the evidence, so "You owe them: send the quote" over
// "Send the quote" is one thing said twice while the reverse would be a
// headline narrower than its own source.
function namesIt(
  headline: string,
  item: ContactMoment["evidence"][number],
): boolean {
  const fold = (text: string) => text.trim().toLowerCase();
  return fold(headline).includes(fold(item.label));
}
