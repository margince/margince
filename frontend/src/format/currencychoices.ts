// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** Never empty: a form opens on the first code and sends it for an empty field. */
import type { Locale } from "../i18n";
import { isAssignedCurrency } from "./assignedcurrencies";
import { holdsEveryWord, searchWords } from "./collate";
import { INTL_LOCALE } from "./intllocale";

export type LeadingCurrencies = readonly [string, ...string[]];

// What a picker leads with before the installation's base currency is known:
// the four a sales team reaches for most. Once the base is known, the base and
// the rate-sheet currencies lead instead (`useLeadingCurrencies`).
export const FALLBACK_LEADING_CURRENCIES: LeadingCurrencies = [
  "EUR",
  "USD",
  "GBP",
  "CHF",
];

/**
 * Every currency code a money field can offer: `leading` first, in the order
 * given, then the code the record already carries, then the platform's ISO
 * 4217 list. Only a code the server assigns is offered as a choice; the code
 * the record already carries stays listed so the field can show it.
 */
export function currencyCodes(
  leading: LeadingCurrencies,
  current?: string | null,
): string[] {
  const platform =
    typeof Intl.supportedValuesOf === "function"
      ? Intl.supportedValuesOf("currency")
      : [];
  return [
    ...new Set([
      ...leading.filter(isAssignedCurrency),
      ...(current ? [current] : []),
      ...platform.filter(isAssignedCurrency),
    ]),
  ];
}

/**
 * The options of a currency `<Select>`. With a `locale`, each code is followed
 * by its name in the reader's language, from the platform's currency table
 * ("VND – Vietnamese dong"), so a reader who knows the money and not its code
 * can find it. Without one, a code is its own label.
 */
export function currencyOptions(
  leading: LeadingCurrencies,
  current?: string | null,
  locale?: Locale,
): { value: string; label: string }[] {
  const named = locale ? currencyNamer(locale) : null;
  return currencyCodes(leading, current).map((code) => ({
    value: code,
    label: named ? named(code) : code,
  }));
}

const currencyNames = new Map<Locale, Intl.DisplayNames>();

function currencyNamer(locale: Locale): (code: string) => string {
  let names = currencyNames.get(locale);
  if (!names) {
    names = new Intl.DisplayNames(INTL_LOCALE[locale], { type: "currency" });
    currencyNames.set(locale, names);
  }
  const table = names;
  return (code) => {
    const name = table.of(code);
    // The table answers a code it does not know with the code itself.
    return name && name !== code ? `${code} – ${name}` : code;
  };
}

/**
 * A search over currency options for a searched picker: every typed word must
 * appear in the code or the name. An empty term lists them all, in order.
 */
export function searchCurrencies(
  options: readonly { value: string; label: string }[],
): (term: string) => Promise<{ id: string; name: string }[]> {
  return (term) => {
    const words = searchWords(term);
    return Promise.resolve(
      options
        .filter((option) => holdsEveryWord(words, [option.label]))
        .map((option) => ({ id: option.value, name: option.label })),
    );
  };
}
