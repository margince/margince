// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The staged-change summary joins `field=value` pairs with ", " (summaryFields
// in the server's compose/agentsummary.go), after a sentence and ": " when the
// server has one. stagedsummary.test.ts holds this against the server's output.
export const FIELD_PAIRS = /^[a-z][a-z0-9_]*=[^,]*(, [a-z][a-z0-9_]*=[^,]*)*$/;

const SEPARATOR = ": ";

/**
 * A staged-change summary, split into the sentence a reader can read and the
 * raw field pairs behind it. Either half may be missing; text that is not a
 * staged summary is all sentence.
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
