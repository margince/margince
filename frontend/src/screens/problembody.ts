// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Readers of an RFC-7807 problem body. They import nothing from the screens,
// so screens/common.tsx can re-export them without an import cycle.

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

// Pull the collided record's id + code out of a duplicate (409) problem body,
// or null when absent / not a duplicate / the row isn't caller-visible.
export function problemExistingId(
  problem: unknown,
): { id: string; code: string } | null {
  if (!problem || typeof problem !== "object") return null;
  const record = problem as Record<string, unknown>;
  const code = typeof record.code === "string" ? record.code : null;
  const details =
    record.details && typeof record.details === "object"
      ? (record.details as Record<string, unknown>)
      : null;
  const id =
    details && typeof details.existing_id === "string"
      ? details.existing_id
      : null;
  if (code && id) return { id, code };
  return null;
}

// problemCode pulls the RFC-7807 `code` out of a problem body, or null when
// absent. A caller keys on the server condition (e.g. webhooks_not_configured),
// not the bare HTTP status, which a transient dependency failure can share.
export function problemCode(problem: unknown): string | null {
  if (!problem || typeof problem !== "object") return null;
  const record = problem as Record<string, unknown>;
  return typeof record.code === "string" ? record.code : null;
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
  if (!isRecord(problem) || problem.code !== VALIDATION_PROBLEM_CODE) return [];
  if (!isRecord(problem.details)) return [];
  const errors = problem.details.errors;
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
