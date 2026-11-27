// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { addMinor, sumMinor } from "./minorsum";

describe("sumMinor", () => {
  it("adds amounts that stay inside the exact range", () => {
    expect(sumMinor([Number.MAX_SAFE_INTEGER - 1, 1])).toBe(
      Number.MAX_SAFE_INTEGER,
    );
    expect(sumMinor([])).toBe(0);
  });

  it("gives no total once the sum passes the exact range either way", () => {
    expect(sumMinor([2 ** 52, 2 ** 52])).toBeUndefined();
    expect(sumMinor([-(2 ** 52), -(2 ** 52)])).toBeUndefined();
  });

  it("stays without a total once it has none, whatever is added after", () => {
    expect(addMinor(undefined, -1)).toBeUndefined();
  });
});
