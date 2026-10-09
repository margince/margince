// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";
import { problemFieldErrorsOf, problemMessageOf } from "./common";

// The capture stores vet an address or a domain with one shared parser. Its
// refusal is English written for a developer, so the reader gets the catalog's.
const VALUE_REFUSAL_CODES: ReadonlySet<string> = new Set([
  "invalid_exclusion",
  "invalid_domain",
]);

const VALUE_REFUSAL_COPY: Readonly<Record<string, MessageKey | undefined>> = {
  address: "captureValue.refusedAddress",
  domain: "captureValue.refusedDomain",
};

/** Why an address or domain was not added, in the reader's language. */
export function captureValueMessage(
  error: unknown,
  kind: string | undefined,
  t: (key: MessageKey) => string,
): string {
  const copy = kind === undefined ? undefined : VALUE_REFUSAL_COPY[kind];
  const refused = problemFieldErrorsOf(error).some((problem) =>
    VALUE_REFUSAL_CODES.has(problem.code),
  );
  return copy !== undefined && refused ? t(copy) : problemMessageOf(error, t);
}
