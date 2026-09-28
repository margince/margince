// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// THE SHAPES A LIST'S DIALS TAKE, and the one piece of arithmetic they share.
//
// A leaf on purpose: the filter menu's value step (`listfiltervalues.tsx`) and
// the surface that opens it both need the chip, and a vocabulary living in
// either one would have that file importing its own consumer.

/**
 * One filter chip. Single-select by design: the list endpoints take one value
 * per filter param, so a multi-select chip would compose a query the API
 * cannot answer.
 */
export type ListChip = {
  key: string;
  label: string;
  /** The "no filter" entry, which is also how a chosen value is cleared. */
  allLabel: string;
  options: readonly { value: string; label: string }[];
  /**
   * A relation filter too large to list whole (a workspace's companies): when
   * present, the value step searches this instead of walking `options`. The
   * "all" entry still clears the filter, and `options` stays required so a
   * chip declared without `search` needs no separate shape.
   */
  search?: (
    query: string,
  ) => Promise<readonly { value: string; label: string }[]>;
};

/** A saved view: a named tab whose meaning is entirely the caller's. */
export type ListView = {
  /**
   * What identifies this tab, when the caller has something steadier than its
   * name. Two saved views may share a name, so a rail keyed on the label
   * collides on the pair and React renders one of them; the label stays the
   * fallback for a rail whose tabs are a fixed set the caller wrote, where the
   * name IS the identity.
   */
  id?: string;
  label: string;
  sort?: string;
  filters?: Readonly<Record<string, string>>;
};

/**
 * One attribute the sort pill can order by. ListSurface never learns what a
 * column is — the table hands it this plain list, keyed by the same server
 * sort field a column header already uses.
 */
export type SortControl = {
  /** The server sort string, e.g. `-created_at`. */
  value: string;
  onChange: (next: string) => void;
};

/**
 * One attribute a list can be ordered by, named as the reader sees it.
 *
 * `field` is the same server sort string the matching column header sends, so
 * the menu and the header are two routes to ONE state rather than two states
 * that can disagree about what the list is ordered by.
 */
export type SortOption = {
  field: string;
  label: string;
  /** Biggest first on the opening press, as the numeric column header does. */
  numeric?: boolean;
};

/** Which way `value` orders `field`, or null when it orders something else. */
export function sortDirection(
  field: string,
  value: string,
): "asc" | "desc" | null {
  if (value === field) {
    return "asc";
  }
  return value === `-${field}` ? "desc" : null;
}

/**
 * The sort a press on `option` produces, given where it stands now.
 *
 * One function for the header and the menu both: a reader who flips a column
 * from its header and then reopens the sort menu must find the direction they
 * just chose, and two copies of this arithmetic are how the two ends up
 * disagreeing about which press descends.
 */
export function nextSortValue(
  option: Readonly<{ field: string; numeric?: boolean }>,
  direction: "asc" | "desc" | null,
): string {
  if (direction === "asc") {
    return `-${option.field}`;
  }
  if (direction === "desc") {
    return option.field;
  }
  // Unsorted, a number almost always wants its biggest value first.
  return option.numeric ? `-${option.field}` : option.field;
}
