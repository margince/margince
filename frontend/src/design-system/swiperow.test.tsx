// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SwipeRow, swipeSide } from "./swiperow";

afterEach(cleanup);

describe("swipeSide", () => {
  it("reads a decisive horizontal drag as its side", () => {
    expect(swipeSide(80, 0)).toBe("end");
    expect(swipeSide(-80, 0)).toBe("start");
  });

  // A row lives in a scrolling list. Were a mostly-vertical drag to answer,
  // a thumb moving down the queue would file a judgement on every row it
  // crossed — which is the whole reason this rule is not the deck's.
  it("refuses a drag that mostly went up or down, however far it travelled", () => {
    expect(swipeSide(60, 200)).toBeNull();
    expect(swipeSide(-60, -200)).toBeNull();
  });

  // Short of the threshold is a tap that wandered, not an answer.
  it("refuses a drag that did not travel far enough", () => {
    expect(swipeSide(20, 0)).toBeNull();
    expect(swipeSide(-20, 2)).toBeNull();
  });

  // Exactly on the boundary is refused, so the two arms cannot both claim it.
  it("refuses the threshold itself", () => {
    expect(swipeSide(55, 0)).toBeNull();
    expect(swipeSide(56, 0)).toBe("end");
  });

  // A perfect diagonal is a SCROLL here, where a deck reads it as a verdict.
  // Stated rather than left to fall out of the comparison, because the two
  // surfaces answer it differently on purpose: a row that guessed on a diagonal
  // would file a judgement for a reader who was moving down the list.
  it("refuses a drag with no dominant direction", () => {
    expect(swipeSide(80, 80)).toBeNull();
    expect(swipeSide(-80, 80)).toBeNull();
  });
});

// The drag STAGES and the press ACTS. Every action offered on a row removes it
// from the reader's view, so a gesture that ran one directly would let a thumb
// sliding down a list file three judgements nobody made.
describe("SwipeRow", () => {
  const drag = (row: HTMLElement, dx: number, dy = 0) => {
    fireEvent.pointerDown(row, { clientX: 0, clientY: 0, isPrimary: true });
    fireEvent.pointerUp(row, { clientX: dx, clientY: dy, isPrimary: true });
  };

  const rowOf = (onAct: () => void) => {
    render(
      <SwipeRow
        cancelLabel="Cancel"
        end={{ label: "Snooze", onAct }}
        start={{ label: "Archive", onAct }}
      >
        <p>the work</p>
      </SwipeRow>,
    );
    return screen.getByTestId("swipe-row");
  };

  it("stages the action a drag chose without running it", () => {
    const onAct = vi.fn();
    drag(rowOf(onAct), 80);

    expect(screen.getByRole("button", { name: "Snooze" })).toBeInTheDocument();
    expect(onAct).not.toHaveBeenCalled();
  });

  it("runs the action only when the reader presses it", () => {
    const onAct = vi.fn();
    drag(rowOf(onAct), 80);
    fireEvent.click(screen.getByRole("button", { name: "Snooze" }));

    expect(onAct).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("button", { name: "Snooze" })).toBeNull();
  });

  it("takes a staged action back, so a drag meant as a scroll costs one tap", () => {
    const onAct = vi.fn();
    drag(rowOf(onAct), 80);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("button", { name: "Snooze" })).toBeNull();
    expect(onAct).not.toHaveBeenCalled();
  });

  it("stages the other side's action when the drag went the other way", () => {
    const onAct = vi.fn();
    drag(rowOf(onAct), -80);

    expect(screen.getByRole("button", { name: "Archive" })).toBeInTheDocument();
  });

  it("stages nothing when the drag was a scroll", () => {
    const onAct = vi.fn();
    drag(rowOf(onAct), 60, 200);

    expect(screen.queryByRole("button", { name: "Snooze" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Archive" })).toBeNull();
  });

  // A side the caller did not offer stages nothing rather than an empty bar.
  it("stages nothing on a side the row does not offer", () => {
    render(
      <SwipeRow cancelLabel="Cancel" end={{ label: "Snooze", onAct: vi.fn() }}>
        <p>the work</p>
      </SwipeRow>,
    );
    drag(screen.getByTestId("swipe-row"), -80);

    expect(screen.queryByRole("status")).toBeNull();
  });

  // A pointer that left the row mid-drag fires no pointerup on it. Without the
  // cancel handler its origin would still be sitting there when the next touch
  // began, measuring one drag from the start of an earlier one.
  it("forgets a cancelled drag rather than measuring the next one from it", () => {
    const onAct = vi.fn();
    const row = rowOf(onAct);
    fireEvent.pointerDown(row, { clientX: 0, clientY: 0, isPrimary: true });
    fireEvent.pointerCancel(row);
    fireEvent.pointerUp(row, { clientX: 80, clientY: 0, isPrimary: true });

    expect(screen.queryByRole("status")).toBeNull();
  });

  // A second finger arriving mid-scroll must not restart the measurement from
  // its own landing point, which would read a two-finger scroll as a drag the
  // width of the hand.
  it("measures the primary pointer only", () => {
    const onAct = vi.fn();
    const row = rowOf(onAct);
    fireEvent.pointerDown(row, { clientX: 300, clientY: 0, isPrimary: false });
    fireEvent.pointerUp(row, { clientX: 380, clientY: 0, isPrimary: false });

    expect(screen.queryByRole("status")).toBeNull();
  });
});
