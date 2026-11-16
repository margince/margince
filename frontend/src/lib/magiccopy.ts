// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { providerName } from "../format/connectorprovider";
import type { Translator } from "../i18n";
import { approvalKindLabel } from "./approvalkindlabel";
import { runOutcomeWord, runReasonWords } from "./automationrun";
import { historyFieldLabel } from "./historyfieldlabels";
import type { MagicLine } from "./magic.queries";
import { magicSentenceKey } from "./magickeys";

/** What a line says happened, without the record it happened to. */
export function lineSentence(line: MagicLine, t: Translator): string {
  const sentence = magicSentenceKey(line.summary.key);
  if (sentence) {
    return t(sentence, readableValues(line.summary.values, t));
  }
  return t(
    line.lane === "needs_you"
      ? "magic.approval.reviewChange"
      : "magic.undescribed",
  );
}

// A proposal's kind and a connector's provider arrive as codes, not words.
function readableValues(
  values: Readonly<Record<string, string>> | undefined,
  t: Translator,
): Record<string, string> | undefined {
  if (!values) return values;
  return {
    ...values,
    ...(values.kind && { kind: approvalKindLabel(values.kind, t) }),
    ...(values.fields && { fields: fieldWords(values.fields, t) }),
    ...(values.provider && { provider: providerName(values.provider, t) }),
    ...(values.outcome && { outcome: runOutcomeWord(values.outcome, t) }),
    ...(values.reason_code && {
      reason: runReasonWords(values.reason_code, t),
    }),
  };
}

// The server names the changed fields by column, with spaces for underscores;
// each is read as the record page names it.
function fieldWords(fields: string, t: Translator): string {
  return fields
    .split(", ")
    .map((field) => historyFieldLabel(field.replaceAll(" ", "_"), t))
    .join(", ");
}

/** A grouped line stays until every receipt it represents is shown elsewhere. */
export function doneLinesApart(
  lines: readonly MagicLine[],
  receiptIds: readonly string[],
): MagicLine[] {
  const shown = new Set(receiptIds);
  return lines.filter(
    (line) =>
      !line.receipt_ids?.length ||
      !line.receipt_ids.every((id) => shown.has(id)),
  );
}
