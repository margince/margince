import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type Company = components["schemas"]["Company"];

// What the account is TO US, multi-valued. Typed against the
// schema union, so a value added upstream fails the build here rather than
// reaching a reader as a raw enum.
export type RelationshipType = NonNullable<
  Company["relationship_types"]
>[number];

export const RELATIONSHIP_TYPE_LABELS: Record<RelationshipType, MessageKey> = {
  customer: "company.relType.customer",
  partner: "company.relType.partner",
  supplier: "company.relType.supplier",
  investor: "company.relType.investor",
  portfolio_company: "company.relType.portfolio_company",
  competitor: "company.relType.competitor",
  other: "company.relType.other",
};
