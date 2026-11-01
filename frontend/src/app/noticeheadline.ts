// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";

// The staged-change summary joins `field=value` pairs with ", " (summaryFields
// in the server's compose/agentsummary.go). Past eight it ends in "+N more".
// A sentence and ": " come first when the server has one.
const KEY = String.raw`\p{Ll}[\p{Ll}\p{N}_]*`;
export const FIELD_PAIRS = new RegExp(
  String.raw`^${KEY}=[^,]*(, ${KEY}=[^,]*)*(, \+\d+ [^,=]+)?$`,
  "u",
);

const SEPARATOR = ": ";

/**
 * A staged-change summary, split into its sentence and the raw field pairs
 * behind it. Text the pattern cannot read whole stays all sentence, which is
 * how it showed before, so a failed split loses nothing.
 */
export function splitStagedSummary(
  summary: string,
): Readonly<{ sentence?: string; fields?: string }> {
  if (FIELD_PAIRS.test(summary)) return { fields: summary };
  for (
    let at = summary.indexOf(SEPARATOR);
    at > 0;
    at = summary.indexOf(SEPARATOR, at + 1)
  ) {
    const fields = summary.slice(at + SEPARATOR.length);
    if (FIELD_PAIRS.test(fields)) {
      return { sentence: summary.slice(0, at), fields };
    }
  }
  return { sentence: summary };
}

/**
 * What a notice is headed with.
 *
 * An approval notice carries the staged change's summary as its subject: a
 * sentence, then the change's raw field pairs. The sentence heads the row and
 * the pairs stay as detail. With no sentence, the row is headed by what the
 * notice is.
 */
export function noticeHeadline(
  notice: Readonly<{ kind: string; subject: string }>,
  t: Translator,
): { headline: string; detail?: string } {
  if (notice.kind !== "approval_pending") {
    return { headline: notice.subject };
  }
  const { sentence, fields } = splitStagedSummary(notice.subject);
  const headline = sentence ?? t("notifications.class.approval_pending.label");
  return fields ? { headline, detail: fields } : { headline };
}
