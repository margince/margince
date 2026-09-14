// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { installFetchStub, meRoute, StoryProviders } from "../story-utils";
import { EdgeDetail } from "./edgedetail";
import "../contactnetwork.css";

// One selected colleague on the contact's map: the strength band leads, then
// the counts it was read from. A pair with no cited messages says the counts
// are all there is, rather than drawing an empty receipt list.

type ContactGraph = components["schemas"]["ContactGraph"];
type GraphNode = components["schemas"]["ContactGraphNode"];

const CONTACT = "018f3a1b-0000-7000-8000-000000000010";
const SOFIA = "018f3a1b-0000-7000-8000-000000000021";
const PEER = "018f3a1b-0000-7000-8000-000000000031";

const anchor: GraphNode = {
  id: `contact:${CONTACT}`,
  type: "contact",
  group: "anchor",
  label: "Dana Buyer",
  contact_id: CONTACT,
};

const graph: ContactGraph = {
  contact_id: CONTACT,
  nodes: [
    anchor,
    {
      id: `user:${SOFIA}`,
      type: "colleague",
      group: "direct",
      label: "Sofia Meier",
      sublabel: "Account Executive",
      user_id: SOFIA,
    },
  ],
  edges: [
    {
      from: `user:${SOFIA}`,
      to: `contact:${CONTACT}`,
      strength_bucket: "moderate",
      interactions_90d: 6,
      inbound_90d: 2,
      outbound_90d: 4,
    },
  ],
  groups_omitted: [],
};

const meta: Meta = {
  title: "Records/Contact network/Edge detail",
  parameters: { layout: "padded" },
};
export default meta;

export const ModerateCountsOnly: StoryObj = {
  render: () => (
    <StoryProviders>
      <EdgeDetail
        graph={graph}
        nodeId={`user:${SOFIA}`}
        anchorId={`contact:${CONTACT}`}
      />
    </StoryProviders>
  ),
};

// A peer the server flags `suggest_edge` on carries the detail's one write, and
// the button's label names the peer. The map's panel slot is 280px beside the
// drawing, so a long name is the case that has to stay inside the card: the
// label wraps rather than running past the card's edge or hiding whom the
// click records.
const withLongPeer: ContactGraph = {
  contact_id: CONTACT,
  nodes: [
    anchor,
    {
      id: `contact:${PEER}`,
      type: "contact",
      group: "peer",
      label: "Maximiliane von Hohenberg-Schwarzenfeld",
      sublabel: "Head of Procurement, Meridian Logistics",
      contact_id: PEER,
      suggest_edge: true,
    },
  ],
  edges: [
    {
      from: `contact:${CONTACT}`,
      to: `contact:${PEER}`,
      strength_bucket: "strong",
      interactions_90d: 14,
      inbound_90d: 6,
      outbound_90d: 8,
    },
  ],
  groups_omitted: [],
};

export const SuggestedPeerLongName: StoryObj = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read"], relationship: ["create"] }),
    });
    return (
      <StoryProviders>
        <div style={{ width: 280 }}>
          <EdgeDetail
            graph={withLongPeer}
            nodeId={`contact:${PEER}`}
            anchorId={`contact:${CONTACT}`}
          />
        </div>
      </StoryProviders>
    );
  },
};
