// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, onTestFinished, vi } from "vitest";
import { useLineClamped } from "./lineclamp";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function Note() {
  const [ref, clipped] = useLineClamped(false, "The board stayed put.");
  return (
    <p ref={ref} data-testid="note" data-clipped={clipped || undefined}>
      The board stayed put.
    </p>
  );
}

// happy-dom lays nothing out, so each case sets the two heights it measures.
function boxHeights(scroll: { value: number }, client: number) {
  vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockImplementation(
    () => scroll.value,
  );
  vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(
    client,
  );
}

describe("useLineClamped", () => {
  it("reads a box whose text fills it exactly as not clipped", () => {
    boxHeights({ value: 40 }, 40);
    render(<Note />);
    expect(screen.getByTestId("note")).not.toHaveAttribute("data-clipped");
  });

  it("measures again once the web fonts have loaded", async () => {
    const scroll = { value: 40 };
    boxHeights(scroll, 40);
    let loaded: () => void = () => {};
    const ready = new Promise<void>((resolve) => {
      loaded = resolve;
    });
    const fonts = Object.getOwnPropertyDescriptor(document, "fonts");
    Object.defineProperty(document, "fonts", {
      configurable: true,
      value: { ready },
    });
    onTestFinished(() => {
      if (fonts) Object.defineProperty(document, "fonts", fonts);
      else Reflect.deleteProperty(document, "fonts");
    });
    render(<Note />);
    expect(screen.getByTestId("note")).not.toHaveAttribute("data-clipped");
    scroll.value = 60;
    await act(async () => {
      loaded();
      await ready;
    });
    expect(screen.getByTestId("note")).toHaveAttribute("data-clipped");
  });

  it("measures again when the box is resized", () => {
    const scroll = { value: 40 };
    boxHeights(scroll, 40);
    let resized: () => void = () => {};
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(fire: () => void) {
          resized = fire;
        }
        observe() {}
        disconnect() {}
      },
    );
    render(<Note />);
    expect(screen.getByTestId("note")).not.toHaveAttribute("data-clipped");
    scroll.value = 60;
    act(() => resized());
    expect(screen.getByTestId("note")).toHaveAttribute("data-clipped");
  });
});
