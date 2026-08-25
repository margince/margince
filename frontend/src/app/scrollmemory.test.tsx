/** @vitest-environment jsdom */
import { render } from "@testing-library/react";
import { act, useRef } from "react";
import { afterEach, describe, expect, it } from "vitest";
import {
  forgetScrollMemory,
  historyEntryId,
  useScrollMemory,
} from "./scrollmemory";

// The content column is the same element on every route, so it carries the
// offset one page was left at into the next unless something intervenes. What
// used to intervene reset it to the top on EVERY address change, Back included
// — which is the reader who scrolled a long list, opened a record, came back,
// and had to find their place again.

afterEach(() => {
  forgetScrollMemory();
  window.history.replaceState(null, "");
});

/**
 * A scrollable column, driven at an address.
 *
 * jsdom lays nothing out, so `scrollTop` is a plain property here and the
 * browser's clamp against a short column does not happen. That is exactly the
 * boundary this test wants: what is asserted is which offset the hook ASKS for
 * on the way in and records on the way out, which is the whole of its logic.
 */
function Column({ address }: Readonly<{ address: string }>) {
  const column = useRef<HTMLDivElement>(null);
  useScrollMemory(column, address);
  return <div ref={column} data-testid="column" />;
}

function offsetOf(container: HTMLElement): number {
  const column = container.querySelector<HTMLElement>('[data-testid="column"]');
  if (!column) {
    throw new Error("the harness drew no column");
  }
  return column.scrollTop;
}

function scrollTo(container: HTMLElement, top: number): void {
  const column = container.querySelector<HTMLElement>('[data-testid="column"]');
  if (!column) {
    throw new Error("the harness drew no column");
  }
  column.scrollTop = top;
  act(() => {
    column.dispatchEvent(new Event("scroll"));
  });
}

describe("a history entry's identity", () => {
  it("stamps an entry that has none, and keeps it", () => {
    const first = historyEntryId();
    expect(first).toBeTruthy();
    // Asked twice for the same entry, it is the same entry.
    expect(historyEntryId()).toBe(first);
  });

  it("leaves whatever else the entry's state carries", () => {
    window.history.replaceState({ borrowed: "keep me" }, "");
    historyEntryId();
    expect((window.history.state as Record<string, unknown>).borrowed).toBe(
      "keep me",
    );
  });

  it("names a new entry separately from the one before it", () => {
    const first = historyEntryId();
    window.history.pushState(null, "", "#/elsewhere");
    expect(historyEntryId()).not.toBe(first);
  });
});

describe("returning to a page", () => {
  it("opens a page nobody has visited at its top", () => {
    const { container } = render(<Column address="#/companies" />);
    expect(offsetOf(container)).toBe(0);
  });

  it("returns the reader to where they left the page", () => {
    const { container, rerender } = render(<Column address="#/companies" />);
    scrollTo(container, 1840);

    // A different address on the SAME entry is the ordinary case a screen makes
    // when it rewrites its own dials, and the offset belongs to the entry.
    rerender(<Column address="#/companies/c-42" />);
    expect(offsetOf(container)).toBe(1840);
  });

  it("does not carry one page's offset into a page the reader has not seen", () => {
    const { container, rerender } = render(<Column address="#/companies" />);
    scrollTo(container, 1840);
    window.history.pushState(null, "", "#/deals");

    rerender(<Column address="#/deals" />);
    expect(offsetOf(container)).toBe(0);
  });

  it("keeps the reader's own scroll rather than putting them back", () => {
    // The restore is retried while the column grows, because a list's rows
    // arrive after its address does. A reader who scrolls in the meantime has
    // said where they want to be, and the retry must stop arguing.
    const { container, rerender } = render(<Column address="#/companies" />);
    scrollTo(container, 1840);
    rerender(<Column address="#/companies/c-42" />);
    expect(offsetOf(container)).toBe(1840);

    scrollTo(container, 40);
    rerender(<Column address="#/companies/c-42?x=1" />);
    // What is recorded is where the READER left it, not the offset the hook
    // had been asking for.
    expect(offsetOf(container)).toBe(40);
  });
});
