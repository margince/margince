// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { NAV, RAIL_LESS_SCREENS, railTrail } from "./nav";
import { parseHash, routeHash } from "./router";

describe("the rail and a composed unit", () => {
  // The product's own destinations, and no group an installation can add to.
  // A unit had a group here once; the composed set is no longer an input to
  // this level at all, which is what the missing argument to railTrail says.
  it("carries only the groups the product names", () => {
    const [primary] = railTrail({ screen: "home" });
    expect(primary.groups.map((group) => group.headingKey)).toEqual([
      undefined,
      "nav.group.records",
      "nav.group.work",
      "nav.group.intelligence",
    ]);
  });

  // The regression the rail row used to prevent, now prevented differently: a
  // unit screen still has no row of its own, so without an answer here the rail
  // would mark nothing current and the page would read as if it sat outside the
  // app. Settings is where the unit is offered and where the reader came from.
  //
  // This is the DATA half only. `settings` is a string, and whether any element
  // on screen resolves it is a different question that reads identically from
  // here — the Settings door is what answers it, and shell.test.tsx asserts
  // that at render. Neither half is sufficient alone; this file has shipped the
  // half-truth before.
  it("marks Settings current on a unit's route", () => {
    const [primary] = railTrail({ screen: "ext", id: "notes" });
    expect(primary.activeId).toBe("settings");
  });

  // Including the malformed one: `#/ext` with no unit renders the not-found
  // surface, and a reader who typed it is still somewhere in Settings' world
  // rather than nowhere.
  it("marks Settings current on a unit route naming no unit", () => {
    const [primary] = railTrail({ screen: "ext" });
    expect(primary.activeId).toBe("settings");
  });

  // A fork's rows are keyed by the screen's KEY rather than by the `x` segment
  // — one segment names every one of them, so a row identified by the segment
  // could only ever be "some fork screen". This is the DATA half, like the unit
  // cases above it: whether an element resolves that id is shell.test.tsx's
  // question, and reads identically from here.
  it("marks a fork screen's own row current on its route", () => {
    const [primary] = railTrail({ screen: "x", id: "warranty" });
    expect(primary.activeId).toBe("warranty");
  });

  // `#/x` naming no screen resolves to nothing and marks nothing: unlike a unit
  // route, there is no section it belongs to — a fork's destinations are its
  // own, spread across the three groups, and the segment names none of them.
  it("marks no row for a fork route naming no screen", () => {
    const [primary] = railTrail({ screen: "x" });
    expect(primary.activeId).toBe("x");
    const rows = primary.groups.flatMap((group) =>
      group.items.map((i) => i.id),
    );
    expect(rows).not.toContain("x");
  });

  it("leaves the product's rows marking themselves", () => {
    const [primary] = railTrail({ screen: "deals" });
    expect(primary.activeId).toBe("deals");
  });
});

// A screen the product ships and this list does not name is reachable only by
// typing its hash, which is the same to a reader as a screen nobody built.
describe("the filter builder's address", () => {
  const workRows = () => {
    const [primary] = railTrail({ screen: "filters" });
    const work = primary.groups.find(
      (group) => group.headingKey === "nav.group.work",
    );
    return work?.items ?? [];
  };

  // With the work, not among the record types it slices: writing a filter is
  // an authoring surface somebody sits down at — it produces a dynamic list, a
  // saved view or an export — rather than a kind of record the workspace keeps.
  // Nothing on the screen aggregates either, so it is not Intelligence.
  it("stands among the work destinations", () => {
    expect(workRows().map((item) => item.id)).toContain("filters");
  });

  // The screen's own title, so the product has one name for this surface. A
  // `nav.*` key of its own would be a fourth spelling to keep in step.
  it("reuses the label the screen already prints", () => {
    expect(workRows().find((item) => item.id === "filters")?.labelKey).toBe(
      "filters.title",
    );
  });

  // The row is the page on the library, whichever group an address scrolls
  // to, and only leads to the page below it: a filter, an opened view, a Live
  // List's filter or one list. There the top bar's trail ends in that page and
  // claims it, so the row says `aria-current="true"` instead.
  it.each([
    [{ screen: "filters" }, false],
    [{ screen: "filters", id: "views" }, false],
    [{ screen: "filters", id: "lists" }, false],
    [{ screen: "filters", id: "companies" }, true],
    [{ screen: "filters", id: "contacts", id2: "v1" }, true],
    [{ screen: "filters", id: "list", id2: "L1" }, true],
    [{ screen: "lists", id: "L1" }, true],
  ] as const)(
    "marks itself current on %o, an ancestor: %s",
    (route, ancestor) => {
      const [primary] = railTrail(route);
      expect(primary.activeId).toBe("filters");
      expect(primary.ancestor).toBe(ancestor);
    },
  );
});

// The other half of the same rule, and why it is not simply "no row is ever an
// ancestor": a record IS a page below its list, the trail in the top bar ends in
// it and claims it, and two elements claiming `aria-current="page"` for
// different things is what the demotion exists to prevent.
describe("a row that leads to a record instead of being one", () => {
  it("steps back to an ancestor while the record is the page", () => {
    const [primary] = railTrail({ screen: "contacts", id: "p-anna" });
    expect(primary.activeId).toBe("contacts");
    expect(primary.ancestor).toBe(true);
  });

  it("is the page again on the list that record was opened from", () => {
    const [primary] = railTrail({ screen: "contacts" });
    expect(primary.ancestor).toBe(false);
  });
});

// A row's href is handed straight back to the router on the next hashchange, so
// a destination the router does not answer is a row that leads to the not-found
// page. The screen union both sides share makes that a compile error; this is
// the runtime half, because the type says the lists agree and this says the
// strings do.
describe("the destinations the rail names", () => {
  it("address only screens the router answers", () => {
    const destinations = [
      ...NAV.map((item) => item.screen),
      ...RAIL_LESS_SCREENS,
    ];
    expect(
      destinations.map((screen) => parseHash(routeHash({ screen })).screen),
    ).toEqual(destinations);
  });
});
