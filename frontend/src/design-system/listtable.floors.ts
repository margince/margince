// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Floors for the column layout: the widest figure each numeric column holds on
// screen, and the longest word of each header.

import { type RefObject, useLayoutEffect, useRef, useState } from "react";
import { flushSync } from "react-dom";
import type { ColumnFloors, SizedColumn } from "./listtable.layout";

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
  return Math.ceil(right - left + paddingOf(cell)) + 1;
}

function paddingOf(cell: HTMLTableCellElement): number {
  const style = getComputedStyle(cell);
  // A side with no padding declared reads as an empty string, which is zero.
  return (
    (Number.parseFloat(style.paddingLeft) || 0) +
    (Number.parseFloat(style.paddingRight) || 0)
  );
}

/**
 * The longest word of a header, with the sort arrow's slot and the cell's
 * padding. A header wraps only between words, so a narrower column clips one.
 */
function headerWidth(cell: HTMLTableCellElement): number {
  const walker = document.createTreeWalker(cell, NodeFilter.SHOW_TEXT);
  const range = document.createRange();
  let widest = 0;
  for (let text = walker.nextNode(); text; text = walker.nextNode()) {
    for (const word of (text.textContent ?? "").matchAll(/\S+/g)) {
      range.setStart(text, word.index);
      range.setEnd(text, word.index + word[0].length);
      // A hyphenated word comes back in pieces, one per line.
      const pieces = Array.from(range.getClientRects());
      widest = Math.max(
        widest,
        pieces.reduce((sum, piece) => sum + piece.width, 0),
      );
    }
  }
  if (widest === 0) {
    return 0;
  }
  const arrow = cell.querySelector(".lt-arrow")?.getBoundingClientRect().width;
  const button = cell.querySelector(".lt-sort");
  const slot =
    arrow && button
      ? arrow + (Number.parseFloat(getComputedStyle(button).columnGap) || 0)
      : 0;
  return Math.ceil(widest + slot + paddingOf(cell)) + 1;
}

/** Where each column that passes `measured` sits, as `index:key` pairs. */
function placesOf(
  shown: readonly SizedColumn[],
  measured: (column: SizedColumn) => boolean,
): string {
  return shown
    .flatMap((column, index) =>
      measured(column) ? [`${index}:${column.key}`] : [],
    )
    .join(" ");
}

function eachPlace(places: string): [number, string][] {
  if (places === "") {
    return [];
  }
  return places.split(" ").map((place) => {
    const colon = place.indexOf(":");
    return [Number(place.slice(0, colon)), place.slice(colon + 1)];
  });
}

// The identity column's minimum already holds its header, and a verb column's
// header is never drawn.
const headed = (column: SizedColumn) => !column.fixed && !column.verbs;

/** Which columns are measured: numeric ones for figures, the rest for headers. */
type Places = Readonly<{ numeric: string; headed: string }>;

/**
 * The widest figure each numeric column holds on screen now. It raises `held`,
 * which never falls, so smaller figures paged in never narrow an undragged
 * column.
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
  for (const [index, key] of eachPlace(places)) {
    for (const row of rows) {
      const cell = row.cells[index];
      if (cell && !cell.querySelector(`.${CELL_STRIP_CLASS}`)) {
        onScreen[key] = Math.max(onScreen[key] ?? 0, inkWidth(cell));
      }
    }
    held[key] = Math.max(held[key] ?? 0, onScreen[key] ?? 0);
  }
  return onScreen;
}

/**
 * Raises `held` to each header's longest word as drawn now. Only `held`: a
 * reader may drag a column narrower than its header.
 */
function readHeaders(
  table: HTMLTableElement,
  places: string,
  held: Record<string, number>,
) {
  if (typeof Range.prototype.getClientRects !== "function") {
    return;
  }
  // happy-dom draws a table section with no `rows`.
  const cells = table.tHead?.rows?.[0]?.cells;
  if (!cells) {
    return;
  }
  for (const [index, key] of eachPlace(places)) {
    const cell = cells[index];
    if (cell) {
      held[key] = Math.max(held[key] ?? 0, headerWidth(cell));
    }
  }
}

/**
 * Measures the table now and again when its text changes or it resizes: a new
 * page, search or locale, or a font arriving. Returns what stops the watch.
 */
function watchFloors(
  table: HTMLTableElement,
  places: Places,
  publish: (floors: string) => void,
): () => void {
  const figures: Record<string, number> = {};
  const read = () => {
    const onScreen = readFigures(table, places.numeric, figures);
    // Headers start over on each read, so a shorter language lets go of the
    // longer one's words; only figures keep what earlier pages held.
    const held = { ...figures };
    readHeaders(table, places.headed, held);
    return JSON.stringify({ held, onScreen } satisfies ColumnFloors);
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
 * changed neither the table nor its columns measures nothing.
 */
export function useColumnFloors(
  shown: readonly SizedColumn[],
  head: RefObject<HTMLTableElement | null>,
): string {
  const places: Places = {
    numeric: placesOf(shown, (column) => column.numeric === true),
    headed: placesOf(shown, headed),
  };
  const key = `${places.numeric}|${places.headed}`;
  const [floors, setFloors] = useState(NO_FLOORS);
  const watched = useRef<Watch | null>(null);
  // Every render, since the table comes and goes with the empty state. The
  // check is two comparisons; the watch above does the walking.
  useLayoutEffect(() => {
    const table = head.current;
    const current = watched.current;
    if (current?.table === table && current.places === key) {
      return;
    }
    current?.stop();
    watched.current =
      table && key !== "|"
        ? { table, places: key, stop: watchFloors(table, places, setFloors) }
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
} satisfies ColumnFloors);
