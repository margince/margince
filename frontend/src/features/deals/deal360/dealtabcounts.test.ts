// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { pageCount, tabFigure } from "./dealtabcounts";

describe("the count beside a deal tab", () => {
  it.each([
    ["the server's total", [1, 2], { has_more: true, total: 9 }, 9],
    ["the rows of a page that holds them all", [1, 2], { has_more: false }, 2],
    ["zero for an empty list", [], { has_more: false }, 0],
    [
      "no figure while rows are left behind",
      [1, 2],
      { has_more: true },
      undefined,
    ],
    ["no figure from a body without a page", [1, 2], undefined, undefined],
  ])("reads %s", (_case, rows, page, count) => {
    expect(pageCount(rows, page)).toBe(count);
  });
});

describe("the figure a deal tab shows", () => {
  it("draws none beside an empty tab", () => {
    expect(tabFigure(0)).toBeUndefined();
  });

  it("keeps a count it has", () => {
    expect(tabFigure(4)).toBe(4);
  });

  it("keeps an unknown count unknown", () => {
    expect(tabFigure(undefined)).toBeUndefined();
  });
});
