/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useHoverIntent } from "./hoverintent";

// The whole point of the hook is what it does NOT do, and none of that is
// visible by eye — a popover that fires on a passing pointer looks identical
// to one that fires correctly, one reader in ten.

function Trigger({
  onOpen,
  onClose,
}: Readonly<{ onOpen: () => void; onClose: () => void }>) {
  const hover = useHoverIntent(onOpen, onClose);
  return (
    <button
      type="button"
      onPointerEnter={hover.onPointerEnter}
      onPointerLeave={hover.onPointerLeave}
    >
      proof
    </button>
  );
}

// One pointermove, at the position and moment the caller names. The hook reads
// `timeStamp` off the event, which jsdom does not fill in from fake timers.
function movePointer(x: number, y: number) {
  const event = new Event("pointermove") as PointerEvent & {
    clientX: number;
    clientY: number;
  };
  Object.defineProperties(event, {
    clientX: { value: x },
    clientY: { value: y },
    timeStamp: { value: performance.now() },
  });
  document.dispatchEvent(event);
}

function advance(ms: number) {
  act(() => {
    vi.advanceTimersByTime(ms);
  });
}

let opened: number;
let closed: number;

beforeEach(() => {
  vi.useFakeTimers({
    // performance.now IS the clock this hook reasons with, so a fake timer
    // that leaves it running measures a real elapsed time against a simulated
    // one and every threshold reads as instantly met.
    toFake: [
      "setTimeout",
      "clearTimeout",
      "setInterval",
      "clearInterval",
      "performance",
    ],
  });
  opened = 0;
  closed = 0;
  render(
    <Trigger
      onOpen={() => {
        opened += 1;
      }}
      onClose={() => {
        closed += 1;
      }}
    />,
  );
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function enter() {
  fireEvent.pointerEnter(screen.getByRole("button"));
}

function leave() {
  fireEvent.pointerLeave(screen.getByRole("button"));
}

describe("useHoverIntent", () => {
  it("opens once the hand has stopped, not the moment it arrives", () => {
    enter();
    // Arrival is not intent. Nothing has been shown yet.
    expect(opened).toBe(0);
    // A hand that has settled sends no move events at all, so the silence
    // itself is what says the pointer stopped.
    advance(100);
    expect(opened).toBe(1);
  });

  it("stays shut under a pointer that is on its way past", () => {
    enter();
    // 4px every 25ms is 0.16 px/ms — twice the settle threshold, and the
    // reader is crossing the trigger rather than reading it.
    for (let step = 1; step <= 8; step += 1) {
      movePointer(step * 4, 0);
      advance(25);
    }
    expect(opened).toBe(0);
  });

  it("fires at the ceiling anyway, so a fidgeting hand is not locked out", () => {
    enter();
    // The pointer never settles, but the reader has plainly stayed put.
    for (let step = 1; step <= 14; step += 1) {
      movePointer(step * 4, 0);
      advance(25);
    }
    expect(opened).toBe(1);
  });

  it("survives the gap between the trigger and the panel under it", () => {
    enter();
    advance(100);
    expect(opened).toBe(1);
    leave();
    // Still open while the pointer is crossing the gap: a popover that shut
    // here would be the page pulling away from the reader reaching for it.
    advance(120);
    expect(closed).toBe(0);
    advance(100);
    expect(closed).toBe(1);
  });

  it("does not let a late close reach a neighbour that has since opened", () => {
    let neighborOpened = 0;
    let neighborClosed = 0;
    render(
      <Trigger
        onOpen={() => {
          neighborOpened += 1;
        }}
        onClose={() => {
          neighborClosed += 1;
        }}
      />,
    );
    const [first, second] = screen.getAllByRole("button");
    fireEvent.pointerEnter(first);
    advance(100);
    expect(opened).toBe(1);
    fireEvent.pointerLeave(first);
    // Within the grace period, so the switching pair applies: the second
    // trigger settles fast enough to become the open one before the first's
    // delayed close fires. 110ms is above the module's switch ceiling.
    fireEvent.pointerEnter(second);
    advance(110);
    expect(neighborOpened).toBe(1);
    // The first trigger's close was scheduled before the switch and is still
    // pending. It must not blank what the second trigger just opened. 180ms
    // is the module's close grace period.
    advance(180);
    expect(closed).toBe(0);
    expect(neighborClosed).toBe(0);
    fireEvent.pointerLeave(first);
    fireEvent.pointerLeave(second);
  });
});
