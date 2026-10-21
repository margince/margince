import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  type AcquisitionKind,
  isAcquisitionKind,
} from "../lib/acquisitionkinds";
import { humanizeToken } from "./humanizetoken";

const KIND_LABEL: Readonly<Record<AcquisitionKind, MessageKey>> = {
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

export function acquisitionKindLabel(kind: string, t: Translator): string {
  return isAcquisitionKind(kind) ? t(KIND_LABEL[kind]) : humanizeToken(kind);
}
