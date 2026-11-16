import type { MessageKey } from "../i18n/en";
import {
  type FieldProblem,
  faultSentenceFor,
  problemFieldErrorsOf,
} from "../lib/problem";
import type { CreateField } from "./create.fields";

// The server's field codes that have catalog copy here. Any other code reads
// the shared wording of a field fault.
const FIELD_CODE_COPY = new Map<string, MessageKey>([
  ["amount_negative", "create.amountNegative"],
  ["reference_not_found", "create.referenceNotFound"],
]);

// The sentence a form shows beside the field one refusal names.
export function fieldRefusalSentence(
  problem: FieldProblem,
  t: (key: MessageKey) => string,
): string {
  const copy = FIELD_CODE_COPY.get(problem.code);
  return copy ? t(copy) : faultSentenceFor(problem, t);
}

export type FieldRefusals = Readonly<{
  // The sentence for each form field the server refused, keyed by field key.
  byField: ReadonlyMap<string, string>;
  // Every refusal found a field to stand beside, so the foot of the form has
  // nothing left to say.
  allPlaced: boolean;
}>;

// A 422 names the wire field. A form field whose key differs (an amount typed
// in major units, sent as amount_minor) declares it as wireField.
export function fieldRefusals(
  error: unknown,
  fields: readonly CreateField[],
  t: (key: MessageKey) => string,
): FieldRefusals {
  const refused = problemFieldErrorsOf(error);
  const byField = new Map<string, string>();
  for (const problem of refused) {
    // An offered box sends its pick under `pickedKey`, and a row list's rows
    // send their values under the row field's key: those refusals are its own.
    const field = fields.find(
      (candidate) =>
        (candidate.wireField ?? candidate.key) === problem.field ||
        candidate.offers?.pickedKey === problem.field ||
        candidate.rowFields?.some((row) => row.key === problem.field),
    );
    if (!field) continue;
    byField.set(field.key, fieldRefusalSentence(problem, t));
  }
  return {
    byField,
    allPlaced: refused.length > 0 && byField.size === refused.length,
  };
}
