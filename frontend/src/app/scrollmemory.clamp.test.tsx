/** @vitest-environment jsdom */
import { render } from "@testing-library/react";
import { act, useRef } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { forgetScrollMemory, useScrollMemory } from "./scrollmemory";

// jsdom lays nothing out: `scrollTop` is a plain property there, `scrollHeight`
// is always 0, and an offset past the end of the content is accepted as readily
// as one inside it. The browser does neither — it CLAMPS — and every defect this
// file pins lives in that difference:
//
//   * a restore asks for 3500, a list still fetching its rows can only give 575,
//     and the scroll event for that 575 is not a reader who scrolled;
//   * leaving a list collapses it, so the offsets it reports on the way out are
//     the browser pinning the reader to a shrinking range, not a place they
//     chose — and one of those replaced the offset Back was about to ask for;
//   * the reader's own gesture outranks both.
//
// So the harness gives the column a range and clamps into it, which is the one
// boundary these tests need and the only thing they mock.

afterEach(() => {
  forgetScrollMemory();
  window.history.replaceState(null, "");
});

/** How tall the content is, and how much of it is on screen. */
type Range = { content: number; visible: number };

function clamping(column: HTMLElement, range: Range): void {
  let top = 0;
  Object.defineProperty(column, "scrollHeight", {
    configurable: true,
    get: () => range.content,
  });
  Object.defineProperty(column, "clientHeight", {
    configurable: true,
    get: () => range.visible,
  });
  Object.defineProperty(column, "scrollTop", {
    configurable: true,
    get: () => top,
    set: (asked: number) => {
      const bottom = Math.max(0, range.content - range.visible);
      const landed = Math.max(0, Math.min(asked, bottom));
      if (landed === top) {
        return;
      }
      top = landed;
      // The browser announces its own clamp exactly as it announces a reader's
      // scroll. Telling the two apart is the thing under test.
      column.dispatchEvent(new Event("scroll"));
    },
  });
}

function Column({
  address,
  range,
}: Readonly<{ address: string; range: Range }>) {
  const column = useRef<HTMLDivElement>(null);
  const armed = useRef(false);
  useScrollMemory(column, address, "rows");
  return (
    <div
      // A callback ref, because the range has to be in place BEFORE the hook's
      // effect runs: refs are attached first, and an element armed a render
      // later would have been restored against jsdom's unclamped default.
      ref={(node) => {
        column.current = node;
        if (node && !armed.current) {
          armed.current = true;
          clamping(node, range);
        }
      }}
      data-testid="rows"
    />
  );
}

function rowsOf(container: HTMLElement): HTMLElement {
  const rows = container.querySelector<HTMLElement>('[data-testid="rows"]');
  if (!rows) {
    throw new Error("the harness drew no rows");
  }
  return rows;
}

/** The reader takes hold, then scrolls — a wheel is not something we can fake. */
function readerScrollsTo(container: HTMLElement, top: number): void {
  const rows = rowsOf(container);
  act(() => {
    rows.dispatchEvent(new WheelEvent("wheel", { bubbles: true, deltaY: 1 }));
    rows.scrollTop = top;
  });
}

/**
 * The rows go, as they do when the reader opens a record: the element is still
 * in the document and its content collapses under the offset they were at.
 */
function rowsCollapse(container: HTMLElement, range: Range, to: number): void {
  act(() => {
    range.content = to;
    // Re-asserting the offset is what the browser does when the range shrinks:
    // it pins the reader to the new bottom and announces it.
    rowsOf(container).scrollTop = Number.MAX_SAFE_INTEGER;
  });
}

/**
 * Leave the list for a record, then come back — as a browser does it.
 *
 * The list UNMOUNTS on the way out (the record takes the column), the record is
 * a new entry with no state of its own, and Back restores the entry the list was
 * on. Nothing here can be shortened to a rerender: an address rewritten on the
 * same entry is deliberately not leaving the page, so a test that only rerendered
 * would assert that the hook does nothing.
 */
function leaveAndReturn(
  rendered: ReturnType<typeof render>,
  range: Range,
  arriving: number,
): ReturnType<typeof render> {
  const entry = window.history.state;
  rendered.unmount();
  window.history.pushState(null, "", "#/companies/c-1");
  window.history.replaceState(entry, "", "#/companies");
  // The list comes back with only the rows it has fetched so far.
  range.content = arriving;
  return render(<Column address="#/companies" range={range} />);
}

