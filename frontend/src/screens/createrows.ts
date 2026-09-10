// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { FormRow } from "./create";

// A row's own kind for grouping primaries. An unanswered row resolves to
// typeDefault rather than its own "" bucket — personformfields.ts passes
// the same default its asEmailType/asPhoneType request mapper falls back
// to, so an unset row and an explicit default-kind row are one kind on
// both sides of the wire, never two kinds that collide once submitted.
export function kindOf(
  row: FormRow,
  typeKey: string | undefined,
  typeDefault: string,
): string {
  return typeKey ? row[typeKey] || typeDefault : "";
}

// One row's value change, plus the primary bookkeeping a kind change owes:
// a row that changes kind stops claiming a primary it held under its OLD
// kind, so it never silently contends for the new kind's primary slot
// without the reader choosing it again.
export function withRowUpdated(
  rows: FormRow[],
  index: number,
  key: string,
  value: string,
  primaryKey: string | undefined,
  typeKey: string | undefined,
): FormRow[] {
  return rows.map((row, rowIndex) => {
    if (rowIndex !== index) {
      return row;
    }
    const next = { ...row, [key]: value };
    if (primaryKey && key === typeKey && row[primaryKey] === "true") {
      next[primaryKey] = "";
    }
    return next;
  });
}

// Marks one row primary among the rows sharing its kind, leaving every
// other kind's rows untouched.
export function withPrimaryMarked(
  rows: FormRow[],
  index: number,
  primaryKey: string,
  typeKey: string | undefined,
  typeDefault: string,
): FormRow[] {
  const targetKind = kindOf(rows[index] ?? {}, typeKey, typeDefault);
  return rows.map((row, rowIndex) => {
    if (typeKey && kindOf(row, typeKey, typeDefault) !== targetKind) {
      return row;
    }
    return { ...row, [primaryKey]: rowIndex === index ? "true" : "" };
  });
}
