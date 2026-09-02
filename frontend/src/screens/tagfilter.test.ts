// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";

import { parseTagIDs, parseTagMode, tagQueryParams } from "./tagfilter";

// An address is text a person edits. Everything here is about what happens
// when what arrives is not what this code wrote.

describe("the tag filter's address", () => {
  it("carries several ids through one string", () => {
    expect(parseTagIDs("a,b,c")).toEqual(["a", "b", "c"]);
  });

  it("drops the blanks a hand-edited address leaves behind", () => {
    expect(parseTagIDs("a,,  ,b")).toEqual(["a", "b"]);
  });

  it("reads an absent filter as no ids rather than as one empty id", () => {
    expect(parseTagIDs(undefined)).toEqual([]);
    expect(parseTagIDs("")).toEqual([]);
  });

  // The widest of the three: a typo shows MORE rows rather than hiding some
  // behind a filter the reader never set.
  it("falls back to the widest mode on anything but the three", () => {
    expect(parseTagMode("all")).toBe("all");
    expect(parseTagMode("none")).toBe("none");
    expect(parseTagMode("banana")).toBe("any");
    expect(parseTagMode(undefined)).toBe("any");
  });
});

describe("what the tag filter sends", () => {
  it("sends the ids and the mode together", () => {
    expect(tagQueryParams(["a", "b"], "all")).toEqual({
      tag_id: ["a", "b"],
      tag_mode: "all",
    });
  });

  // A mode with nothing to combine is not a filter, so sending it would put a
  // dial in the request that changes nothing.
  it("sends nothing at all when no tag is selected", () => {
    expect(tagQueryParams([], "none")).toEqual({});
  });
});
