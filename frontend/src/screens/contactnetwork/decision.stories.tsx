// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { meFixture } from "../../app/mefixture";
import type { IntroRequest } from "../introrequests";
import { installFetchStub, meRoute, StoryProviders } from "../story-utils";
import { DecisionStrip } from "./decision";
import "../contactnetwork.css";

// The three readings the network tab answers with: how many ways in there are,
// what moved on the relationship lately, and where the ask has got to.
//
// Every slot here states a fact and addresses nobody. The reader is ranked
// among the routes like any colleague, so they are COUNTED with the rest and
// named on a line of their own — the shapes this gallery exists to hold are
// the ones that used to say "Only you" and "You and 2 colleagues".
//
// Dates are fixed rather than relative: `make fe-clock-drift` runs the suite
// at +200 days and must reach the same verdict.

type RouteCandidate = components["schemas"]["ContactGraphRouteCandidate"];
type RelationshipChange = components["schemas"]["ContactRelationshipChange"];

const CONTACT = "018f3a1b-0000-7000-8000-000000000010";
const SOFIA = "018f3a1b-0000-7000-8000-000000000021";
const LENA = "018f3a1b-0000-7000-8000-000000000022";
// The reader themselves, read off the session fixture the stub answers with:
// the "own relationship among them" line is about the two matching, so typing
// the id a second time here would let the story and the page disagree.
const READER = meFixture({}).user.id;

function route(over: Partial<RouteCandidate> = {}): RouteCandidate {
  return {
    route_id: `direct:${SOFIA}`,
    route_type: "direct",
    via_user_id: SOFIA,
    via_display_name: "Sofia Meier",
    strength_bucket: "strong",
    evidence: { interactions_90d: 6, two_way: true },
    availability: "available",
    ...over,
  };
}

const viaContact = route({
  route_id: "through:1",
  route_type: "through_contact",
  via_user_id: LENA,
  via_display_name: "Lena Hoff",
  through_contact_id: "018f3a1b-0000-7000-8000-0000000000b1",
  through_display_name: "Philipp Königs",
  strength_bucket: "moderate",
});

const ownRoute = route({
  route_id: `direct:${READER}`,
  via_user_id: READER,
  via_display_name: "Test User",
});

function ask(over: Partial<IntroRequest> = {}): IntroRequest {
  return {
    id: "018f3a1b-0000-7000-8000-0000000000a1",
    contact_id: CONTACT,
    requester_user_id: READER,
    requester_display_name: "Test User",
    introducer_user_id: SOFIA,
    introducer_display_name: "Sofia Meier",
    route_type: "direct",
    status: "requested",
    internal_reason: "Dana reopened the retrofit conversation after 41 days.",
    name_drop_allowed: false,
    note_generated_by: "human",
    note_ai_generated: false,
    fallback_policy: "none",
    requested_at: "2026-08-30T08:00:00Z",
    due_at: "2026-09-06T08:00:00Z",
    version: 1,
    ...over,
  };
}

const meta: Meta<typeof DecisionStrip> = {
  title: "Records/Contact/Decision strip",
  component: DecisionStrip,
  parameters: { layout: "padded" },
  beforeEach: () => {
    // The strip asks the session who the reader is, for the route that is
    // theirs and the ask whose requester the payload leaves unnamed.
    installFetchStub({ "GET /me": meRoute({ contact: ["read"] }) });
  },
};
export default meta;
type Story = StoryObj<typeof DecisionStrip>;

function strip(props: Partial<Parameters<typeof DecisionStrip>[0]> = {}) {
  return () => (
    <StoryProviders>
      <DecisionStrip
        routes={[]}
        legacyVia={undefined}
        change={undefined}
        changeWithheld={false}
        open={undefined}
        {...props}
      />
    </StoryProviders>
  );
}

const repliedAfterGap: RelationshipChange = {
  kind: "replied_after_gap",
  at: "2026-08-20T09:00:00Z",
  days: 41,
};

/** Several colleagues reach them, one of them through a contact at the
 *  account. The count is the reading; the mix under it is what it rests on. */
export const SeveralWaysIn: Story = {
  render: strip({
    routes: [route(), viaContact, route({ route_id: "direct:2" })],
    change: repliedAfterGap,
    open: ask(),
  }),
};

/** The reader is one of the ways in. They are counted with the rest, and
 *  their own relationship takes a second line rather than rewriting the
 *  value as an address. */
export const ReaderIsAWayIn: Story = {
  render: strip({ routes: [ownRoute, route()], change: repliedAfterGap }),
};

/** Nobody corresponds with them. "None" is the reading, and the sentence
 *  under it is why there is none — the value used to be the sentence. */
export const NoWayIn: Story = {
  render: strip({ change: { kind: "went_quiet", at: "2026-08-01", days: 62 } }),
};

/** A band move: the two bands ARE the detail, because "the relationship moved"
 *  without saying from what to what is a claim a reader has to take on trust. */
export const RelationshipWarmed: Story = {
  render: strip({
    routes: [route()],
    change: {
      kind: "warmed",
      at: "2026-08-25T09:00:00Z",
      from_bucket: "weak",
      to_bucket: "strong",
    },
    open: ask({ status: "accepted", decided_at: "2026-08-31T09:00:00Z" }),
  }),
};

/** The changes section was refused. A slot that said "Nothing new" here would
 *  be reporting a relationship as unmoved out of a permission boundary. */
export const ChangesWithheld: Story = {
  render: strip({ routes: [route()], changeWithheld: true, open: ask() }),
};

/** Nothing asked for and nothing moved: three readings that all have an
 *  honest absence to state, and none of them a blank. */
export const NothingYet: Story = { render: strip({ routes: [route()] }) };

/** An ask that ran out of time. The status is the reading and the reason sits
 *  under it, rather than the reason standing in for the status. */
export const AskExpired: Story = {
  render: strip({
    routes: [route(), viaContact],
    change: repliedAfterGap,
    open: ask({ status: "expired", decided_at: "2026-09-06T08:00:00Z" }),
  }),
};

/** At 390px the plate folds to full-width rows, where the two-line detail on
 *  the first slot has the least room. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: strip({
    routes: [ownRoute, route(), viaContact],
    change: repliedAfterGap,
    open: ask(),
  }),
};

/** The same three readings in the dark theme. */
export const SeveralWaysInDark: Story = {
  globals: { theme: "dark" },
  render: strip({
    routes: [route(), viaContact],
    change: repliedAfterGap,
    open: ask(),
  }),
};
