// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, test } from "vitest";
import {
  COL_W,
  GUTTER,
  LANE_CAP,
  layout,
  NODE_H,
  PAD,
  type RelationshipMapModel,
  routeFor,
  travelOrder,
  truncate,
  WIDTH,
} from "./relationshipmap.layout";

function model(over: Partial<RelationshipMapModel> = {}): RelationshipMapModel {
  return {
    nodes: [
      { id: "u-1", kind: "user", label: "Sofia Meier" },
      { id: "u-2", kind: "user", label: "Lars Meyer" },
      { id: "o-1", kind: "organization", label: "Brandt GmbH" },
      { id: "d-1", kind: "deal", label: "Retrofit 2026" },
      { id: "p-1", kind: "person", label: "Philipp Königs" },
      { id: "p-2", kind: "person", label: "Ute Sommer" },
    ],
    lanes: [
      {
        id: "users",
        column: "left",
        label: "Our side",
        nodeIds: ["u-1", "u-2"],
      },
      { id: "centre", column: "center", label: "", nodeIds: ["o-1", "d-1"] },
      {
        id: "economic_buyer",
        column: "right",
        label: "Economic buyer",
        nodeIds: ["p-1", "p-2"],
      },
    ],
    edges: [
      {
        id: "e-1",
        from: "u-1",
        to: "p-1",
        kind: "route",
        band: "developing",
        lastAt: "2026-08-20T09:00:00Z",
        words: "awaiting reply",
      },
      {
        id: "e-2",
        from: "u-2",
        to: "p-1",
        kind: "route",
        band: "cold",
        lastAt: "2026-08-29T09:00:00Z",
        words: "never written to",
      },
      {
        id: "m-1",
        from: "p-1",
        to: "d-1",
        kind: "membership",
        words: "on the deal",
      },
    ],
    ...over,
  };
}

// The same model must draw the same picture on every read: a reader learns
// where to look, and a layout that moved would make them learn again.
test("places the same model at the same coordinates every time", () => {
  const first = layout(model());
  const second = layout(model());
  expect(second).toEqual(first);
});

// The lanes carry the order. Shuffling the node ARRAY must change nothing,
// because a caller's node list is a bag and its lane list is the drawing.
test("takes its order from the lanes, not the node array", () => {
  const ordered = layout(model());
  const shuffled = layout(model({ nodes: [...model().nodes].reverse() }));
  expect(shuffled.placed).toEqual(ordered.placed);
});

test("puts each column where the constants say", () => {
  const placed = layout(model()).placed;
  const at = (id: string) => placed.find((node) => node.id === id);
  expect(at("u-1")?.x).toBe(PAD);
  expect(at("o-1")?.x).toBe(PAD + COL_W.left + GUTTER);
  expect(at("p-1")?.x).toBe(PAD + COL_W.left + GUTTER + COL_W.center + GUTTER);
  expect(layout(model()).width).toBe(WIDTH);
});

// A lane longer than the cap draws what fits and offers the rest, rather than
// growing without limit or silently dropping people.
test("caps a long lane and offers the remainder", () => {
  const many = Array.from({ length: LANE_CAP + 6 }, (_, i) => `p-${i}`);
  const long = model({
    nodes: many.map((id) => ({ id, kind: "person" as const, label: id })),
    lanes: [
      {
        id: "influencer",
        column: "right",
        label: "Influencers",
        nodeIds: many,
      },
    ],
    edges: [],
  });
  const capped = layout(long);
  expect(capped.placed.filter((n) => n.kind === "person")).toHaveLength(
    LANE_CAP,
  );
  expect(capped.placed.some((n) => n.id === "more:influencer")).toBe(true);
  expect(capped.heads.find((h) => h.id === "influencer")?.hidden).toBe(6);

  const opened = layout(long, new Set(["influencer"]));
  expect(opened.placed.filter((n) => n.kind === "person")).toHaveLength(
    LANE_CAP + 6,
  );
  expect(opened.placed.some((n) => n.id === "more:influencer")).toBe(false);
});

