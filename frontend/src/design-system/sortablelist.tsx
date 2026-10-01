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

const STEP: Readonly<Record<string, number>> = { ArrowUp: -1, ArrowDown: 1 };

type Carry = {
  key: string;
  start: readonly string[];
  order: readonly string[];
};

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
  // A write is out. The handles stay focusable and refuse, so a second order
  // cannot be drawn from one the server has not answered yet.
  busy?: boolean;
  labels: SortableLabels<T>;
  // A sentence the page draws saying how to reorder, named by every handle.
  hintId?: string;
  // The row the surface around the list is showing, drawn as chosen.
  selectedKey?: string;
}>) {
  const [draft, setDraft] = useState<Readonly<{
    key: string;
    order: readonly string[];
  }> | null>(null);
  const [announcement, setAnnouncement] = useState("");
  const carry = useRef<Carry | null>(null);
  const rows = useRef(new Map<string, HTMLLIElement>());
  // The gesture outlives the render it started in, so what it commits to is
  // read from here rather than from that render's closure.
  const latest = useRef({ items, onReorder, labels, busy });
  useEffect(() => {
    latest.current = { items, onReorder, labels, busy };
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

  const byKey = new Map(items.map((item) => [item.key, item]));
  const order = draft?.order ?? items.map((item) => item.key);
  const shown = order.flatMap((key) => {
    const item = byKey.get(key);
    return item ? [item] : [];
  });
  const count = shown.length;

  const commit = (keys: string[], moved: T) => {
    latest.current.onReorder?.(keys, moved);
    setAnnouncement(
      latest.current.labels.moved(
        moved,
        keys.indexOf(moved.key) + 1,
        keys.length,
      ),
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
    // list the server has not answered yet, so it lands nowhere.
    const current = latest.current;
    const moved = current.items.find((each) => each.key === carried.key);
    if (
      landed &&
      !current.busy &&
      moved &&
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
    const next = orderAtPointer(carried.order, carried.key, y, (key) => {
      const box = rows.current.get(key)?.getBoundingClientRect();
      return box ? box.top + box.height / 2 : Number.POSITIVE_INFINITY;
    });
    if (!sameOrder(next, carried.order)) {
      carried.order = next;
      setDraft({ key: carried.key, order: next });
    }
  };

  const pickUp = (item: T, event: PointerEvent<HTMLButtonElement>) => {
    if (busy || event.button !== 0 || carry.current) {
      return;
    }
    // No text selection and no scroll under a carried row: the gesture is the
    // handle's alone.
    event.preventDefault();
    const pointerId = event.pointerId;
    const keys = items.map((each) => each.key);
    carry.current = { key: item.key, start: keys, order: keys };
    setDraft({ key: item.key, order: keys });
    const onMove = (move: globalThis.PointerEvent) => {
      if (move.pointerId === pointerId) {
        travel(move.clientY);
      }
    };
    const onUp = (up: globalThis.PointerEvent) => {
      if (up.pointerId === pointerId) {
        finish(true);
      }
    };
    const onCancel = (cancel: globalThis.PointerEvent) => {
      if (cancel.pointerId === pointerId) {
        finish(false);
      }
    };
    const onKey = (key: globalThis.KeyboardEvent) => {
      if (key.key === "Escape") {
        key.preventDefault();
        finish(false);
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

  const step = (
    item: T,
    position: number,
    event: KeyboardEvent<HTMLButtonElement>,
  ) => {
    const delta = STEP[event.key];
    if (delta === undefined || carry.current) {
      return;
    }
    event.preventDefault();
    const to = position - 1 + delta;
    if (busy || to < 0 || to >= count) {
      return;
    }
    focusAfter.current = item.key;
    commit(moveKey(order, item.key, to), item);
  };

  return (
    <>
      <ol
        className={draft ? "sortable-list is-sorting" : "sortable-list"}
        aria-label={label}
      >
        {shown.map((item, index) => (
          <li
            key={item.key}
            ref={(node) => {
              if (node) {
                rows.current.set(item.key, node);
              } else {
                rows.current.delete(item.key);
              }
            }}
            className={[
              "sortable-item",
              draft?.key === item.key ? "is-lifted" : "",
              selectedKey === item.key ? "is-selected" : "",
            ]
              .filter(Boolean)
              .join(" ")}
          >
            {onReorder && (
              <button
                type="button"
                className="iconbtn sortable-handle"
                aria-label={labels.handle(item, index + 1, count)}
                aria-describedby={hintId}
                aria-disabled={busy || undefined}
                onPointerDown={(event) => pickUp(item, event)}
                onKeyDown={(event) => step(item, index + 1, event)}
              >
                <GripVertical aria-hidden="true" />
              </button>
            )}
            <div className="sortable-body">{renderItem(item, index + 1)}</div>
          </li>
        ))}
      </ol>
      <span className="sr-only" aria-live="polite">
        {announcement}
      </span>
    </>
  );
}
