// The header cells name their roles for the reason listgrid.tsx gives.
// biome-ignore-all lint/a11y/useSemanticElements: the semantic element is already in use

import { ChevronDown } from "lucide-react";
import { useRef, useState } from "react";
import { useT } from "../i18n";
import type { ListColumn } from "./listcolumn";
import { nextSortValue, type SortControl } from "./listsurface";

/** Narrow enough to tuck a column away, wide enough to still read a header. */
const MIN_COLUMN_WIDTH = 72;

/**
 * A column header. Sortable only when the column names a server sort field,
 * so the arrow never appears on a column the API cannot order by.
 */
export function HeaderCell<Row>({
  column,
  sort,
  state,
  className,
  resizable,
  onResizeStart,
  onResize,
  onResizeEnd,
}: Readonly<{
  column: ListColumn<Row>;
  sort?: SortControl;
  state: "asc" | "desc" | null;
  className?: string;
  // False on the trailing column, which carries no divider for the grip to
  // light (`th:nth-last-child(2)` clears it) and no neighbour to take the
  // width from. A grip there drew a line against nothing.
  resizable: boolean;
  onResizeStart: () => void;
  onResize: (key: string, width: number) => void;
  onResizeEnd: () => void;
}>) {
  const t = useT();
  const grip = resizable ? (
    <ResizeGrip
      onStart={onResizeStart}
      onResize={(next) => onResize(column.key, next)}
      onEnd={onResizeEnd}
    />
  ) : null;
  if (!column.sort || !sort) {
    return (
      <th className={className} role="columnheader">
        {/* A menu column's heading is read, never drawn: forced colours would show any ink. */}
        {column.verbs === "menu" ? (
          <span className="sr-only">{column.header}</span>
        ) : (
          column.header
        )}
        {grip}
      </th>
    );
  }
  // The same arithmetic the sort menu presses, so a reader who flips a column
  // here finds that direction there.
  const next = nextSortValue(
    { field: column.sort, numeric: column.numeric },
    state,
  );
  return (
    <th
      className={className}
      role="columnheader"
      aria-sort={
        state === "asc"
          ? "ascending"
          : state === "desc"
            ? "descending"
            : undefined
      }
    >
      <button
        type="button"
        className={`lt-sort${state ? " on" : ""}`}
        aria-label={t("table.sortBy", { column: column.header })}
        onClick={() => sort.onChange(next)}
      >
        {column.header}
        <ChevronDown
          size={12}
          strokeWidth={2}
          aria-hidden="true"
          className={`lt-arrow${state === "asc" ? " up" : ""}`}
        />
      </button>
      {grip}
    </th>
  );
}

/**
 * The handle on a column's trailing edge, dragged with a pointer.
 *
 * It draws nothing at rest. The line a reader sees between two columns is the
 * cell's own `border-right`, already there for every column. A second line
 * inset beside it read as a rendering fault rather than as an affordance.
 * The grip lights that edge on hover, and keeps it lit for the length of a
 * drag. It uses `is-dragging` rather than `:hover`, because a pointer dragged
 * past the neighbouring column is no longer over the element it is moving.
 *
 * It is hidden from assistive technology. A labelled control inside a `th`
 * joins that header's accessible name. Every column would then announce as
 * "Value, resize the Value column". A keyboard affordance here would make
 * every header read worse for the contacts who rely on the name most. The
 * Display menu already gives keyboard users control over what a table shows,
 * and a width is presentation rather than content.
 */
function ResizeGrip({
  onStart,
  onResize,
  onEnd,
}: Readonly<{
  onStart: () => void;
  onResize: (width: number) => void;
  onEnd: () => void;
}>) {
  const drag = useRef<{ startX: number; startWidth: number } | null>(null);
  const self = useRef<HTMLSpanElement>(null);
  const [dragging, setDragging] = useState(false);
  const cellWidth = (target: HTMLElement) =>
    target.closest("th")?.getBoundingClientRect().width ?? MIN_COLUMN_WIDTH;

  return (
    <span
      className={`lt-grip${dragging ? " is-dragging" : ""}`}
      ref={self}
      aria-hidden="true"
      // The grip lives inside the header button's cell; without this a drag or
      // a click on it would also sort the column it is resizing.
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => {
        event.stopPropagation();
        event.preventDefault();
        const target: HTMLElement = event.currentTarget;
        target.setPointerCapture(event.pointerId);
        drag.current = {
          startX: event.clientX,
          startWidth: cellWidth(target),
        };
        setDragging(true);
        onStart();
      }}
      onPointerMove={(event) => {
        const from = drag.current;
        if (!from) {
          return;
        }
        onResize(
          Math.max(
            MIN_COLUMN_WIDTH,
            from.startWidth + event.clientX - from.startX,
          ),
        );
      }}
      onPointerUp={(event) => {
        const wasDragging = drag.current !== null;
        drag.current = null;
        setDragging(false);
        event.currentTarget.releasePointerCapture(event.pointerId);
        if (wasDragging) {
          onEnd();
        }
      }}
      // A pointer cancelled mid-drag (a system gesture, a lost capture) never
      // raises pointerup. Without this the table would stay in its dragging
      // dress with nothing moving it. The state the user sees has to end
      // wherever the drag did.
      onPointerCancel={() => {
        const wasDragging = drag.current !== null;
        drag.current = null;
        setDragging(false);
        if (wasDragging) {
          onEnd();
        }
      }}
    />
  );
}
