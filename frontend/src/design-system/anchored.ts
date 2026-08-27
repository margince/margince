// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Putting a portalled panel where its trigger is.
//
// Its own module because two controls need it and neither owns it: the
// overflow menu (atoms.tsx) and the popover (popover.tsx) both hang a fixed
// panel off a button, and a hook living inside one of them would make the
// other import a menu to borrow arithmetic.

import { type RefObject, useEffect, useState } from "react";

// Where the portalled panel sits: beside the trigger, edge to edge on the side
// the caller names, and INSIDE the viewport on both axes.
//
// The panel is fixed, so the viewport is all the room there is — a panel placed
// below a trigger near the bottom edge puts its actions where no amount of page
// scrolling reaches them. So it opens upward when the space below cannot hold
// it, takes whichever side has more room when neither can, and is capped to
// that space so a panel with many items scrolls inside itself.
//
// Measured on OPEN and again whenever anything moves it. Scroll is listened to
// in the CAPTURE phase because a scroll event does not bubble — the trigger may
// sit inside a scrolling region, and a panel that stayed at the coordinates it
// was opened at would drift away from the button it belongs to.
export function useAnchoredToTrigger(
  open: boolean,
  trigger: RefObject<HTMLElement | null>,
  panel: RefObject<HTMLElement | null>,
  // Which of the trigger's edges the panel lines up with. A menu hangs off the
  // END of its button, which is a small mark at a known place. A panel opened
  // by a SENTENCE cannot: the sentence runs the width of the row, so its end
  // is somewhere off at the margin and the panel arrives detached from the
  // words that opened it. Those triggers ask for "start" and the panel begins
  // where the reading does.
  align: "start" | "end" = "end",
): { top: number; left: number; maxHeight: number } {
  const [at, setAt] = useState({ top: 0, left: 0, maxHeight: 0 });
  useEffect(() => {
    if (!open) {
      return;
    }
    const place = () => {
      const anchor = trigger.current?.getBoundingClientRect();
      if (!anchor) {
        return;
      }
      const width = panel.current?.offsetWidth ?? 0;
      const room = globalThis.innerWidth - width - MENU_EDGE_GAP;
      const wanted = align === "start" ? anchor.left : anchor.right - width;
      setAt({
        ...verticalPlacement(anchor, panel.current?.offsetHeight ?? 0),
        left: Math.max(MENU_EDGE_GAP, Math.min(wanted, room)),
      });
    };
    place();
    globalThis.addEventListener("resize", place);
    globalThis.addEventListener("scroll", place, true);
    return () => {
      globalThis.removeEventListener("resize", place);
      globalThis.removeEventListener("scroll", place, true);
    };
  }, [open, trigger, panel, align]);
  return at;
}

// Below the trigger while the panel fits there, above it when it does not, and
// on the roomier side when neither fits — capped to that side either way.
//
// Exported for its own test: jsdom gives every element a zero-sized rectangle,
// so the only way to state this rule as a test is to state it over the
// measurements themselves.
export function verticalPlacement(
  anchor: DOMRect,
  height: number,
): { top: number; maxHeight: number } {
  const below = globalThis.innerHeight - anchor.bottom - MENU_EDGE_GAP * 2;
  const above = anchor.top - MENU_EDGE_GAP * 2;
  const opensDown = height <= below || below >= above;
  if (opensDown) {
    return { top: anchor.bottom + MENU_EDGE_GAP, maxHeight: below };
  }
  return {
    top: Math.max(MENU_EDGE_GAP, anchor.top - MENU_EDGE_GAP - height),
    maxHeight: above,
  };
}

// The breathing room between the panel and both the trigger above it and the
// viewport edge beside it, in px because it is arithmetic rather than a
// stylesheet value: --space-1.
const MENU_EDGE_GAP = 4;
