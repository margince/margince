import type { MessageKey } from "../../i18n/en";
import type { CompanyFieldName } from "../onboarding";

/**
 * What to say about a field the read came back without: the hint explains
 * what the answer should contain and why the CRM wants it, the example shows
 * what a filled-in answer looks like. Lives here, not inline in the card,
 * because it is a lookup by field rather than something the card's own JSX
 * decides — the same shape `coldFieldLabel`'s map already keeps for the
 * label, one level up.
 *
 * Keyed loosely rather than on `CompanyFieldName` so a field this product
 * does not ask the deck about (`website`) is not forced to carry an entry.
 */
const FIELD_GUIDANCE: Record<
  string,
  { hint: MessageKey; example: MessageKey }
> = {
  display_name: {
    hint: "ob.fieldHint.display_name",
    example: "ob.fieldEg.display_name",
  },
  offer_summary: {
    hint: "ob.fieldHint.offer_summary",
    example: "ob.fieldEg.offer_summary",
  },
  icp: { hint: "ob.fieldHint.icp", example: "ob.fieldEg.icp" },
  buying_center: {
    hint: "ob.fieldHint.buying_center",
    example: "ob.fieldEg.buying_center",
  },
  value_proposition: {
    hint: "ob.fieldHint.value_proposition",
    example: "ob.fieldEg.value_proposition",
  },
  usp: { hint: "ob.fieldHint.usp", example: "ob.fieldEg.usp" },
  customer_pains: {
    hint: "ob.fieldHint.customer_pains",
    example: "ob.fieldEg.customer_pains",
  },
  desired_outcomes: {
    hint: "ob.fieldHint.desired_outcomes",
    example: "ob.fieldEg.desired_outcomes",
  },
  buying_intents: {
    hint: "ob.fieldHint.buying_intents",
    example: "ob.fieldEg.buying_intents",
  },
  common_objections: {
    hint: "ob.fieldHint.common_objections",
    example: "ob.fieldEg.common_objections",
  },
  sales_motion: {
    hint: "ob.fieldHint.sales_motion",
    example: "ob.fieldEg.sales_motion",
  },
  legal_name: {
    hint: "ob.fieldHint.legal_name",
    example: "ob.fieldEg.legal_name",
  },
  registered_address: {
    hint: "ob.fieldHint.registered_address",
    example: "ob.fieldEg.registered_address",
  },
  register_vat: {
    hint: "ob.fieldHint.register_vat",
    example: "ob.fieldEg.register_vat",
  },
  legal_form: {
    hint: "ob.fieldHint.legal_form",
    example: "ob.fieldEg.legal_form",
  },
  register_court: {
    hint: "ob.fieldHint.register_court",
    example: "ob.fieldEg.register_court",
  },
  register_number: {
    hint: "ob.fieldHint.register_number",
    example: "ob.fieldEg.register_number",
  },
  industry: { hint: "ob.fieldHint.industry", example: "ob.fieldEg.industry" },
  history: { hint: "ob.fieldHint.history", example: "ob.fieldEg.history" },
};

export function fieldGuidance(
  field: CompanyFieldName,
): { hint: MessageKey; example: MessageKey } | undefined {
  return FIELD_GUIDANCE[field];
}
