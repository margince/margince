/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, waitFor } from "@testing-library/react";
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

function laidOutText() {
  const box: DOMRect = {
    ...{ x: 0, y: 0, top: 0, left: 0, width: INK, height: 16 },
    ...{ right: INK, bottom: 16, toJSON: () => ({}) },
  };
  Object.defineProperty(Range.prototype, "getBoundingClientRect", {
    configurable: true,
    value: () => box,
  });
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
