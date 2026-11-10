// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatNumber } from "../format/format";
import { calendarLateness } from "../format/lateness";
import type { Words } from "../i18n";

/** A task somebody filed, or a promise read out of a conversation. */
export type PromiseKind = "task" | "claim";

/** How a late promise reads: the badge beside it and the sentence under it. */
export type LatePromise = Readonly<{ badge: string; sentence: string }>;

/**
 * The badge and the sentence of a late promise, both from one calendar-day
 * count in the reader's zone. The server words lateness in elapsed days, so a
 * record page shows this sentence in place of the server's for a late promise.
 * Nothing when the promise is not late.
 */
export function latePromise(
  dueAt: string,
  asOf: string,
  zone: string,
  kind: PromiseKind,
  words: Words,
): LatePromise | undefined {
  const { late, days } = calendarLateness(dueAt, asOf, zone);
  if (!late) {
    return undefined;
  }
  if (days === 0) {
    return {
      badge: words.t("co.next.overdue"),
      sentence: words.t(
        kind === "task" ? "promise.late.today" : "promise.claim.today",
      ),
    };
  }
  const count = { days: formatNumber(days, words.locale) };
  return {
    badge: words.plural("co.next.daysLate", days, count),
    sentence: words.plural(
      kind === "task" ? "promise.late.sentence" : "promise.claim.sentence",
      days,
      count,
    ),
  };
}
