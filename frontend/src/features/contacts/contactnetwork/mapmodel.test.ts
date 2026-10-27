// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import {
  type MapCopy,
  mapModelFromContactGraph,
  WRITE_EMAIL_ACTION,
} from "./mapmodel";

// Which contacts the picture places, and under which head.
//
// The peer arm is the one worth pinning: a peer node is the only node the
// server flags `suggest_edge` on, the map's panel is the only place that
// offers to record the acquaintance, and the panel can only describe a node
// the layout placed. A peer the model drops therefore takes the product's one
// writer of `works_with` off every screen — and nothing about the graph read,
// the flag or the endpoint changes to say so.

type ContactGraph = components["schemas"]["ContactGraph"];

const ANCHOR = "018f3a1b-0000-7000-8000-000000000010";
const PEER = "018f3a1b-0000-7000-8000-000000000030";
const READER = "018f3a1b-0000-7000-8000-000000000040";
const COLLEAGUE = "018f3a1b-0000-7000-8000-000000000050";

// Nobody signed in that the graph names, which is every case but the reader's.
const NO_VIEWER = { userId: undefined, mayWrite: true };

// Distinguishable words rather than the real catalogue: what a lane is CALLED
// is the caller's business, and asserting on translated copy would fail on a
// rewording that changed nothing about which nodes are drawn.
const COPY: MapCopy = {
  ourTeam: "lane:ours",
  theirCompany: "lane:theirs",
  peers: "lane:peers",
  target: "lane:target",
  useThisRoute: "use",
  you: "you",
  writeEmail: "write",
  withheldDirect: "withheld:direct",
  withheldAccount: "withheld:account",
  edgeDirect: (name) => `direct:${name}`,
  edgeDirectYou: "direct:you",
  edgeAccount: (name) => `account:${name}`,
};

const anchor: ContactGraph["nodes"][number] = {
  id: `contact:${ANCHOR}`,
  type: "contact",
  group: "anchor",
  label: "Dana Buyer",
  contact_id: ANCHOR,
};

const peer: ContactGraph["nodes"][number] = {
  id: `contact:${PEER}`,
  type: "contact",
  group: "peer",
  label: "Rui Peer",
  contact_id: PEER,
  suggest_edge: true,
};

function graph(over: Partial<ContactGraph> = {}): ContactGraph {
  return {
    contact_id: ANCHOR,
    nodes: [anchor],
    edges: [],
    routes: [],
    groups_omitted: [],
    ...over,
  } as ContactGraph;
}

function laneOf(
  model: ReturnType<typeof mapModelFromContactGraph>,
  id: string,
) {
  return model.lanes.find((lane) => lane.id === id);
}

describe("the contacts around a contact", () => {
  it("places an observed peer, in its own lane", () => {
    const model = mapModelFromContactGraph(
      graph({ nodes: [anchor, peer] }),
      COPY,
      NO_VIEWER,
    );

    expect(model.nodes.map((node) => node.id)).toContain(peer.id);
    expect(laneOf(model, "peers")?.nodeIds).toEqual([peer.id]);
    expect(laneOf(model, "peers")?.label).toBe(COPY.peers);
  });

  // A lane with nobody in it draws no head at all, so a contact observed with
  // nobody outside their company pays nothing for the lane existing.
  it("leaves the lane empty for a contact with no observed peers", () => {
    const model = mapModelFromContactGraph(graph(), COPY, NO_VIEWER);

    expect(laneOf(model, "peers")?.nodeIds).toEqual([]);
  });

  // The edge the peer arm sends runs anchor → peer. Drawn only once both ends
  // are placed, which is exactly what a dropped peer node took away.
  it("draws the line the peer arm sent, now that both ends are placed", () => {
    const model = mapModelFromContactGraph(
      graph({
        nodes: [anchor, peer],
        edges: [
          {
            from: anchor.id,
            to: peer.id,
            strength_bucket: "strong",
            interactions_90d: 6,
          },
        ],
      }),
      COPY,
      NO_VIEWER,
    );

    expect(model.edges.map((edge) => [edge.from, edge.to])).toEqual([
      [anchor.id, peer.id],
    ]);
  });
});

describe("the reader's own node", () => {
  const colleague = (id: string, label: string) =>
    ({
      id: `user:${id}`,
      type: "colleague",
      group: "direct",
      label,
      user_id: id,
    }) satisfies ContactGraph["nodes"][number];
  const reader = colleague(READER, "Seat A");
  const other = colleague(COLLEAGUE, "Kim Colleague");
  const both = graph({
    nodes: [anchor, reader, other],
    edges: [
      {
        from: reader.id,
        to: anchor.id,
        strength_bucket: "weak",
        interactions_90d: 1,
      },
    ],
  });

  // Nobody asks themselves for an introduction: the reader is "You", and
  // their way in is the email they would write.
  it("is named You and offers to write rather than an introduction", () => {
    const model = mapModelFromContactGraph(both, COPY, {
      userId: READER,
      mayWrite: true,
    });
    const mine = model.nodes.find((node) => node.id === reader.id);
    const theirs = model.nodes.find((node) => node.id === other.id);

    expect(mine?.label).toBe("you");
    expect(mine?.actions?.map((action) => action.id)).toEqual([
      WRITE_EMAIL_ACTION,
    ]);
    expect(theirs?.label).toBe("Kim Colleague");
    expect(theirs?.actions?.map((action) => action.id)).toEqual(["use-route"]);
    expect(model.edges.map((edge) => edge.words)).toEqual(["direct:you"]);
  });

  it("offers nothing when the page refuses this reader mail", () => {
    const model = mapModelFromContactGraph(both, COPY, {
      userId: READER,
      mayWrite: false,
    });

    expect(model.nodes.find((node) => node.id === reader.id)?.actions).toEqual(
      [],
    );
  });
});
