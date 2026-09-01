/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import { RelationshipMap, type RelationshipMapLabels } from "./relationshipmap";
import type { RelationshipMapModel } from "./relationshipmap.layout";

afterEach(cleanup);

const LABELS: RelationshipMapLabels = {
  region: "Relationship map",
  band: { strong: "strong", developing: "developing", cold: "cold" },
  bestRoute: "Best route",
  alternatives: "Alternatives",
  noRoute: "No route recorded",
  laneMore: (hidden) => `Show ${hidden} more`,
  clearFocus: "Clear",
  emptyTitle: "No route recorded yet",
  emptyBody: "Assign the buying roles or import interactions",
  nothingSelected: "Select a person to see the best route",
};

function model(over: Partial<RelationshipMapModel> = {}): RelationshipMapModel {
  return {
    nodes: [
      { id: "u-1", kind: "user", label: "Sofia Meier" },
      { id: "u-2", kind: "user", label: "Lars Meyer" },
      { id: "d-1", kind: "deal", label: "Retrofit 2026" },
      {
        id: "p-1",
        kind: "person",
        label: "Philipp Königs",
        sublabel: "CFO",
        engagement: "untried",
        engagementLabel: "Not approached",
        actions: [{ id: "write", label: "Write to Philipp", primary: true }],
      },
      {
        id: "p-2",
        kind: "person",
        label: "Ute Sommer",
        engagement: "answered",
        engagementLabel: "Answered",
      },
      { id: "gap:champion", kind: "gap", label: "Champion missing" },
    ],
    lanes: [
      {
        id: "users",
        column: "left",
        label: "Our side",
        nodeIds: ["u-1", "u-2"],
      },
      { id: "centre", column: "center", label: "", nodeIds: ["d-1"] },
      {
        id: "champion",
        column: "right",
        label: "Champion",
        nodeIds: ["gap:champion"],
      },
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

function draw(over: Partial<Parameters<typeof RelationshipMap>[0]> = {}) {
  const onFocus = vi.fn();
  render(
    <RelationshipMap
      model={model()}
      focusId={null}
      onFocus={onFocus}
      completenessText="Showing 5 of 105 contacts · selected deal only."
      labels={LABELS}
      {...over}
    />,
  );
  return { onFocus };
}

// Every node says who it is, what it is and where it stands — in its own
// accessible name, because a reader who cannot see the drawing gets the facts
// rather than the picture.
test("names every node with its lane and its engagement", () => {
  draw();
  const philipp = screen.getByRole("button", {
    name: /Philipp Königs, CFO, Economic buyer, Not approached/,
  });
  expect(philipp).not.toBeNull();
});

test("draws a gap as a node rather than a sentence somewhere else", () => {
  draw();
  expect(
    screen.getByRole("button", { name: /Champion missing, Champion/ }),
  ).not.toBeNull();
});

test("selecting a person fades what the route does not touch", () => {
  draw({ focusId: "p-1" });
  const ute = screen.getByRole("button", { name: /Ute Sommer/ });
  expect(ute.getAttribute("data-faded")).toBe("true");
  // Both colleagues who can reach Philipp stay lit; only one route is drawn.
  expect(
    screen
      .getByRole("button", { name: /Sofia Meier/ })
      .getAttribute("data-faded"),
  ).toBeNull();
  expect(
    screen
      .getByRole("button", { name: /Philipp Königs/ })
      .getAttribute("aria-pressed"),
  ).toBe("true");
});

test("the panel says in words which route was chosen and what was not", () => {
  draw({ focusId: "p-1" });
  expect(screen.getByText(/Sofia Meier → Philipp Königs/)).not.toBeNull();
  expect(screen.getByText(/developing/)).not.toBeNull();
  // The rejected route is listed, so a reader can disagree with the pick.
  expect(screen.getByText(/Lars Meyer · cold/)).not.toBeNull();
});

test("a person with no route says so rather than showing an empty panel", () => {
  render(
    <RelationshipMap
      model={model({ edges: [] })}
      focusId="p-1"
      onFocus={() => {}}
      completenessText=""
      labels={LABELS}
    />,
  );
  expect(screen.getByText("No route recorded")).not.toBeNull();
});

test("pressing a node focuses it, and pressing it again clears", async () => {
  const user = userEvent.setup();
  const { onFocus } = draw();
  await user.click(screen.getByRole("button", { name: /Philipp Königs/ }));
  expect(onFocus).toHaveBeenCalledWith("p-1");

  cleanup();
  const second = draw({ focusId: "p-1" });
  await user.click(screen.getByRole("button", { name: /Philipp Königs/ }));
  expect(second.onFocus).toHaveBeenCalledWith(null);
});

test("offers the focused node's own actions", async () => {
  const user = userEvent.setup();
  const onAction = vi.fn();
  draw({ focusId: "p-1", onAction });
  await user.click(screen.getByRole("button", { name: "Write to Philipp" }));
  expect(onAction).toHaveBeenCalledWith("p-1", "write");
});

test("states its scope, so a capped picture cannot read as the whole account", () => {
  draw();
  expect(
    screen.getByText("Showing 5 of 105 contacts · selected deal only."),
  ).not.toBeNull();
});

test("holds exactly one tab stop and walks with the arrow keys", async () => {
  const user = userEvent.setup();
  draw();
  const tabbable = screen
    .getAllByRole("button")
    .filter((node) => node.getAttribute("tabindex") === "0");
  expect(tabbable).toHaveLength(1);

  await user.tab();
  const first = document.activeElement?.getAttribute("data-node-id");
  await user.keyboard("{ArrowDown}");
  // FOCUS moves, not just the tabindex. Moving the attribute alone leaves the
  // ring, the screen reader and Enter all pointing at the node the reader
  // walked away from — the arrows look like they work and act on the wrong
  // person.
  expect(document.activeElement?.getAttribute("data-node-id")).not.toBe(first);
  expect(document.activeElement?.getAttribute("tabindex")).toBe("0");
});

// A selection set from outside must not pin the cursor to it: the effect that
// followed the prop ran on every render and snapped the cursor back, so a
// reader could never walk away from whatever was selected.
test("walks away from a node the caller selected", async () => {
  const user = userEvent.setup();
  draw({ focusId: "p-1" });
  await user.tab();
  const start = document.activeElement?.getAttribute("data-node-id");
  await user.keyboard("{ArrowDown}");
  expect(document.activeElement?.getAttribute("data-node-id")).not.toBe(start);
});

// Expanding a lane replaces the row the reader is standing on with the people
// it was hiding. Focus has to land on one of them rather than falling out of
// the map.
test("keeps the reader inside the map when a lane opens", async () => {
  const user = userEvent.setup();
  const many = Array.from({ length: 11 }, (_, i) => `x-${i}`);
  render(
    <RelationshipMap
      model={{
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
      }}
      focusId={null}
      onFocus={() => {}}
      completenessText=""
      labels={LABELS}
    />,
  );
  const more = screen.getByRole("button", { name: "Show 3 more" });
  await user.click(more);
  expect(document.activeElement?.getAttribute("data-node-id")).toBe("x-8");
});

// A focus the model no longer holds must not push the whole account back.
test("does not fade the map for a selection that is not there", () => {
  draw({ focusId: "p-gone" });
  for (const node of screen.getAllByRole("button")) {
    expect(node.getAttribute("data-faded")).toBeNull();
  }
});

test("reads a colleague's route in the direction they asked", () => {
  draw({ focusId: "u-1" });
  // Not "Sofia Meier → Sofia Meier": the reader selected our side, so the
  // other end is the person they can reach.
  const line = document.querySelector(".rmap-panel-line")?.textContent ?? "";
  expect(line).toContain("Sofia Meier → Philipp Königs");
  expect(line).not.toContain("Sofia Meier → Sofia Meier");
});

test("Escape clears the focus", async () => {
  const user = userEvent.setup();
  const { onFocus } = draw({ focusId: "p-1" });
  await user.tab();
  await user.keyboard("{Escape}");
  expect(onFocus).toHaveBeenCalledWith(null);
});

// Every edge is aria-hidden: the same facts reach a reader through the node
// names and the panel, and a screen reader walking twenty invisible lines
// would be reading the picture rather than the account.
test("hides the edges from assistive technology", () => {
  const { container } = render(
    <RelationshipMap
      model={model()}
      focusId={null}
      onFocus={() => {}}
      completenessText=""
      labels={LABELS}
    />,
  );
  const edges = container.querySelectorAll(".rmap-edge");
  expect(edges.length).toBeGreaterThan(0);
  // Hidden as one LAYER rather than a hidden attribute per line: a <g> is not
  // focusable in any engine, so the intent is unambiguous, and every edge is
  // inside it.
  for (const edge of edges) {
    expect(edge.closest("[aria-hidden='true']")).not.toBeNull();
  }
});

test("stops animating when the reader asks for less motion", () => {
  const { container } = render(
    <RelationshipMap
      model={model()}
      focusId={null}
      onFocus={() => {}}
      completenessText=""
      labels={LABELS}
      reducedMotion
    />,
  );
  expect(container.querySelector(".rmap")?.getAttribute("data-motion")).toBe(
    "none",
  );
});

test("an account with nothing recorded says what to do about it", () => {
  render(
    <RelationshipMap
      model={{ nodes: [], lanes: [], edges: [] }}
      focusId={null}
      onFocus={() => {}}
      completenessText=""
      labels={LABELS}
    />,
  );
  expect(screen.getByText("No route recorded yet")).not.toBeNull();
});
