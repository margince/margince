// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { ACQUISITION_KINDS } from "./acquisitionkinds";

// The question a source review asks: "How did you receive their contact
// details?". Each answer is written the way a rep remembers it, and each maps
// to one kind of the existing acquisition vocabulary. No answer adds a kind.

export type SourceKind = components["schemas"]["AssessNoticeSource"]["kind"];

/** Every kind a reviewer may confirm. `unknown_legacy` is the gap itself. */
export const SOURCE_KINDS: readonly SourceKind[] = ACQUISITION_KINDS.filter(
  (kind): kind is SourceKind => kind !== "unknown_legacy",
);

export type SourceAnswer =
  | "contacted"
  | "asked"
  | "met"
  | "introduced"
  | "other"
  | "unsure";

// The answers that name one kind on their own.
//
// Meeting face to face maps to `event_or_form`, whatever they agreed to. Their
// agreement to be contacted does not show they were told who holds their
// details, so the Art. 13 duty stays open. `in_person_permission` closes that
// duty, and only an officer who knows the notice was given picks it, under
// "Another source".
const DIRECT: Readonly<
  Record<Exclude<SourceAnswer, "other" | "unsure">, SourceKind>
> = {
  contacted: "subject_initiated",
  asked: "requested_quote_or_meeting",
  met: "event_or_form",
  introduced: "referral",
};

// The kinds the first list does not say in its own words. "Another source"
// opens them as a second list. `event_or_form` is in it too: an event or a web
// form is not always a meeting face to face.
const NAMED_FIRST: ReadonlySet<SourceKind> = new Set([
  "subject_initiated",
  "requested_quote_or_meeting",
  "referral",
]);

export const OTHER_KINDS: readonly SourceKind[] = SOURCE_KINDS.filter(
  (kind) => !NAMED_FIRST.has(kind),
);

export const SOURCE_ANSWERS: readonly SourceAnswer[] = [
  "contacted",
  "asked",
  "met",
  "introduced",
  "other",
  "unsure",
];

export const SOURCE_ANSWER_LABEL: Readonly<Record<SourceAnswer, MessageKey>> = {
  contacted: "notice.source.answer.contacted",
  asked: "notice.source.answer.asked",
  met: "notice.source.answer.met",
  introduced: "notice.source.answer.introduced",
  other: "notice.source.answer.other",
  unsure: "notice.source.answer.unsure",
};

/**
 * The kind an answer confirms, or null when there is nothing to save yet.
 *
 * "Not sure" is always null. It leaves the case unresolved, and the
 * assessment endpoint never receives an invented kind for it. "Another source"
 * is null until the second list has an answer.
 */
export function sourceKindFor(
  answer: SourceAnswer | "",
  other: SourceKind | "",
): SourceKind | null {
  if (answer === "" || answer === "unsure") return null;
  if (answer === "other") return other === "" ? null : other;
  return DIRECT[answer];
}
