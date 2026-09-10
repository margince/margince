// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { RoutesCard } from "./personroutes";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The ways in to a contact, as a zone of the network tab.
//
// Three states carry the whole surface: the full list headed by the
// recommendation, the same list with its head drawn elsewhere (which retitles
// the pane and changes the sentence explaining the ranking), and a graph with
// no route at all. The last one is the state a reader meets most often on a
// cold contact, and the one a card is easiest to ship without.

type PersonGraph = components["schemas"]["PersonGraph"];
type RouteCandidate = components["schemas"]["PersonGraphRouteCandidate"];

const PERSON = "018f3a1b-0000-7000-8000-000000000010";
const SOFIA = "018f3a1b-0000-7000-8000-000000000021";
const MARTIN = "018f3a1b-0000-7000-8000-000000000022";

function route(over: Partial<RouteCandidate>): RouteCandidate {
  return {
    route_id: `direct:${SOFIA}`,
    route_type: "direct",
    via_user_id: SOFIA,
    via_display_name: "Sofia Meier",
    strength_bucket: "strong",
    availability: "available",
    evidence: {
      interactions_90d: 14,
      inbound_90d: 6,
      outbound_90d: 8,
      two_way: true,
      last_at: "2026-08-28T09:00:00Z",
      days_since_last: 4,
    },
    ...over,
  };
}

const routes: RouteCandidate[] = [
  route({}),
  route({
    route_id: `direct:${MARTIN}`,
    via_user_id: MARTIN,
    via_display_name: "Martin Weber",
    strength_bucket: "weak",
    availability: "declined",
    evidence: {
      interactions_90d: 3,
      inbound_90d: 0,
      outbound_90d: 3,
      two_way: false,
      last_at: "2026-06-02T09:00:00Z",
      days_since_last: 91,
    },
  }),
];

// The server sends `route` and `routes` together — `route` is `routes[0]` —
// so a fixture carrying one without the other describes a payload nothing
// produces.
function graph(candidates: RouteCandidate[]): PersonGraph {
  const lead = candidates[0];
  return {
    person_id: PERSON,
    nodes: [],
    edges: [],
    groups_omitted: [],
    routes: candidates,
    route: lead
      ? {
          via_user_id: lead.via_user_id,
          via_display_name: lead.via_display_name,
          why: "Carried beside the candidate list, as the server carries it.",
        }
      : undefined,
  };
}

// The card asks who the reader is, because a route THEY are cannot be asked
// for and is written in the second person. Without the session that question
// has no answer and every row reads as somebody else's.
function card(payload: PersonGraph, skipLead: boolean) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        person: ["read"],
        introduction: ["read", "create"],
      }),
    });
    return (
      <StoryProviders>
        <RoutesCard graph={payload} skipLead={skipLead} onAsk={() => {}} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof RoutesCard> = {
  title: "Records/Person network/Ways in",
  component: RoutesCard,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof RoutesCard>;

/** "Ways in": the whole list, headed by the server's recommendation. */
export const WaysIn: Story = { render: card(graph(routes), false) };

/**
 * "Other ways in": the caller has drawn the recommendation as its own lead, so
 * this pane holds the alternatives and says so in its title.
 */
export const OtherWaysIn: Story = { render: card(graph(routes), true) };

/** No route at all — the honest answer, not an empty list. */
export const NoWayIn: Story = { render: card(graph([]), false) };

/**
 * Dark, because the pane's ground, its head band and the strength meter's bars
 * are three surfaces whose separation moves with the theme.
 */
export const WaysInDark: Story = {
  name: "Ways in — dark",
  globals: { theme: "dark" },
  render: card(graph(routes), false),
};
