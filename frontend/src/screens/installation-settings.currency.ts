// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT, type Words } from "../i18n";

type InstallationSettings = components["schemas"]["InstallationSettings"];
type BaseCurrencyLock = components["schemas"]["BaseCurrencyLock"];

// A frozen currency is frozen for an admin too. The lock reason replaces the
// advice about what to type, since nobody can set the value.
export function currencyNote(
  settings: InstallationSettings,
  words: Words,
): string {
  if (!settings.base_currency_locked) {
    return words.t("installationSettings.baseCurrencyHint");
  }
  const worded = settings.base_currency_lock
    ? lockSentence(settings.base_currency_lock, words)
    : undefined;
  return (
    worded ??
    settings.base_currency_locked_reason ??
    words.t("installationSettings.baseCurrencyLocked")
  );
}

// A reason this build does not know answers undefined, so a newer server's
// English sentence shows instead of a wrong one.
function lockSentence(
  lock: BaseCurrencyLock,
  { plural, locale }: Words,
): string | undefined {
  const count = formatNumber(lock.count, locale);
  switch (lock.reason) {
    case "rates_frozen":
      return plural(
        "installationSettings.baseCurrencyLock.ratesFrozen",
        lock.count,
        { count },
      );
    case "rate_sheet":
      return lock.base_currency === undefined
        ? plural(
            "installationSettings.baseCurrencyLock.rateSheetUnnamed",
            lock.count,
            { count },
          )
        : plural(
            "installationSettings.baseCurrencyLock.rateSheet",
            lock.count,
            { count, base: lock.base_currency },
          );
    default:
      return undefined;
  }
}

// The Base currency row and the field inside the edit dialog both read this,
// so the two cannot word the lock differently.
export function useCurrencyNote(): (settings: InstallationSettings) => string {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (settings) => currencyNote(settings, { t, plural, locale });
}
