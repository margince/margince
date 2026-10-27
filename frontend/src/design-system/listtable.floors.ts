// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The widest figure each numeric column of a list table holds on screen, which
// the column layout takes as that column's floor.

import { type RefObject, useLayoutEffect, useRef, useState } from "react";
import type { SizedColumn } from "./listtable.layout";

/**
 * The class of `CellStrip`. A strip ends in an ellipsis by design, so a cell
 * that holds one is not a figure and sets no floor.
 */
export const CELL_STRIP_CLASS = "lt-strip";

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
  // A side with no padding declared reads as an empty string, which is zero.
  const padding =
    (Number.parseFloat(style.paddingLeft) || 0) +
    (Number.parseFloat(style.paddingRight) || 0);
  return Math.ceil(right - left + padding) + 1;
}

/** Where each numeric column sits, as `index:key` pairs joined by spaces. */
function numericPlaces(shown: readonly SizedColumn[]): string {
  return shown
    .flatMap((column, index) =>
      column.numeric ? [`${index}:${column.key}`] : [],
    )
    .join(" ");
}

/**
 * Raises `held` to the widest figure each numeric column holds in the table.
 * It only rises, so paging to smaller figures does not move the columns.
 */
function raiseFloors(
  table: HTMLTableElement,
  places: string,
  held: Record<string, number>,
) {
  const rows = table.tBodies[0]?.rows;
  // Without layout (jsdom) a range has no box, and nothing is raised.
  if (!rows || typeof Range.prototype.getBoundingClientRect !== "function") {
    return;
  }
  for (const place of places.split(" ")) {
    const colon = place.indexOf(":");
    const key = place.slice(colon + 1);
    for (const row of rows) {
      const cell = row.cells[Number(place.slice(0, colon))];
      if (cell && !cell.querySelector(`.${CELL_STRIP_CLASS}`)) {
        held[key] = Math.max(held[key] ?? 0, inkWidth(cell));
      }
    }
  }
}

/**
 * Measures the table now and again whenever its text changes or it resizes:
 * a new page, search or locale, or a font arriving. Observers report before
 * the frame paints. Returns what stops the watch.
 */
function watchFloors(
  table: HTMLTableElement,
  places: string,
  publish: (floors: string) => void,
): () => void {
  const held: Record<string, number> = {};
  const read = () => {
    raiseFloors(table, places, held);
    publish(JSON.stringify(held));
  };
  read();
  const mutations = new MutationObserver(read);
  mutations.observe(table, {
    childList: true,
    subtree: true,
    characterData: true,
  });
  const resizes =
    typeof ResizeObserver === "undefined" ? null : new ResizeObserver(read);
  resizes?.observe(table);
  return () => {
    mutations.disconnect();
    resizes?.disconnect();
  };
}

type Watch = Readonly<{
  table: HTMLTableElement;
  places: string;
  stop: () => void;
}>;

/**
 * The floors as JSON, so an unchanged reading renders nothing. A render that
 * changed neither the table nor its numeric columns measures nothing.
 */
export function useNumericFloors(
  shown: readonly SizedColumn[],
  head: RefObject<HTMLTableElement | null>,
): string {
  const places = numericPlaces(shown);
  const [floors, setFloors] = useState(NO_FLOORS);
  const watched = useRef<Watch | null>(null);
  // Every render, since the table comes and goes with the empty state. The
  // check is two comparisons; the watch above does the walking.
  useLayoutEffect(() => {
    const table = head.current;
    const current = watched.current;
    if (current?.table === table && current.places === places) {
      return;
    }
    current?.stop();
    watched.current =
      table && places !== ""
        ? { table, places, stop: watchFloors(table, places, setFloors) }
        : null;
    if (!watched.current) {
      setFloors(NO_FLOORS);
    }
  });
  // Forgotten as well as stopped. A remount, which Strict Mode runs, then
  // watches the table again instead of trusting a stopped watch.
  useLayoutEffect(
    () => () => {
      watched.current?.stop();
      watched.current = null;
    },
    [],
  );
  return floors;
}

const NO_FLOORS = "{}";
