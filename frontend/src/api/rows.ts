// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The contract promises the arrays. A body that lost one reads as the empty
// list it claims, not as a crash in the render.
export function rowsOf<Row>(
  rows: readonly Row[] | null | undefined,
): readonly Row[] {
  return Array.isArray(rows) ? rows : [];
}
