import type { MessageKey } from "../i18n/en";

export const COMPANY_LIFECYCLE_STAGES = [
  "unknown",
  "target",
  "prospect",
  "opportunity",
  "customer",
  "former_customer",
  "disqualified",
] as const;

// The same catalog keys the account page's stage badge reads, so the inbox and
// the record cannot call one stage two things. Keyed off the list above:
// a stage added there with no entry here fails the type.
export const COMPANY_LIFECYCLE_LABELS: Readonly<
  Record<(typeof COMPANY_LIFECYCLE_STAGES)[number], MessageKey>
> = {
  unknown: "company.lifecycle.unknown",
  target: "company.lifecycle.target",
  prospect: "company.lifecycle.prospect",
  opportunity: "company.lifecycle.opportunity",
  customer: "company.lifecycle.customer",
  former_customer: "company.lifecycle.former_customer",
  disqualified: "company.lifecycle.disqualified",
};
