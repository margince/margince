// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Its own module with no imports, so the MCP apps bundle reads it too.

/**
 * A running total of minor units, or undefined once it has passed 2^53. Each
 * amount the server sends is bounded; a sum of many is not, and past 2^53 it
 * silently becomes another number. A caller shows the undefined total as too
 * large rather than as a figure.
 */
export function addMinor(
  total: number | undefined,
  amount: number,
): number | undefined {
  if (total === undefined) return undefined;
  const sum = total + amount;
  return Number.isSafeInteger(sum) ? sum : undefined;
}

/** The exact sum of amounts, or undefined once it passes the exact range. */
export function sumMinor(amounts: readonly number[]): number | undefined {
  return amounts.reduce<number | undefined>(addMinor, 0);
}
