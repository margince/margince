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
 * selected, so a row that left the result set cannot linger as a selection
 * nobody can see to clear. A tick past `limit` adds nothing.
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
  const selectedRows = rows.filter((row) => picked.has(keyOf(row)));
  const selected: ReadonlySet<string> = new Set(selectedRows.map(keyOf));
  return {
    selected,
    selectedRows,
    limit,
    onChange: (changed: readonly Row[], on: boolean) =>
      setPicked((prev) => {
        // A key the list no longer holds must not count against the limit.
        const next = new Set([...prev].filter((key) => heldKeys.has(key)));
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
    /** Keep exactly these keys selected, e.g. the rows a bulk run refused. */
    replace: (keys: Iterable<string>) => setPicked(new Set(keys)),
    clear: () => setPicked(NOTHING),
  };
}

/**
 * What the table offers over a selection: the header box for the rendered
 * page, and "Select all" for every row that matches, up to the limit.
 *
 * "Select all" asks for the list's next pages one at a time until it holds
 * every row or enough to fill the limit, then ticks them. Each answer grows
 * `rows`, and that growth is what asks for the next page, so no read is ever
 * asked for twice.
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
  const [walking, setWalking] = useState(false);
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
  // The walk ends once the ticks it asked for have landed, or a read failed.
  // Set while rendering, so no effect has to clear it.
  if (walking && (failed || complete || atLimit)) {
    setWalking(false);
  }

  useEffect(() => {
    if (!walking || failed || !selection) {
      return;
    }
    if (holdsEnough) {
      selection.onChange(held, true);
    } else if (rows.length !== askedAt.current) {
      askedAt.current = rows.length;
      onLoadMore?.();
    }
  }, [walking, failed, holdsEnough, rows.length, selection, held, onLoadMore]);

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
      setWalking(true);
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
}: Readonly<{ selection?: ListSelection<Row>; all: SelectAll }>) {
  if (!selection || selection.selected.size === 0) {
    return null;
  }
  return (
    <SelectionBar>
      {selection.bar}
      <SelectAllControls all={all} />
    </SelectionBar>
  );
}
