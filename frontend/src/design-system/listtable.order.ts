// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The order a reader put a list table's columns in, and which ones they hid.
// Stored per table under the name its widths use, so both hold between visits.

import { useMemo, useState } from "react";
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

// Only the columns the reader ticked or unticked are stored. The rest follow
// `initiallyHidden`, which a filter preview changes with its filter.
function choicesIn(value: unknown): Record<string, boolean> | null {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return null;
  }
  return Object.fromEntries(
    Object.entries(value).filter(
      (entry): entry is [string, boolean] => typeof entry[1] === "boolean",
    ),
  );
}

/**
 * The presentation dials of the Display menu: which optional columns stand,
 * and how tight the rows are. Local state, unlike the query dials.
 */
export function useColumnDisplay(
  columns: readonly Readonly<{ key: string; initiallyHidden?: boolean }>[],
  widthsKey?: string,
) {
  const [choices, setChoices] = useState<Readonly<Record<string, boolean>>>(
    () =>
      (widthsKey
        ? readStoredJson(
            { family: STORAGE_KEYS.tableHidden, member: widthsKey },
            choicesIn,
          )
        : null) ?? {},
  );
  const hidden: ReadonlySet<string> = useMemo(
    () =>
      new Set(
        columns.flatMap((c) =>
          (choices[c.key] ?? c.initiallyHidden) ? [c.key] : [],
        ),
      ),
    [columns, choices],
  );
  const [dense, setDense] = useState(false);
  return {
    hidden,
    onToggleColumn: (key: string) => {
      const next = { ...choices, [key]: !hidden.has(key) };
      setChoices(next);
      if (widthsKey) {
        writeStored(
          { family: STORAGE_KEYS.tableHidden, member: widthsKey },
          JSON.stringify(next),
        );
      }
    },
    dense,
    onDense: () => setDense(!dense),
  };
}
