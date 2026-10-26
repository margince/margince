// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The widest figure each numeric column of a list table holds on screen, which
// the column layout takes as that column's floor.

import { type RefObject, useMemo, useSyncExternalStore } from "react";
import type { SizedColumn } from "./listtable.layout";

/**
 * The ink width of a cell's text plus its padding. Text, not boxes: a box that
 * gives way to its cell reports the cell's width back. The floor would then
 * hold the column where it already stands.
 */
function inkWidth(cell: HTMLTableCellElement): number {
  const walker = document.createTreeWalker(cell, NodeFilter.SHOW_TEXT);
  const range = document.createRange();
  let left = Number.POSITIVE_INFINITY;
  let right = Number.NEGATIVE_INFINITY;
  for (let text = walker.nextNode(); text; text = walker.nextNode()) {
    range.selectNodeContents(text);
    const box = range.getBoundingClientRect();
    if (box.width > 0) {
      left = Math.min(left, box.left);
      right = Math.max(right, box.right);
    }
  }
  if (right <= left) {
    return 0;
  }
  const style = getComputedStyle(cell);
  const padding =
    Number.parseFloat(style.paddingLeft) +
    Number.parseFloat(style.paddingRight);
  return Math.ceil(right - left + padding) + 1;
}

/** The widest figure each numeric column holds in a rendered table. */
function numericFloors(
  table: HTMLTableElement,
  shown: readonly SizedColumn[],
): Record<string, number> {
  const rows = table.tBodies[0]?.rows;
  const floors: Record<string, number> = {};
  // Without layout (jsdom) a range has no box, and nothing is raised.
  if (!rows || typeof Range.prototype.getBoundingClientRect !== "function") {
    return floors;
  }
  shown.forEach((column, index) => {
    if (!column.numeric) {
      return;
    }
    for (const row of rows) {
      const cell = row.cells[index];
      const width = cell ? inkWidth(cell) : 0;
      if (width > (floors[column.key] ?? 0)) {
        floors[column.key] = width;
      }
    }
  });
  return floors;
}

/**
 * The floors as JSON, so an unchanged reading is an unchanged snapshot. Read
 * on every render and again after the commit, as `useClippedEdges` reads its
 * box. Text does not move with its column, so the reading settles.
 */
export function useNumericFloors(
  shown: readonly SizedColumn[],
  head: RefObject<HTMLTableElement | null>,
): string {
  const subscribe = useMemo(() => watchTable(head), [head]);
  return useSyncExternalStore(
    subscribe,
    () =>
      head.current
        ? JSON.stringify(numericFloors(head.current, shown))
        : NO_FLOORS,
    () => NO_FLOORS,
  );
}

const NO_FLOORS = "{}";

function watchTable(head: RefObject<HTMLTableElement | null>) {
  return (changed: () => void) => {
    const table = head.current;
    if (!table || typeof ResizeObserver === "undefined") {
      return () => undefined;
    }
    const observer = new ResizeObserver(changed);
    observer.observe(table);
    return () => observer.disconnect();
  };
}
