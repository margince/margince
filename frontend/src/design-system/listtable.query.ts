// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The server-backed query dials of a list table: search, sort, filter chips and
// saved views. The table reports each change upward and the caller re-reads.

import {
  type ListChip,
  type ListView,
  type SortControl,
  type SortOption,
  sortDirection,
} from "./listsurface";

/** The column fields the sort controls read. */
export type SortableColumn = Readonly<{
  header: string;
  sort?: string;
  numeric?: boolean;
}>;

/** The dials a list table hands to its surface, as the caller passed them. */
export type QueryDials = Readonly<{
  search?: { value: string; onChange: (next: string) => void };
  sort?: SortControl;
  chips: readonly ListChip[];
  chosen: Readonly<Record<string, string>>;
  onChipChange?: (key: string, value: string) => void;
  views: readonly ListView[];
  onViewChange?: (index: number) => void;
  scopeKey: string;
}>;

/** Is this column the one currently sorted, and which way? */
export function sortState(
  column: { sort?: string },
  value: string,
): "asc" | "desc" | null {
  return column.sort ? sortDirection(column.sort, value) : null;
}

/**
 * Every orderable column, the hidden ones too. A reader who hid a column to
 * fit a phone can still order by it, and the menu is the only route left.
 */
export function sortOptionsOf(
  columns: readonly SortableColumn[],
): readonly SortOption[] {
  return columns.flatMap((column) =>
    column.sort
      ? [{ field: column.sort, label: column.header, numeric: column.numeric }]
      : [],
  );
}

/** The column the server orders by, so the count line can name it. */
export function sortedColumn<Column extends SortableColumn>(
  columns: readonly Column[],
  sort: SortControl | undefined,
): Column | undefined {
  if (!sort) {
    return undefined;
  }
  return columns.find((column) => sortState(column, sort.value) !== null);
}

/**
 * Whether a dial the reader can clear is narrowing the set. A view is not one
 * by itself, since applying it writes its filters into `chosen`. Show archived
 * widens the set, so it is not one either.
 */
export function filteredBy(dials: QueryDials): boolean {
  return (
    Boolean(dials.search?.value) || Object.values(dials.chosen).some(Boolean)
  );
}

/**
 * Whether anything cuts the set down, a screen's own scope included. No
 * button here can clear a scope, so this decides the empty sentence and
 * `filteredBy` decides whether a Clear is offered.
 */
export function narrowedBy(dials: QueryDials): boolean {
  return filteredBy(dials) || Boolean(dials.scopeKey);
}

function filterKeys(dials: QueryDials, extra: readonly string[] = []) {
  return new Set([
    ...dials.chips.map((chip) => chip.key),
    ...Object.keys(dials.chosen),
    ...extra,
  ]);
}

/**
 * Clears whatever narrows the list, not only what a chip names. A filter that
 * a view applied without a chip is still one the reader wants gone.
 */
export function clearAll(dials: QueryDials) {
  dials.search?.onChange("");
  for (const key of filterKeys(dials)) {
    dials.onChipChange?.(key, "");
  }
  dials.onViewChange?.(0);
}

/**
 * Applies a saved view. Every filter is rewritten, not merged, since a view
 * describes the whole filter state. The keys include the view's own, because
 * a view may narrow by something no chip offers, such as a minimum score.
 */
export function applyView(dials: QueryDials, index: number) {
  dials.onViewChange?.(index);
  const view = dials.views[index];
  if (dials.sort) {
    dials.sort.onChange(view?.sort ?? "");
  }
  for (const key of filterKeys(dials, Object.keys(view?.filters ?? {}))) {
    dials.onChipChange?.(key, view?.filters?.[key] ?? "");
  }
}
