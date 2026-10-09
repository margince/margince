// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { provenanceLabel } from "../design-system/provenance";
import { formatDate } from "../format/format";
import type { Locale, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { humanizeToken } from "./audit";
import { provenanceOf } from "./common";

export type NoticeAcquisition = NonNullable<
  components["schemas"]["NoticeAcquisition"]
>;

const KIND_LABEL: Partial<Record<string, MessageKey>> = {
  subject_initiated: "notice.acq.subjectInitiated",
  customer_contract: "notice.acq.customerContract",
  requested_quote_or_meeting: "notice.acq.requested",
  in_person_permission: "notice.acq.faceToFace",
  referral: "notice.acq.referral",
  event_or_form: "notice.acq.eventOrForm",
  public_or_business_source: "notice.acq.publicSource",
  purchased_or_imported: "notice.acq.purchasedOrImported",
  crm_migration: "notice.acq.crmMigration",
  mailbox_history: "notice.acq.mailboxHistory",
  unknown_legacy: "notice.acq.unknownLegacy",
};

const RULE: Partial<Record<string, { label: MessageKey; hint: MessageKey }>> = {
  art13: { label: "notice.rule.art13", hint: "notice.ruleHint.art13" },
  art14: { label: "notice.rule.art14", hint: "notice.ruleHint.art14" },
};

export function acquisitionKindLabel(kind: string, t: Translator): string {
  const key = KIND_LABEL[kind];
  return key ? t(key) : humanizeToken(kind);
}

export function noticeRuleLabel(rule: string, t: Translator): string {
  const entry = RULE[rule];
  return entry ? t(entry.label) : humanizeToken(rule);
}

export function noticeRuleHint(
  rule: string,
  t: Translator,
): string | undefined {
  const entry = RULE[rule];
  return entry ? t(entry.hint) : undefined;
}

// Who recorded the evidence. A human whose name no longer resolves is a member
// who left.
export function acquisitionRecorder(
  acquisition: NoticeAcquisition,
  t: Translator,
): string {
  if (acquisition.captured_by_name) {
    return acquisition.captured_by_name;
  }
  const source = provenanceOf(acquisition.captured_by);
  if (source.kind === "human") {
    return t("notice.recorderGone");
  }
  const label = provenanceLabel(source, t, undefined);
  return typeof label === "string" ? label : t("trust.sourceUnknown");
}

// When the contact was obtained, or when it was recorded where nobody said.
export function acquisitionWhen(
  acquisition: NoticeAcquisition,
  t: Translator,
  locale: Locale,
  tz: string,
): string {
  const kind = acquisitionKindLabel(acquisition.kind, t);
  return acquisition.occurred_at
    ? t("notice.acqOn", {
        kind,
        date: formatDate(acquisition.occurred_at, locale, tz),
      })
    : t("notice.acqRecorded", {
        kind,
        date: formatDate(acquisition.captured_at, locale, tz),
      });
}

export function acquisitionCaption(
  acquisition: NoticeAcquisition | null | undefined,
  t: Translator,
  locale: Locale,
  tz: string,
): string {
  if (!acquisition) {
    return t("notice.noAcquisition");
  }
  return [
    acquisitionWhen(acquisition, t, locale, tz),
    acquisitionRecorder(acquisition, t),
  ].join(" · ");
}
