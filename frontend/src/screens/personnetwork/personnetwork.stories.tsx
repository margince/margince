// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../story-utils";
import { PersonNetworkTab } from "./index";

// The contact's network, in the readings a rep actually meets.
//
// The tab is a decision surface before it is a picture, so these stories are
// ordered as that decision degrades: a warm direct route, an indirect one, a
// choice between several, a route already asked for, and then the cases where
// the page has less to say than it looks — withheld groups, a truncated graph,
// and no way in at all.
//
// Each one is a state the layout has to survive, and several of them were
// wrong on this page before: the map drew one lane when the target's colleagues
// were put in the wrong column, and a withheld group read as a company nobody
// knows rather than one this reader may not see.

type PersonGraph = components["schemas"]["PersonGraph"];
type IntroRequest = components["schemas"]["IntroRequest"];
type RouteCandidate = components["schemas"]["PersonGraphRouteCandidate"];

const PERSON = "018f3a1b-0000-7000-8000-000000000010";
const SOFIA = "018f3a1b-0000-7000-8000-000000000021";
const MARTIN = "018f3a1b-0000-7000-8000-000000000022";
const PHILIPP = "018f3a1b-0000-7000-8000-000000000031";

// The anchor is the contact this graph is about, and the tab reads its label
// for every sentence that names them.
const anchor: PersonGraph["nodes"][number] = {
  id: `person:${PERSON}`,
  type: "contact",
  group: "anchor",
  label: "Dana Buyer",
  sublabel: "Head of Operations, Northgate",
  person_id: PERSON,
};

const sofia: PersonGraph["nodes"][number] = {
  id: `user:${SOFIA}`,
  type: "colleague",
  group: "direct",
  label: "Sofia Meier",
  sublabel: "Account Executive",
  user_id: SOFIA,
};

const martin: PersonGraph["nodes"][number] = {
  id: `user:${MARTIN}`,
  type: "colleague",
  group: "direct",
  label: "Martin Weber",
  sublabel: "Solutions Engineer",
  user_id: MARTIN,
};

// Somebody at the contact's own company, which is what an indirect route runs
// through.
const philipp: PersonGraph["nodes"][number] = {
  id: `person:${PHILIPP}`,
  type: "contact",
  group: "account",
  label: "Philipp Königs",
  sublabel: "Finance Director, Northgate",
  person_id: PHILIPP,
};

