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

const figures = (price: number) => ({
  held: { price },
  onScreen: { price },
});

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

  it("draws the identity column at 200px by default when the box has no room", () => {
    const layout = columnLayout(catalogue, {}, 0);
    expect(layout.widthOf(catalogue[0])).toBe("200px");
  });

  it("holds every column at its minimum when the box has no room for them", () => {
    const layout = columnLayout(catalogue, {}, 400);
    expect(catalogue.map((column) => px(layout.widthOf(column)))).toEqual([
      200, 110, 130, 130, 56,
    ]);
    expect(layout.floor).toBe(626);
  });

  it("gives the room past every minimum to the identity column first", () => {
    const layout = columnLayout(catalogue, {}, 806);
    expect(catalogue.map((column) => px(layout.widthOf(column)))).toEqual([
      380, 110, 130, 130, 56,
    ]);
    expect(layout.floor).toBeCloseTo(806);
  });

  it("lets the other columns grow by their shares once the identity column holds 400px", () => {
    const layout = columnLayout(catalogue, {}, 1000);
    expect(px(layout.widthOf(catalogue[0]))).toBe(400);
    expect(px(layout.widthOf(catalogue[2]))).toBeGreaterThan(130);
    expect(layout.floor).toBeCloseTo(1000);

    const wide = columnLayout(catalogue, {}, 1400);
    expect(px(wide.widthOf(catalogue[0]))).toBeGreaterThan(400);
  });

  it("splits the room past every minimum between two identity columns", () => {
    const pair: SizedColumn[] = [
      { key: "name", fixed: true },
      { key: "alias", fixed: true },
      { key: "status" },
    ];
    const layout = columnLayout(pair, {}, 630);
    expect(pair.map((column) => px(layout.widthOf(column)))).toEqual([
      250, 250, 130,
    ]);
  });

  it("splits only the room a measured figure leaves between two identity columns", () => {
    const pair: SizedColumn[] = [
      { key: "name", fixed: true, numeric: true },
      { key: "alias", fixed: true },
      { key: "status" },
    ];
    const floors = { held: { name: 350 }, onScreen: { name: 350 } };
    const layout = columnLayout(pair, {}, 730, floors);
    expect(px(layout.widthOf(pair[0]))).toBeGreaterThanOrEqual(350);
    expect(layout.floor).toBeCloseTo(730);
  });

  it("gives a dragged identity column no room ahead of the others", () => {
    const layout = columnLayout(catalogue, { name: 300 }, 806);
    expect(layout.widthOf(catalogue[0])).toBe("300px");
    expect(px(layout.widthOf(catalogue[2]))).toBeGreaterThan(130);
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

  it("keeps a score column at 80px on any page, and the rest take the width", () => {
    const leads: SizedColumn[] = [
      { key: "name", fixed: true },
      { key: "score", numeric: true, holds: "score" },
      { key: "status" },
    ];
    for (const available of [300, 900, 1800]) {
      const layout = columnLayout(leads, {}, available);
      expect(layout.widthOf(leads[1])).toBe("80px");
    }
    const wide = columnLayout(leads, {}, 900);
    expect(wide.floor).toBeCloseTo(900);
  });

  it("widens a score column to a figure wider than its score", () => {
    const score: SizedColumn = { key: "price", numeric: true, holds: "score" };
    const layout = columnLayout([score], {}, 900, figures(96));
    expect(layout.widthOf(score)).toBe("96px");
  });

  // "₫450,000,000,000" is wider than a numeric column's share; cut short, it
  // reads as a different number.
  it("never sets a numeric column narrower than the widest figure it holds", () => {
    const layout = columnLayout(catalogue, {}, 400, figures(150));
    expect(px(layout.widthOf(catalogue[1]))).toBe(150);
    expect(layout.floor).toBe(666);
  });

  it("lets a figure wider than the numeric cap widen its column past it", () => {
    const layout = columnLayout(catalogue, {}, 1800, figures(260));
    expect(px(layout.widthOf(catalogue[1]))).toBe(260);
    expect(layout.floor).toBeCloseTo(1800);
  });

  it("holds a dragged numeric column at its figure, not under it", () => {
    const layout = columnLayout(catalogue, { price: 90 }, 1000, figures(150));
    expect(layout.widthOf(catalogue[1])).toBe("150px");
  });

  // A search showed a wide figure and was cleared; the narrow page now on
  // screen does not hold the reader's drag at that figure.
  it("holds a dragged numeric column only at the figures on screen now", () => {
    const floors = { held: { price: 160 }, onScreen: { price: 80 } };
    const layout = columnLayout(catalogue, { price: 90 }, 1000, floors);
    expect(layout.widthOf(catalogue[1])).toBe("90px");
    expect(
      px(columnLayout(catalogue, {}, 400, floors).widthOf(catalogue[1])),
    ).toBe(160);
  });

  // A reader may drag any column under its default minimum; the cell then ends
  // its text in an ellipsis.
  it("keeps a width the reader set under the column's default minimum", () => {
    const layout = columnLayout(catalogue, { name: 100, price: 60 }, 1000);
    expect(layout.widthOf(catalogue[0])).toBe("100px");
    expect(layout.widthOf(catalogue[1])).toBe("60px");
  });
});
