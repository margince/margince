// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The reading words the strip cards share.

import { calendarDaysBetween } from "../../format/format";
import type { MessageKey } from "../../i18n/en";

// A reading the caller's grants withheld, in the word every stat card in the
// product uses. `record.notShown` stays on the contact record's readings row
// and rail: retargeting it would restyle two surfaces nobody looked at here.
export const WITHHELD_READING: MessageKey = "reading.restricted";

// How long ago an instant fell, against the read's own `as_of` and never the
// reader's clock. `undefined` is TODAY, decided here and nowhere else: zero
// days is today, and a NEGATIVE span is skew between a timestamp and the read
// instant, which no slot may print.
export function daysAgo(
  at: string,
  asOf: string,
  zone: string,
): number | undefined {
  const days = calendarDaysBetween(new Date(at), new Date(asOf), zone);
  return days > 0 ? days : undefined;
}
