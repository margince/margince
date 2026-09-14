// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent,
  useState,
} from "react";
import type { DecisionApproval } from "./decisioncard";
import type {
  DecisionDeckChips,
  DecisionDeckItem,
  DecisionDeckLabels,
  DecisionSharedFacts,
  DeckVerdict,
} from "./decisiondeck";
import { DeckItemCard } from "./decisiondeck.item";
import { dragVerdict } from "./decisiondeck.verdicts";

// THE STACK, and the finger that moves it.
//
// One card at a time with the edges of the two behind it, leaning as it is
// dragged and leaving from the point the hand let go. It is the deck's MOTION,
// and it lives apart from the deck for the reason the motion exists: a reader
// answering a pile one card at a time has to see the pile, and everything in
// this file is about making that legible — how deep the peek goes, how far a
// drag must travel, which way a card flies out.
//
// The deck itself decides nothing here. It hands over the live item and takes
// back a verdict, so a surface that draws no stack at all (the list) loses none
// of the queue's behaviour with it.
/** How much the live card leans as it is dragged: one degree per this many px. */
const DRAG_ROTATION_DIVISOR = 22;

/** How many card edges peek out behind the live one. */
const PEEK_DEPTH = 2;

// A drag in progress. The ORIGIN is kept and the offset derived from it, rather
// than accumulating `movementX`: a pointer that leaves the element, or a host
// that reports no movement deltas at all, still yields the honest distance from
// where the finger went down.
export type Drag = Readonly<{
  pointerId: number;
  startX: number;
  startY: number;
  dx: number;
  dy: number;
}>;

/**
 * The card on its way out: which verdict sent it, and WHERE THE HAND LET GO.
 *
 * The offset is the whole point of this being an object rather than a verdict.
 * A card that leaves from the middle of the plate after a swipe that ended 90px
 * to the right reads as a second, different card — the eye tracks the one it was
 * dragging, and losing it at the release point is what made the exit look like a
 * snap-back. Staged from the keyboard there is no gesture to continue, and the
 * offsets are zero.
 */
export type Leaving = Readonly<{
  verdict: DeckVerdict;
  dx: number;
  dy: number;
}>;

/** The release point, handed to the exit animation as custom properties. */
type GhostVars = CSSProperties & Record<`--${string}`, string>;

/**
 * The release point as the style the exit opens on. Declared as `GhostVars`
 * rather than asserted at the call site: the type is what makes the custom
 * properties checkable, and an assertion would say the same thing while
 * switching the checking off.
 */
function ghostVars(leaving: Leaving): GhostVars {
  return {
    "--ddeck-from-x": `${leaving.dx}px`,
    "--ddeck-from-y": `${leaving.dy}px`,
    "--ddeck-from-rot": `${leaving.dx / DRAG_ROTATION_DIVISOR}deg`,
  };
}

/**
 * The DRAG, and the four handlers that make one.
 *
 * A hook rather than four closures in the deck, because a gesture is a state
 * machine with its own vocabulary — where the finger went down, how far it has
 * travelled, whether this pointer is the one being tracked — and none of that
 * is what the deck is about. The deck asks it for two things: the offset to
 * lean the live card by, and a verdict when the hand lets go.
 *
 * It stages through the CALLER rather than holding a verdict of its own: a
 * gesture that decided anything would be a second answer to what a swipe means,
 * and `dragVerdict` is already the first.
 */
