// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Hover INTENT: the difference between a pointer that has arrived somewhere
// and a pointer that is on its way past.
//
// Opening on contact makes every popover between a reader and where they were
// going fire and shut, which reads as the page flinching. A flat delay does
// not fix it: it makes a reader who has already stopped moving sit and wait,
// while a reader crossing the page slowly still triggers everything on the
// way. Both mistakes come from asking WHEN the pointer arrived instead of
// whether it has stopped.
//
// So this measures the pointer. A hand that has settled is sending nothing at
// all, and a hand crossing the screen is sending a lot — the speed separates
// the two cleanly, and the clock only bounds the answer at either end.

import { useCallback, useEffect, useRef } from "react";

// Below this the pointer counts as stopped. In px/ms, so it holds the same
// meaning on a trackpad flick and a slow mouse drag.
const SETTLE = 0.08;

// A resting hand sends no move events at all, so silence has to BE the answer
// rather than leaving the last speed standing.
const STILL_AFTER_MS = 70;

// The smoothing is weighted to history: one stuttered sample from a hand about
// to stop should not read as travel, and one slow sample mid-flight should not
// read as arrival.
const KEEP_OLD = 0.55;

// Never fire before the floor — a pointer cannot have shown intent in less
// time than this. Fire anyway at the ceiling, so a reader whose hand fidgets
// is not locked out of a popover they are plainly looking at.
const FLOOR_MS = 70;
const CEILING_MS = 260;

// Moving between two proofs uses the shorter pair. The reader has already
// shown intent once, and asking for it a second time reads as lag.
const SWITCH_FLOOR_MS = 30;
const SWITCH_CEILING_MS = 110;

// The popovers sit under their trigger with a gap between the words and the
// panel. A pointer crossing that gap must not shut the thing it is travelling
// toward.
const CLOSE_GRACE_MS = 180;

// Polled rather than driven by pointermove, because the event that proves the
// pointer has stopped is the one that never arrives.
const POLL_MS = 25;

// ONE document listener for every trigger on the page, ref-counted. A listener
// per popover would multiply the same work by the number of things a reader is
// not currently looking at.
let watchers = 0;
let speed = 0;
// Whether `speed` holds a reading at all. A separate flag rather than a zero
// or a sentinel timestamp, because both are legitimate values: a resting hand
// really does read zero, and the page's first move can land at time zero.
let sampled = false;
let lastMoveAt = 0;
let lastX = 0;
let lastY = 0;
// How recently ANY of these closed: what makes the next one a switch. Starts
// at negative infinity rather than zero, because zero is a real moment on this
// clock — the page's first millisecond — and a plain zero would tell the very
// first hover of a session that something had just closed under it.
let lastClosedAt = Number.NEGATIVE_INFINITY;
let openCount = 0;
// Which instance's `timers` object last opened. A close is an answer to "is
// MY panel still the one showing", not "did my own delay run out" — a
// trigger can settle and hand the shared display to a neighbour while this
// one's grace period is still ticking, and the stale timer firing afterward
// must not reach back and blank what the neighbour just opened.
let activeOwner: object | undefined;

function trackPointer(event: PointerEvent): void {
  const now = event.timeStamp;
  const elapsed = now - lastMoveAt;
  if (elapsed > 0) {
    const sample =
      Math.hypot(event.clientX - lastX, event.clientY - lastY) / elapsed;
    // The FIRST sample is taken whole. Smoothing it up from zero would report
    // a pointer crossing the page at well under half its real speed for the
    // first few readings, which is exactly the window the floor covers — so a
    // fast pass would read as an arrival and open the thing it passed.
    speed = sampled ? KEEP_OLD * speed + (1 - KEEP_OLD) * sample : sample;
    sampled = true;
  }
  lastMoveAt = now;
  lastX = event.clientX;
  lastY = event.clientY;
}

function pointerSpeed(): number {
  // Silence means the hand is resting, not that it is still going at whatever
  // it was doing when it last reported.
  if (!sampled || performance.now() - lastMoveAt > STILL_AFTER_MS) {
    return 0;
  }
  return speed;
}

