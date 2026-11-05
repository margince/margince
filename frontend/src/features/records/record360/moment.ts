// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The moment, as every record page reads it: whether what it rests on says
// anything the moment has not already said.
//
// Vocabulary, not a row. Every page that draws a moment answers this the same
// way, so no two pages drift on which evidence a card repeats.

import type { components } from "../../../api/schema";

type ContactMoment = components["schemas"]["ContactMoment"];
type MomentEvidence = ContactMoment["evidence"][number];

/**
 * Whether the records the moment rests on name anything its headline does not.
 *
 * A promise's headline IS the task's subject, so the single chip under it
 * captions "what this is based on" over the sentence the reader has just
 * finished reading — the claim again in smaller type rather than the working
 * a reader can check. A verbatim snippet is the other case: those are the
 * words the moment was read out of, which no headline carries, so an item
 * carrying one always stands.
 *
 * Counted over the DEDUPED evidence, the same key `MomentEvidence` draws
 * with: the server can send one record twice — a task read as evidence and
 * again as the promise it carries — and counting before the duplicate goes
 * calls one chip two, which is enough to draw a caption over a chip that
 * repeats the ask.
 */
export function basisAddsARecord(moment: ContactMoment): boolean {
  const [only, ...rest] = dedupedEvidence(moment.evidence);
  if (!only) {
    return false;
  }
  if (rest.length > 0 || only.snippet) {
    return true;
  }
  return !(COMPOSED_FROM_EVIDENCE.has(moment.rule) && namesIt(moment, only));
}

// The rungs whose headline the server composes FROM the record it rests on —
// the promise rungs write "You owe them: <task subject>", meeting prep writes
// the meeting's own name. Only on these does a headline containing the label
// mean the chip repeats the ask. Every other rung's headline is written about
// the RELATIONSHIP, where a label it happens to contain ("Reply" inside "No
// reply for 14 days") is a different fact and the reader's only way into the
// record.
const COMPOSED_FROM_EVIDENCE: ReadonlySet<ContactMoment["rule"]> = new Set([
  "open_promise",
  "overdue_promise",
  "meeting_prep",
]);

// The headline names the record when it already contains the record's label.
// Containment rather than equality, and in this direction: on these rungs the
// server writes the headline FROM the evidence, so "You owe them: send the
// quote" over "Send the quote" is one thing said twice, while the reverse
// would be a headline narrower than its own source.
function namesIt(moment: ContactMoment, item: MomentEvidence): boolean {
  const fold = (text: string) => text.trim().toLowerCase();
  return fold(moment.headline).includes(fold(item.label));
}

// One entry per record, keyed as `MomentEvidence` keys its chips so the two
// cannot disagree about how many records there are.
function dedupedEvidence(
  evidence: readonly MomentEvidence[],
): MomentEvidence[] {
  return [
    ...new Map(
      evidence.map((item) => [`${item.type}:${item.id ?? item.label}`, item]),
    ).values(),
  ];
}
