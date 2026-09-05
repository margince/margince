// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { meFixture } from "../../app/mefixture";
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
const RUI = "018f3a1b-0000-7000-8000-000000000041";
// The reader themselves, which is the id `meFixture` answers /me with. The
// story is ABOUT the two matching, so it is read from the session fixture
// rather than typed a second time here.
const READER = meFixture({}).user.id;

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

// The same colleague as the server draws her when her only edge is to somebody
// at the contact's company: `account`, not `direct`. The group is assigned from
// which edge put her in the graph (persongraphaccount.go), so an indirect-only
// story that left her `direct` would put her in the wrong lane of the map and
// describe a payload the server cannot produce.
const sofiaOnAccount: PersonGraph["nodes"][number] = {
  ...sofia,
  group: "account",
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

// Somebody the contact is observed corresponding with who is neither ours nor
// at their company. `suggest_edge` is set on peer nodes and on no other kind:
// the pair keeps appearing on the same threads and no `works_with` edge exists
// yet, so the map's panel offers to record one.
const rui: PersonGraph["nodes"][number] = {
  id: `person:${RUI}`,
  type: "contact",
  group: "peer",
  label: "Rui Almeida",
  sublabel: "Programme Lead, Meridian",
  person_id: RUI,
  suggest_edge: true,
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
    // The messages the counts were read from. Only a direct route carries
    // them, and the verdict panel lists them beside its figures.
    receipts: [
      {
        activity_id: "018f3a1b-0000-7000-8000-0000000000e1",
        subject: "Re: Q4 rollout — site list",
        occurred_at: "2026-08-28T09:00:00Z",
        kind: "email",
      },
      {
        activity_id: "018f3a1b-0000-7000-8000-0000000000e2",
        subject: "Northgate pilot: next steps",
        occurred_at: "2026-08-21T14:30:00Z",
        kind: "meeting",
      },
    ],
    ...over,
  };
}

// The server sets `route` and `routes` together — `route` is `routes[0]`
// (persongraph.go) — so a fixture carrying one without the other describes a
// payload nothing produces. `over` may still replace both.
function graph(over: Partial<PersonGraph> = {}): PersonGraph {
  const routes = over.routes ?? [directRoute()];
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
    groups_omitted: [],
    ...over,
    routes,
    route: routes[0]
      ? {
          via_user_id: routes[0].via_user_id,
          via_display_name: routes[0].via_display_name,
          why: "Carried beside the candidate list, as the server carries it.",
        }
      : undefined,
  };
}

// The tab reads two endpoints, and a story that routed only one would render
// the other's loading state under a name claiming something else.
function stub(
  payload: PersonGraph,
  asks: IntroRequest[] = [],
  allow: Parameters<typeof meRoute>[0] = {
    person: ["read"],
    introduction: ["read", "create"],
  },
) {
  installFetchStub({
    "GET /me": meRoute(allow),
    "GET /people/{id}/graph": () => jsonResponse(payload),
    [`GET /people/${PERSON}/graph`]: () => jsonResponse(payload),
    [`GET /people/${PERSON}/intro-requests`]: () => jsonResponse(asks),
  });
}

type Moments = Pick<
  components["schemas"]["Person360"],
  "relationship_changes" | "sections_omitted"
>;

