import type { components } from "../../api/schema";

export { CERT_LABELS, MARGIN_TIER_LABELS } from "../../format/partnervocab";

export type Partner = components["schemas"]["Partner"];

export type UpsertPartnerRequest =
  components["schemas"]["UpsertPartnerRequest"];

export type CertStatus = Partner["cert_status"];

export type MarginTier = NonNullable<UpsertPartnerRequest["margin_tier"]>;

export const CERT_STATUSES: readonly CertStatus[] = [
  "applied",
  "certified",
  "suspended",
];

export const MARGIN_TIERS: readonly MarginTier[] = [
  "tier1_15",
  "tier2_20",
  "tier3_25",
];
