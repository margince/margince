// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { walkedTo } from "./suggestlist";

const ROWS = [{}, {}, {}];

describe("walkedTo", () => {
  it("walks from an edge when the index is past the rows", () => {
    expect(walkedTo(5, "ArrowDown", ROWS)).toBe(0);
    expect(walkedTo(5, "ArrowUp", ROWS)).toBe(2);
  });

  it("names no row when the rows are gone", () => {
    expect(walkedTo(2, "ArrowDown", [])).toBe(-1);
  });

  it("stays on the last row rather than wrapping", () => {
    expect(walkedTo(2, "ArrowDown", ROWS)).toBe(2);
  });
});
