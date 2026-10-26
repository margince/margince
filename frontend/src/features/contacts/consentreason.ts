// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { formatDateAbbrev } from "../../format/format";
import type { Locale, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type GuardEntry = components["schemas"]["ContactConsentGuardEntry"];
type QualifyingKind = components["schemas"]["ConsentQualifyingEvent"]["kind"];

// The verdict's reason in the reader's language, from the code the server
// sends beside its English sentence.
const REASON: Record<GuardEntry["reason_code"], MessageKey> = {
  objection: "consent.reason.objection",
  suppressed_objection: "consent.reason.suppressed_objection",
  suppressed_restricted: "consent.reason.suppressed_restricted",
  suppressed_subject_request: "consent.reason.suppressed_subject_request",
  suppressed_hard_bounce: "consent.reason.suppressed_hard_bounce",
  suppressed: "consent.reason.suppressed",
  contract_notice: "consent.reason.contract_notice",
  no_call_path: "consent.reason.no_call_path",
  said_we_may_write: "consent.reason.said_we_may_write",
  unconfirmed_double_opt_in: "consent.reason.unconfirmed_double_opt_in",
  no_connection: "consent.reason.no_connection",
  qualifying_event: "consent.reason.qualifying_event",
  consent_confirmed: "consent.reason.consent_confirmed",
  consent_withdrawn: "consent.reason.consent_withdrawn",
  no_consent_exception_not_offered:
    "consent.reason.no_consent_exception_not_offered",
  no_consent: "consent.reason.no_consent",
};

const QUALIFYING: Record<QualifyingKind, MessageKey> = {
  inbound_message: "consent.reason.qualifying.inbound_message",
  inquiry: "consent.reason.qualifying.inquiry",
  active_deal: "consent.reason.qualifying.active_deal",
  in_person: "consent.reason.qualifying.in_person",
  meeting: "consent.reason.qualifying.meeting",
  requested_by_subject: "consent.reason.qualifying.requested_by_subject",
};

// The three purposes the product seeds, named for the rep reading the panel.
// An operator's own purpose keeps the name it was given.
const PURPOSE_NAME: Record<string, MessageKey> = {
  business_correspondence: "consent.purposeName.business_correspondence",
  transactional: "consent.purposeName.transactional",
  marketing_email: "consent.purposeName.marketing_email",
};

export function consentPurposeName(entry: GuardEntry, t: Translator): string {
  const key = PURPOSE_NAME[entry.purpose_key];
  return key ? t(key) : (entry.purpose_label ?? entry.purpose_key);
}

/**
 * The reason line under a purpose. A code this build does not know, from a
 * newer server, falls back to the server's English sentence.
 */
export function consentReason(
  entry: GuardEntry,
  t: Translator,
  locale: Locale,
  zone: string,
): string {
  const date = (iso: string) => formatDateAbbrev(iso, locale, zone);
  const event = entry.qualifying_event;
  if (entry.reason_code === "qualifying_event" && event) {
    const key = QUALIFYING[event.kind];
    return key
      ? t(key, { date: date(event.occurred_at), note: event.note ?? "" })
      : entry.reason;
  }
  if (entry.reason_code === "objection" && entry.reason_at) {
    return t("consent.reason.objectionOn", { date: date(entry.reason_at) });
  }
  const key = REASON[entry.reason_code];
  return key ? t(key) : entry.reason;
}
