import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

// The catalog keys the server may name on a built-in list value or review
// question. backend/migrations/shippedlabels_integration_test.go holds this
// list equal, both ways, to the keys the migrations seed.
export const SHIPPED_LABEL_KEYS = [
  "acqSources.builtin.inbound",
  "acqSources.builtin.outbound",
  "acqSources.builtin.referral",
  "acqSources.builtin.partner",
  "acqSources.builtin.event",
  "acqSources.builtin.existing_customer",
  "acqSources.builtin.employee_referral",
  "acqSources.builtin.other",
  "lossReasons.builtin.price",
  "lossReasons.builtin.competitor",
  "lossReasons.builtin.no_budget",
  "lossReasons.builtin.no_decision",
  "lossReasons.builtin.timing",
  "lossReasons.builtin.product_fit",
  "lossReasons.builtin.went_silent",
  "lossReasons.builtin.other",
  "leadOrigins.builtin.event",
  "leadOrigins.builtin.linkedin",
  "leadOrigins.builtin.referral",
  "leadOrigins.builtin.partner",
  "leadOrigins.builtin.website",
  "leadOrigins.builtin.cold_outreach",
  "leadOrigins.builtin.other",
  "leadReasons.builtin.not_a_good_fit",
  "leadReasons.builtin.bad_timing",
  "leadReasons.builtin.no_budget",
  "leadReasons.builtin.no_decision_power",
  "leadReasons.builtin.chose_a_competitor",
  "leadReasons.builtin.no_interest",
  "leadReasons.builtin.not_reachable",
  "leadReasons.builtin.duplicate_or_spam",
  "partnerStages.builtin.research",
  "partnerStages.builtin.identified",
  "partnerStages.builtin.contacted",
  "partnerStages.builtin.in_conversation",
  "partnerStages.builtin.fit_confirmed",
  "partnerStages.builtin.agreement_pending",
  "partnerStages.builtin.contract_signed",
  "partnerStages.builtin.ready",
  "partnerStages.builtin.activated",
  "partnerStages.builtin.active",
  "partnerStages.builtin.active_referring",
  "partnerStages.builtin.dormant",
  "partnerStages.builtin.no_fit",
  "recordRoles.builtin.account_manager",
  "recordRoles.builtin.sales_engineer",
  "recordRoles.builtin.delivery_lead",
  "recordRoles.builtin.project_manager",
  "recordRoles.builtin.technical_contact",
  "recordRoles.builtin.executive_sponsor",
  "recordRoles.builtin.support_team",
  "reviewTemplates.builtin.win_review",
  "reviewTemplates.builtin.loss_review",
  "reviewTemplates.builtin.question.why_we_won",
  "reviewTemplates.builtin.question.competitors",
  "reviewTemplates.builtin.question.decisive_factor",
  "reviewTemplates.builtin.question.why_we_lost",
  "reviewTemplates.builtin.question.lost_to",
  "reviewTemplates.builtin.question.what_would_change_it",
] as const satisfies readonly MessageKey[];

type ShippedLabelKey = (typeof SHIPPED_LABEL_KEYS)[number];

function isShippedLabelKey(key: string): key is ShippedLabelKey {
  return SHIPPED_LABEL_KEYS.some((shipped) => shipped === key);
}

type Labelled = Readonly<{ label: string; label_key?: string | null }>;

/**
 * The words a built-in value reads as: the catalog's, in the reader's
 * language, until an administrator relabels it and the server drops the key.
 * A key this build does not know reads as the stored label.
 */
export function shippedLabel(row: Labelled, t: Translator): string {
  const key = row.label_key;
  return key && isShippedLabelKey(key) ? t(key) : row.label;
}

/** The same rows with every built-in label in the reader's language. */
export function withShippedLabels<Row extends Labelled>(
  rows: readonly Row[],
  t: Translator,
): Row[] {
  return rows.map((row) => ({ ...row, label: shippedLabel(row, t) }));
}
