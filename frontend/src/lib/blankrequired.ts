// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { type FieldLabel, fieldCheckSentence } from "./faultsentence";

// Each required field left blank, keyed for `useFieldFaults().show`, with the
// sentence the server's refusal of the same blank would carry. A label left
// out, as for a question, words it without the name.
export function blankRequired(
  locale: Locale,
  fields: Readonly<
    Record<string, readonly [label: FieldLabel | undefined, value: string]>
  >,
): Map<string, string> {
  return new Map(
    Object.entries(fields)
      .filter(([, [, value]]) => value.trim() === "")
      .map(([key, [label]]) => [
        key,
        fieldCheckSentence("required", locale, label),
      ]),
  );
}
