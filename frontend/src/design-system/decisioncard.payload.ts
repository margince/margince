// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a staged proposal's payload SAYS — read once, in words, with no markup.
//
// Split out of `decisioncard.tsx` because reading an open map and drawing a card
// are two jobs: `proposed_change` is untyped in the contract, so deciding which
// of its keys is a drafted message, which pair is a field change and which
// remainder is worth printing is a parsing question the card then renders the
// answer to. Kept together with the card, that parsing was read past by anybody
// looking for the card's anatomy, and the card's anatomy was read past by
// anybody looking for the parsing.
//
// Every function here returns DATA. The card owns the element each field is
// drawn in, so a caption's markup cannot be decided in two places.

/** The old→new sides of one field the proposal would change. */
export type DecisionDiff = Readonly<{
  field: string;
  from: string | null;
  to: string | null;
}>;

/** The drafted message a send-shaped payload carries. */
export type DecisionDraft = Readonly<{
  subject: string | null;
  body: string | null;
}>;

/**
 * What one payload field is called and how to read it, already resolved into
 * the reader's own language by the caller.
 *
 * Resolved rather than looked up here, for the reason DecisionToolChip takes a
 * verb rather than a kind: which fields a kind shows is the product's
 * vocabulary, and a primitive holding a copy of it would be a second author of
 * it. This tier knows how to DRAW a labelled fact; it does not know that a
 * close-date correction has a `basis`.
 */
export type DecisionDisplay = Readonly<{
  /** The payload key this describes. */
  field: string;
  /** What to call it, in the reader's language. */
  label: string;
  /** The value, already formatted — a date on the reader's calendar, an enum
   *  in words. Null where the payload does not carry the field. */
  value: string | null;
  /** Leads the body as a sentence rather than sitting in the fact list. */
  lead?: boolean;
}>;

/** One remaining payload field, as the two strings a fact list draws. */
export type PayloadField = Readonly<{
  /** The payload key, which is also the row's identity in the list. */
  key: string;
  /** What the row is called: a declared caption, or the wire key itself. */
  label: string;
  value: string;
}>;

// A payload value as one line of text. `proposed_change` is an open map in the
// contract, so anything can be under any key: a string is shown as written, a
// scalar as its own digits, and a nested document as its JSON rather than as
// "[object Object]" — which is what a reader saw on the one card that hit it.
export function asText(value: unknown): string | null {
  if (value === null || value === undefined) {
    return null;
  }
  if (typeof value === "string") {
    return value.trim() === "" ? null : value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return JSON.stringify(value);
}

// The drafted message, for the kinds whose whole question is words somebody is
// about to send in the reader's name (`held_draft`, `send_email`). Narrowed
// rather than asserted: a kind that puts something other than a string under
// `subject` reads as no subject at all, which is what the inbox already did.
export function draftOf(
  change: Readonly<Record<string, unknown>>,
): DecisionDraft {
  const subject = typeof change.subject === "string" ? change.subject : null;
  const body = typeof change.body === "string" ? change.body : null;
  return { subject: asText(subject), body: asText(body) };
}

// The `current_<name>` / `proposed_<name>` pairs, which is how this product's
// stagers spell a field change: `compose/signalproposals.go` puts
// current_lifecycle beside proposed_lifecycle precisely so "the card must show
// both sides", and the company context surface reads current_value /
// proposed_value the same way. A `proposed_` key with no sibling is NOT a diff
// — it is a value the proposal adds, and drawing it against a struck-through
// blank would claim we know the old one was empty.
const PROPOSED = "proposed_";
const CURRENT = "current_";

export function diffsOf(
  change: Readonly<Record<string, unknown>>,
): readonly DecisionDiff[] {
  const diffs: DecisionDiff[] = [];
  for (const [key, value] of Object.entries(change)) {
    if (!key.startsWith(PROPOSED)) {
      continue;
    }
    const field = key.slice(PROPOSED.length);
    const currentKey = `${CURRENT}${field}`;
    if (!Object.hasOwn(change, currentKey)) {
      continue;
    }
    diffs.push({ field, from: asText(change[currentKey]), to: asText(value) });
  }
  return diffs;
}

// Everything the readings above did not consume, as label→value rows.
//
// A kind that declares a display policy shows exactly what it declared: the
// caller resolved those fields, so the payload's remaining keys are identifiers
// and bookkeeping that answer nothing a contact was asked. Printing them was how
// a business question came to read as a database row — `deal_id`,
// `target_version`, `flags: ["unrealistic_stale"]` under a headline about a
// deal going quiet.
//
// A kind that declares nothing keeps the old reading: wire keys as written.
// That is not a nicer fallback, it is an honest one — the raw-args kinds carry
// an agent's tool arguments or an automation's action, with no typed payload to
// describe, and inventing captions for a bag of unknown keys would be guessing
// at what the software meant. Drawn only in the deck layout, where the row
// offers a way through to the whole payload instead.
export function restFields(
  change: Readonly<Record<string, unknown>>,
  draft: DecisionDraft,
  diffs: readonly DecisionDiff[],
  display: readonly DecisionDisplay[],
): readonly PayloadField[] {
  if (display.length > 0) {
    return display.flatMap((entry) =>
      entry.lead || entry.value === null
        ? []
        : [{ key: entry.field, label: entry.label, value: entry.value }],
    );
  }
  const consumed = new Set<string>();
  if (draft.subject) {
    consumed.add("subject");
  }
  if (draft.body) {
    consumed.add("body");
  }
  for (const diff of diffs) {
    consumed.add(`${PROPOSED}${diff.field}`);
    consumed.add(`${CURRENT}${diff.field}`);
  }
  return Object.entries(change).flatMap(([key, value]) => {
    const text = consumed.has(key) ? null : asText(value);
    return text === null ? [] : [{ key, label: key, value: text }];
  });
}
