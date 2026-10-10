// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { columnLayout, type SizedColumn } from "./listtable.layout";

const catalogue: SizedColumn[] = [
  { key: "name", fixed: true },
  { key: "price", numeric: true },
  { key: "billing" },
  { key: "status" },
  { key: "actions", verbs: "menu" },
];

const px = (width: string | undefined) => Number.parseFloat(width ?? "NaN");

describe("columnLayout", () => {
  it("gives a row menu its narrow fixed width", () => {
    const layout = columnLayout(catalogue, {}, 707);
    expect(layout.widthOf(catalogue[4])).toBe("56px");
  });

  it("fits a settings column when one share falls under its minimum", () => {
    const layout = columnLayout(catalogue, {}, 707);
    expect(layout.floor).toBeCloseTo(707);
    expect(px(layout.widthOf(catalogue[1]))).toBe(110);
    expect(px(layout.widthOf(catalogue[0]))).toBeGreaterThan(200);
  });

  it("holds every column at its minimum when the box has no room for them", () => {
    const layout = columnLayout(catalogue, {}, 400);
    expect(catalogue.map((column) => px(layout.widthOf(column)))).toEqual([
      200, 110, 130, 130, 56,
    ]);
    expect(layout.floor).toBe(626);
  });

  it("keeps a dragged width and lets the shares divide the rest", () => {
    const layout = columnLayout(catalogue, { name: 300 }, 1000);
    expect(layout.widthOf(catalogue[0])).toBe("300px");
    expect(layout.floor).toBeCloseTo(1000);
  });
});