export function useDeckDrag({
  live,
  onStage,
}: Readonly<{
  live: DecisionDeckItem | undefined;
  onStage: (
    item: DecisionDeckItem,
    verdict: DeckVerdict,
    from: { dx: number; dy: number },
  ) => void;
}>) {
  const [drag, setDrag] = useState<Drag | null>(null);

  const onPointerDown = (event: PointerEvent<HTMLFieldSetElement>) => {
    if (!live || event.button !== 0) {
      return;
    }
    // A press that STARTS on a control belongs to that control, and this guard
    // is what makes the four buttons on a deck card work at all. Pointer capture
    // retargets every later event for that pointer — including the compatibility
    // `click` — at the capturing element, so capturing here sent the click to
    // this fieldset and the button under the finger never heard about it. Accept
    // did nothing in the deck and worked in the list, which is exactly the shape
    // of the bug that was reported. `closest` rather than a tag test: the press
    // lands on the label or the icon inside the button.
    if (
      event.target instanceof Element &&
      event.target.closest("button, a, input, textarea, select, summary")
    ) {
      return;
    }
    event.currentTarget.setPointerCapture?.(event.pointerId);
    setDrag({
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      dx: 0,
      dy: 0,
    });
  };

  const onPointerMove = (event: PointerEvent<HTMLFieldSetElement>) => {
    const { clientX, clientY, pointerId } = event;
    setDrag((held) =>
      held && held.pointerId === pointerId
        ? { ...held, dx: clientX - held.startX, dy: clientY - held.startY }
        : held,
    );
  };

  const onPointerUp = (event: PointerEvent<HTMLFieldSetElement>) => {
    if (!drag || drag.pointerId !== event.pointerId) {
      return;
    }
    setDrag(null);
    // Measured from THIS event, not from the last move. A pointer that travels
    // and lifts without another `pointermove` in between — a fast flick, a
    // synthetic release, a pen leaving the surface — would otherwise be judged
    // on where it last was seen rather than on where it let go, which decides
    // both the verdict and where the exit starts.
    const dx = event.clientX - drag.startX;
    const dy = event.clientY - drag.startY;
    const verdict = dragVerdict(dx, dy);
    if (verdict && live) {
      // Where the hand let go, so the card continues out from there rather than
      // restarting its flight from the middle of the plate.
      onStage(live, verdict, { dx, dy });
    }
  };

  /**
   * The gesture taken away rather than finished — the browser cancelling the
   * pointer (a system gesture, a scroll takeover, the pen leaving range).
   *
   * Nothing is staged. A cancelled drag is one the contact never completed, and
   * treating it as a release meant the system could decide a proposal on the
   * reader's behalf: a swipe interrupted past the threshold sent the card. The
   * card springs back instead, which is what a cancellation looks like.
   */
  const onPointerCancel = (event: PointerEvent<HTMLFieldSetElement>) => {
    if (drag && drag.pointerId === event.pointerId) {
      setDrag(null);
    }
  };

  return {
    drag,
    handlers: { onPointerDown, onPointerMove, onPointerUp, onPointerCancel },
  };
}