function watchPointer(): () => void {
  if (watchers === 0) {
    lastMoveAt = performance.now();
    // Capture + passive: this only reads the event, and reading it must not
    // depend on some handler below deciding to let it through.
    document.addEventListener("pointermove", trackPointer, {
      capture: true,
      passive: true,
    });
  }
  watchers += 1;
  return () => {
    watchers -= 1;
    if (watchers === 0) {
      document.removeEventListener("pointermove", trackPointer, true);
      // Nothing on the page can be hovered any more, so there is nothing left
      // to be switching BETWEEN — and a stale "one just closed" would give the
      // next trigger to mount the short pair on a pointer that has shown
      // nothing.
      lastMoveAt = 0;
      speed = 0;
      sampled = false;
      lastClosedAt = Number.NEGATIVE_INFINITY;
      openCount = 0;
      activeOwner = undefined;
    }
  };
}

/** What a trigger spreads onto its own element. */
export type HoverIntent = Readonly<{
  onPointerEnter: () => void;
  onPointerLeave: () => void;
}>;

/**
 * Open `onOpen` when the pointer has settled on the trigger; close it after a
 * grace period once the pointer leaves.
 *
 * Keyboard focus is NOT routed through this and should call `onOpen` directly:
 * a reader who has tabbed to something has stated their intent outright, and
 * there is no pointer to measure.
 *
 * The pending open lives on the instance rather than in state. It is not
 * something the page draws, and holding it in state would redraw the whole
 * record for every pointer that merely passes over a trigger.
 */
export function useHoverIntent(
  onOpen: () => void,
  onClose: () => void,
): HoverIntent {
  const acts = useRef({ onOpen, onClose });
  acts.current = { onOpen, onClose };
  // Timer handles as whatever the host hands back. A browser returns a number
  // and Node's typings return a Timeout, and this file is typechecked under
  // both — a hard `number` here compiles in one lane and fails in the other.
  const timers = useRef<{
    poll?: ReturnType<typeof setInterval>;
    close?: ReturnType<typeof setTimeout>;
    open: boolean;
  }>({
    open: false,
  });

  const stopPoll = useCallback(() => {
    if (timers.current.poll !== undefined) {
      globalThis.clearInterval(timers.current.poll);
      timers.current.poll = undefined;
    }
  }, []);

  const settle = useCallback(() => {
    stopPoll();
    if (!timers.current.open) {
      timers.current.open = true;
      openCount += 1;
      activeOwner = timers.current;
      acts.current.onOpen();
    }
  }, [stopPoll]);

  const shut = useCallback(() => {
    if (timers.current.open) {
      timers.current.open = false;
      openCount = Math.max(0, openCount - 1);
      lastClosedAt = performance.now();
      // Read at fire time, not schedule time: a neighbour can have become the
      // open one while this close was still waiting out its grace period, and
      // this instance no longer speaks for what the reader is looking at.
      if (activeOwner === timers.current) {
        activeOwner = undefined;
        acts.current.onClose();
      }
    }
  }, []);

  const onPointerEnter = useCallback(() => {
    if (timers.current.close !== undefined) {
      globalThis.clearTimeout(timers.current.close);
      timers.current.close = undefined;
    }
    if (timers.current.open || timers.current.poll !== undefined) {
      return;
    }
    const switching =
      openCount > 0 || performance.now() - lastClosedAt < CLOSE_GRACE_MS;
    const floor = switching ? SWITCH_FLOOR_MS : FLOOR_MS;
    const ceiling = switching ? SWITCH_CEILING_MS : CEILING_MS;
    const enteredAt = performance.now();
    timers.current.poll = globalThis.setInterval(() => {
      const waited = performance.now() - enteredAt;
      if (waited >= ceiling || (waited >= floor && pointerSpeed() < SETTLE)) {
        settle();
      }
    }, POLL_MS);
  }, [settle]);

  const onPointerLeave = useCallback(() => {
    stopPoll();
    if (!timers.current.open) {
      return;
    }
    timers.current.close = globalThis.setTimeout(shut, CLOSE_GRACE_MS);
  }, [shut, stopPoll]);

  // A trigger that unmounts while its popover is open must not leave the count
  // of open popovers standing: the next hover would read the page as switching
  // and fire on a pointer that has shown nothing.
  //
  // Declared BEFORE the pointer watch below, because React tears effects down
  // in declaration order and this one has to run while the shared state it
  // adjusts is still the state the watch will reset.
  useEffect(
    () => () => {
      stopPoll();
      if (timers.current.close !== undefined) {
        globalThis.clearTimeout(timers.current.close);
      }
      if (timers.current.open) {
        timers.current.open = false;
        openCount = Math.max(0, openCount - 1);
        lastClosedAt = performance.now();
        if (activeOwner === timers.current) {
          activeOwner = undefined;
        }
      }
    },
    [stopPoll],
  );

  useEffect(() => watchPointer(), []);

  return { onPointerEnter, onPointerLeave };
}
