// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";
import { problemFieldErrorsOf, problemMessageOf } from "../../lib/problem";

const VALUE_FIELDS: ReadonlySet<string> = new Set(["value", "domain"]);
const VALUE_REFUSAL_CODES: ReadonlySet<string> = new Set([
  "invalid_exclusion",
  "invalid_domain",
]);

// Refusals the server words once for API readers. Each needs the typed value,
// so the catalog sentence names it.
const NAMED_REFUSALS: ReadonlyMap<string, MessageKey> = new Map([
  ["own_domain", "captureValue.ownDomain"],
  ["already_on_shipped_list", "captureValue.alreadyShipped"],
]);

// Only a value with no shape of its kind gets the catalog's sentence. The
// server names every other refusal, such as a public suffix, better.
// The patterns match the reasons `ValidExclusionValue` and `ValidOwnDomain` give.
const SHAPE_REFUSALS: Readonly<
  Record<string, { copy: MessageKey; reason: RegExp } | undefined>
> = {
  address: {
    copy: "captureValue.refusedAddress",
    reason: /^give one email address\b/,
  },
  domain: {
    copy: "captureValue.refusedDomain",
    reason: /^give a (bare )?domain\b| is not a domain$/,
  },
};

/** Why an address or domain was not added, in the reader's language. */
export function captureValueMessage(
  error: unknown,
  kind: string | undefined,
  t: (key: MessageKey, params?: Record<string, string>) => string,
  value = "",
): string {
  for (const problem of problemFieldErrorsOf(error)) {
    const named = NAMED_REFUSALS.get(problem.code);
    if (named !== undefined && VALUE_FIELDS.has(problem.field)) {
      return t(named, { value });
    }
  }
  const shape = kind === undefined ? undefined : SHAPE_REFUSALS[kind];
  if (shape === undefined) {
    return problemMessageOf(error, t);
  }
  const unshaped = problemFieldErrorsOf(error).some(
    (problem) =>
      VALUE_FIELDS.has(problem.field) &&
      VALUE_REFUSAL_CODES.has(problem.code) &&
      shape.reason.test(problem.message),
  );
  return unshaped ? t(shape.copy) : problemMessageOf(error, t);
}
