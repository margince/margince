// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * Orders strings by UTF-16 code unit, the order a bare `.sort()` uses. Lists
 * that are compared, joined or written to a baseline keep their exact order.
 * `localeCompare` would reorder them by locale rules.
 */
export const byCodeUnit = (a: string, b: string): number =>
  a < b ? -1 : a > b ? 1 : 0;
