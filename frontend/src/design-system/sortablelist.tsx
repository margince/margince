// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * A list a reader puts in order by hand: a handle on each row that carries it,
 * the same handle taking ↑ and ↓ from the keyboard, and the new place said
 * aloud. The row's content is the caller's; the ORDER is this component's.
 *
 * Pointer events rather than HTML drag and drop. Native drag and drop is a
 * mouse gesture most phones never deliver, and its drag image is a bitmap the
 * page cannot style; a pointer is one path for mouse, pen and touch, and the
 * row itself travels under the finger.
 *
 * The rows make room LIVE while one is carried, and the list commits once, on
 * release: `onReorder` receives the whole new order, which is the shape the
 * reorder endpoints take. Released where it started, or cancelled by Escape or
 * a cancelled pointer, it commits nothing.
 */

import { GripVertical } from "lucide-react";
import {
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
  useEffect,
  useRef,
  useState,
} from "react";
import "./sortablelist.css";

export type SortableEntry = Readonly<{ key: string }>;

export type SortableLabels<T> = Readonly<{
  // The handle's accessible name: which row it carries and where it stands.
  handle: (item: T, position: number, count: number) => string;
  // What the live region says once a move has landed.
  moved: (item: T, position: number, count: number) => string;
}>;

/**
 * The order a carried row makes at pointer height `y`: it goes before the first
 * other row whose midpoint lies below the pointer. Only the OTHER rows vote, so
 * the carried row never decides its own place from where it is drawn.
 */
export function orderAtPointer(
  order: readonly string[],
  carried: string,
  y: number,
  midpointOf: (key: string) => number,
): string[] {
  const others = order.filter((key) => key !== carried);
  const below = others.findIndex((key) => y < midpointOf(key));
  return moveKey(order, carried, below < 0 ? others.length : below);
}

/** `order` with `key` taken out and put back at index `to`. */
export function moveKey(
  order: readonly string[],
  key: string,
  to: number,
): string[] {
  const others = order.filter((each) => each !== key);
  return [...others.slice(0, to), key, ...others.slice(to)];
}

function sameOrder(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((key, index) => key === b[index]);
}

// Keys are unique within a list, so equal size and containment is set equality.
function sameKeys(a: readonly string[], b: readonly string[]): boolean {
  const held = new Set(b);
  return a.length === b.length && a.every((key) => held.has(key));
}

const STEP: Readonly<Record<string, number>> = { ArrowUp: -1, ArrowDown: 1 };

// The run of movable keys around `key`, as [from, to). A fixed row is a wall:
// no move carries a row past one.
function runOf(
  keys: readonly string[],
  key: string,
  movable: (key: string) => boolean,
): readonly [number, number] {
  const at = keys.indexOf(key);
  const movableAt = (index: number) => {
    const candidate = keys[index];
    return candidate !== undefined && movable(candidate);
  };
  let from = at;
  while (from > 0 && movableAt(from - 1)) from -= 1;
  let to = at + 1;
  while (to < keys.length && movableAt(to)) to += 1;
  return [from, to];
}

type Carry = {
  key: string;
  start: readonly string[];
  order: readonly string[];
  run: readonly [number, number];
};

/** What makes rows sortable; absent, nothing is carried and no grip is drawn. */
export type SortableRows<T> = Readonly<{
  onReorder: (keys: string[], moved: T) => void;
  labels: SortableLabels<T>;
  // A write is out. The handles stay focusable and refuse, so a second order
  // cannot be drawn from one the server has not answered yet.
  busy?: boolean;
  // A sentence the page draws saying how to reorder, named by every handle.
  hintId?: string;
  // A row that answers false draws no grip and is a wall the others stay
  // inside; positions are counted within the run between two walls.
  can?: (item: T) => boolean;
}>;

/**
 * The gesture and keyboard core behind every hand-ordered list: the caller
 * draws the rows in `shown` order, hangs `rowRef` on each and places `grip`.
 */
