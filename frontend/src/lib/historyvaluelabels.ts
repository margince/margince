// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { OccurredPrecision } from "../format/activitywhen";
import { type Locale, translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { CALL_OUTCOME_LABEL } from "./calloutcome";

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

const HISTORY_VALUE_LABELS = new Map<
  string,
  Readonly<Record<string, MessageKey>>
>([
  ["occurred_precision", PRECISION_WORDS],
  ["call_outcome", CALL_OUTCOME_LABEL],
  ["qualifying_event", EXCHANGE_WORDS],
  ["qualifying_event_retracted", EXCHANGE_WORDS],
]);

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
