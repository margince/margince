import type { MessageKey } from "../i18n/en";
import { problemFieldErrorsOf } from "../lib/problem";
import type { CreateField } from "./create.fields";

// The server's field codes that have catalog copy. Any other code shows the
// server's own sentence, which already names the field.
const FIELD_CODE_COPY = new Map<string, MessageKey>([
  ["amount_negative", "create.amountNegative"],
  ["reference_not_found", "create.referenceNotFound"],
]);

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
    // An offered box sends its pick under `pickedKey`: that refusal is its own.
    const field = fields.find(
      (candidate) =>
        (candidate.wireField ?? candidate.key) === problem.field ||
        candidate.offers?.pickedKey === problem.field,
    );
    if (!field) continue;
    const copy = FIELD_CODE_COPY.get(problem.code);
    byField.set(field.key, copy ? t(copy) : problem.message);
  }
  return {
    byField,
    allPlaced: refused.length > 0 && byField.size === refused.length,
  };
}
