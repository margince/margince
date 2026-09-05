// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A list row answered with the thumb.
//
// A phone row has room for the work and not for the verbs beside it. The
// worklist's four set-aside verbs measure 326px against 308px of usable width
// at 390px, so they wrap to a second 44px band and the row outgrows the ceiling
// a queue has to keep to be worked with a thumb. Shrinking them is not open:
// 44px is the target a mis-hit costs somebody else's judgement on a customer's
// message.
//
// So the verbs LEAVE the row and the row itself answers them. A drag past the
// threshold stages one action and shows what it will do; the press that runs it
// is the reader's own, which is what keeps a flick of the wrist from filing a
// judgement nobody made.
//
// NOT A DECK. design-system/decisiondeck.tsx swipes too, and it is a different
// surface with a different contract: a full plate showing ONE card at a time,
// four verdicts on two axes, staged into a tray and committed together. This is
// a row in a list that stays a list, with one action per side and no tray. The
// two share the rule below and nothing else.

import { useRef, useState } from "react";

import { Button } from "./atoms";

import "./swiperow.css";

/**
 * How far a drag must travel before it means anything, in px.
 *
 * Short enough that a deliberate flick reaches it, long enough that a thumb
 * moving down a scrolling list does not. The deck asks 72px of a plate the
 * finger starts in the middle of; a row is answered from wherever it was
 * touched, so it asks less.
 */
const SWIPE_THRESHOLD_PX = 56;

/** Which side a finished drag chose, or null for one that springs back. */
export type SwipeSide = "start" | "end";

/**
 * The side a drag means, or null.
 *
 * HORIZONTAL ONLY, and that is the difference from a deck's rule rather than an
 * omission: a row lives in a scrolling list, so a mostly-vertical drag is the
 * reader scrolling past it and must never be read as an answer. The deck owns
 * the whole plate and can claim both axes.
 */
export function swipeSide(dx: number, dy: number): SwipeSide | null {
  if (Math.abs(dx) <= Math.abs(dy) || Math.abs(dx) < SWIPE_THRESHOLD_PX) {
    return null;
  }
  return dx > 0 ? "end" : "start";
}

export type SwipeRowAction = Readonly<{
  /** What the reader is about to do, in their own language. */
  label: string;
  /** Run it. Called by the confirm press, never by the drag itself. */
  onAct: () => void;
}>;

export type SwipeRowProps = Readonly<{
  /** Dragging right. Omitted when the row offers nothing that way. */
  end?: SwipeRowAction;
  /** Dragging left. Omitted when the row offers nothing that way. */
  start?: SwipeRowAction;
  /** The word that takes a staged action back. */
  cancelLabel: string;
  /**
   * A drag staged something on this side. Called on the GESTURE, not on the
   * confirm, so a caller offering more actions than a direction has can walk
   * them: a counter advanced on the confirm would never move for a reader who
   * dismisses, and the second action on a side would be unreachable. The side
   * is passed because a walk is a position in ONE direction's list.
   */
  onStage?: (side: SwipeSide) => void;
  children: React.ReactNode;
}>;

/**
 * A row that can be swiped to stage one of two actions.
 *
 * STAGE, THEN CONFIRM. The drag reveals what it would do and the reader presses
 * to run it, for the reason the deck stages rather than sends: every action
 * offered here REMOVES the row from the reader's view, and a surface where a
 * flick files a judgement is one where a thumb sliding down a list files three.
 * The staged action is dismissable, so a drag that was meant as a scroll costs
 * a tap and nothing else.
 *
 * The gesture is an ADDITION, never the only way. Callers keep drawing their
 * buttons wherever there is room for them; this replaces them only where there
 * is not, which is why the staged bar carries the same words the buttons do.
 * A capability reachable only by dragging is one a keyboard cannot reach.
 */
export function SwipeRow({
  end,
  start,
  cancelLabel,
  onStage,
  children,
}: SwipeRowProps) {
  const [staged, setStaged] = useState<SwipeRowAction | null>(null);
  const from = useRef<{ x: number; y: number } | null>(null);

  const begin = (event: React.PointerEvent) => {
    // A primary pointer only. A second finger arriving mid-scroll would
    // otherwise restart the measurement from its own landing point and turn a
    // two-finger scroll into a drag that travelled the width of the hand.
    //
    // AND NOT A PRESS THAT LANDED ON A CONTROL. The caller mounts this around
    // a whole row, so its buttons and links are inside the surface and their
    // pointer events bubble here: a thumb that slips 56px while reaching for a
    // menu or a verb would otherwise stage a judgement nobody asked for, and
    // the staged bar would then stand over the control that was actually being
    // pressed. A row's own controls answer their own presses.
    const on = event.target instanceof Element ? event.target : null;
    if (!event.isPrimary || on?.closest("button, a, [role='button']")) {
      return;
    }
    from.current = { x: event.clientX, y: event.clientY };
  };

  const settle = (event: React.PointerEvent) => {
    const origin = from.current;
    from.current = null;
    if (origin === null) {
      return;
    }
    const side = swipeSide(event.clientX - origin.x, event.clientY - origin.y);
    if (side === null) {
      return;
    }
    const chosen = side === "end" ? end : start;
    if (chosen !== undefined) {
      setStaged(chosen);
      onStage?.(side);
    }
  };

  return (
    <div
      className="swipe-row"
      onPointerDown={begin}
      onPointerUp={settle}
      // A pointer that leaves the row mid-drag never fires pointerup on it, and
      // the origin would then still be sitting there when the next touch began
      // — measuring one drag from the start of an earlier one.
      onPointerCancel={() => {
        from.current = null;
      }}
      // NO NAME AND NO ROLE. A swipe is not a control an assistive technology
      // can operate, so naming this box would announce a gesture a keyboard or
      // a screen reader cannot perform. What IS announced is the staged bar
      // below, whose buttons are real and reachable by tab — which is why the
      // caller must keep another path to these actions rather than treating the
      // gesture as the only one.
      data-testid="swipe-row"
    >
      {children}
      {staged !== null && (
        // `role="status"` so the bar is ANNOUNCED when a drag stages it: the
        // gesture that opened it is silent, and a reader who cannot see the row
        // move would otherwise be given a button with no idea where it came
        // from.
        <div className="swipe-row-staged" role="status">
          <Button
            small
            onClick={() => {
              staged.onAct();
              setStaged(null);
            }}
          >
            {staged.label}
          </Button>
          <Button small variant="ghost" onClick={() => setStaged(null)}>
            {cancelLabel}
          </Button>
        </div>
      )}
    </div>
  );
}
