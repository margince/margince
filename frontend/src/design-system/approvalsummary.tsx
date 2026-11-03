// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { type Fact, FactList } from "./factlist";

export type SummaryParts = components["schemas"]["ApprovalSummaryParts"];

/** The words a summary's fields are shown in: a label per request field name,
 *  and the line for the fields the list leaves out. */
export type SummaryWords = Readonly<{
  field: (name: string) => string;
  more: (count: number) => string;
}>;

type Summarized = Readonly<{
  summary?: string | null;
  summary_parts?: SummaryParts;
}>;

/**
 * The act a proposal names, without the field values listed after it. A row
 * staged before the parts were kept has only the joined line, which is shown
 * whole.
 */
export function summarySentence(summarized: Summarized): string | null {
  return summarized.summary_parts?.sentence || summarized.summary || null;
}

/**
 * The parts without the fields a surface already shows by name, so a value is
 * not drawn twice. Undefined when nothing is left to list.
 */
export function summaryPartsWithout(
  parts: SummaryParts | undefined,
  shown: ReadonlySet<string>,
): SummaryParts | undefined {
  if (!listable(parts)) return undefined;
  const fields = parts.fields.filter((entry) => !shown.has(entry.field));
  return fields.length > 0 || parts.more_fields > 0
    ? { ...parts, fields }
    : undefined;
}

// Parts with no sentence leave the whole summary as the headline, and that
// already carries the values, so they are not listed again.
function listable(parts: SummaryParts | undefined): parts is SummaryParts {
  return parts !== undefined && parts.sentence !== "";
}

function summaryFacts(parts: SummaryParts, words: SummaryWords): Fact[] {
  const facts: Fact[] = parts.fields.map((entry, index) => ({
    key: `${index}:${entry.field}`,
    term: words.field(entry.field),
    value: entry.value,
  }));
  if (parts.more_fields > 0) {
    facts.push({ key: "more", term: "", value: words.more(parts.more_fields) });
  }
  return facts;
}

/** The fields one line can hold: "Name: Acme, Inc. · Stage: Won". */
export function summaryFieldsLine(
  parts: SummaryParts | undefined,
  words: SummaryWords,
): string | undefined {
  if (!listable(parts)) return undefined;
  const line = summaryFacts(parts, words)
    .map((fact) => (fact.term ? `${fact.term}: ${fact.value}` : fact.value))
    .join(" · ");
  return line || undefined;
}

/** The proposed values under a proposal's sentence, one row per field. */
export function SummaryFields({
  parts,
  words,
}: Readonly<{ parts: SummaryParts | undefined; words: SummaryWords }>) {
  if (!listable(parts)) return null;
  const facts = summaryFacts(parts, words);
  return facts.length > 0 ? <FactList facts={facts} /> : null;
}
