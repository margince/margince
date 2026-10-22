import type { components } from "../api/schema";
import { providerBrandName } from "../design-system/provider-mark";
import { profileFieldLabel } from "../format/companyprofilelabels";
import { providerName } from "../format/connectorprovider";
import { EMAIL_TYPE_LABEL, PHONE_TYPE_LABEL } from "../format/contactchannels";
import { INTRO_STATUS_LABELS } from "../format/introstatus";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { FACT_CATEGORY_LABELS, FACT_FIELD_LABELS } from "./factview";
import { historyFieldLabel } from "./historyfieldlabels";

type ProviderRun = components["schemas"]["ProviderRun"];
const RUN_LABELS: Record<ProviderRun["state"], MessageKey> = {
  queued: "approval.merge.value.queued",
  submitting: "approval.merge.value.submitting",
  in_progress: "approval.merge.value.in_progress",
  completed: "approval.merge.value.completed",
  no_match: "approval.merge.value.no_match",
  skipped: "approval.merge.value.skipped",
  submission_unknown: "approval.merge.value.submission_unknown",
  failed: "approval.merge.value.failed",
  cancelled: "approval.merge.value.cancelled",
};
const SKIP_LABELS: Record<
  NonNullable<ProviderRun["skip_reason"]>,
  MessageKey
> = {
  budget_exhausted: "approval.merge.value.budget_exhausted",
  low_balance: "approval.merge.value.low_balance",
  suppressed: "approval.merge.value.suppressed",
  not_eligible: "approval.merge.value.not_eligible",
  duplicate_subject_candidate:
    "approval.merge.value.duplicate_subject_candidate",
  rate_limited: "approval.merge.value.rate_limited",
  already_fresh: "approval.merge.value.already_fresh",
  no_identifiers: "approval.merge.value.no_identifiers",
  category_in_flight: "approval.merge.value.category_in_flight",
};
const ACCESS_LABELS: Readonly<Record<string, MessageKey>> = {
  read: "share.access.read",
  write: "share.access.write",
};
const CATALOGS: Readonly<Record<string, Readonly<Record<string, MessageKey>>>> =
  {
    merge_address_kind: {
      email: "history.field.email",
      phone: "history.field.phone",
      domain: "history.field.domain",
    },
    merge_address_usage: { ...EMAIL_TYPE_LABEL, ...PHONE_TYPE_LABEL },
    merge_grant_source: ACCESS_LABELS,
    merge_grant_before: ACCESS_LABELS,
    merge_grant_after: ACCESS_LABELS,
    merge_company_fact_category: FACT_CATEGORY_LABELS,
    merge_company_fact_field: FACT_FIELD_LABELS,
    read_fact_field: FACT_FIELD_LABELS,
    merge_introduction_before: INTRO_STATUS_LABELS,
    merge_introduction_after: INTRO_STATUS_LABELS,
    merge_feedback_verdict: {
      suppressed: "approval.merge.value.suppressed",
      corrected: "approval.merge.value.corrected",
      confirmed: "approval.merge.value.confirmed",
    },
    merge_run_before: RUN_LABELS,
    merge_run_after: RUN_LABELS,
    merge_run_skip: SKIP_LABELS,
  };

export function mergeValueLabel(
  field: string,
  value: string,
  t: Translator,
): string | undefined {
  if (field === "merge_company_profile_field" || field === "read_profile_field")
    return profileFieldLabel(value, t);
  if (field === "merge_profile_field") return historyFieldLabel(value, t);
  if (field === "merge_provider")
    return providerBrandName(value) ?? providerName(value, t);
  if (field === "merge_channel_provider") return providerName(value, t);
  const catalog = CATALOGS[field];
  if (!catalog) return undefined;
  const label = catalog[value];
  return t(label ?? "approval.preview.unavailable");
}
