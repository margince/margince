// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { ACTIVITY_KIND_LABEL } from "../design-system/timelineentry";
import type { OccurredPrecision } from "../format/activitywhen";
import { LIFECYCLE_LABELS } from "../format/companylifecycle";
import { DEAL_STATUS_LABEL } from "../format/dealstatus";
import {
  DEAL_ATTRIBUTION_LABEL,
  DEAL_MOTION_LABEL,
  DEAL_PRIORITY_LABEL,
} from "../format/dealvocab";
import { GREETING_REGISTER_LABEL } from "../format/greetingregister";
import { LEAD_STATUS_LABEL } from "../format/leadstatus";
import { CERT_LABELS, MARGIN_TIER_LABELS } from "../format/partnervocab";
import { PROMOTION_TRIGGER_LABEL } from "../format/promotion";
import { type Locale, translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { CALL_OUTCOME_LABEL } from "./calloutcome";
import { KIND_LABELS } from "./partnerstages.queries";
import { TASK_TYPE_LABELS } from "./tasktype";

// The words for a closed vocabulary a change row stores by its wire spelling,
// beside the field names in lib/historyfieldlabels.ts.
const PRECISION_WORDS: Readonly<Record<OccurredPrecision, MessageKey>> = {
  instant: "history.value.occurred_precision.instant",
  day: "history.value.occurred_precision.day",
};

const EXCHANGE_WORDS: Readonly<
  Record<
    components["schemas"]["RecordQualifyingEventRequest"]["kind"],
    MessageKey
  >
> = {
  in_person: "compose.fact.met",
  requested_by_subject: "history.value.exchange.requested",
};

const DIRECTION_WORDS: Readonly<
  Record<
    NonNullable<components["schemas"]["Activity"]["direction"]>,
    MessageKey
  >
> = {
  inbound: "history.value.direction.inbound",
  outbound: "history.value.direction.outbound",
};

const MEETING_STATUS_WORDS: Readonly<
  Record<
    NonNullable<components["schemas"]["Activity"]["meeting_status"]>,
    MessageKey
  >
> = {
  booked: "history.value.meeting_status.booked",
  held: "history.value.meeting_status.held",
  no_show: "history.value.meeting_status.no_show",
  canceled: "history.value.meeting_status.canceled",
};

// The reason a cleared or carried impersonation warning names. `unrecorded` is the
// audit's own word for a warning stored without one.
const IMPERSONATION_WORDS: Readonly<
  Record<
    | NonNullable<
        components["schemas"]["ContactImpersonationWarning"]["reason"]
      >
    | "unrecorded",
    MessageKey
  >
> = {
  punycode_domain: "history.value.impersonation.punycode_domain",
  display_name_other_domain:
    "history.value.impersonation.display_name_other_domain",
  unrecorded: "history.value.impersonation.unrecorded",
};

const MARK_WORDS: Readonly<
  Record<
    NonNullable<components["schemas"]["Contact"]["relationship_mark"]>,
    MessageKey
  >
> = {
  vendor_marketing: "history.value.relationship_mark.vendor_marketing",
};

const VISIBILITY_WORDS: Readonly<
  Record<
    components["schemas"]["UpdateCompanyRequest"]["visibility"] & string,
    MessageKey
  >
> = {
  workspace: "visibility.workspace",
  owner: "visibility.private",
};

// A lead, a deal and a company all store a `status`. Only `disqualified` is a
// code of two of them, and the lead's word for it reads right for a company too.
const STATUS_WORDS: Readonly<Record<string, MessageKey>> = {
  ...LIFECYCLE_LABELS,
  ...LEAD_STATUS_LABEL,
  ...DEAL_STATUS_LABEL,
};

const HISTORY_VALUE_LABELS = new Map<
  string,
  Readonly<Record<string, MessageKey>>
>([
  ["occurred_precision", PRECISION_WORDS],
  ["call_outcome", CALL_OUTCOME_LABEL],
  ["qualifying_event", EXCHANGE_WORDS],
  ["qualifying_event_retracted", EXCHANGE_WORDS],
  ["lifecycle", LIFECYCLE_LABELS],
  ["status", STATUS_WORDS],
  ["trigger", PROMOTION_TRIGGER_LABEL],
  ["margin_tier", MARGIN_TIER_LABELS],
  ["cert_status", CERT_LABELS],
  ["partner_role", KIND_LABELS],
  ["task_type", TASK_TYPE_LABELS],
  ["commercial_motion", DEAL_MOTION_LABEL],
  ["priority", DEAL_PRIORITY_LABEL],
  ["partner_attribution", DEAL_ATTRIBUTION_LABEL],
  ["greeting_register", GREETING_REGISTER_LABEL],
  ["relationship_mark", MARK_WORDS],
  ["impersonation_warning_cleared", IMPERSONATION_WORDS],
  ["impersonation_warning_carried", IMPERSONATION_WORDS],
  ["visibility", VISIBILITY_WORDS],
  ["direction", DIRECTION_WORDS],
  ["meeting_status", MEETING_STATUS_WORDS],
  ["kind", ACTIVITY_KIND_LABEL],
]);

/** The fields whose stored codes this file words, for the census over the contract. */
export function historyValueFields(): string[] {
  return [...HISTORY_VALUE_LABELS.keys()];
}

/** A stored value of a closed vocabulary, in words, or undefined. */
export function historyValueLabel(
  field: string,
  value: string,
  locale: Locale,
): string | undefined {
  const words = HISTORY_VALUE_LABELS.get(field);
  const key = words && Object.hasOwn(words, value) ? words[value] : undefined;
  if (key === undefined && words === EXCHANGE_WORDS) {
    return translate(locale, "history.field.qualifying_event");
  }
  return key === undefined ? undefined : translate(locale, key);
}
