// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { amountFault } from "../format/minorunits";
import type { MessageKey } from "../i18n/en";

// Apart from the deal amount field: a custom money field on Home's activity
// form asks it too. The deal field itself stays with the deal screens.

export type Translate = (
  key: MessageKey,
  params?: Record<string, string>,
) => string;

/** What to tell the reader about an amount `currency` cannot hold. */
export function amountRefusal(
  t: Translate,
  value: string,
  currency: string,
): string | undefined {
  switch (amountFault(value, currency)) {
    case "too_precise":
      return t("record.amountTooPrecise", { currency });
    case "too_large":
      return t("record.amountTooLarge");
    default:
      return undefined;
  }
}
