// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

type Priced = Readonly<{
  amount_minor?: number | null;
  currency?: string | null;
}>;

/** What a set of deals is worth together, or why no single sum can be said. */
export type OpenValue =
  | Readonly<{ kind: "sum"; minor: number; currency: string }>
  | Readonly<{ kind: "mixed" }>
  | Readonly<{ kind: "unknown" }>;

/**
 * The sum of deals in one currency. A deal with no readable amount, or a set
 * not read in full, has no sum. A total of the visible part would read as the
 * whole. Unlike money is never added.
 */
export function openValue(
  deals: readonly Priced[],
  complete: boolean,
): OpenValue {
  if (!complete || deals.length === 0) return { kind: "unknown" };
  let minor = 0;
  const currencies = new Set<string>();
  for (const deal of deals) {
    if (deal.amount_minor == null || !deal.currency) return { kind: "unknown" };
    minor += deal.amount_minor;
    // Past 2^53 a sum rounds, and a rounded total is not one the server holds.
    if (!Number.isSafeInteger(minor)) return { kind: "unknown" };
    currencies.add(deal.currency);
  }
  const [currency] = currencies;
  if (currencies.size !== 1 || !currency) return { kind: "mixed" };
  return { kind: "sum", minor, currency };
}
