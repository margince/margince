import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type Company = components["schemas"]["Company"];
export type Lifecycle = NonNullable<Company["lifecycle"]>;

// Where the account stands with us, in wire order, so a picker reads as a
// progression rather than an alphabet. The one list every lifecycle picker,
// filter and approval editor offers; companylookups.ts re-exports it.
export const LIFECYCLE_OPTIONS = [
  "unknown",
  "target",
  "prospect",
  "opportunity",
  "onboarding",
  "live",
  "customer",
  "former_customer",
  "disqualified",
] as const satisfies readonly Lifecycle[];

// Typed against the schema union, so a stage added upstream with no label
// fails the build rather than reaching a reader as a raw enum. The inbox and
// the record read the same keys, so neither can call one stage two things.
export const LIFECYCLE_LABELS: Readonly<Record<Lifecycle, MessageKey>> = {
  unknown: "company.lifecycle.unknown",
  target: "company.lifecycle.target",
  prospect: "company.lifecycle.prospect",
  opportunity: "company.lifecycle.opportunity",
  onboarding: "company.lifecycle.onboarding",
  live: "company.lifecycle.live",
  customer: "company.lifecycle.customer",
  former_customer: "company.lifecycle.former_customer",
  disqualified: "company.lifecycle.disqualified",
};
