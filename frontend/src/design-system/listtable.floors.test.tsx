/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { CellStrip, type ListColumn, ListTable } from "./listtable";
import { columns, type Row, testRows } from "./listtable.testkit";

afterEach(() => {
  cleanup();
  Reflect.deleteProperty(Range.prototype, "getBoundingClientRect");
});

// jsdom lays nothing out, so every text run is given the width of a long
// figure. happy-dom has no `rows` on a table section, which the walk reads.
const INK = 150;

function laidOutText(inkOf: (text: string) => number = () => INK) {
  Object.defineProperty(Range.prototype, "getBoundingClientRect", {
    configurable: true,
    value(this: Range): DOMRect {
      const width = inkOf(this.toString());
      return {
        ...{ x: 0, y: 0, top: 0, left: 0, width, height: 16 },
        ...{ right: width, bottom: 16, toJSON: () => ({}) },
      };
    },
  });
}

/** Ten pixels a character, so a long figure inks wider than a short one. */
const PER_CHARACTER = 10;

/** Drags the Value column's right edge from nothing out to `width`. */
function dragValueTo(container: HTMLElement, width: number) {
  const grip = container
    .querySelectorAll("thead th")[1]
    ?.querySelector(".lt-grip");
  if (!(grip instanceof HTMLElement)) {
    throw new Error("the Value column has no grip");
  }
  // jsdom has no pointer capture, so the grip's own calls need a stand-in.
  grip.setPointerCapture = () => undefined;
  grip.releasePointerCapture = () => undefined;
  fireEvent.pointerDown(grip, { clientX: 0, pointerId: 1 });
  fireEvent.pointerMove(grip, { clientX: width, pointerId: 1 });
  fireEvent.pointerUp(grip, { pointerId: 1 });
}

function table(rows: Row[], shown: readonly ListColumn<Row>[] = columns) {
  return (
    <StrictMode>
      <LocaleProvider initial="en">
        <ListTable
          rows={rows}
          pending={rows.length === 0}
          columns={shown}
          rowKey={(row) => row.id}
          unit="rows"
        />
      </LocaleProvider>
    </StrictMode>
  );
}

function widthOfColumn(container: HTMLElement, index: number) {
  return container.querySelectorAll("col")[index]?.style.width;
}

describe("a numeric column's floor", () => {
  // Strict Mode mounts effects twice, and the rows land after the first paint,
  // as they do on every screen that fetches them.
  it("takes the widest figure from rows that arrive after the table mounts", async () => {
    laidOutText();
    const { container, rerender } = render(table([]));
    rerender(table(testRows(3)));
    // Past the 110px numeric default: the ink, then the cell's own padding.
    await waitFor(() =>
      expect(
        Number.parseFloat(widthOfColumn(container, 1) ?? ""),
      ).toBeGreaterThan(INK),
    );
  });

  // A floor set a frame late paints the new page with its figures cut short.
  it("widens the column before the rows that need it can paint", async () => {
    laidOutText();
    const { container, rerender } = render(table([]));
    rerender(table(testRows(3)));
    // The mutation observer runs as a microtask, ahead of the next frame.
    await Promise.resolve();
    expect(
      Number.parseFloat(widthOfColumn(container, 1) ?? ""),
    ).toBeGreaterThan(INK);
  });

  it("lets a drag go under a figure that is no longer on screen", async () => {
    laidOutText((text) => text.length * PER_CHARACTER);
    const wide = { ...testRows(1)[0], value: 1_234_567_890_123_456 };
    const { container, rerender } = render(table([wide]));
    await waitFor(() =>
      expect(
        Number.parseFloat(widthOfColumn(container, 1) ?? ""),
      ).toBeGreaterThan(160),
    );
    rerender(table(testRows(3)));
    dragValueTo(container, 90);
    await waitFor(() => expect(widthOfColumn(container, 1)).toBe("90px"));
  });

  it("sets no floor from a cell that holds a strip", async () => {
    laidOutText();
    const strip: ListColumn<Row> = {
      key: "value",
      header: "Value",
      numeric: true,
      cell: (row) => (
        <CellStrip>
          <span>{String(row.value)}</span>
        </CellStrip>
      ),
    };
    const name: ListColumn<Row> = {
      key: "name",
      header: "Name",
      fixed: true,
      cell: (row) => row.name,
    };
    const { container } = render(table(testRows(3), [name, strip]));
    await waitFor(() => expect(widthOfColumn(container, 1)).toBe("110px"));
  });
});