function directRoute(over: Partial<RouteCandidate> = {}): RouteCandidate {
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

function graph(over: Partial<PersonGraph> = {}): PersonGraph {
  return {
    person_id: PERSON,
    nodes: [anchor, sofia],
    edges: [
      {
        from: `user:${SOFIA}`,
        to: `person:${PERSON}`,
        strength_bucket: "strong",
        interactions_90d: 14,
        inbound_90d: 6,
        outbound_90d: 8,
        last_at: "2026-08-28T09:00:00Z",
      },
    ],
    routes: [directRoute()],
    groups_omitted: [],
    ...over,
  };
}

// The tab reads two endpoints, and a story that routed only one would render
// the other's loading state under a name claiming something else.
function stub(payload: PersonGraph, asks: IntroRequest[] = []) {
  installFetchStub({
    "GET /me": meRoute({ person: ["read"], introduction: ["read", "create"] }),
    "GET /people/{id}/graph": () => jsonResponse(payload),
    [`GET /people/${PERSON}/graph`]: () => jsonResponse(payload),
    [`GET /people/${PERSON}/intro-requests`]: () => jsonResponse(asks),
  });
}

function render(payload: PersonGraph, asks: IntroRequest[] = []) {
  stub(payload, asks);
  return (
    <StoryProviders>
      <PersonNetworkTab personId={PERSON} />
    </StoryProviders>
  );
}

const meta: Meta<typeof PersonNetworkTab> = {
  title: "Records/Person/NetworkTab",
  component: PersonNetworkTab,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof PersonNetworkTab>;

// The case the page is designed around: one colleague who genuinely knows the
// contact, and a rep who can act on it without reading anything else.
export const StrongDirectRoute: Story = {
  render: () => render(graph()),
};

// Nobody here knows the contact, but a colleague knows somebody at their
// company. The lead panel has to say what the route actually is — a hand-off
// through a third person is a different favour from an introduction.
export const IndirectOnly: Story = {
  render: () =>
    render(
      graph({
        nodes: [anchor, sofia, philipp],
        edges: [
          {
            from: `user:${SOFIA}`,
            to: `person:${PHILIPP}`,
            strength_bucket: "moderate",
            interactions_90d: 6,
            last_at: "2026-08-15T09:00:00Z",
          },
        ],
        routes: [
          directRoute({
            route_id: `through:${SOFIA}:${PHILIPP}`,
            route_type: "through_contact",
            through_person_id: PHILIPP,
            through_display_name: "Philipp Königs",
            strength_bucket: "moderate",
            evidence: {
              interactions_90d: 6,
              two_way: true,
              last_at: "2026-08-15T09:00:00Z",
              days_since_last: 17,
            },
          }),
        ],
      }),
    ),
};

// Two ways in. The lead is drawn once at the top and the alternatives listed
// below it — never the same route twice, which would ask the reader which of
// two identical lines is the recommendation.
export const TwoAlternatives: Story = {
  render: () =>
    render(
      graph({
        nodes: [anchor, sofia, martin],
        edges: [
          {
            from: `user:${SOFIA}`,
            to: `person:${PERSON}`,
            strength_bucket: "strong",
            interactions_90d: 14,
            last_at: "2026-08-28T09:00:00Z",
          },
          {
            from: `user:${MARTIN}`,
            to: `person:${PERSON}`,
            strength_bucket: "weak",
            interactions_90d: 2,
            last_at: "2026-06-02T09:00:00Z",
          },
        ],
        routes: [
          directRoute(),
          directRoute({
            route_id: `direct:${MARTIN}`,
            via_user_id: MARTIN,
            via_display_name: "Martin Weber",
            strength_bucket: "weak",
            evidence: {
              interactions_90d: 2,
              two_way: false,
              last_at: "2026-06-02T09:00:00Z",
              days_since_last: 91,
            },
          }),
        ],
      }),
    ),
};

// The route is real and cannot be asked for, because it already was. The panel
// says so instead of offering a button that would 409 — a rep pressing it and
// being told "already requested" learned nothing they could not have been told
// first.
export const AlreadyAsked: Story = {
  render: () =>
    render(
      graph({ routes: [directRoute({ availability: "already_requested" })] }),
      [
        {
          id: "018f3a1b-0000-7000-8000-0000000000a1",
          person_id: PERSON,
          requester_user_id: "018f3a1b-0000-7000-8000-000000000001",
          introducer_user_id: SOFIA,
          introducer_display_name: "Sofia Meier",
          route_type: "direct",
          status: "requested",
          internal_reason:
            "Dana reopened the retrofit conversation after 41 days.",
          name_drop_allowed: false,
          note_generated_by: "human",
          note_ai_generated: false,
          fallback_policy: "none",
          requested_at: "2026-08-30T08:00:00Z",
          due_at: "2026-09-06T08:00:00Z",
          version: 1,
        },
      ],
    ),
};

// A colleague said "use my name" and the rep did. This is the state the product
// most has to keep separate from `introduced`: permission to mention somebody
// is not a handshake, and a page that collapsed them would tell a rep a door
// had been opened that nobody opened.
export const NameDropped: Story = {
  render: () =>
    render(
      graph({ routes: [directRoute({ availability: "already_requested" })] }),
      [
        {
          id: "018f3a1b-0000-7000-8000-0000000000a2",
          person_id: PERSON,
          requester_user_id: "018f3a1b-0000-7000-8000-000000000001",
          introducer_user_id: SOFIA,
          introducer_display_name: "Sofia Meier",
          route_type: "direct",
          status: "name_dropped",
          internal_reason:
            "Dana reopened the retrofit conversation after 41 days.",
          name_drop_allowed: true,
          note_generated_by: "model",
          note_ai_generated: true,
          fallback_policy: "none",
          requested_at: "2026-08-24T08:00:00Z",
          due_at: "2026-08-31T08:00:00Z",
          name_dropped_at: "2026-08-29T14:00:00Z",
          version: 4,
        },
      ],
    ),
};

// A group withheld for lack of a grant, said in the product's one spelling of
// that fact. Before this was drawn, an account this reader may not see looked
// exactly like a company nobody here knows — the opposite conclusion.
export const AccountWithheld: Story = {
  render: () =>
    render(
      graph({
        groups_omitted: ["account"],
      }),
    ),
};

// The graph hit its caps. The count is stated rather than the list silently
// cut: a picture that quietly drops people is one a reader trusts for a
// completeness it does not have.
export const Truncated: Story = {
  render: () =>
    render(
      graph({
        nodes: [anchor, sofia, martin, philipp],
        dropped_count: { direct: 3, account: 12, peer: 5 },
      }),
    ),
};

// Nobody here has a way in. The honest empty state, and the one a rep needs to
// see quickly rather than after scrolling a picture of people who cannot help.
export const NoRoute: Story = {
  render: () =>
    render(
      graph({
        nodes: [anchor],
        edges: [],
        routes: [],
      }),
    ),
};

// The narrow width, where the two columns stack rather than crush. The map is
// 744px wide and scrolls inside its own box — it must not push the page
// sideways, which is what it did when it sat inside a half-width column.
export const Narrow390: Story = {
  parameters: { viewport: { defaultViewport: "mobile1" } },
  render: () => render(graph()),
};
