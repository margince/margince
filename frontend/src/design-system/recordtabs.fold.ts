// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which tabs fold behind the strip's "More" control when the row is too narrow.
// The arithmetic is a plain function here, so a test can hold to it.

import {
  type FocusEvent,
  type RefObject,
  useLayoutEffect,
  useMemo,
  useRef,
  useSyncExternalStore,
} from "react";

/**
 * The tabs that do not fit, in their own order. The open tab always stays on
 * the row. The rest keep a run from the start, because a gap in the middle
 * would read as a reordered strip. `inset` is the padding the row's first tab
 * gives back, since every width is measured as if the tab stood later.
 */
export function foldToFit<Option extends string>(
  options: readonly Option[],
  open: Option,
  widthOf: (option: Option) => number,
  space: { room: number; more: number; inset: number },
): Option[] {
  const room = space.room + space.inset;
  const total = options.reduce((sum, option) => sum + widthOf(option), 0);
  if (total <= room) return [];
  let left = room - space.more - widthOf(open);
  const kept = new Set<Option>([open]);
  for (const option of options) {
    if (option === open) continue;
    if (widthOf(option) > left) break;
    left -= widthOf(option);
    kept.add(option);
  }
  return options.filter((option) => !kept.has(option));
}

// A folded tab is not on the page to measure, so each strip remembers the
// width every tab had when it was last drawn.
const lastWidths = new WeakMap<HTMLElement, Map<string, number>>();

const SEPARATOR = "\u0000";

/**
 * Follows which tabs fold, read again whenever the strip changes size. Where
 * nothing can be measured (no `ResizeObserver`), nothing folds and the strip
 * scrolls as it always has.
 */
export function useFoldedTabs<Option extends string>(
  strip: RefObject<HTMLElement | null>,
  options: readonly Option[],
  open: Option,
): Option[] {
  const subscribe = useMemo(() => watchSize(strip), [strip]);
  const key = useSyncExternalStore(
    subscribe,
    () => (strip.current ? foldKey(strip.current, options, open) : ""),
    () => "",
  );
  return options.filter((option) => key.split(SEPARATOR).includes(option));
}

function foldKey<Option extends string>(
  element: HTMLElement,
  options: readonly Option[],
  open: Option,
): string {
  const widths = lastWidths.get(element) ?? new Map<string, number>();
  lastWidths.set(element, widths);
  // The first tab drops its start padding, so each width is stored as if the
  // tab stood later in the row. Otherwise a fold changes the widths it reads.
  let inset = 0;
  for (const tab of element.querySelectorAll<HTMLElement>("[data-tab]")) {
    const { paddingInlineStart, paddingInlineEnd } = getComputedStyle(tab);
    const missing = Math.max(
      0,
      parseFloat(paddingInlineEnd) - parseFloat(paddingInlineStart) || 0,
    );
    inset = Math.max(inset, missing);
    widths.set(tab.dataset.tab ?? "", tab.offsetWidth + missing);
  }
  const more = element.querySelector<HTMLElement>(".recordtabs-more");
  return foldToFit(options, open, (option) => widths.get(option) ?? 0, {
    room: element.clientWidth,
    more: more?.offsetWidth ?? 0,
    inset,
  }).join(SEPARATOR);
}

function watchSize(strip: RefObject<HTMLElement | null>) {
  return (changed: () => void) => {
    const element = strip.current;
    if (!element || typeof ResizeObserver === "undefined") {
      return () => undefined;
    }
    const observer = new ResizeObserver(changed);
    observer.observe(element);
    return () => observer.disconnect();
  };
}

/**
 * When the last folded tab comes back, focus on "More" or in its menu moves
 * to the open tab. A keyboard then keeps its place on the row.
 */
export function useFocusKeptOnUnfold(
  strip: RefObject<HTMLElement | null>,
  idle: boolean,
  open: string,
) {
  const inMore = useRef(false);
  useLayoutEffect(() => {
    const lost = document.activeElement === document.body;
    if (!idle || !inMore.current || !lost) return;
    inMore.current = false;
    strip.current
      ?.querySelector<HTMLElement>(`[data-tab="${CSS.escape(open)}"]`)
      ?.focus();
  }, [strip, idle, open]);
  return {
    onFocus: () => {
      inMore.current = true;
    },
    // Focus moving inside the menu blurs and then focuses again; only a move
    // to somewhere named elsewhere leaves.
    onBlur: (event: FocusEvent) => {
      if (event.relatedTarget) inMore.current = false;
    },
  };
}