export function useSortableRows<T>(
  items: readonly T[],
  keyOf: (item: T) => string,
  sort: SortableRows<T> | undefined,
) {
  const [draft, setDraft] = useState<Readonly<{
    key: string;
    order: readonly string[];
  }> | null>(null);
  const [announcement, setAnnouncement] = useState("");
  const carry = useRef<Carry | null>(null);
  const rows = useRef(new Map<string, HTMLElement>());
  // The gesture outlives the render it started in, so what it commits to is
  // read from here rather than from that render's closure.
  const latest = useRef({ items, keyOf, sort });
  useEffect(() => {
    latest.current = { items, keyOf, sort };
  });
  // The gesture listens on the window, not on the handle: reordering can move
  // the carried row's own element, and a moved element loses pointer capture
  // in some engines, which would drop the row mid-carry.
  const stopListening = useRef<(() => void) | null>(null);
  useEffect(() => () => stopListening.current?.(), []);
  // A keyboard move re-renders the row somewhere else, and a moved element does
  // not keep focus in every engine — so the handle is found again after the
  // render has put it there.
  const focusAfter = useRef<string | null>(null);
  useEffect(() => {
    const key = focusAfter.current;
    if (key === null) {
      return;
    }
    focusAfter.current = null;
    rows.current
      .get(key)
      ?.querySelector<HTMLButtonElement>(".sortable-handle")
      ?.focus();
  });

  const byKey = new Map(items.map((item) => [keyOf(item), item]));
  const order = draft?.order ?? items.map(keyOf);
  const shown = order.flatMap((key) => {
    const item = byKey.get(key);
    return item ? [item] : [];
  });
  const keys = shown.map(keyOf);
  const busy = sort?.busy ?? false;

  const movableIn = (current: typeof latest.current) => {
    const rowOf = new Map(
      current.items.map((each) => [current.keyOf(each), each]),
    );
    return (key: string) => {
      const item = rowOf.get(key);
      return item !== undefined && (current.sort?.can?.(item) ?? true);
    };
  };
  const movable = movableIn({ items, keyOf, sort });

  const commit = (next: string[], moved: T) => {
    const current = latest.current;
    if (!current.sort) {
      return;
    }
    current.sort.onReorder(next, moved);
    const key = keyOf(moved);
    const [from, to] = runOf(next, key, movableIn(current));
    setAnnouncement(
      current.sort.labels.moved(moved, next.indexOf(key) - from + 1, to - from),
    );
  };

  const finish = (landed: boolean) => {
    const carried = carry.current;
    if (!carried) {
      return;
    }
    carry.current = null;
    stopListening.current?.();
    stopListening.current = null;
    setDraft(null);
    // A write that started mid-carry makes the carried order one drawn from a
    // list the server has not answered yet, so it lands nowhere. So does a row
    // added or taken away while the row was carried.
    const current = latest.current;
    const moved = current.items.find(
      (each) => current.keyOf(each) === carried.key,
    );
    if (
      landed &&
      !current.sort?.busy &&
      moved &&
      sameKeys(current.items.map(current.keyOf), carried.start) &&
      !sameOrder(carried.order, carried.start)
    ) {
      commit([...carried.order], moved);
    }
  };

  const travel = (y: number) => {
    const carried = carry.current;
    if (!carried) {
      return;
    }
    const [from, to] = carried.run;
    const run = orderAtPointer(
      carried.order.slice(from, to),
      carried.key,
      y,
      (key) => {
        const box = rows.current.get(key)?.getBoundingClientRect();
        return box ? box.top + box.height / 2 : Number.POSITIVE_INFINITY;
      },
    );
    const next = [
      ...carried.order.slice(0, from),
      ...run,
      ...carried.order.slice(to),
    ];
    if (!sameOrder(next, carried.order)) {
      carried.order = next;
      setDraft({ key: carried.key, order: next });
    }
  };

  const pickUp = (item: T, event: PointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0 || carry.current) {
      return;
    }
    if (busy) {
      swallowClickOnRelease(event.pointerId);
      return;
    }
    // No text selection and no scroll under a carried row: the gesture is the
    // handle's alone.
    event.preventDefault();
    const pointerId = event.pointerId;
    const key = keyOf(item);
    const run = runOf(keys, key, movable);
    carry.current = { key, start: keys, order: keys, run };
    setDraft({ key, order: keys });
    const onMove = (move: globalThis.PointerEvent) => {
      if (move.pointerId === pointerId) {
        travel(move.clientY);
      }
    };
    const onUp = (up: globalThis.PointerEvent) => {
      if (up.pointerId === pointerId) {
        finish(true);
        swallowNextClick();
      }
    };
    const onCancel = (cancel: globalThis.PointerEvent) => {
      if (cancel.pointerId === pointerId) {
        finish(false);
      }
    };
    const onKey = (press: globalThis.KeyboardEvent) => {
      if (press.key === "Escape") {
        press.preventDefault();
        finish(false);
        swallowClickOnRelease(pointerId);
      }
    };
    globalThis.addEventListener("pointermove", onMove);
    globalThis.addEventListener("pointerup", onUp);
    globalThis.addEventListener("pointercancel", onCancel);
    globalThis.addEventListener("keydown", onKey);
    stopListening.current = () => {
      globalThis.removeEventListener("pointermove", onMove);
      globalThis.removeEventListener("pointerup", onUp);
      globalThis.removeEventListener("pointercancel", onCancel);
      globalThis.removeEventListener("keydown", onKey);
    };
  };

  const step = (item: T, event: KeyboardEvent<HTMLButtonElement>) => {
    const delta = STEP[event.key];
    if (delta === undefined || carry.current) {
      return;
    }
    event.preventDefault();
    const key = keyOf(item);
    const to = keys.indexOf(key) + delta;
    const [from, end] = runOf(keys, key, movable);
    if (busy || to < from || to >= end) {
      return;
    }
    focusAfter.current = key;
    commit(moveKey(keys, key, to), item);
  };

  const grip = (item: T): ReactNode => {
    const key = keyOf(item);
    if (!sort || !movable(key)) {
      return null;
    }
    const [from, to] = runOf(keys, key, movable);
    return (
      <button
        type="button"
        className="iconbtn sortable-handle"
        aria-label={sort.labels.handle(
          item,
          keys.indexOf(key) - from + 1,
          to - from,
        )}
        aria-describedby={sort.hintId}
        aria-disabled={busy || undefined}
        onPointerDown={(event) => pickUp(item, event)}
        onKeyDown={(event) => step(item, event)}
      >
        <GripVertical aria-hidden="true" />
      </button>
    );
  };

  const rowRef = (key: string) => (node: HTMLElement | null) => {
    if (node) {
      rows.current.set(key, node);
    } else {
      rows.current.delete(key);
    }
  };

  return {
    shown,
    lifted: draft?.key,
    sorting: draft !== null,
    rowRef,
    grip,
    live: (
      <span className="sr-only" aria-live="polite">
        {announcement}
      </span>
    ),
  };
}

