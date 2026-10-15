// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Readers of an RFC-7807 problem body. They import nothing from the screens,
// so lib/problem.ts can re-export them without an import cycle.

// An array is an object too, but no body field read here is ever one.
export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// problemDetails reaches a problem's free-form `details` object, or null when
// the body has none or is not shaped like a problem at all.
export function problemDetails(
  problem: unknown,
): Record<string, unknown> | null {
  return isRecord(problem) && isRecord(problem.details)
    ? problem.details
    : null;
}

// Pull the collided record's id + code out of a duplicate (409) problem body,
// or null when absent / not a duplicate / the row isn't caller-visible.
export function problemExistingId(
  problem: unknown,
): { id: string; code: string } | null {
  const code = problemCode(problem);
  const id = problemDetails(problem)?.existing_id;
  return code && typeof id === "string" && id !== "" ? { id, code } : null;
}

// problemCode pulls the RFC-7807 `code` out of a problem body, or null when
// absent. A caller keys on the server condition (e.g. webhooks_not_configured),
// not the bare HTTP status, which a transient dependency failure can share.
export function problemCode(problem: unknown): string | null {
  return isRecord(problem) && typeof problem.code === "string"
    ? problem.code
    : null;
}

// One assertion a 422 makes about one submitted field. Every validation
// problem carries the top-level `code` "validation_error", so `problemCode`
// cannot tell two refusals apart. Only the field + code pair names the rule.
export type FieldProblem = Readonly<{
  field: string;
  code: string;
  message: string;
}>;

// The top-level code every 422 carries. httperr.Validation is the only
// emitter of the per-field `details.errors[]` shape below.
const VALIDATION_PROBLEM_CODE = "validation_error";

// Pull `details.errors[]` out of a validation problem body, dropping any entry
// that is not a complete {field, code, message}. Filling its holes with empty
// strings would let a caller key on a rule the server never asserted.
// The validation code is required because `details` is a free-form extension
// any problem may carry, and an unrelated `errors` array must not read as one.
export function problemFieldErrors(problem: unknown): FieldProblem[] {
  if (problemCode(problem) !== VALIDATION_PROBLEM_CODE) return [];
  const errors = problemDetails(problem)?.errors;
  if (!Array.isArray(errors)) return [];
  const out: FieldProblem[] = [];
  for (const entry of errors) {
    if (!isRecord(entry)) continue;
    const { field, code, message } = entry;
    if (
      typeof field === "string" &&
      typeof code === "string" &&
      typeof message === "string"
    ) {
      out.push({ field, code, message });
    }
  }
  return out;
}
