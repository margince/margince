// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  searchEmailRoute,
  searchHitDestination,
  searchHitRoute,
} from "./searchkinds";

// Where a hit goes, asked of the one place that knows.

describe("searchHitRoute", () => {
  it("sends a record to its own 360", () => {
    expect(searchHitRoute("person", "p1")).toEqual({
      screen: "contacts",
      id: "p1",
    });
  });

  it("sends a tag to its own page, which is the point of finding one", () => {
    expect(searchHitRoute("tag", "t1")).toEqual({ screen: "tags", id: "t1" });
  });

  // An activity is a link rather than a thing links hang off. Where it IS a
  // message, searchEmailRoute answers instead — a question about the hit, not
  // about its type, which is why this arm stays null.
  it("gives an activity no page of its own", () => {
    expect(searchHitRoute("activity", "a1")).toBeNull();
  });
});

describe("searchEmailRoute", () => {
  // The palette owns no page and every Command carries a route, so an email
  // hit used to drop out of the palette entirely. It now goes to the one page
  // that already owns this drawer.
  it("sends a message to the results, with itself open", () => {
    expect(searchEmailRoute("renewal", "a1")).toEqual({
      screen: "search",
      id: "renewal",
      id2: "a1",
    });
  });

  // The query rides along because the screen IS the results and they have to
  // be the results for something: landing on an empty search with a drawer
  // over it would give the reader nothing to go back to.
  it("keeps the query the reader typed", () => {
    expect(searchEmailRoute("Rennsteig terms", "a1").id).toBe(
      "Rennsteig terms",
    );
  });
});

// Where the HIT goes, as against where its TYPE lives — the difference #3850
// turns on.
describe("searchHitDestination", () => {
  it("gives an email hit a destination its type has none of", () => {
    expect(
      searchHitDestination(
        { type: "activity", id: "a1", email_summary: { activity_id: "a1" } },
        "renewal",
      ),
    ).toEqual({ screen: "search", id: "renewal", id2: "a1" });
  });

  // A call, a note, a task and a meeting are activities too, and the server
  // sends no summary for them. Branching on the FIELD rather than the kind
  // word is what keeps them out.
  it("still gives a non-email activity nowhere to go", () => {
    expect(
      searchHitDestination({ type: "activity", id: "a2" }, "renewal"),
    ).toBeNull();
  });

  it("leaves every other type on the destination it had", () => {
    expect(searchHitDestination({ type: "person", id: "p1" }, "ada")).toEqual({
      screen: "contacts",
      id: "p1",
    });
  });
});
