// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";

import { listQueryParams } from "./tagfilter";

// The address holds several tag ids in ONE string, because ListQuery.filters is
// a flat map of strings. What reaches the endpoint has to be an array again, or
// the filter selects on a comma-joined value no tag has.

describe("what a list sends for its tag filter", () => {
  it("expands the joined ids back into an array", () => {
    expect(listQueryParams({ tag_id: "a,b", tag_mode: "all" })).toEqual({
      tag_id: ["a", "b"],
      tag_mode: "all",
    });
  });

  // Every other filter is already a wire param name and passes through
  // untouched — that is what lets a screen add a chip and get an addressable
  // filter for free.
  it("leaves every other filter exactly as it found it", () => {
    expect(listQueryParams({ owner_id: "u-1", stalled: "true" })).toEqual({
      owner_id: "u-1",
      stalled: "true",
    });
  });

  // A mode with nothing to combine is not a filter. Sending it alone would put
  // a dial on the request that changes nothing.
  it("sends no mode when no tag is selected", () => {
    expect(listQueryParams({ tag_mode: "none", owner_id: "u-1" })).toEqual({
      owner_id: "u-1",
    });
  });
});
