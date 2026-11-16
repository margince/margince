// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { PublicPage } from "./publicpage";

// The centred column glides to where a change of its height puts it. happy-dom
// lays nothing out, so each case moves the column's top itself and fires the
// observer the browser would.

const resized: (() => void)[] = [];
const layout = { top: 300 };

function reducedMotion(reduce: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: reduce && query.includes("reduce"),
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

beforeEach(() => {
  vi.useFakeTimers({
    toFake: ["requestAnimationFrame", "cancelAnimationFrame"],
  });
  layout.top = 300;
  vi.spyOn(HTMLElement.prototype, "offsetTop", "get").mockImplementation(
    () => layout.top,
  );
  vi.stubGlobal(
    "ResizeObserver",
    class {
      private readonly fire: () => void;
      constructor(fire: () => void) {
        this.fire = fire;
      }
      observe() {
        resized.push(this.fire);
      }
      disconnect() {
        resized.splice(resized.indexOf(this.fire), 1);
      }
    },
  );
  reducedMotion(false);
});

afterEach(() => {
  cleanup();
  resized.length = 0;
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderPage() {
  render(
    <LocaleProvider>
      <PublicPage>
        <button type="button">Save</button>
      </PublicPage>
    </LocaleProvider>,
  );
  const button = screen.getByRole("button", { name: "Save" });
  const column = button.closest<HTMLElement>(".pref-center");
  if (column === null) {
    throw new Error("PublicPage rendered no .pref-center column");
  }
  return { button, column };
}

function columnMovesTo(top: number) {
  layout.top = top;
  act(() => {
    for (const fire of resized) fire();
  });
}

describe("the public page's column after a change of its height", () => {
  it("starts where it stood after a press, then glides home and settles", () => {
    const { button, column } = renderPage();
    fireEvent.pointerDown(button);
    columnMovesTo(200);
    expect(column.style.transform).toBe("translateY(100px)");
    expect(column).not.toHaveAttribute("data-gliding");

    act(() => vi.advanceTimersToNextFrame());
    expect(column).toHaveAttribute("data-gliding");
    expect(column.style.transform).toBe("");

    fireEvent(
      column,
      Object.assign(new Event("transitionend"), { propertyName: "transform" }),
    );
    expect(column).not.toHaveAttribute("data-gliding");
  });

  it("stays put when the page settles after its load, before any press", () => {
    const { column } = renderPage();
    columnMovesTo(200);
    expect(column.style.transform).toBe("");
    expect(column).not.toHaveAttribute("data-gliding");
  });

  it("moves at once for a reader who asked for reduced motion", () => {
    reducedMotion(true);
    const { button, column } = renderPage();
    fireEvent.keyDown(button, { key: "Enter" });
    columnMovesTo(200);
    act(() => vi.advanceTimersToNextFrame());
    expect(column.style.transform).toBe("");
    expect(column).not.toHaveAttribute("data-gliding");
  });
});
