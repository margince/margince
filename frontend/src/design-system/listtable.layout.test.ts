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

  const contacts: SizedColumn[] = [
    { key: "name", fixed: true },
    { key: "email" },
    { key: "owner", holds: "colleague" },
    { key: "lastActivity", holds: "date" },
    { key: "created", holds: "date" },
  ];

  it("gives a wide page's spare width to the text columns, not the dates", () => {
    const layout = columnLayout(contacts, {}, 1800);
    expect(contacts.map((column) => px(layout.widthOf(column)))).toEqual([
      expect.any(Number),
      expect.any(Number),
      240,
      150,
      150,
    ]);
    expect(px(layout.widthOf(contacts[1]))).toBeGreaterThan(300);
    expect(layout.floor).toBeCloseTo(1800);
    expect(layout.slack).toBe("0px");
  });

  it("leaves the trailing gap the width a lone date column cannot use", () => {
    const dragged = { name: 300, email: 200, owner: 250, lastActivity: 130 };
    const layout = columnLayout(contacts, dragged, 1800);
    expect(layout.widthOf(contacts[4])).toBe("150px");
    expect(layout.floor).toBe(1030);
    expect(layout.slack).toBeUndefined();
  });

  it("holds a date at its minimum on a narrow page and fits the row", () => {
    const layout = columnLayout(contacts, {}, 900);
    expect(px(layout.widthOf(contacts[3]))).toBe(120);
    expect(px(layout.widthOf(contacts[2]))).toBe(150);
    expect(layout.floor).toBeCloseTo(900);
  });

  it("keeps a dragged width and lets the shares divide the rest", () => {
    const layout = columnLayout(catalogue, { name: 300 }, 1000);
    expect(layout.widthOf(catalogue[0])).toBe("300px");
    expect(layout.floor).toBeCloseTo(1000);
  });

  // "₫450,000,000,000" is wider than a numeric column's share; cut short, it
  // reads as a different number.
  it("never sets a numeric column narrower than the widest figure it holds", () => {
    const layout = columnLayout(catalogue, {}, 400, { price: 150 });
    expect(px(layout.widthOf(catalogue[1]))).toBe(150);
    expect(layout.floor).toBe(666);
  });

  it("lets a figure wider than the numeric cap widen its column past it", () => {
    const layout = columnLayout(catalogue, {}, 1800, { price: 260 });
    expect(px(layout.widthOf(catalogue[1]))).toBe(260);
    expect(layout.floor).toBeCloseTo(1800);
  });

  it("holds a dragged numeric column at its figure, not under it", () => {
    const layout = columnLayout(catalogue, { price: 90 }, 1000, { price: 150 });
    expect(layout.widthOf(catalogue[1])).toBe("150px");
  });
});
