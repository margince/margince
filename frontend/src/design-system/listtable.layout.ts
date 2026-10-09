// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How a list table divides its width between its columns. A column the reader
// dragged keeps its width between visits. The frozen edge is measured here too.

import {
  type CSSProperties,
  type RefObject,
  type UIEvent,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { readStoredJson, STORAGE_KEYS, writeStored } from "../app/storage";

/**
 * How the table divides its width when the reader has not resized anything.
 * Sized to its content, a name column with an avatar and a badge takes half
 * the page. So each column takes a share weighted by what it holds.
 *
 * The minimum binds per column, not as a sum, so a narrow share never cuts a
 * header off. A `share` of null takes its minimum in pixels, which suits a
 * column of buttons whose width is its translated labels.
 */
type ColumnSize = Readonly<{ share: number | null; min: number }>;

const COLUMN_SIZES: Readonly<
  Record<"identity" | "numeric" | "standard" | "verbs", ColumnSize>
> = {
  identity: { share: 2.4, min: 200 },
  numeric: { share: 0.9, min: 110 },
  standard: { share: 1.3, min: 130 },
  // Two labelled ghost buttons side by side in the longest locale this tree
  // ships: German turns "Edit product" into "Produkt bearbeiten".
  verbs: { share: null, min: 320 },
};

/** The column flags that decide a column's size. */
export type SizedColumn = Readonly<{
  key: string;
  fixed?: boolean;
  numeric?: boolean;
  verbs?: boolean;
}>;

/**
 * Whether a fresh reading of the scroller's width should be adopted.
 *
 * With classic scrollbars the reading can flip between two widths, as each
 * scrollbar brings in the other. React then fails with "Maximum update depth
 * exceeded". So a return to the width of a render ago is refused, and a real
 * resize still lands with a third width.
 */
export function widthWorthAdopting(
  next: number,
  current: number,
  beforeThat: number,
): boolean {
  return next !== current && next !== beforeThat;
}

function sizeOf(column: SizedColumn): ColumnSize {
  if (column.fixed) {
    return COLUMN_SIZES.identity;
  }
  if (column.verbs) {
    return COLUMN_SIZES.verbs;
  }
  return column.numeric ? COLUMN_SIZES.numeric : COLUMN_SIZES.standard;
}

/**
 * The table's width floor, as the custom property the stylesheet reads.
 *
 * Declared rather than asserted onto `CSSProperties`: a cast would claim React
 * knows this property. The intersection says what is true.
 */
type FloorStyle = CSSProperties & Readonly<{ "--lt-floor": string }>;

export function floorStyle(floor: number): FloorStyle {
  return { "--lt-floor": `${floor}px` };
}

// Stored per table, so a column widened for one list's data stays that way
// tomorrow and never reshapes another list.
function readWidths(key?: string): Record<string, number> {
  if (!key) {
    return {};
  }
  return (
    readStoredJson(
      { family: STORAGE_KEYS.tableWidths, member: key },
      widthsIn,
    ) ?? {}
  );
}

function widthsIn(value: unknown): Record<string, number> | null {
  if (typeof value !== "object" || value === null) {
    return null;
  }
  return Object.fromEntries(
    Object.entries(value).filter(
      (entry): entry is [string, number] =>
        typeof entry[1] === "number" && Number.isFinite(entry[1]),
    ),
  );
}

function writeWidths(key: string | undefined, widths: Record<string, number>) {
  if (!key) {
    return;
  }
  writeStored(
    { family: STORAGE_KEYS.tableWidths, member: key },
    JSON.stringify(widths),
  );
}

type Widths = Readonly<Record<string, number>>;

/** Every shown column's width, and where the row stops shrinking. */
export type ColumnLayout = Readonly<{
  widthOf: (column: SizedColumn) => string | undefined;
  floor: number;
  slack: string | undefined;
}>;

/**
 * Lays the shown columns out in pixels, never below each one's minimum. A
 * `<col>` under fixed layout takes a bare percentage only, so no minimum fits.
 *
 * Dragged columns and verb columns keep their pixels. The rest divide what is
 * left by their shares, so hiding a column widens the others.
 */
export function columnLayout(
  shown: readonly SizedColumn[],
  widths: Widths,
  available: number,
): ColumnLayout {
  const pinnedWidth = (column: SizedColumn) => {
    const resized = widths[column.key];
    if (resized) {
      return resized;
    }
    const { share, min } = sizeOf(column);
    return share === null ? min : undefined;
  };
  const shareOf = (column: SizedColumn) =>
    pinnedWidth(column) === undefined ? (sizeOf(column).share ?? 0) : 0;
  const shares = shown.reduce((total, column) => total + shareOf(column), 0);
  // The shares divide what the pinned columns leave, not the whole width, or
  // the last column would be pushed off the edge.
  const claimed = shown.reduce(
    (total, column) => total + (pinnedWidth(column) ?? 0),
    0,
  );
  const spare = Math.max(0, available - claimed);
  // Not rounded: rounding each column up makes the table wider than its box,
  // which draws a scrollbar over nothing.
  const widthPxOf = (column: SizedColumn) =>
    pinnedWidth(column) ??
    Math.max(sizeOf(column).min, (spare * shareOf(column)) / shares);
  return {
    widthOf: (column) =>
      shares <= 0 && pinnedWidth(column) === undefined
        ? undefined
        : `${widthPxOf(column)}px`,
    floor: shown.reduce((total, column) => total + widthPxOf(column), 0),
    // Once every column is pinned narrower than the page, a trailing gap takes
    // the rest. Handing it back to the columns would undo the widths just set.
    slack: shares === 0 ? undefined : "0px",
  };
}

/**
 * How much room the columns have, which only the browser knows. It is read
 * before paint, so the first frame is right. It is re-read after every render,
 * so a body that arrives late is measured.
 */
function useAvailableWidth(
  scroller: RefObject<HTMLDivElement | null>,
  drawsOwnTable: boolean,
): number {
  const [available, setAvailable] = useState(0);
  const [, setResized] = useState(0);
  // The width a render ago, which makes re-measuring after every render safe.
  const previous = useRef(0);
  useLayoutEffect(() => {
    if (!scroller.current) {
      return;
    }
    const next = scroller.current.clientWidth;
    if (!widthWorthAdopting(next, available, previous.current)) {
      return;
    }
    previous.current = available;
    setAvailable(next);
  });
  // Re-attached when the scroller comes or goes, since an observer holding a
  // detached node follows nothing. The dep is a trigger the effect never reads.
  // biome-ignore lint/correctness/useExhaustiveDependencies: trigger-only dep
  useEffect(() => {
    const scrolling = scroller.current;
    // Without an observer (jsdom) the widths stay right for this render and
    // simply stop following a resize.
    if (!scrolling || typeof ResizeObserver === "undefined") {
      return;
    }
    // The observer asks for a fresh look instead of measuring, so every width
    // adopted passes the oscillation guard in the layout effect above.
    const observer = new ResizeObserver(() => setResized((n) => n + 1));
    observer.observe(scrolling);
    return () => observer.disconnect();
  }, [drawsOwnTable]);
  return available;
}

/** What a header's grip calls while the reader drags a column edge. */
export type ColumnResize = Readonly<{
  resizing: boolean;
  onResizeStart: () => void;
  onResize: (key: string, width: number) => void;
  onResizeEnd: () => void;
}>;

/**
 * The column widths a reader dragged, remembered under `widthsKey`. A drag
 * measures the other columns once when it starts and writes storage once when
 * it ends. Those edges read the ref, since a handler holds stale state.
 */
function useResizedWidths(
  widthsKey: string | undefined,
  measured: () => Record<string, number>,
): ColumnResize & { widths: Widths } {
  const [widths, setWidths] = useState<Widths>(() => readWidths(widthsKey));
  const live = useRef(widths);
  const applyWidths = (next: Widths) => {
    live.current = next;
    setWidths(next);
  };
  // The table wears this while a drag runs. The grid then stops selecting text
  // and keeps the resize cursor under a pointer that left the grip.
  const [resizing, setResizing] = useState(false);
  return {
    widths,
    resizing,
    // The other columns are pinned at what they measure first, so dragging one
    // edge moves that edge and the table grows or shrinks around it.
    onResizeStart: () => {
      setResizing(true);
      applyWidths({ ...measured(), ...live.current });
    },
    onResize: (key, width) => applyWidths({ ...live.current, [key]: width }),
    onResizeEnd: () => {
      setResizing(false);
      writeWidths(widthsKey, live.current);
    },
  };
}

/**
 * The widths of the shown columns, the floor the row stops shrinking at, and
 * the handlers that let a reader drag a column edge.
 */
export function useColumnWidths(
  shown: readonly SizedColumn[],
  widthsKey: string | undefined,
  refs: Readonly<{
    scroller: RefObject<HTMLDivElement | null>;
    head: RefObject<HTMLTableElement | null>;
  }>,
  drawsOwnTable: boolean,
): ColumnLayout & ColumnResize {
  // What the columns are on screen now, read off the rendered header: only it
  // knows what a share actually came out as.
  const measured = (): Record<string, number> => {
    const cells = refs.head.current?.tHead?.rows[0]?.cells;
    if (!cells) {
      return {};
    }
    return Object.fromEntries(
      shown.map((column, index) => [
        column.key,
        Math.round(cells[index]?.getBoundingClientRect().width ?? 0),
      ]),
    );
  };
  const { widths, ...resize } = useResizedWidths(widthsKey, measured);
  const available = useAvailableWidth(refs.scroller, drawsOwnTable);
  return { ...columnLayout(shown, widths, available), ...resize };
}

/**
 * The frozen identity column's edge. It casts a shadow only once columns have
 * slid under it, since a shadow over open space reads as a seam. CSS cannot
 * know where the frozen column ends or how tall the body is, so both are set.
 */
export function useFrozenEdge(
  scroller: RefObject<HTMLDivElement | null>,
  shownCount: number,
  dense: boolean,
) {
  const [shifted, setShifted] = useState(false);
  // The column count and the row height are triggers to re-measure, not values
  // the effect reads.
  // biome-ignore lint/correctness/useExhaustiveDependencies: trigger-only deps
  useEffect(() => {
    const body = scroller.current;
    if (!body) {
      return;
    }
    const measure = () => {
      const frozen = body.querySelector("thead .lt-identity");
      const width = frozen ? frozen.getBoundingClientRect().width : 0;
      body.style.setProperty("--lt-freeze", `${Math.round(width)}px`);
      body.style.setProperty("--lt-body", `${body.clientHeight}px`);
    };
    measure();
    // Without an observer the numbers stay right for this render and stop
    // following a resize. A table is worth more than a perfect shadow.
    if (typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(measure);
    observer.observe(body);
    const frozen = body.querySelector("thead .lt-identity");
    if (frozen) {
      observer.observe(frozen);
    }
    return () => observer.disconnect();
  }, [shownCount, dense]);
  return {
    shifted,
    onScroll: (event: UIEvent<HTMLDivElement>) => {
      const next = event.currentTarget.scrollLeft > 0;
      if (next !== shifted) {
        setShifted(next);
      }
    },
  };
}
