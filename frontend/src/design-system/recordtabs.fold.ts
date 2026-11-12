// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which tabs fold behind the strip's "More" control when the row is too narrow.
// The arithmetic is a plain function here, so a test can hold to it.

import {
  type RefObject,
  useEffect,
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
    () =>
      strip.current && typeof ResizeObserver !== "undefined"
        ? foldKey(strip.current, options, open)
        : "",
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
    // A late web font widens the tabs without resizing the strip.
    let live = true;
    const settled = () => {
      if (live) changed();
    };
    document.fonts?.ready.then(settled, settled);
    return () => {
      live = false;
      observer.disconnect();
    };
  };
}

/**
 * Keeps keyboard focus on the row while tabs fold and come back. A focused tab
 * that folds hands focus to "More"; focus in "More" when the last tab returns
 * lands on the open tab.
 */
export function useFocusKeptOnFold(
  strip: RefObject<HTMLElement | null>,
  folded: readonly string[],
  open: string,
) {
  const inStrip = useRef(false);
  const foldedKey = folded.join(SEPARATOR);
  // Passive, so it runs after the closing menu's own focus restore.
  useEffect(() => {
    const element = strip.current;
    if (!inStrip.current || !element) return;
    const idle = foldedKey === "";
    const more = element.querySelector<HTMLElement>(".recordtabs-more");
    if (!focusLost(more, idle)) return;
    const target = idle
      ? element.querySelector<HTMLElement>(`[data-tab="${CSS.escape(open)}"]`)
      : more?.querySelector<HTMLElement>("button");
    target?.focus();
  }, [strip, foldedKey, open]);
  return {
    onFocus: () => {
      inStrip.current = true;
    },
    // A removed element fires no blur, so focus that fold or unfold took
    // away still counts as on the row.
    onBlur: () => {
      inStrip.current = false;
    },
  };
}

// Lost means on the page, on a removed tab or entry, or on the unseen trigger
// a closing menu hands it back to.
function focusLost(more: HTMLElement | null, idle: boolean): boolean {
  const active = document.activeElement;
  if (!active || active === document.body || !active.isConnected) return true;
  return idle && more?.contains(active) === true;
}
