// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The widest figure each numeric column of a list table holds on screen, which
// the column layout takes as that column's floor.

import { type RefObject, useLayoutEffect, useRef, useState } from "react";
import { flushSync } from "react-dom";
import type { NumericFloors, SizedColumn } from "./listtable.layout";

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
 * The widest figure each numeric column holds on screen now. It raises `held`,
 * which never falls, so paging to smaller figures leaves the columns still.
 */
function readFigures(
  table: HTMLTableElement,
  places: string,
  held: Record<string, number>,
): Record<string, number> {
  const onScreen: Record<string, number> = {};
  const rows = table.tBodies[0]?.rows;
  // Without layout (jsdom) a range has no box, and nothing is read.
  if (!rows || typeof Range.prototype.getBoundingClientRect !== "function") {
    return onScreen;
  }
  for (const place of places.split(" ")) {
    const colon = place.indexOf(":");
    const key = place.slice(colon + 1);
    for (const row of rows) {
      const cell = row.cells[Number(place.slice(0, colon))];
      if (cell && !cell.querySelector(`.${CELL_STRIP_CLASS}`)) {
        onScreen[key] = Math.max(onScreen[key] ?? 0, inkWidth(cell));
      }
    }
    held[key] = Math.max(held[key] ?? 0, onScreen[key] ?? 0);
  }
  return onScreen;
}

/**
 * Measures the table now and again when its text changes or it resizes: a new
 * page, search or locale, or a font arriving. Returns what stops the watch.
 */
function watchFloors(
  table: HTMLTableElement,
  places: string,
  publish: (floors: string) => void,
): () => void {
  const held: Record<string, number> = {};
  const read = () => {
    const onScreen = readFigures(table, places, held);
    return JSON.stringify({ held, onScreen } satisfies NumericFloors);
  };
  publish(read());
  // Renders before the frame paints, so new rows show no figure cut short. A
  // resize stays scheduled, since rendering inside it resizes the table again.
  const mutations = new MutationObserver(() => {
    const floors = read();
    flushSync(() => publish(floors));
  });
  mutations.observe(table, {
    childList: true,
    subtree: true,
    characterData: true,
  });
  const resizes =
    typeof ResizeObserver === "undefined"
      ? null
      : new ResizeObserver(() => publish(read()));
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

const NO_FLOORS = JSON.stringify({
  held: {},
  onScreen: {},
} satisfies NumericFloors);
