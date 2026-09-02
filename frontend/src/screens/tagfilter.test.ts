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

  // An unknown mode is DROPPED, not defaulted. The server refuses one for the
  // same reason: `any` is not a superset of `none`, so a typo answered as
  // `any` returns a different slice rather than a wider one, and a filter that
  // quietly changes what it selects is worse than one that fails.
  it("drops a mode it does not recognise rather than choosing one", () => {
    expect(parseTagMode("all")).toBe("all");
    expect(parseTagMode("none")).toBe("none");
    expect(parseTagMode("banana")).toBeUndefined();
    expect(parseTagMode(undefined)).toBeUndefined();
  });
});

describe("what the tag filter sends", () => {
  it("sends the ids and the mode together", () => {
    expect(tagQueryParams(["a", "b"], "all")).toEqual({
      tag_id: ["a", "b"],
      tag_mode: "all",
    });
  });

  // No mode at all rather than one this tier invented: the endpoint's own
  // default is `any`, so omitting it says the same thing without deciding what
  // an unrecognised mode meant.
  it("sends the ids alone when the address named no mode it knows", () => {
    expect(tagQueryParams(["a"], undefined)).toEqual({ tag_id: ["a"] });
  });

  // A mode with nothing to combine is not a filter, so sending it would put a
  // dial in the request that changes nothing.
  it("sends nothing at all when no tag is selected", () => {
    expect(tagQueryParams([], "none")).toEqual({});
  });
});