// The stack: the live card, the edges of the two behind it, and how many more
// there are. The peeked plates are blank and `aria-hidden` — they are depth, not
// content, and a screen reader announcing two empty cards would be describing
// the drawing rather than the queue.
export function DeckStack({
  live,
  waiting,
  drag,
  leaving,
  now,
  labels,
  chips,
  onStage,
  onKeyDown,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onPointerCancel,
  onLeaveEnd,
}: Readonly<{
  live: DecisionDeckItem | undefined;
  waiting: readonly DecisionDeckItem[];
  drag: Drag | null;
  leaving: Leaving | null;
  now: number;
  labels: DecisionDeckLabels;
  chips?: (
    approval: DecisionApproval,
    shared: DecisionSharedFacts,
  ) => DecisionDeckChips;
  onStage: (
    item: DecisionDeckItem,
    verdict: DeckVerdict,
    from?: { dx: number; dy: number },
  ) => void;
  onKeyDown: (event: KeyboardEvent<HTMLFieldSetElement>) => void;
  onPointerDown: (event: PointerEvent<HTMLFieldSetElement>) => void;
  onPointerMove: (event: PointerEvent<HTMLFieldSetElement>) => void;
  onPointerUp: (event: PointerEvent<HTMLFieldSetElement>) => void;
  onPointerCancel: (event: PointerEvent<HTMLFieldSetElement>) => void;
  onLeaveEnd: () => void;
}>) {
  if (!live) {
    return null;
  }
  const behind = waiting.length - 1;
  const peeks = Math.min(behind, PEEK_DEPTH);
  return (
    <div className="ddeck-stack">
      {/*
        The plate: the live card and the edges behind it, and NOTHING else. The
        peeked layers are absolutely positioned against this box rather than
        against the whole stack, which is what keeps them off the two lines
        underneath — sized to the stack they covered the count and the keyboard
        legend, and the legend is the only thing that tells a reader the arrow
        keys exist at all.
      */}
      <div className="ddeck-plate">
        {Array.from({ length: peeks }, (_, index) => (
          <div
            className="ddeck-peek"
            key={waiting[index + 1].id}
            data-depth={index + 1}
            aria-hidden="true"
          />
        ))}
        {leaving && (
          <div
            className="ddeck-ghost"
            data-verdict={leaving.verdict}
            aria-hidden="true"
            // Where the flight STARTS: the point the hand released, as the
            // transform the keyframes open on. Custom properties rather than a
            // second keyframe set per verdict — the offset is data, and eight
            // hand-written keyframes to say four directions from an arbitrary
            // point is not.
            style={ghostVars(leaving)}
            onAnimationEnd={onLeaveEnd}
          />
        )}
        {/*
        The keyboard surface, and a real `fieldset` rather than a div wearing
        `role="group"`: what is inside it IS a set of controls that belong
        together — the four verbs — which is the same reasoning `ChoiceList` and
        the list surface's `Menu` are built on, and the browser then exposes the
        grouping without an ARIA role on top of it.

        It takes a tab stop of its own, on top of the four buttons inside it, and
        that is the point rather than an oversight: the arrow keys ARE the deck,
        so a reader who cannot use a pointer needs somewhere to stand in order to
        press them. `aria-keyshortcuts` is how they are told, and the legend
        under the card is how everyone else is.
      */}
        <fieldset
          className="ddeck-live"
          // biome-ignore lint/a11y/noNoninteractiveTabindex: the tab stop is what makes the four arrow-key verdicts reachable at all — a swipe surface with no keyboard equivalent is a surface only a pointer can answer
          tabIndex={0}
          aria-label={labels.deckLabel}
          aria-keyshortcuts="ArrowRight ArrowLeft ArrowUp ArrowDown U Enter"
          onKeyDown={onKeyDown}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerCancel}
        >
          {/*
            The moving part, and KEYED ON THE CARD.

            Everything that moves lives in here rather than on the fieldset, and
            the split is what fixes two things at once. Keyed, React mounts a
            fresh box for the next card — so the transform the swipe left behind
            cannot transition back to zero on the card that has just arrived,
            which is exactly the "the same card snapped back into the pile" this
            deck was reported for, and the arrival animation plays because a
            remount is what plays it. And because the KEYBOARD surface is the
            fieldset outside it, none of that touches focus: a reader working the
            queue with the arrow keys keeps their tab stop through every verdict.
          */}
          <div
            key={live.id}
            className="ddeck-card"
            data-dragging={drag ? "" : undefined}
            // The verdict this drag WOULD stage, once it has travelled far
            // enough to be one. Drawn as a ring in the verdict's own colour, so
            // a reader learns what right and left mean while their finger is
            // still down rather than after the card has gone. It says nothing an
            // assistive reader is missing: the four buttons inside carry the same
            // four verdicts in words at every moment.
            data-verdict={
              (drag ? dragVerdict(drag.dx, drag.dy) : null) ?? undefined
            }
            style={
              drag
                ? {
                    transform: `translate(${drag.dx}px, ${drag.dy}px) rotate(${
                      drag.dx / DRAG_ROTATION_DIVISOR
                    }deg)`,
                  }
                : undefined
            }
          >
            <DeckItemCard
              item={live}
              layout="deck"
              now={now}
              labels={labels}
              chips={chips}
              onStage={onStage}
            />
          </div>
        </fieldset>
      </div>
      {/* Omitted at zero. On the last card there is nothing behind it to
          count, and "0 more behind" is a line of furniture over the one card
          a reader is being asked to answer — the plate itself already says
          it is the only one, by having no edges peeking out from under it. */}
      {behind > 0 && (
        <p className="t-caption ddeck-behind">{labels.behind(behind)}</p>
      )}
      <p className="t-caption ddeck-keys">{labels.keys}</p>
    </div>
  );
}
