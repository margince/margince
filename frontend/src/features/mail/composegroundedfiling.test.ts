// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { groundedFiling } from "./composesaveddraftcalls";

// What a restored draft files under.
//
// An agent draft records the records it was written from. Without reading them
// back, the composer rebuilds links from whichever page opened it.
//
// A draft grounded in a deal then sends under the contact alone, and is
// missing from the deal's timeline.
describe("the filing a restored draft starts from", () => {
  it("takes the deal and project the draft was written against", () => {
    expect(
      groundedFiling([
        { type: "contact", id: "c-1" },
        { type: "deal", id: "d-1" },
        { type: "project", id: "p-1" },
      ]),
    ).toEqual({ dealId: "d-1", projectId: "p-1" });
  });

  // The contact comes from the recipient, and an activity is what a reply
  // answers rather than a filing the composer can set.
  it("ignores the kinds a composer does not file under", () => {
    expect(
      groundedFiling([
        { type: "contact", id: "c-1" },
        { type: "activity", id: "a-1" },
        { type: "company", id: "co-1" },
        { type: "lead", id: "l-1" },
      ]),
    ).toEqual({ dealId: "", projectId: "" });
  });

  // A human draft carries none, and the field is absent rather than empty on
  // the way out, so both shapes answer the same way.
  it("answers an empty filing for a draft with no grounding", () => {
    expect(groundedFiling([])).toEqual({ dealId: "", projectId: "" });
    expect(groundedFiling(undefined)).toEqual({ dealId: "", projectId: "" });
  });

  // The composer holds one deal and one project, so a draft naming two of a
  // kind keeps the first rather than the last.
  it("keeps the first of a kind, because the composer holds one", () => {
    expect(
      groundedFiling([
        { type: "deal", id: "first" },
        { type: "deal", id: "second" },
      ]),
    ).toEqual({ dealId: "first", projectId: "" });
  });
});
