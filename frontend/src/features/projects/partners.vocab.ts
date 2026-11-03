import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";

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

export const CERT_LABELS: Record<CertStatus, MessageKey> = {
  applied: "partner.cert.applied",
  certified: "partner.cert.certified",
  suspended: "partner.cert.suspended",
};

export const MARGIN_TIER_LABELS: Record<MarginTier, MessageKey> = {
  tier1_15: "partner.marginTier.tier1",
  tier2_20: "partner.marginTier.tier2",
  tier3_25: "partner.marginTier.tier3",
};