// The press that ends a carry lands on whatever is under the pointer, often the
// carried row itself, and must not open it.
function swallowNextClick() {
  const swallow = (click: MouseEvent) => {
    click.preventDefault();
    click.stopPropagation();
  };
  globalThis.addEventListener("click", swallow, { capture: true, once: true });
  setTimeout(
    () => globalThis.removeEventListener("click", swallow, { capture: true }),
    0,
  );
}

// A press that carries nothing, refused or cancelled, still ends in a click.
function swallowClickOnRelease(pointerId: number) {
  const release = (end: globalThis.PointerEvent) => {
    if (end.pointerId !== pointerId) {
      return;
    }
    globalThis.removeEventListener("pointerup", release);
    globalThis.removeEventListener("pointercancel", release);
    if (end.type === "pointerup") {
      swallowNextClick();
    }
  };
  globalThis.addEventListener("pointerup", release);
  globalThis.addEventListener("pointercancel", release);
}

const entryKey = (entry: SortableEntry) => entry.key;

export function SortableList<T extends SortableEntry>({
  label,
  items,
  renderItem,
  onReorder,
  busy = false,
  labels,
  hintId,
  selectedKey,
}: Readonly<{
  // What the list is a list OF, for the reader who lands on it.
  label: string;
  items: readonly T[];
  renderItem: (item: T, position: number) => ReactNode;
  // Absent, the list is read-only: no handles, nothing to carry. A reader who
  // may not change the order is not shown a grip that refuses them.
  onReorder?: (keys: string[], moved: T) => void;
  busy?: boolean;
  labels: SortableLabels<T>;
  hintId?: string;
  // The row the surface around the list is showing, drawn as chosen.
  selectedKey?: string;
}>) {
  const sortable = useSortableRows(
    items,
    entryKey,
    onReorder && { onReorder, labels, busy, hintId },
  );
  return (
    <>
      <ol
        className={
          sortable.sorting ? "sortable-list is-sorting" : "sortable-list"
        }
        aria-label={label}
      >
        {sortable.shown.map((item, index) => (
          <li
            key={item.key}
            ref={sortable.rowRef(item.key)}
            className={[
              "sortable-item",
              sortable.lifted === item.key ? "is-lifted" : "",
              selectedKey === item.key ? "is-selected" : "",
            ]
              .filter(Boolean)
              .join(" ")}
          >
            {sortable.grip(item)}
            <div className="sortable-body">{renderItem(item, index + 1)}</div>
          </li>
        ))}
      </ol>
      {sortable.live}
    </>
  );
}
