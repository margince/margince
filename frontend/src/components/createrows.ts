// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { FormRow } from "./create";

// One row's value change.
export function withRowUpdated(
  rows: FormRow[],
  index: number,
  key: string,
  value: string,
): FormRow[] {
  return rows.map((row, rowIndex) =>
    rowIndex === index ? { ...row, [key]: value } : row,
  );
}

// One row moved a single position up or down. Order is a real gesture on these
// rows — a reader puts the number they hand out first — and contactformfields.ts
// maps the array index straight to each row's `position`, so the move rewrites
// exactly what the save persists. A move off either end returns the SAME array,
// not a fresh copy, so a disabled-edge press cannot re-render the list for nothing.
export function withRowMoved(
  rows: FormRow[],
  index: number,
  direction: "up" | "down",
): FormRow[] {
  const target = direction === "up" ? index - 1 : index + 1;
  const moving = rows[index];
  const displaced = rows[target];
  if (moving === undefined || displaced === undefined) {
    return rows;
  }
  const next = [...rows];
  next[index] = displaced;
  next[target] = moving;
  return next;
}

// Marks one row primary and clears the flag on every other row: a contact has
// one primary address and one primary number, whatever their types.
export function withPrimaryMarked(
  rows: FormRow[],
  index: number,
  primaryKey: string,
): FormRow[] {
  return rows.map((row, rowIndex) => ({
    ...row,
    [primaryKey]: rowIndex === index ? "true" : "",
  }));
}

// The rows as sent: when none is marked primary, the first is, so a contact
// that has addresses always has one the app writes to.
export function withPrimaryDefault<T extends { is_primary: boolean }>(
  rows: T[],
): T[] {
  return rows.some((row) => row.is_primary)
    ? rows
    : rows.map((row, index) =>
        index === 0 ? { ...row, is_primary: true } : row,
      );
}

// The first value a second row repeats, as that row spells it, when the
// field's values are stored once. Case and edge spaces name no other value.
export function repeatedRowValue(
  rows: readonly FormRow[],
  valueKey: string,
): string | undefined {
  const seen = new Set<string>();
  for (const row of rows) {
    const value = (row[valueKey] ?? "").trim();
    const key = value.toLowerCase();
    if (!key) continue;
    if (seen.has(key)) return value;
    seen.add(key);
  }
  return undefined;
}
