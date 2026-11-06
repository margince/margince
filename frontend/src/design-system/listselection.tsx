// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useEffect, useRef, useState } from "react";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { Button, Checkbox } from "./atoms";
import type { ListSelection } from "./listcolumn";
import { SelectionBar } from "./selectionbar";

const NOTHING: ReadonlySet<string> = new Set();

/**
 * A list's row selection, by key. Only rows the list holds now count as
 * selected. A row that left the result set cannot linger unseen. A tick past
 * `limit` adds nothing.
 */
export function useRowSelection<Row>({
  rows,
  keyOf,
  limit = Number.POSITIVE_INFINITY,
}: Readonly<{
  rows: readonly Row[];
  keyOf: (row: Row) => string;
  limit?: number;
}>) {
  const [picked, setPicked] = useState<ReadonlySet<string>>(NOTHING);
  const heldKeys = new Set(rows.map(keyOf));
  // A key the list no longer holds is dropped, so returning to an earlier
  // filter does not bring an old selection back.
  if ([...picked].some((key) => !heldKeys.has(key))) {
    setPicked(new Set([...picked].filter((key) => heldKeys.has(key))));
  }
  const selectedRows = rows.filter((row) => picked.has(keyOf(row)));
  const selected: ReadonlySet<string> = new Set(selectedRows.map(keyOf));
  return {
    selected,
    selectedRows,
    limit,
    onChange: (changed: readonly Row[], on: boolean) =>
      setPicked((prev) => {
        const next = new Set(prev);
        for (const row of changed) {
          const key = keyOf(row);
          if (!on) {
            next.delete(key);
          } else if (next.size < limit) {
            next.add(key);
          }
        }
        return next;
      }),
    /** Keep only these keys selected, e.g. the rows a bulk run refused. */
    replace: (keys: Iterable<string>) => setPicked(new Set(keys)),
    clear: () => setPicked(NOTHING),
  };
}

/**
 * What the table offers over a selection: the header box for the page, and
 * "Select all" for every matching row, up to the limit.
 *
 * "Select all" asks for the next pages one at a time, then ticks the rows. It
 * stops once it holds every row or enough to fill the limit. Each answer grows
 * `rows`, and only that growth asks for the next page. No read is asked twice.
 */
export function useSelectAll<Row>({
  rows,
  pageRows,
  rowKey,
  selection,
  hasMore,
  onLoadMore,
  failed,
}: Readonly<{
  rows: readonly Row[];
  pageRows: readonly Row[];
  rowKey: (row: Row) => string;
  selection?: ListSelection<Row>;
  hasMore: boolean;
  onLoadMore?: () => void;
  failed: boolean;
}>) {
  // The row count when Select all was pressed; null while not walking.
  const [walkFrom, setWalkFrom] = useState<number | null>(null);
  const walking = walkFrom !== null;
  // The row count when the walk last asked for a page.
  const askedAt = useRef(-1);
  const isSelectable = (row: Row) => selection?.selectable?.(row) !== false;
  const isSelected = (row: Row) =>
    selection?.selected.has(rowKey(row)) ?? false;
  const held = rows.filter(isSelectable);
  const onPage = pageRows.filter(isSelectable);
  const limit = selection?.limit ?? Number.POSITIVE_INFINITY;
  const count = selection?.selected.size ?? 0;
  const canLoad = hasMore && onLoadMore !== undefined;
  const holdsEnough = !canLoad || held.length >= limit;
  const complete = !canLoad && held.every(isSelected);
  const atLimit = count >= limit;
  // The walk ends once its ticks have landed or a read failed. Fewer rows than
  // at the press means the list was narrowed, which ends it too.
  const narrowed = walkFrom !== null && rows.length < walkFrom;
  if (walking && (failed || complete || atLimit || narrowed)) {
    setWalkFrom(null);
  }

  useEffect(() => {
    if (!walking || failed || narrowed || !selection) {
      return;
    }
    if (holdsEnough) {
      selection.onChange(held, true);
    } else if (rows.length !== askedAt.current) {
      askedAt.current = rows.length;
      onLoadMore?.();
    }
  }, [
    walking,
    failed,
    narrowed,
    holdsEnough,
    rows.length,
    selection,
    held,
    onLoadMore,
  ]);

  const pageOn = onPage.length > 0 && onPage.every(isSelected);
  return {
    page: {
      shown: selection !== undefined && onPage.length > 0,
      checked: pageOn,
      mixed: !pageOn && onPage.some(isSelected),
      toggle: () => selection?.onChange(onPage, !pageOn),
    },
    complete,
    atLimit,
    // Rows match that the limit leaves out of the selection.
    capped: atLimit && (canLoad || held.length > count),
    pending: walking,
    run: () => {
      askedAt.current = -1;
      setWalkFrom(rows.length);
    },
    clear: () => selection?.onChange(held, false),
    limit,
  };
}

export type RowSelection<Row> = ReturnType<typeof useRowSelection<Row>>;

export type SelectAll = ReturnType<typeof useSelectAll>;

/** The header box: ticks or unticks every row on the rendered page. */
export function PageSelectBox({ all }: Readonly<{ all: SelectAll }>) {
  const t = useT();
  if (!all.page.shown) {
    return null;
  }
  return (
    <Checkbox
      className="lt-select"
      label={<span className="sr-only">{t("bulk.selectPage")}</span>}
      checked={all.page.checked}
      indeterminate={all.page.mixed}
      onChange={all.page.toggle}
    />
  );
}

/**
 * "Select all" and "Clear selection", at the end of the bulk bar. "Select all"
 * goes once every matching row is selected, or the limit is reached.
 */
export function SelectAllControls({
  all,
}: Readonly<{ all: SelectAll }>): ReactNode {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <span className="lt-bulkall">
      {all.capped && (
        <span className="t-caption">
          {plural("bulk.selectionLimit", all.limit, {
            count: formatNumber(all.limit, locale),
          })}
        </span>
      )}
      {!all.complete && !all.atLimit && (
        <Button variant="ghost" pending={all.pending} onClick={all.run}>
          {t("bulk.selectAll")}
        </Button>
      )}
      <Button variant="ghost" onClick={all.clear}>
        {t("bulk.clearSelection")}
      </Button>
    </span>
  );
}

/** The bulk bar over the grid, while anything is selected. */
export function ListBulkBar<Row>({
  selection,
  all,
  hidden,
}: Readonly<{
  selection?: ListSelection<Row>;
  all: SelectAll;
  /** Another body stands in for the grid; its rows cannot be seen or unticked. */
  hidden: boolean;
}>) {
  if (hidden || !selection || selection.selected.size === 0) {
    return null;
  }
  return (
    <SelectionBar>
      {selection.bar}
      <SelectAllControls all={all} />
    </SelectionBar>
  );
}
