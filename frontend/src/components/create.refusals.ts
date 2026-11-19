import type { MessageKey } from "../i18n/en";
import {
  type FieldProblem,
  faultSentenceFor,
  refusalSentences,
} from "../lib/problem";
import { type CreateField, fieldLabel } from "./create.fields";

// The sentence a form shows beside the field one refusal names.
export function fieldRefusalSentence(
  problem: FieldProblem,
  t: (key: MessageKey) => string,
): string {
  return faultSentenceFor(problem, t);
}

export type FieldRefusals = Readonly<{
  // The sentence for each form field the server refused, keyed by field key.
  byField: ReadonlyMap<string, string>;
  // Every refusal found a field to stand beside, so the foot of the form has
  // nothing left to say.
  allPlaced: boolean;
}>;

// A 422 names the wire field. A form field whose key differs (an amount typed
// in major units, sent as amount_minor) declares it as wireField. An offered
// box sends its pick under `pickedKey`.
export function fieldRefusals(
  error: unknown,
  fields: readonly CreateField[],
  t: (key: MessageKey) => string,
): FieldRefusals {
  const labels: Record<string, string> = {};
  const fieldKeys = new Map<string, string>();
  for (const field of fields) {
    for (const wire of [
      field.wireField ?? field.key,
      field.offers?.pickedKey,
    ]) {
      if (wire === undefined || fieldKeys.has(wire)) continue;
      labels[wire] = fieldLabel(field, t);
      fieldKeys.set(wire, field.key);
    }
  }
  const refused = refusalSentences(error, t, labels);
  const byField = new Map<string, string>();
  for (const [wire, sentence] of refused.byField) {
    byField.set(fieldKeys.get(wire) ?? wire, sentence);
  }
  return { byField, allPlaced: byField.size > 0 && refused.rest === null };
}
