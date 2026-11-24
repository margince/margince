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

// The registered property the shell animates the rail's width through.
const RAIL_MOVE = "--railW";

function railMoving(app: Element): boolean {
  return (app.getAnimations?.() ?? []).some(
    (move) =>
      "transitionProperty" in move && move.transitionProperty === RAIL_MOVE,
  );
}

/**
 * A row's tip, fixed at its right edge so the scrolling list cannot clip it.
 * While the rail's edge moves it waits, and stands at the edge it settles on.
 */
export function NavTip({ children }: Readonly<{ children: ReactNode }>) {
  const tip = useRef<HTMLSpanElement>(null);
  useLayoutEffect(() => {
    const box = tip.current;
    const app = box?.closest<HTMLElement>(".app");
    if (!box) return;
    const place = () => {
      const row = box.parentElement?.getBoundingClientRect();
      if (!row) return;
      box.style.left = `${row.right}px`;
      box.style.top = `${row.top + row.height / 2}px`;
    };
    const hide = (event: TransitionEvent) => {
      if (event.propertyName === RAIL_MOVE) box.style.visibility = "hidden";
    };
    const show = (event: TransitionEvent) => {
      if (event.propertyName !== RAIL_MOVE || (app && railMoving(app))) return;
      place();
      box.style.visibility = "";
    };
    if (app && railMoving(app)) {
      box.style.visibility = "hidden";
    } else {
      place();
    }
    // Capture: a scroll does not bubble from the list the row stands in.
    globalThis.addEventListener("scroll", place, true);
    app?.addEventListener("transitionrun", hide);
    app?.addEventListener("transitionend", show);
    app?.addEventListener("transitioncancel", show);
    return () => {
      globalThis.removeEventListener("scroll", place, true);
      app?.removeEventListener("transitionrun", hide);
      app?.removeEventListener("transitionend", show);
      app?.removeEventListener("transitioncancel", show);
    };
  }, []);
  return (
    <span className="navtip" role="tooltip" ref={tip}>
      {children}
    </span>
  );
}
