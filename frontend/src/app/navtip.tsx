// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type ReactNode,
  type RefObject,
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { useEscapeDismisses } from "../design-system/tooltip";

type TipRaise = "pointer" | "focus";

/** The rail's one tip at a time, which Escape hides as any tip. */
export function useNavTip(
  collapsed: boolean,
  nav: RefObject<HTMLElement | null>,
) {
  const [tip, setTip] = useState<string | null>(null);
  const raisedBy = useRef<TipRaise>("focus");
  const onTip = useCallback((key: string | null, by?: TipRaise) => {
    if (by !== undefined) {
      raisedBy.current = by;
    }
    setTip(key);
  }, []);
  const hide = useCallback(() => setTip(null), []);
  useEscapeDismisses(tip !== null, nav, raisedBy, hide);
  return { collapsed, tip, onTip };
}

/** A row's tip, fixed at its right edge so the scrolling list cannot clip it. */
export function NavTip({ children }: Readonly<{ children: ReactNode }>) {
  const tip = useRef<HTMLSpanElement>(null);
  useLayoutEffect(() => {
    const place = () => {
      const row = tip.current?.parentElement?.getBoundingClientRect();
      if (!tip.current || !row) return;
      tip.current.style.left = `${row.right}px`;
      tip.current.style.top = `${row.top + row.height / 2}px`;
    };
    place();
    // Capture: a scroll does not bubble from the list the row stands in.
    globalThis.addEventListener("scroll", place, true);
    return () => globalThis.removeEventListener("scroll", place, true);
  }, []);
  return (
    <span className="navtip" role="tooltip" ref={tip}>
      {children}
    </span>
  );
}
