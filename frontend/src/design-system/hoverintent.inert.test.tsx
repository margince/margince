/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { armHoverIntent } from "./hoverintent-testing";
import { useTooltip } from "./tooltip";

// That the suite's hover-intent triggers stay shut under a pointer unless a
// case asks otherwise. vitest.setup.ts arms that; this is what fails if it
// stops, or if the mock no longer reaches a module a test file imports.
//
// Driven through a real caller rather than the hook itself, because the flake
// lived in screens that reach the hook two imports down.

function Anchor() {
  const tip = useTooltip<HTMLButtonElement>("Deals");
  return (
    <button type="button" ref={tip.ref} {...tip.trigger}>
      icon
      {tip.tip}
    </button>
  );
}

// Far past the hook's ceiling: whatever the pointer did, the real hook has
// settled by now.
const LONG_AFTER_ANY_CEILING = 10_000;

beforeEach(() => {
  vi.useFakeTimers({
    toFake: [
      "setTimeout",
      "clearTimeout",
      "setInterval",
      "clearInterval",
      "performance",
    ],
  });
});

afterEach(() => {
  vi.useRealTimers();
});

function restPointerOn(trigger: HTMLElement) {
  fireEvent.pointerEnter(trigger);
  act(() => {
    vi.advanceTimersByTime(LONG_AFTER_ANY_CEILING);
  });
}

it("opens nothing under a resting pointer, however long the case waits", () => {
  render(<Anchor />);

  restPointerOn(screen.getByRole("button"));

  expect(screen.queryByRole("tooltip")).toBeNull();
});

it("opens under a resting pointer once the case arms the real hook", () => {
  armHoverIntent();
  render(<Anchor />);

  restPointerOn(screen.getByRole("button"));

  expect(screen.getByRole("tooltip").textContent).toBe("Deals");
});

it("starts inert again after a case that armed it", () => {
  render(<Anchor />);

  restPointerOn(screen.getByRole("button"));

  expect(screen.queryByRole("tooltip")).toBeNull();
});

it("still opens on keyboard focus, which never went through the hook", () => {
  render(<Anchor />);

  act(() => {
    screen.getByRole("button").focus();
  });

  expect(screen.getByRole("tooltip").textContent).toBe("Deals");
});
