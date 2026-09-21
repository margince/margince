// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Putting a portalled panel where its trigger is.
//
// Its own module because three controls need it and none owns it: the overflow
// menu (atoms.tsx), the popover (popover.tsx) and the evidence mark's receipt
// (evidencemark.tsx) all hang a fixed panel off a button, and a hook living
// inside one of them would make the others import a menu to borrow arithmetic.

import {
  type CSSProperties,
  type RefObject,
  useLayoutEffect,
  useState,
} from "react";

// Which edge of the viewport the panel is anchored from, and how tall it may be.
export type VerticalPlacement = Readonly<{
  maxHeight: number;
  // Exactly one of these is set. A panel with room below its trigger is
  // anchored by its TOP; a flipped one by its BOTTOM, because anchoring a
  // flipped panel by its top would need the panel's rendered height — which
  // `verticalPlacement` deliberately never reads, for the reason stated there.
  top?: number;
  bottom?: number;
}>;

export type AnchoredPanel = VerticalPlacement & Readonly<{ left: number }>;

/**
 * The inline box for a panel anchored to its trigger.
 *
 * One writer, because the three panels sharing the placement each used to
 * spell it out: a placement that anchors by `bottom` is only as good as the
 * callers that read it, and a caller still writing `top` from a placement that
 * no longer sets one pins the panel to the top of the page.
 */
export function anchoredPanelBox(at: AnchoredPanel): CSSProperties {
  return {
    top: at.top,
    bottom: at.bottom,
    left: at.left,
    maxHeight: at.maxHeight,
  };
}

// Where the portalled panel sits: beside the trigger, edge to edge on the side
// the caller names, and INSIDE the viewport on both axes.
//
// The panel is fixed, so the viewport is all the room there is — a panel placed
// below a trigger near the bottom edge puts its actions where no amount of page
// scrolling reaches them. So it opens upward when the room below is too little
// to open into, takes whichever side has more when neither is enough, and is
// capped to that room so a panel with more to show scrolls inside itself.
//
// Measured on OPEN and again whenever anything moves it. Scroll is listened to
// in the CAPTURE phase because a scroll event does not bubble — the trigger may
// sit inside a scrolling region, and a panel that stayed at the coordinates it
// was opened at would drift away from the button it belongs to.
//
// A layout effect rather than an effect, as the listbox and tooltip positioners
// are: the placement is computed before the browser paints, so the panel never
// appears at the wrong place for one frame.
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
): AnchoredPanel {
  const [at, setAt] = useState<AnchoredPanel>({
    top: 0,
    left: 0,
    maxHeight: 0,
  });
  useLayoutEffect(() => {
    if (!open) {
      return;
    }
    const place = () => {
      const anchor = trigger.current?.getBoundingClientRect();
      if (!anchor) {
        return;
      }
      // The panel's WIDTH is its own — a stylesheet decides it (`max-content`
      // under a ceiling), so reading it back is reading the panel's own answer.
      // Its height is not, which is the whole of `verticalPlacement`'s note.
      const width = panel.current?.offsetWidth ?? 0;
      const room = globalThis.innerWidth - width - MENU_EDGE_GAP;
      const wanted = align === "start" ? anchor.left : anchor.right - width;
      setAt({
        ...verticalPlacement(anchor),
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

// Below the trigger while there is room there worth opening into, above it when
// there is not, and on the roomier side when neither has enough — capped to
// that room either way.
//
// It never reads the panel's own height, and that is the rule rather than an
// omission. The panel is capped BY this placement, so `offsetHeight` reports
// the cap and not the content: the first placement of every panel measured its
// own `max-height: 0`, decided from a 26px box of padding what belonged in a
// 143px one, and anchored a flipped panel 26px above the trigger — from where
// it drew its real height straight past the bottom edge, fixed, with nothing
// able to scroll it back. `scrollHeight` WOULD see through the cap, and is
// deliberately not read either: a side chosen from the panel's content makes
// the placement depend on measuring before placing, and what decides whether a
// side is worth opening into is how much room it has. Same rule and same
// threshold as the listbox popup's own positioner (anchoredpopup.ts).
//
// Exported for its own test (anchored.test.ts): the test environment gives
// every element a zero-sized rectangle, so the only way to state this rule as a
// test is to state it over the measurements themselves.
export function verticalPlacement(anchor: DOMRect): VerticalPlacement {
  const view = globalThis.innerHeight;
  // The panel's own edge on each side, held inside the viewport. A trigger can
  // be scrolled clean past either end while its panel stands open — which is
  // what the capture-phase listener above exists for — and an edge taken from
  // it unclamped puts the panel where the viewport is not: a trigger 20px below
  // the bottom edge anchored a panel's lower edge at 816 in an 800px viewport
  // and allowed it 812px, so it drew its first 16px above the top of the screen
  // with nothing able to scroll it back.
  const under = insideView(anchor.bottom + MENU_EDGE_GAP, view);
  const over = insideView(anchor.top - MENU_EDGE_GAP, view);
  // Room is measured from the CLAMPED edge to the far end of the viewport, so a
  // cap can neither exceed the screen nor come out negative — a negative
  // max-height is not a length, and the browser drops the declaration, which
  // opens the panel uncapped and is the one outcome this placement prevents.
  const below = view - MENU_EDGE_GAP - under;
  const above = over - MENU_EDGE_GAP;
  if (below < MIN_PANEL_ROOM && above > below) {
    // Anchored by its lower edge, so how tall the panel turns out to be stays
    // its own business.
    return { bottom: view - over, maxHeight: above };
  }
  return { top: under, maxHeight: below };
}

// One edge of the panel, kept a gap inside both ends of the viewport.
function insideView(edge: number, view: number): number {
  return Math.min(Math.max(edge, MENU_EDGE_GAP), view - MENU_EDGE_GAP);
}

// The breathing room between the panel and both the trigger above it and the
// viewport edge beside it, in px because it is arithmetic rather than a
// stylesheet value: --space-1.
const MENU_EDGE_GAP = 4;

// Less room than this is not worth opening a panel into, so it flips rather
// than being squeezed: every panel here carries at least a line of prose and
// usually a control under it, and neither is readable in less. It is the choice
// of SIDE and nothing else — it never becomes the panel's height.
const MIN_PANEL_ROOM = 96;
