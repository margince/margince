import type { MessageKey } from "../i18n/en";

// The cold-start / enrichment field vocabulary (compose/enrichextract.go)
// rendered as human labels. An unmapped field falls back to its key with the
// underscores spaced out: readable, never raw snake_case.
const COLD_FIELD_LABELS: Record<string, MessageKey> = {
  // display_name is the company form's own field, not one a read-back can
  // ground. It shares this map so both surfaces name it the same way.
  display_name: "ob.field.display_name",
  offer_summary: "ob.field.offer_summary",
  icp: "ob.field.icp",
  buying_center: "ob.field.buying_center",
  value_proposition: "ob.field.value_proposition",
  usp: "ob.field.usp",
  customer_pains: "ob.field.customer_pains",
  desired_outcomes: "ob.field.desired_outcomes",
  buying_intents: "ob.field.buying_intents",
  common_objections: "ob.field.common_objections",
  sales_motion: "ob.field.sales_motion",
  legal_name: "ob.field.legal_name",
  registered_address: "ob.field.registered_address",
  register_vat: "ob.field.register_vat",
  legal_form: "ob.field.legal_form",
  register_court: "ob.field.register_court",
  register_number: "ob.field.register_number",
  industry: "ob.field.industry",
  history: "ob.field.history",
};

export function coldFieldLabel(
  field: string,
  t: (key: MessageKey) => string,
): string {
  const key = COLD_FIELD_LABELS[field];
  return key ? t(key) : field.replace(/_/g, " ");
}

// For pure (non-rendering) callers that carry the label key until a component
// translates it. Same map, same fallback contract as coldFieldLabel.
export function coldFieldLabelKey(field: string): MessageKey | undefined {
  return COLD_FIELD_LABELS[field];
}
