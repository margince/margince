// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { FormRow } from "./create";

// Marks one row primary among the rows that share its kind — "" (untyped)
// counts as its own kind — leaving every other kind's rows untouched. Kept
// out of create.tsx because that file is already at its size ceiling; this
// is the one place a WORK/PERSONAL split (or none, when typeKey is unset)
// decides which rows a primary swap may touch.
export function withPrimaryMarked(
  rows: FormRow[],
  index: number,
  primaryKey: string,
  typeKey: string | undefined,
): FormRow[] {
  const kindOf = (row: FormRow) => (typeKey ? (row[typeKey] ?? "") : "");
  const targetKind = kindOf(rows[index] ?? {});
  return rows.map((row, rowIndex) => {
    if (typeKey && kindOf(row) !== targetKind) {
      return row;
    }
    return { ...row, [primaryKey]: rowIndex === index ? "true" : "" };
  });
}
