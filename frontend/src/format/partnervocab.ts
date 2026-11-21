import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type Partner = components["schemas"]["Partner"];

export const CERT_LABELS: Record<Partner["cert_status"], MessageKey> = {
  applied: "partner.cert.applied",
  certified: "partner.cert.certified",
  suspended: "partner.cert.suspended",
};

export const MARGIN_TIER_LABELS: Record<
  NonNullable<Partner["margin_tier"]>,
  MessageKey
> = {
  tier1_15: "partner.marginTier.tier1",
  tier2_20: "partner.marginTier.tier2",
  tier3_25: "partner.marginTier.tier3",
};
