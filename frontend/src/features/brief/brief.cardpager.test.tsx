/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { CardPager } from "./brief.cardpager";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("browses with arrow keys without completing the task", async () => {
  const user = userEvent.setup();
  const previous = vi.fn();
  const next = vi.fn();
  render(
    <LocaleProvider initial="en">
      <CardPager title="Task A" previous={previous} next={next}>
        <p>Task A</p>
      </CardPager>
    </LocaleProvider>,
  );
  screen.getByRole("button", { name: "Next task" }).focus();
  await user.keyboard("{ArrowRight}{ArrowLeft}");
  expect(next).toHaveBeenCalledTimes(1);
  expect(previous).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("status").textContent).toBe("Task A");
});

it("ignores short and vertical drags and accepts a horizontal swipe", () => {
  const next = vi.fn();
  Object.defineProperty(HTMLElement.prototype, "setPointerCapture", {
    configurable: true,
    value: vi.fn(),
  });
  render(
    <LocaleProvider initial="en">
      <CardPager title="Task A" next={next}>
        <p>Task A</p>
      </CardPager>
    </LocaleProvider>,
  );
  const surface = screen.getByText("Task A", { selector: "p" }).parentElement;
  if (!surface) throw new Error("missing card surface");
  const swipe = (x: number, y: number) => {
    fireEvent.pointerDown(surface, {
      isPrimary: true,
      button: 0,
      pointerId: 1,
      clientX: 100,
      clientY: 100,
    });
    fireEvent.pointerUp(surface, {
      pointerId: 1,
      clientX: 100 + x,
      clientY: 100 + y,
    });
  };
  swipe(-20, 0);
  swipe(-80, 100);
  expect(next).not.toHaveBeenCalled();
  swipe(-80, 0);
  expect(next).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("button", { name: "Previous task" })).toHaveProperty(
    "disabled",
    true,
  );
});
