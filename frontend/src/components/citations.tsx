// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Grounded prose and the receipts under it.
//
// Every record page renders sentences a model or the deterministic fallback
// wrote, each citing the records it rests on. This is that rendering, once: a
// chip opens wherever its host mounts the door for its kind and is prose
// everywhere else, never a control that does nothing.

import type { components } from "../api/schema";

export {
  type CitationChip,
  type CitedSibling,
  citationChips,
  type OpenReceipt,
} from "./citations.chips";
export { Citations, SentenceList, WrittenBy } from "./citations.list";

/**
 * One sentence of grounded prose, with what it rests on.
 *
 * Typed against the contract's own shared sentence — which the contract
 * spells `CompanyBriefSentence` and uses for the company brief, the deal
 * status card, Contact360 and the growth-fit panel alike. The name is the
 * contract's; the shape has never been a company's.
 */
export type BriefSentence = components["schemas"]["CompanyBriefSentence"];

/** One record a sentence was written from. */
export type Cited = BriefSentence["evidence"][number];

/** Which kind of record a citation points at. */
export type CitedKind = Cited["entity_type"];

/** How prose says which writer produced it. */
export type WrittenByWriter = components["schemas"]["WrittenBy"];

/**
 * The receipt behind one chip: the record's own words, when they are dated,
 * and where they came from. What a reader rests on the chip to see, so the
 * claim is checked against the evidence rather than against the chip's kind
 * repeated back at them. Carried only by a chip standing for ONE record —
 * a count cannot quote.
 */
export type CitationReceipt = {
  quote?: string;
  at?: string;
  origin?: string;
};
