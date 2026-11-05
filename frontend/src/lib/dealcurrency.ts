// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The currency to send beside a deal's figures, or null when none was typed.
 * The server refuses a currency with no figure, so the pair travels together.
 */
export function currencyBeside(
  currency: string,
  ...figures: readonly (string | undefined)[]
): string | null {
  return figures.some((figure) => figure) ? currency : null;
}
