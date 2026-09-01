// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { PersonNetworkTab } from "./personnetwork";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

/**
 * Who can open a door to this person, and through whom.
 *
 * The panel had no story, which is how its node list shipped spelling
 * `className="btn-ghost btn-small"` — a variant with no base `btn` beside it,
 * and a size class that exists in no stylesheet in this tree (the real one is
 * `.btn-sm`). Under Tailwind's preflight that is a naked native button: no
 * padding, no boundary, no hover, no focus ring. Its own docblock says the
 * nodes "are real buttons and selecting one drives a live detail region", and
 * they did work — they simply did not look like anything.
 */
const meta: Meta<typeof PersonNetworkTab> = {
  title: "Records/Person record/Relationship graph",
  component: PersonNetworkTab,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof PersonNetworkTab>;

const graph = {
  person_id: "p-1",
  nodes: [
    {
      id: "person:p-1",
      type: "contact",
      group: "anchor",
      label: "Dana Buyer",
      sublabel: "Head of Fleet",
    },
    {
      id: "user:u-1",
      type: "colleague",
      group: "direct",
      label: "Lars Brandt",
      sublabel: "six exchanges in 90 days",
    },
    {
      id: "user:u-2",
      type: "colleague",
      group: "direct",
      label: "Mara Vogel",
      sublabel: "one exchange in 90 days",
    },
    {
      id: "org:o-1",
      type: "organization",
      group: "second_degree",
      label: "Brandt Logistik",
    },
  ],
  edges: [
    {
      from: "user:u-1",
      to: "person:p-1",
      strength_bucket: "strong",
      interactions_90d: 6,
      inbound_90d: 3,
      outbound_90d: 3,
    },
    {
      from: "user:u-2",
      to: "person:p-1",
      strength_bucket: "weak",
      interactions_90d: 1,
      inbound_90d: 0,
      outbound_90d: 1,
    },
  ],
  groups_omitted: [],
  route: {
    via_user_id: "u-1",
    via_display_name: "Lars Brandt",
    why: "6 two-way exchanges in 90 days · last contact yesterday",
  },
  // The server sends both, and `route` is `routes[0]`. The fixture keeps that
  // true: a story where the card recommends one colleague while the list leads
  // with another models a payload the server cannot produce.
  routes: [
    {
      route_id: "direct:u-1",
      route_type: "direct",
      via_user_id: "u-1",
      via_display_name: "Lars Brandt",
      strength_bucket: "strong",
      evidence: {
        interactions_90d: 6,
        inbound_90d: 3,
        outbound_90d: 3,
        two_way: true,
        days_since_last: 1,
      },
      availability: "available",
    },
    {
      route_id: "direct:u-2",
      route_type: "direct",
      via_user_id: "u-2",
      via_display_name: "Mara Vogel",
      strength_bucket: "weak",
      evidence: {
        interactions_90d: 1,
        inbound_90d: 0,
        outbound_90d: 1,
        two_way: false,
        days_since_last: 12,
      },
      availability: "available",
    },
  ],
};

// The 360 the moments card reads. Only the fields it touches are set; the
// contract's own type keeps the fixture honest about their shapes.
const movedLately: Pick<
  components["schemas"]["Person360"],
  "relationship_changes" | "sections_omitted"
> = {
  relationship_changes: [
    { kind: "replied_after_gap", at: "2026-08-20T09:00:00Z", days: 41 },
    {
      kind: "warmed",
      at: "2026-08-25T09:00:00Z",
      from_bucket: "weak",
      to_bucket: "strong",
    },
  ],
  sections_omitted: [],
};

function stub(body: unknown) {
  installFetchStub({ "GET /people/p-1/graph": () => jsonResponse(body) });
}

/** The answer leads; the node list is the working underneath it. */
export const Routed: Story = {
  render: () => {
    stub(graph);
    return (
      <StoryProviders>
        <PersonNetworkTab personId="p-1" />
      </StoryProviders>
    );
  },
};

/**
 * A node selected. The pressed state is what says which door the detail region
 * below is describing, and it is carried by the button's own chrome — so a node
 * that draws no chrome cannot show it at all.
 */
export const NodeSelected: Story = {
  render: () => {
    stub(graph);
    return (
      <StoryProviders>
        <PersonNetworkTab personId="p-1" />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByRole("button", { name: /Lars/ }));
  },
};

/**
 * Nobody knows them yet. The panel says so rather than drawing an empty list,
 * because an empty list and a list that failed to render are the same shape.
 */
export const NoRoute: Story = {
  render: () => {
    stub({
      person_id: "p-1",
      nodes: [
        {
          id: "person:p-1",
          type: "contact",
          group: "anchor",
          label: "Dana Buyer",
        },
      ],
      edges: [],
      groups_omitted: [],
      route: null,
    });
    return (
      <StoryProviders>
        <PersonNetworkTab personId="p-1" />
      </StoryProviders>
    );
  },
};

/**
 * Dark, where the node buttons sit on the card's own ground and their boundary
 * is the only thing separating them from it.
 */
export const RoutedDark: Story = {
  name: "Routed — dark",
  globals: { theme: "dark" },
  render: () => {
    stub(graph);
    return (
      <StoryProviders>
        <PersonNetworkTab personId="p-1" />
      </StoryProviders>
    );
  },
};

/**
 * An arm the reader may not see. Withheld is not empty: the account group says
 * "you cannot see this" INSTEAD of "there is nobody", because a withheld arm
 * arrives with no nodes and the two sentences together state an absence the
 * server never claimed.
 */
export const AccountWithheld: Story = {
  render: () => {
    stub({
      ...graph,
      nodes: graph.nodes.filter((node) => node.group !== "account"),
      edges: graph.edges.filter((edge) => !edge.to.startsWith("person:p-2")),
      groups_omitted: ["account"],
    });
    return (
      <StoryProviders>
        <PersonNetworkTab personId="p-1" />
      </StoryProviders>
    );
  },
};

/**
 * Nodes were dropped. Silent truncation reads as "this is everyone", which is
 * the one thing a graph must never imply falsely.
 */
export const Truncated: Story = {
  render: () => {
    stub({ ...graph, dropped_count: { direct: 3, account: 12 } });
    return (
      <StoryProviders>
        <PersonNetworkTab personId="p-1" />
      </StoryProviders>
    );
  },
};

/**
 * With the 360 in hand the tab closes with what MOVED. Without it — the old
 * contacts screen — the card is absent rather than claiming nothing has.
 */
export const WithMoments: Story = {
  render: () => {
    stub(graph);
    return (
      <StoryProviders>
        <PersonNetworkTab personId="p-1" view={movedLately} />
      </StoryProviders>
    );
  },
};
