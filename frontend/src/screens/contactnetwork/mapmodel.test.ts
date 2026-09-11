// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { type MapCopy, mapModelFromContactGraph } from "./mapmodel";

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

// Distinguishable words rather than the real catalogue: what a lane is CALLED
// is the caller's business, and asserting on translated copy would fail on a
// rewording that changed nothing about which nodes are drawn.
const COPY: MapCopy = {
  ourTeam: "lane:ours",
  theirCompany: "lane:theirs",
  peers: "lane:peers",
  target: "lane:target",
  useThisRoute: "use",
  withheldDirect: "withheld:direct",
  withheldAccount: "withheld:account",
  edgeDirect: (name) => `direct:${name}`,
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
    );

    expect(model.nodes.map((node) => node.id)).toContain(peer.id);
    expect(laneOf(model, "peers")?.nodeIds).toEqual([peer.id]);
    expect(laneOf(model, "peers")?.label).toBe(COPY.peers);
  });

  // A lane with nobody in it draws no head at all, so a contact observed with
  // nobody outside their company pays nothing for the lane existing.
  it("leaves the lane empty for a contact with no observed peers", () => {
    const model = mapModelFromContactGraph(graph(), COPY);

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
    );

    expect(model.edges.map((edge) => [edge.from, edge.to])).toEqual([
      [anchor.id, peer.id],
    ]);
  });
});
