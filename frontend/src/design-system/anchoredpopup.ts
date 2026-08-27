// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type RefObject,
  useEffect,
  useLayoutEffect,
  useState,
} from "react";

/**
 * Anchoring, dismissal and scroll-into-view for a portalled listbox popup.
 *
 * Extracted because `Select` and `ComboBox` open the SAME popup over different
 * triggers — a button in one, a text box in the other — and every rule here is
 * about the popup rather than about what opened it: which side it flips to,
 * what a scroll inside it means, which press closes it. Two copies of that
 * would be two answers to where a dropdown may sit on a page, and they would
 * drift apart at the first viewport nobody tested.
 *
 * What is NOT here is what genuinely differs: the trigger's own keyboard
 * grammar. A button opens on Enter and a text box types into it.
 */

// The popup sits this far from the trigger, never closer than this to a viewport
// edge, and below MIN_ROOM there is not enough room to show a list worth reading
// — so it flips to the other side rather than being squeezed. MIN_ROOM is that
// choice of side and nothing else: it never becomes the popup's height.
const ANCHOR_GAP = 4;
const VIEWPORT_MARGIN = 8;
const MIN_ROOM = 96;

export type PopupFrame = Readonly<{
  left: number;
  width: number;
  maxHeight: number;
  // Exactly one of these is set. A popup below the trigger is anchored by its
  // top; a flipped one by its bottom, because anchoring it by `top` would need
  // its rendered height, which is not known until after it has painted.
  top?: number;
  bottom?: number;
  above: boolean;
}>;

function frameFor(rect: DOMRect, view: { width: number; height: number }) {
  const roomBelow = view.height - rect.bottom - ANCHOR_GAP - VIEWPORT_MARGIN;
  const roomAbove = rect.top - ANCHOR_GAP - VIEWPORT_MARGIN;
  const flip = roomBelow < MIN_ROOM && roomAbove > roomBelow;
  const rightEdge = view.width - rect.width - VIEWPORT_MARGIN;
  const frame = {
    left: Math.max(VIEWPORT_MARGIN, Math.min(rect.left, rightEdge)),
    width: rect.width,
    // The room on the side it opens on is a CEILING, never a floor. On a short
    // viewport — or at browser zoom, which shrinks the viewport in CSS pixels —
    // both sides can be under MIN_ROOM, and a popup allowed 96px in 40px of room
    // runs off the screen with its last options unreachable. It takes the room
    // there is and scrolls inside it. Zero is the floor because a negative
    // max-height is not a length.
    maxHeight: Math.max(flip ? roomAbove : roomBelow, 0),
  };
  return flip
    ? { ...frame, bottom: view.height - rect.top + ANCHOR_GAP, above: true }
    : { ...frame, top: rect.bottom + ANCHOR_GAP, above: false };
}

// The trigger has scrolled out of sight, so the popup has nothing left to point
// at. A box of zeros is NOT that case: it means nothing has been laid out (a
// jsdom render, a trigger inside a collapsed container), and there is no
// position to judge yet.
function outOfView(rect: DOMRect, viewHeight: number): boolean {
  const laidOut = rect.width > 0 || rect.height > 0;
  return laidOut && (rect.bottom <= 0 || rect.top >= viewHeight);
}

/**
 * Anchor a fixed-position popup to a trigger's box.
 *
 * The popup is portalled to the body because most of these controls sit in a
 * toolbar inside `.scroll` (app/shell.css), which is `overflow-y: auto;
 * position: relative` — it clips an absolutely positioned child and scrolls it
 * away from its own trigger. Leaving the scroller costs the popup everything
 * that used to move it, so the frame is recomputed on every scroll and resize,
 * and `onLost` closes it once the trigger itself is gone from view.
 *
 * The scroll listener is CAPTURE-phase: scroll does not bubble, so a listener on
 * the window never hears the toolbar's own scroller otherwise. That reach is also
 * why it has to know the popup: a long option list scrolls inside the popup
 * itself, which moves the trigger not at all, so those scrolls are not
 * re-anchoring events.
 */
export function useAnchoredPopup(
  anchor: RefObject<HTMLElement | null>,
  popup: RefObject<HTMLElement | null>,
  open: boolean,
  onLost: () => void,
): PopupFrame | null {
  const [frame, setFrame] = useState<PopupFrame | null>(null);

  // Layout effect, not effect: the position is computed before the browser
  // paints, so the popup never appears at the wrong place for one frame.
  useLayoutEffect(() => {
    if (!open) {
      setFrame(null);
      return;
    }
    const place = (event?: Event) => {
      // A scroll from inside the popup is the reader moving through the option
      // list, not the anchor moving under it. Recomputing the frame on every
      // wheel tick there would fight the list they are reading.
      const scrolled = event?.target;
      if (scrolled instanceof Node && popup.current?.contains(scrolled)) {
        return;
      }
      const element = anchor.current;
      if (!element) {
        return;
      }
      const rect = element.getBoundingClientRect();
      const view = {
        width: globalThis.innerWidth,
        height: globalThis.innerHeight,
      };
      if (outOfView(rect, view.height)) {
        onLost();
        return;
      }
      setFrame(frameFor(rect, view));
    };
    place();
    globalThis.addEventListener("scroll", place, true);
    globalThis.addEventListener("resize", place);
    return () => {
      globalThis.removeEventListener("scroll", place, true);
      globalThis.removeEventListener("resize", place);
    };
  }, [open, anchor, popup, onLost]);

  return frame;
}

// A pointer press outside both the trigger and the popup closes the list.
// Capture phase so a surface that stops the event on its own container cannot
// leave the popup stranded open, and `pointerdown` rather than `click` so the
// list is gone by the time the press lands on whatever the reader was reaching
// for underneath it.
export function useDismissOnOutsidePress(
  open: boolean,
  dismiss: () => void,
  trigger: RefObject<HTMLElement | null>,
  popup: RefObject<HTMLElement | null>,
) {
  useEffect(() => {
    if (!open) {
      return;
    }
    const onPointerDown = (event: Event) => {
      const target = event.target;
      if (!(target instanceof Node)) {
        return;
      }
      if (
        trigger.current?.contains(target) ||
        popup.current?.contains(target)
      ) {
        return;
      }
      dismiss();
    };
    globalThis.addEventListener("pointerdown", onPointerDown, true);
    return () =>
      globalThis.removeEventListener("pointerdown", onPointerDown, true);
  }, [open, dismiss, trigger, popup]);
}

// Keeps the active option visible while a reader arrows through a list longer
// than the popup. The call is optional because jsdom implements no
// scrollIntoView, and the keyboard path has to stay testable.
export function useActiveOptionVisible(
  open: boolean,
  active: number,
  listboxId: string,
) {
  useEffect(() => {
    if (!open || active === -1) {
      return;
    }
    const element = document.getElementById(`${listboxId}-option-${active}`);
    element?.scrollIntoView?.({ block: "nearest" });
  }, [open, active, listboxId]);
}
