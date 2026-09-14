// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The billing classification as the wire takes it.
//
// Its own module because the same pair is written from two forms — the product
// editor and the offer line editor — and the server refuses half of one from
// either. A second copy of this rule would eventually disagree with the first
// about what an empty selection means, and an empty selection is the answer
// every row written before this field existed carries.

// The classification as the wire takes it: a pair, or a pair of nulls.
//
// Sent together because the server refuses half of one — a model with no
// cadence, or a cadence with no model, is not a classification anybody can
// read. An empty selection means unclassified, which is a real answer and the
// one the field defaults to.
// text narrows one value off a form's Record<string, unknown> without
// asserting: a field the reader left alone is absent, not an empty string.
function text(value: unknown): string {
  return typeof value === "string" ? value : "";
}

export function billingOf(values: Record<string, unknown>): {
  billing_model: "one_time" | "recurring" | null;
  billing_interval_months: 1 | 3 | 6 | 12 | null;
} {
  const model = text(values.billing_model);
  if (model !== "one_time" && model !== "recurring") {
    return { billing_model: null, billing_interval_months: null };
  }
  if (model === "one_time") {
    // A one-off price has no cadence, whatever the other control still shows.
    return { billing_model: "one_time", billing_interval_months: null };
  }
  const months = Number(text(values.billing_interval_months));
  return {
    billing_model: "recurring",
    billing_interval_months:
      months === 1 || months === 3 || months === 6 || months === 12
        ? months
        : null,
  };
}

/**
 * The same classification for an UPDATE, where saying nothing needs a word.
 *
 * On a create, a null model is unambiguous: the product is born unclassified.
 * On a patch it is not — the generated field is a pointer, so a JSON null and
 * an omitted field both arrive as nil, and an omitted field has to keep meaning
 * "leave the classification alone". Without `not_specified`, the form's "Not
 * specified" option would be a control that silently does nothing, and a
 * product once classified could never be un-classified.
 */
export function billingPatchOf(values: Record<string, unknown>): {
  billing_model: "one_time" | "recurring" | "not_specified";
  billing_interval_months: 1 | 3 | 6 | 12 | null;
} {
  const settled = billingOf(values);
  return {
    billing_model: settled.billing_model ?? "not_specified",
    billing_interval_months: settled.billing_interval_months,
  };
}
