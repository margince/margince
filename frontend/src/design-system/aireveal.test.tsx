/** @vitest-environment happy-dom */
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
import { AiReveal } from "./aireveal";

// Stands in for the browser's measure of what is on screen: each observed
// block reports in when the test scrolls it in.
const watched: Array<{
  target: Element;
  report: IntersectionObserverCallback;
}> = [];

class ScreenObserver {
  constructor(private readonly callback: IntersectionObserverCallback) {}
  observe(target: Element) {
    watched.push({ target, report: this.callback });
  }
  disconnect() {}
  unobserve() {}
  takeRecords() {
    return [];
  }
}

function scrollIntoSight(target: Element) {
  act(() => {
    for (const entry of watched.filter((w) => w.target === target)) {
      entry.report(
        [{ target, isIntersecting: true } as IntersectionObserverEntry],
        {} as IntersectionObserver,
      );
    }
  });
}

function blockOf(words: string): HTMLElement {
  const wrapper = screen.getByText(words).parentElement;
  if (!wrapper) {
    throw new Error("the words have no wrapper");
  }
  return wrapper;
}

function answer(words: string) {
  return (
    <AiReveal text={words}>
      <p>{words}</p>
    </AiReveal>
  );
}

beforeEach(() => {
  watched.length = 0;
  vi.stubGlobal("IntersectionObserver", ScreenObserver);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("AiReveal plays the first time its words are on screen", () => {
  it("holds words that land off screen, and plays them once they scroll in", () => {
    render(answer("Lena signs before the 14th."));
    const block = blockOf("Lena signs before the 14th.");
    expect(block.getAttribute("data-ai-reveal")).toBe("wait");
    scrollIntoSight(block);
    expect(block.getAttribute("data-ai-reveal")).toBe("run");
  });

  it("stands still for words this reader has already seen", () => {
    const { unmount } = render(answer("Procurement wants it in writing."));
    scrollIntoSight(blockOf("Procurement wants it in writing."));
    unmount();
    // The tab reopened: the same brief, read again from the cache.
    render(answer("Procurement wants it in writing."));
    expect(
      blockOf("Procurement wants it in writing.").hasAttribute(
        "data-ai-reveal",
      ),
    ).toBe(false);
  });

  it("plays again when the words change, as a rewrite does", () => {
    const { rerender } = render(answer("Does Thursday work?"));
    scrollIntoSight(blockOf("Does Thursday work?"));
    rerender(answer("Can you confirm by Wednesday?"));
    const block = blockOf("Can you confirm by Wednesday?");
    scrollIntoSight(block);
    expect(block.getAttribute("data-ai-reveal")).toBe("run");
  });

  it("ends with the colour, not the wipe", () => {
    render(answer("Two of five buyers opened it."));
    const block = blockOf("Two of five buyers opened it.");
    scrollIntoSight(block);
    fireEvent.animationEnd(block, { animationName: "aireveal-wipe" });
    expect(block.getAttribute("data-ai-reveal")).toBe("run");
    fireEvent.animationEnd(block, { animationName: "aireveal-color" });
    expect(block.hasAttribute("data-ai-reveal")).toBe(false);
  });

  it("shows a block taller than the screen at rest, and counts it as seen", () => {
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      height: window.innerHeight * 3,
    } as DOMRect);
    const { unmount } = render(answer("A very long draft."));
    scrollIntoSight(blockOf("A very long draft."));
    expect(blockOf("A very long draft.").hasAttribute("data-ai-reveal")).toBe(
      false,
    );
    unmount();
    vi.restoreAllMocks();
    render(answer("A very long draft."));
    expect(blockOf("A very long draft.").hasAttribute("data-ai-reveal")).toBe(
      false,
    );
  });
});
