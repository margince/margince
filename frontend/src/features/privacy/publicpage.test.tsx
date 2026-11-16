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
// lays nothing out, so each case sets the layout itself and fires the observer
// the browser would, naming the boxes that changed size.

type Resized = (entries: { target: Element }[]) => void;

const observers: { fire: Resized; boxes: Element[] }[] = [];
const layout = { top: 300, pageWidth: 1280, shown: true };

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
  Object.assign(layout, { top: 300, pageWidth: 1280, shown: true });
  vi.spyOn(HTMLElement.prototype, "offsetTop", "get").mockImplementation(
    () => layout.top,
  );
  vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockImplementation(
    () => layout.pageWidth,
  );
  // happy-dom has no offsetParent at all, so there is nothing to spy on.
  Object.defineProperty(HTMLElement.prototype, "offsetParent", {
    configurable: true,
    get: () => (layout.shown ? document.body : null),
  });
  vi.stubGlobal(
    "ResizeObserver",
    class {
      private readonly watching: { fire: Resized; boxes: Element[] };
      constructor(fire: Resized) {
        this.watching = { fire, boxes: [] };
        observers.push(this.watching);
      }
      observe(box: Element) {
        this.watching.boxes.push(box);
      }
      disconnect() {
        observers.splice(observers.indexOf(this.watching), 1);
      }
    },
  );
  reducedMotion(false);
});

afterEach(() => {
  cleanup();
  observers.length = 0;
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  Reflect.deleteProperty(HTMLElement.prototype, "offsetParent");
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
  const page = column?.parentElement;
  if (!column || !page) {
    throw new Error("PublicPage rendered no .pref-center column in a page");
  }
  return { button, column, page };
}

function resize(top: number, ...boxes: Element[]) {
  layout.top = top;
  act(() => {
    for (const { fire, boxes: watched } of observers) {
      const seen = boxes.filter((box) => watched.includes(box));
      if (seen.length > 0) fire(seen.map((target) => ({ target })));
    }
  });
}

function pointerPress(button: Element) {
  fireEvent.pointerDown(button);
}

function keyPress(button: Element) {
  fireEvent.keyDown(button, { key: "Enter" });
}

describe("the public page's column after a change of its height", () => {
  it.each([
    ["a pointer press", pointerPress],
    ["a key press", keyPress],
  ])(
    "after %s, starts where it stood, then glides home and settles",
    (_, press) => {
      const { button, column } = renderPage();
      press(button);
      resize(200, column);
      expect(column.style.transform).toBe("translateY(100px)");
      expect(column).not.toHaveAttribute("data-gliding");

      act(() => vi.advanceTimersToNextFrame());
      expect(column).toHaveAttribute("data-gliding");
      expect(column.style.transform).toBe("");

      fireEvent(
        column,
        Object.assign(new Event("transitionend"), {
          propertyName: "transform",
        }),
      );
      expect(column).not.toHaveAttribute("data-gliding");
    },
  );

  it("stays put when the page settles after its load, before any press", () => {
    const { column } = renderPage();
    resize(200, column);
    expect(column.style.transform).toBe("");
    expect(column).not.toHaveAttribute("data-gliding");
  });

  it("takes a window that changed height as the new resting place", () => {
    const { button, column, page } = renderPage();
    pointerPress(button);
    resize(139, page);
    expect(column.style.transform).toBe("");

    resize(48, column, page);
    expect(column.style.transform).toBe("translateY(91px)");
  });

  it("does not glide when a window that changed width reflowed the column", () => {
    const { button, column, page } = renderPage();
    pointerPress(button);
    layout.pageWidth = 390;
    resize(200, column, page);
    expect(column.style.transform).toBe("");
  });

  it("drops a glide when the column leaves the page, and restarts from rest", () => {
    const { button, column } = renderPage();
    pointerPress(button);
    resize(200, column);
    layout.shown = false;
    resize(0, column);
    expect(column.style.transform).toBe("");
    expect(column).not.toHaveAttribute("data-gliding");

    layout.shown = true;
    resize(250, column);
    expect(column.style.transform).toBe("");
  });

  it("moves at once for a reader who asked for reduced motion", () => {
    reducedMotion(true);
    const { button, column } = renderPage();
    keyPress(button);
    resize(200, column);
    act(() => vi.advanceTimersToNextFrame());
    expect(column.style.transform).toBe("");
    expect(column).not.toHaveAttribute("data-gliding");
  });
});