function render(
  payload: PersonGraph,
  asks: IntroRequest[] = [],
  view?: Moments,
  allow?: Parameters<typeof stub>[2],
) {
  stub(payload, asks, allow);
  return (
    <StoryProviders>
      <PersonNetworkTab personId={PERSON} view={view} />
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

// THE READER IS THE WAY IN.
//
// Nothing excludes the person reading from the colleagues the server ranks, so
// on a contact they correspond with themselves the warmest route is their own
// — and every sentence here is otherwise written about somebody to go and ask.
// This is the story that shows the second person: the verdict, the chain, the
// split legend and the strip all address the reader, and the ask is replaced by
// what to do instead, because the server refuses an introduction whose
// introducer is the person requesting it.
export const TheReadersOwnRoute: Story = {
  render: () =>
    render(
      graph({
        nodes: [
          anchor,
          {
            id: `user:${READER}`,
            type: "colleague",
            group: "direct",
            label: "Demo Admin",
            sublabel: "Workspace owner",
            user_id: READER,
          },
        ],
        edges: [
          {
            from: `user:${READER}`,
            to: `person:${PERSON}`,
            strength_bucket: "strong",
            interactions_90d: 12,
            inbound_90d: 7,
            outbound_90d: 5,
            last_at: "2026-08-27T09:00:00Z",
          },
        ],
        routes: [
          directRoute({
            route_id: `direct:${READER}`,
            via_user_id: READER,
            via_display_name: "Demo Admin",
          }),
        ],
      }),
    ),
};

// Nobody here knows the contact, but a colleague knows somebody at their
// company. The lead panel has to say what the route actually is — a hand-off
// through a third person is a different favour from an introduction.
export const IndirectOnly: Story = {
  render: () =>
    render(
      graph({
        nodes: [anchor, sofiaOnAccount, philipp],
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
//
// NOT REACHABLE TODAY, and the story says so rather than implying otherwise:
// chooseRoutes stamps every candidate `available`, because the availability
// seam its own comment describes was never bound (issue #3520). So the panel
// below is correct and the server cannot yet ask it for this. The story earns
// its place by holding the rendering ready — and by being the thing that
// noticed.
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

// A colleague said "use my name" and the rep did. Reachable — the ASK carries
// this status from the introductions module; only the route's `availability`
// beside it is the unbound seam the story above describes.
//
// This is the state the product most has to keep separate from `introduced`: permission to mention somebody
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
//
// The numbers are the REMAINDER past each cap, not a total, so a fixture has to
// show a full lane to be describing the state it names. The server's caps are
// ten direct and twelve account, and this draws two of each with a plausible
// remainder behind them — an earlier version claimed twelve account contacts
// dropped while drawing one, which is a payload nothing produces.
export const Truncated: Story = {
  render: () =>
    render(
      graph({
        nodes: [anchor, sofia, martin, philipp],
        dropped_count: { direct: 4, account: 7 },
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

// The same page in the dark theme. Its own story because the design system
// defines both palettes and only a render says whether this page reads in each
// — a token used in one and missing in the other is invisible in the source.
export const RoutedDark: Story = {
  name: "Strong direct route — dark",
  globals: { theme: "dark" },
  render: () => render(graph()),
};

// What moved lately, from the 360. The card is ABSENT rather than empty when no
// view is passed — the older contacts screen holds none — so this is the only
// story that draws it.
export const WithMoments: Story = {
  render: () =>
    render(graph(), [], {
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
    }),
};

// The peer lane. These are the only nodes the server flags `suggest_edge` on,
// and recording the acquaintance is offered from the selected node's detail —
// so a picture with no peer in it is a picture from which `works_with` cannot
// be written at all.
export const ObservedPeers: Story = {
  render: () =>
    render(
      graph({
        nodes: [anchor, sofia, rui],
        edges: [
          {
            from: `user:${SOFIA}`,
            to: `person:${PERSON}`,
            strength_bucket: "strong",
            interactions_90d: 14,
            last_at: "2026-08-28T09:00:00Z",
          },
          {
            from: `person:${PERSON}`,
            to: `person:${RUI}`,
            strength_bucket: "moderate",
            interactions_90d: 6,
            last_at: "2026-08-20T09:00:00Z",
          },
        ],
      }),
      [],
      undefined,
      {
        person: ["read"],
        introduction: ["read", "create"],
        relationship: ["create"],
      },
    ),
};

// The same picture for a seat that may not write a relationship. The peer is
// still placed and still readable — who a contact talks to is not the thing
// being withheld — and the offer to record it is simply not made.
export const ObservedPeersNoWrite: Story = {
  render: () =>
    render(
      graph({
        nodes: [anchor, sofia, rui],
        edges: [
          {
            from: `person:${PERSON}`,
            to: `person:${RUI}`,
            strength_bucket: "moderate",
            interactions_90d: 6,
            last_at: "2026-08-20T09:00:00Z",
          },
        ],
      }),
    ),
};
