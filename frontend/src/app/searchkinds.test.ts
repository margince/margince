// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  groupSearchHits,
  SEARCH_GROUP_KEY,
  SEARCH_GROUP_ORDER,
  SEARCH_HIT_ORDER,
  searchEmailRoute,
  searchGroupType,
  searchHitDestination,
  searchHitHasCard,
  searchHitRoute,
} from "./searchkinds";

// Where a hit goes, asked of the one place that knows.

describe("searchHitRoute", () => {
  it("sends a record to its own 360", () => {
    expect(searchHitRoute("contact", "p1")).toEqual({
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
    expect(searchHitDestination({ type: "contact", id: "p1" }, "ada")).toEqual({
      screen: "contacts",
      id: "p1",
    });
  });
});

describe("groupSearchHits", () => {
  // The order is a LIST and the headings are a RECORD the compiler holds to the
  // contract's enum. A group the list forgot is a group whose hits are dropped
  // on the way to the screen — how project hits once went missing — so the
  // list is checked against the record rather than against a third copy.
  it("draws every group a heading exists for, once", () => {
    expect([...SEARCH_GROUP_ORDER].sort()).toEqual(
      Object.keys(SEARCH_GROUP_KEY).sort(),
    );
  });

  it("offers a pill for every type and none for an email, which is an activity", () => {
    expect(SEARCH_HIT_ORDER).not.toContain("email");
    expect(SEARCH_HIT_ORDER).toEqual(
      SEARCH_GROUP_ORDER.filter((group) => group !== "email"),
    );
  });

  it("files a message apart from the calls and notes beside it", () => {
    const groups = groupSearchHits([
      { type: "activity", id: "call" },
      { type: "activity", id: "mail", email_summary: { activity_id: "mail" } },
    ]);
    expect(groups.map(({ group }) => group)).toEqual(["email", "activity"]);
    expect(searchGroupType("email")).toBe("activity");
  });

  // Groups follow the display order whatever the scores said; inside a group
  // the server's ranking stands.
  it("puts records before mail and keeps each group in the server's order", () => {
    const groups = groupSearchHits([
      { type: "activity", id: "m1", email_summary: { activity_id: "m1" } },
      { type: "company", id: "o2" },
      { type: "company", id: "o1" },
    ]);
    expect(
      groups.map(({ group, hits }) => [group, hits.map((hit) => hit.id)]),
    ).toEqual([
      ["company", ["o2", "o1"]],
      ["email", ["m1"]],
    ]);
  });
});

describe("searchHitHasCard", () => {
  // The two records a chip stands for, and nothing else: a deal or a tag
  // drawn with a monogram would read as a contact or a company.
  it("draws a card for a contact and a company and for no other kind", () => {
    expect(SEARCH_HIT_ORDER.filter(searchHitHasCard)).toEqual([
      "contact",
      "company",
    ]);
  });
});
