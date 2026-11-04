// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The order a reader put a list table's columns in. Stored per table under the
// name its widths use, so the order holds between visits like a dragged width.

import { useState } from "react";
import { readStoredJson, STORAGE_KEYS, writeStored } from "../lib/storage";

type Placeable = Readonly<{ key: string; fixed?: boolean }>;

/**
 * The columns in the reader's order. Fixed columns come first, then the stored
 * keys that still name a column. Any column the order never named follows in
 * declared order. A stored key whose column is gone is dropped.
 */
export function orderedColumns<C extends Placeable>(
  columns: readonly C[],
  order: readonly string[],
): C[] {
  const fixed = columns.filter((column) => column.fixed);
  const movable = columns.filter((column) => !column.fixed);
  const byKey = new Map(movable.map((column) => [column.key, column]));
  const placed = order.flatMap((key) => {
    const column = byKey.get(key);
    return column ? [column] : [];
  });
  const named = new Set(placed.map((column) => column.key));
  return [
    ...fixed,
    ...placed,
    ...movable.filter((column) => !named.has(column.key)),
  ];
}

function keysIn(value: unknown): string[] | null {
  return Array.isArray(value)
    ? value.filter((key): key is string => typeof key === "string")
    : null;
}

/** The stored column order of one table; no key keeps it for this mount only. */
export function useColumnOrder(widthsKey?: string) {
  const [order, setOrder] = useState<readonly string[]>(() =>
    widthsKey
      ? (readStoredJson(
          { family: STORAGE_KEYS.tableOrder, member: widthsKey },
          keysIn,
        ) ?? [])
      : [],
  );
  return {
    order,
    onReorder: (keys: readonly string[]) => {
      setOrder(keys);
      if (widthsKey) {
        writeStored(
          { family: STORAGE_KEYS.tableOrder, member: widthsKey },
          JSON.stringify(keys),
        );
      }
    },
  };
}
