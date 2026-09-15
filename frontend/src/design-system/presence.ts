// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useLayoutEffect, useState } from "react";

// Keeping a surface on screen long enough to leave it.
//
// React unmounts on the render that says `open === false`, which is the frame
// the exit animation would have started in — so an overlay that closes has no
// exit at all, however carefully its keyframes are written. Every answer to
// this in the wild is a timer that restates the CSS duration in JavaScript, and
// a restated duration is a second source of truth: change `--dur-move` and the
// unmount lands early (a panel cut off mid-slide) or late (a dead overlay
// eating clicks), with nothing failing either way.
//
// So this asks the ELEMENT what it is doing. `getAnimations({ subtree: true })`
// is the running animations and transitions on the overlay and everything
// inside it, and their `finished` promises are the only honest answer to "is
// the exit over". Nothing here knows a duration, and nothing needs to.

/** Whether the surface is settled or on its way out. */
export type PresenceState = "open" | "closing";

/**
 * Hold a closing surface mounted until its exit animation has actually ended.
 *
 * `element` is the node the exit plays on — the overlay, not the box inside it,
 * because the scrim and the panel animate together and the subtree walk is what
 * gathers both. The caller renders while `mounted`, and spells `state` onto
 * that node (`data-state`) so the CSS has something to select the exit with.
 *
 * `mounted` goes false in the SAME effect that found nothing running, with no
 * timer and no frame in between. That is the reduced-motion path — the rules
 * that animate sit behind `prefers-reduced-motion: no-preference`, so a reader
 * who asked for less motion creates no animations, and their dialog closes the
 * instant it is dismissed. It is also the jsdom path: a test environment has no
 * `getAnimations`, so `open={false}` renders nothing, exactly as it did before
 * this hook existed.
 */
export function usePresence({
  open,
  element,
}: Readonly<{
  open: boolean;
  element: RefObject<HTMLElement | null>;
}>): Readonly<{ mounted: boolean; state: PresenceState }> {
  const [closing, setClosing] = useState(false);
  const [wasOpen, setWasOpen] = useState(open);

  // The flip is answered DURING the render that carries it, not in an effect
  // after it, and the difference is the whole surface.
  //
  // An effect runs once its render has been COMMITTED. The render where `open`
  // first reads false would therefore have said `mounted: false`, React would
  // have taken the node out of the tree, and the effect would have put a fresh
  // one back marked closing. Measured, that bought an exit that jumped to its
  // end state, every child remounting as the surface left (reads re-firing,
  // entry animations replaying on a pane that was on its way out) and an
  // unmount that took 586ms instead of 200. Adjusting state while rendering
  // re-runs this component before anything is committed, so the node never
  // leaves the tree at all — which is the case React documents the pattern for.
  //
  // It answers the FLIP and not the state: a surface that has never been open
  // has nothing to close, and starting a closing phase for one would put an
  // empty overlay over the page for as long as the wait below takes.
  if (wasOpen !== open) {
    setWasOpen(open);
    // Re-opening mid-exit is the reader changing their mind, and it ends the
    // closing phase at once. The exit's keyframes are dropped by the state
    // change and their promises reject, which `settled` below treats as
    // finished — but that resolution arrives a microtask later, and until it
    // does the overlay would still be `inert`.
    setClosing(!open);
  }

  // A second effect, and the split is load-bearing: the exit animation does not
  // exist until the render carrying `data-state="closing"` has been committed,
  // so asking the element above — before that commit — finds nothing running
  // and unmounts the surface it was supposed to animate away.
  useLayoutEffect(() => {
    if (!closing) {
      return;
    }
    const running = (
      element.current?.getAnimations?.({ subtree: true }) ?? []
    ).filter(ends);
    if (running.length === 0) {
      setClosing(false);
      return;
    }
    let live = true;
    void Promise.all(running.map(settled)).then(() => {
      if (live) {
        setClosing(false);
      }
    });
    return () => {
      live = false;
    };
  }, [closing, element]);

  return { mounted: open || closing, state: closing ? "closing" : "open" };
}

// Whether an animation is one this can wait for at all.
//
// The subtree walk is indiscriminate on purpose — it is how the scrim and the
// panel are found without either being named here — and what it also finds is
// every LOOP inside the dialog: a skeleton's shimmer, a spinner, the pending
// pulse under a write that is still out. Those never finish, and a surface
// waiting on one stays on the screen forever, which is worse than having no
// exit at all. A loop is not an exit; it is something the dialog was doing.
function ends(animation: Animation): boolean {
  const timing = animation.effect?.getComputedTiming();
  return timing !== undefined && timing.iterations !== Number.POSITIVE_INFINITY;
}

// Settled either way. `finished` REJECTS when an animation is CANCELLED — the
// element re-opened, or a rule replaced its keyframes — and a cancelled exit is
// every bit as over as a completed one. Letting that rejection through would
// leave the surface mounted forever, which is the one outcome worse than no
// animation at all.
function settled(animation: Animation): Promise<void> {
  return animation.finished.then(
    () => undefined,
    () => undefined,
  );
}