test("stacks a lane by each node's own height", () => {
  const placed = layout(model()).placed;
  const first = placed.find((n) => n.id === "p-1");
  const second = placed.find((n) => n.id === "p-2");
  expect((second?.y ?? 0) - (first?.y ?? 0)).toBe(NODE_H.person + 8);
});

// The strongest band wins, even when a weaker edge is more recent — a reader
// choosing whom to ask wants the best relationship, not the latest message.
test("lights the strongest route rather than the most recent", () => {
  // The weaker edge is listed FIRST and is more recent, so a pick that walked
  // the array or preferred recency would take it. Only a pick that reads the
  // band lands on the developing one.
  const ordered = model({
    edges: [
      {
        id: "e-cold",
        from: "u-2",
        to: "p-1",
        kind: "route",
        band: "cold",
        lastAt: "2026-08-29T09:00:00Z",
        words: "never written to",
      },
      {
        id: "e-warm",
        from: "u-1",
        to: "p-1",
        kind: "route",
        band: "developing",
        lastAt: "2026-08-20T09:00:00Z",
        words: "awaiting reply",
      },
    ],
  });
  const { route } = routeFor(ordered, "p-1");
  expect(route?.edgeIds).toContain("e-warm");
  expect(route?.edgeIds).not.toContain("e-cold");
});

test("breaks a band tie by the most recent exchange", () => {
  const tied = model({
    edges: [
      {
        id: "e-1",
        from: "u-1",
        to: "p-1",
        kind: "route",
        band: "cold",
        lastAt: "2026-01-01T00:00:00Z",
        words: "old",
      },
      {
        id: "e-2",
        from: "u-2",
        to: "p-1",
        kind: "route",
        band: "cold",
        lastAt: "2026-08-01T00:00:00Z",
        words: "newer",
      },
    ],
  });
  expect(routeFor(tied, "p-1").route?.edgeIds).toContain("e-2");
});

test("breaks a full tie by edge id, so the same model lights the same path", () => {
  const tied = model({
    edges: [
      {
        id: "e-b",
        from: "u-1",
        to: "p-1",
        kind: "route",
        band: "cold",
        words: "",
      },
      {
        id: "e-a",
        from: "u-2",
        to: "p-1",
        kind: "route",
        band: "cold",
        words: "",
      },
    ],
  });
  expect(routeFor(tied, "p-1").route?.edgeIds).toEqual(["e-a"]);
});

test("carries the route on to the deal the contact sits on", () => {
  const { route, related } = routeFor(model(), "p-1");
  expect(route?.nodeIds).toEqual(["u-1", "p-1", "d-1"]);
  expect(related.has("d-1")).toBe(true);
  // Both colleagues who can reach them stay lit, even though only one route
  // is drawn: the alternatives are what the panel lists.
  expect(related.has("u-2")).toBe(true);
});

test("mirrors the walk when a colleague is focused", () => {
  const { route } = routeFor(model(), "u-1");
  expect(route?.nodeIds).toEqual(["u-1", "p-1", "d-1"]);
});

// A contact nobody has a route to still focuses: the map fades the rest and
// the panel says there is no route, which is the honest answer.
test("focuses a person with no route without inventing one", () => {
  const alone = model({ edges: [] });
  const { related, route } = routeFor(alone, "p-1");
  expect(route).toBeNull();
  expect(related.has("p-1")).toBe(true);
});

// A URL naming a contact this account no longer has must not blank the map.
test("treats an unknown focus as no focus", () => {
  const { related, route } = routeFor(model(), "p-gone");
  expect(route).toBeNull();
  expect(related.size).toBe(0);
});

test("walks lane by lane in the order the lanes were declared", () => {
  const order = travelOrder(layout(model()));
  expect(order.indexOf("u-1")).toBeLessThan(order.indexOf("p-1"));
  expect(order.indexOf("u-1")).toBeLessThan(order.indexOf("u-2"));
});

test("truncates a long label and leaves a short one alone", () => {
  expect(truncate("Ute Sommer")).toBe("Ute Sommer");
  expect(truncate("A name far longer than any node can draw")).toHaveLength(22);
  expect(truncate("A name far longer than any node can draw")).toMatch(/…$/);
});
