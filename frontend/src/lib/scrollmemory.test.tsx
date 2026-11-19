/** @vitest-environment happy-dom */
import { render } from "@testing-library/react";
import { act, useRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  forgetScrollMemory,
  historyEntryId,
  keepPlaceOnNextMove,
  seekScrollTop,
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

// A column whose offset clamps to its range, as a browser's does.
function clampedColumn(container: HTMLElement) {
  const element = container.querySelector<HTMLElement>(
    '[data-testid="column"]',
  );
  if (!element) throw new Error("the harness drew no column");
  return clamped(element);
}

function clamped(element: HTMLElement) {
  let bottom = 10_000;
  let top = element.scrollTop;
  Object.defineProperty(element, "scrollTop", {
    configurable: true,
    get: () => top,
    set: (next: number) => {
      top = Math.max(0, Math.min(next, bottom));
    },
  });
  return {
    element,
    shorten: (height: number) => {
      bottom = height;
      top = Math.min(top, bottom);
    },
  };
}

describe("seeking an offset", () => {
  // Only the boxes a test grows report, as a browser's observer does.
  class Boxes {
    static watching = new Map<Element, Boxes>();
    constructor(private readonly report: () => void) {}
    observe(target: Element) {
      Boxes.watching.set(target, this);
    }
    disconnect() {
      for (const [target, owner] of Boxes.watching) {
        if (owner === this) Boxes.watching.delete(target);
      }
    }
    static grow(target: Element) {
      Boxes.watching.get(target)?.report();
    }
  }

  afterEach(() => {
    vi.unstubAllGlobals();
    Boxes.watching.clear();
  });

  it("lands once a child grows with no node added and the pane's box unchanged", () => {
    vi.stubGlobal("ResizeObserver", Boxes);
    const pane = document.createElement("div");
    const body = document.createElement("div");
    pane.append(body);
    document.body.append(pane);
    const column = clamped(pane);
    column.shorten(200);
    const stop = seekScrollTop(pane, 1000);
    expect(pane.scrollTop).toBe(200);

    // An image inside the body loads: the body's box grows, nothing else.
    column.shorten(10_000);
    Boxes.grow(body);
    expect(pane.scrollTop).toBe(1000);
    stop();
    pane.remove();
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
    useScrollMemory(column, address, { view });
    return <div ref={column} data-testid="column" />;
  }

  // A press on the record's tab strip, which asks to keep the place.
  function openTimeline() {
    const { container, rerender } = render(
      <Page address="#/contacts/c-1" view="#/contacts/c-1" />,
    );
    scrollTo(container, 900);
    const overview = window.history.state;
    keepPlaceOnNextMove();
    window.history.pushState(null, "", "#/contacts/c-1/timeline");
    rerender(<Page address="#/contacts/c-1/timeline" view="#/contacts/c-1" />);
    return { container, rerender, overview };
  }

  it("remembers where a held press is headed, not the clamp on the way", () => {
    const view = "#/contacts/c-1";
    const { container, rerender } = render(
      <Page address="#/contacts/c-1" view={view} />,
    );
    const column = clampedColumn(container);
    scrollTo(container, 1000);
    keepPlaceOnNextMove();
    window.history.pushState(null, "", "#/contacts/c-1/timeline");
    // The new tab's body arrives short, and the strip holds its place.
    column.shorten(200);
    const stop = seekScrollTop(column.element, 1000);
    rerender(<Page address="#/contacts/c-1/timeline" view={view} />);
    act(() => {
      column.element.dispatchEvent(new Event("scroll"));
    });
    const timeline = window.history.state;

    // The reader leaves before the body grows, and comes back after.
    stop();
    window.history.pushState(null, "", "#/deals");
    rerender(<Page address="#/deals" view="#/deals" />);
    column.shorten(10_000);
    window.history.pushState(timeline, "", "#/contacts/c-1/timeline");
    rerender(<Page address="#/contacts/c-1/timeline" view={view} />);
    expect(column.element.scrollTop).toBe(1000);
  });

  it("keeps the reader's place when the new entry shows the same record", () => {
    const { container } = openTimeline();
    expect(offsetOf(container)).toBe(900);
  });

  it("opens another tab at its top when no press asked to keep the place", () => {
    const { container, rerender } = render(
      <Page address="#/contacts/c-1" view="#/contacts/c-1" />,
    );
    scrollTo(container, 900);
    window.history.pushState(null, "", "#/contacts/c-1/tasks");
    rerender(<Page address="#/contacts/c-1/tasks" view="#/contacts/c-1" />);
    expect(offsetOf(container)).toBe(0);
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
    useScrollMemory(column, address, { lane: "drawer", span: "load" });
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
