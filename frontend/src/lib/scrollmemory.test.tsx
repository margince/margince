/** @vitest-environment happy-dom */
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
    expect(window.history.state).toMatchObject({ borrowed: "keep me" });
  });

  it("cannot mint an id an entry from a previous page load already holds", () => {
    // `history.state` outlives the page it was written on; the offsets do not.
    // With ids counted from one on every load, the first entry stamped after a
    // reload answered to a name an older entry still held, and a reader's place
    // in one list was restored onto another.
    const before = historyEntryId();
    // A reload: the module's memory goes, the entry's stamp stays.
    forgetScrollMemory();
    window.history.pushState(null, "", "#/deals");
    expect(historyEntryId()).not.toBe(before);
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

describe("another tab of the same record", () => {
  // `view` is what the address is about, as lib/routeidentity.ts derives it:
  // a record's tabs share one, and each record has its own.
  function Page({
    address,
    view,
  }: Readonly<{ address: string; view: string }>) {
    const column = useRef<HTMLDivElement>(null);
    useScrollMemory(column, address, "page", "entry", view);
    return <div ref={column} data-testid="column" />;
  }

  function openTimeline() {
    const { container, rerender } = render(
      <Page address="#/contacts/c-1" view="#/contacts/c-1" />,
    );
    scrollTo(container, 900);
    const overview = window.history.state;
    window.history.pushState(null, "", "#/contacts/c-1/timeline");
    rerender(<Page address="#/contacts/c-1/timeline" view="#/contacts/c-1" />);
    return { container, rerender, overview };
  }

  it("keeps the reader's place when the new entry shows the same record", () => {
    const { container } = openTimeline();
    expect(offsetOf(container)).toBe(900);
  });

  it("opens another record at its top", () => {
    const { container, rerender } = openTimeline();
    window.history.pushState(null, "", "#/contacts/c-2");
    rerender(<Page address="#/contacts/c-2" view="#/contacts/c-2" />);
    expect(offsetOf(container)).toBe(0);
  });

  it("restores the offset a tab was left at when the reader goes Back to it", () => {
    const { container, rerender, overview } = openTimeline();
    scrollTo(container, 300);
    // Back: the browser puts the earlier entry's state, its id included, back.
    window.history.pushState(overview, "", "#/contacts/c-1");
    rerender(<Page address="#/contacts/c-1" view="#/contacts/c-1" />);
    expect(offsetOf(container)).toBe(900);
  });

  it("remembers the kept place for the tab, so Back to it returns there", () => {
    const { container, rerender } = openTimeline();
    const timeline = window.history.state;
    window.history.pushState(null, "", "#/deals");
    rerender(<Page address="#/deals" view="#/deals" />);
    expect(offsetOf(container)).toBe(0);
    window.history.pushState(timeline, "", "#/contacts/c-1/timeline");
    rerender(<Page address="#/contacts/c-1/timeline" view="#/contacts/c-1" />);
    expect(offsetOf(container)).toBe(900);
  });
});

describe("a lane kept for the whole load", () => {
  function Drawer({ address }: Readonly<{ address: string }>) {
    const column = useRef<HTMLDivElement>(null);
    useScrollMemory(column, address, "drawer", "load");
    return <div ref={column} data-testid="column" />;
  }

  it("returns the reader to their place through a NEW history entry", () => {
    // A record's way back to the Worklist drawer pushes an address rather than
    // stepping Back, so a memory keyed on the entry would open it at the top.
    const first = render(<Drawer address="#/home?queue=1" />);
    scrollTo(first.container, 900);
    first.unmount();
    window.history.pushState(null, "", "#/contacts/c-1");
    window.history.pushState(null, "", "#/home?queue=1");

    const again = render(<Drawer address="#/home?queue=1" />);
    expect(offsetOf(again.container)).toBe(900);
  });
});
