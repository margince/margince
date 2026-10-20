import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";

export type Partner = components["schemas"]["Partner"];

export type UpsertPartnerRequest =
  components["schemas"]["UpsertPartnerRequest"];

export type PartnerRole = NonNullable<UpsertPartnerRequest["partner_role"]>;

export type CertStatus = Partner["cert_status"];

export type MarginTier = NonNullable<UpsertPartnerRequest["margin_tier"]>;

export type RelationshipStage = Partner["relationship_stage"];

export const PARTNER_ROLES: readonly PartnerRole[] = [
  "hosting",
  "consulting",
  "strategic",
];

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

export const RELATIONSHIP_STAGES: readonly RelationshipStage[] = [
  "research",
  "identified",
  "contacted",
  "in_conversation",
  "fit_confirmed",
  "agreement_pending",
  "active",
  "active_referring",
  "dormant",
  "no_fit",
];

export const ROLE_LABELS: Record<PartnerRole, MessageKey> = {
  hosting: "partner.role.hosting",
  consulting: "partner.role.consulting",
  strategic: "partner.role.strategic",
};

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

export const STAGE_LABELS: Record<RelationshipStage, MessageKey> = {
  research: "partner.stage.research",
  identified: "partner.stage.identified",
  contacted: "partner.stage.contacted",
  in_conversation: "partner.stage.inConversation",
  fit_confirmed: "partner.stage.fitConfirmed",
  agreement_pending: "partner.stage.agreementPending",
  active: "partner.stage.active",
  active_referring: "partner.stage.activeReferring",
  dormant: "partner.stage.dormant",
  no_fit: "partner.stage.noFit",
};
