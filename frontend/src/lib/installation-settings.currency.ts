// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../api/schema";
import { formatNumber } from "../format/format";
import {
  type Locale,
  translatePlural,
  useLocale,
  useT,
  type Words,
} from "../i18n";
import type { FieldProblem } from "./problembody";

type InstallationSettings = components["schemas"]["InstallationSettings"];
type BaseCurrencyLock = components["schemas"]["BaseCurrencyLock"];

// A frozen currency is frozen for an admin too. The lock reason replaces the
// advice about what to type, since nobody can set the value.
export function currencyNote(
  settings: InstallationSettings,
  words: Pick<Words, "t" | "locale">,
): string {
  if (!settings.base_currency_locked) {
    return words.t("installationSettings.baseCurrencyHint");
  }
  const worded = settings.base_currency_lock
    ? lockSentence(settings.base_currency_lock, words.locale)
    : undefined;
  return (
    worded ||
    settings.base_currency_locked_reason ||
    words.t("installationSettings.baseCurrencyLocked")
  );
}

// A reason this build does not know answers undefined, so a newer server's
// English sentence shows instead of a wrong one.
function lockSentence(
  lock: BaseCurrencyLock,
  locale: Locale,
): string | undefined {
  const count = formatNumber(lock.count, locale);
  switch (lock.reason) {
    case "rates_frozen":
      return translatePlural(
        locale,
        "installationSettings.baseCurrencyLock.ratesFrozen",
        lock.count,
        { count },
      );
    case "rate_sheet":
      return lock.base_currency === undefined
        ? translatePlural(
            locale,
            "installationSettings.baseCurrencyLock.rateSheetUnnamed",
            lock.count,
            { count },
          )
        : translatePlural(
            locale,
            "installationSettings.baseCurrencyLock.rateSheet",
            lock.count,
            { count, base: lock.base_currency },
          );
    default:
      lock.reason satisfies never;
      return undefined;
  }
}

// Every surface that shows the base currency reads the lock through this, so
// no two of them word it differently.
export function useCurrencyNote(): (settings: InstallationSettings) => string {
  const t = useT();
  const { locale } = useLocale();
  return (settings) => currencyNote(settings, { t, locale });
}

// The lock's arrival drops a code typed before it, which the server refuses
// and the disabled field cannot undo. An unlock then shows the stored code.
export function useLockDropsTypedCurrency(
  locked: boolean,
  drop: () => void,
): void {
  const [seen, setSeen] = useState(locked);
  if (seen !== locked) {
    setSeen(locked);
    if (locked) drop();
  }
}

const BASE_CURRENCY_SETTING = "installation.base_currency";
const LOCK_REASONS: readonly BaseCurrencyLock["reason"][] = [
  "rates_frozen",
  "rate_sheet",
];

// The lock a refused base currency names in its params. Undefined for facts
// this build cannot read, so the refusal keeps its own words.
export function refusedLock(fault: FieldProblem): BaseCurrencyLock | undefined {
  const { field, code, params } = fault;
  if (code !== "setting_frozen" || field !== BASE_CURRENCY_SETTING) {
    return undefined;
  }
  const reason = LOCK_REASONS.find((known) => known === params?.reason);
  const count = params?.count;
  if (reason === undefined || count === undefined || !/^\d+$/.test(count)) {
    return undefined;
  }
  const base = params?.base_currency;
  return {
    reason,
    count: Number(count),
    ...(base === undefined ? null : { base_currency: base }),
  };
}

// A refused base currency in the sentence its field's hint gives that lock.
export function refusedLockSentence(
  fault: FieldProblem,
  locale: Locale,
): string | undefined {
  const lock = refusedLock(fault);
  return lock === undefined ? undefined : lockSentence(lock, locale);
}