/** The rest of the rows land, which is a DOM change and nothing else. */
async function rowsArrive(
  container: HTMLElement,
  range: Range,
  content: number,
): Promise<void> {
  await act(async () => {
    range.content = content;
    rowsOf(container).append(document.createElement("tr"));
    // The observer answers on a microtask.
    await Promise.resolve();
  });
}

describe("a list's rows, remembered across a move", () => {
  it("returns the reader to where they were, not to the browser's clamp", async () => {
    const range: Range = { content: 9000, visible: 400 };
    const first = render(<Column address="#/companies" range={range} />);
    readerScrollsTo(first.container, 3500);
    expect(rowsOf(first.container).scrollTop).toBe(3500);

    // Opening a record collapses the rows to one page while this hook is still
    // listening, so every offset the element reports from here is a clamp.
    rowsCollapse(first.container, range, 975);
    expect(rowsOf(first.container).scrollTop).toBe(575);

    const second = leaveAndReturn(first, range, 9000);
    // The reader's own place, not the 575 the collapse left behind.
    expect(rowsOf(second.container).scrollTop).toBe(3500);
    second.unmount();
  });

  it("keeps aiming while the rows arrive, past the clamp it lands on first", async () => {
    const range: Range = { content: 9000, visible: 400 };
    const first = render(<Column address="#/companies" range={range} />);
    readerScrollsTo(first.container, 3500);

    // Back, with only the first page drawn: 3500 is out of reach and the browser
    // gives what it can.
    const second = leaveAndReturn(first, range, 975);
    expect(rowsOf(second.container).scrollTop).toBe(575);

    await rowsArrive(second.container, range, 9000);
    expect(rowsOf(second.container).scrollTop).toBe(3500);
    second.unmount();
  });

  it("lets the reader overrule the place it was aiming for", async () => {
    const range: Range = { content: 9000, visible: 400 };
    const first = render(<Column address="#/companies" range={range} />);
    readerScrollsTo(first.container, 3500);
    // They think better of it and go somewhere shallower. That is a choice, and
    // the one Back has to honour.
    readerScrollsTo(first.container, 800);
    rowsCollapse(first.container, range, 975);

    const second = leaveAndReturn(first, range, 9000);
    expect(rowsOf(second.container).scrollTop).toBe(800);
    second.unmount();
  });

  it("does not fight a reader who scrolls while the rows are still arriving", async () => {
    const range: Range = { content: 9000, visible: 400 };
    const first = render(<Column address="#/companies" range={range} />);
    readerScrollsTo(first.container, 3500);

    const second = leaveAndReturn(first, range, 975);
    // Mid-restore, and they take hold. What they choose stands, however much of
    // the list arrives afterwards.
    readerScrollsTo(second.container, 120);
    await rowsArrive(second.container, range, 9000);
    expect(rowsOf(second.container).scrollTop).toBe(120);
    second.unmount();
  });

  it("opens a list the reader has never seen at its first row", () => {
    const range: Range = { content: 9000, visible: 400 };
    const { container, unmount } = render(
      <Column address="#/deals" range={range} />,
    );
    expect(rowsOf(container).scrollTop).toBe(0);
    unmount();
  });

  it("keeps the rows' place apart from the page column's", () => {
    // Two lanes on one entry. Restoring a list's rows into the page column would
    // scroll the wrong box by the right amount, which is why the lane is part of
    // what an offset is filed under.
    const range: Range = { content: 9000, visible: 400 };
    const rows = render(<Column address="#/companies" range={range} />);
    readerScrollsTo(rows.container, 3500);

    function PageColumn({ address }: Readonly<{ address: string }>) {
      const column = useRef<HTMLDivElement>(null);
      useScrollMemory(column, address, "page");
      return <div ref={column} data-testid="page" />;
    }
    const page = render(<PageColumn address="#/companies" />);
    expect(
      page.container.querySelector<HTMLElement>('[data-testid="page"]')
        ?.scrollTop,
    ).toBe(0);
    rows.unmount();
    page.unmount();
  });
});
