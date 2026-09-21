// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, within } from "storybook/test";
import type { components } from "../api/schema";
import { IntroDrawer } from "./introdrawer";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";
import "./contactnetwork.css";

// The requester's side of an introduction: pick the route, say why, write the
// note.
//
// Three fields because they are three different messages, and the drawer has
// to keep saying so — the reason and the value are read by the COLLEAGUE, the
// note is the only line a contact outside the company ever sees. A surface
// that let those three read as one form would put the internal case for the
// ask in front of the contact it is about.
//
// The route arrives as a prop, chosen on the routes list, so the two states
// worth a picture are "a route was chosen" and "there is none" — the second is
// what a cold contact actually shows, and it is the one where the verb has to
// stay refused.

type RouteCandidate = components["schemas"]["ContactGraphRouteCandidate"];

const CONTACT = "018f3a1b-0000-7000-8000-000000000010";
const SOFIA = "018f3a1b-0000-7000-8000-000000000021";

const route: RouteCandidate = {
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
};

// This surface is a Modal, portalled to document.body, so `#storybook-root`
// holds the preview decorator and nothing else however well the drawer renders
// — which leaves the render gate unable to tell a drawer that mounted from one
// that never did. So every frame drives a play that names what it expects: a
// rejecting play IS a failure the gate reports.
const meta: Meta<typeof IntroDrawer> = {
  title: "Records/Contact network/Intro ask",
  component: IntroDrawer,
  play: async () => {
    const drawer = within(await screen.findByRole("dialog"));
    await drawer.findByText("Why you are asking");
  },
};
export default meta;

type Story = StoryObj<typeof IntroDrawer>;

function drawer(chosen: RouteCandidate | undefined) {
  return () => {
    // The drawer only writes, and only when the reader presses the verb — but
    // the route line reads the session to decide whether the route goes via
    // the READER, and a session it cannot read would make every route somebody
    // else's. The grants are empty because nothing on this surface is gated on
    // one; who the reader is, is the whole question the route line asks.
    installFetchStub({ "GET /me": meRoute({}) });
    return (
      <StoryProviders>
        <IntroDrawer
          contactId={CONTACT}
          contactName="Dana Buyer"
          route={chosen}
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  };
}

/**
 * A colleague who corresponds with the contact directly, named at the top so
 * the reader can see whose favour they are about to ask before they argue for
 * it.
 */
export const AskingThroughAColleague: Story = { render: drawer(route) };

/**
 * The same drawer with no route behind it. The line says nobody here
 * corresponds with them yet, and the verb stays refused — an ask with no
 * introducer is one the server refuses too, and learning that here costs the
 * reader nothing.
 */
export const WithNoRouteToAskThrough: Story = { render: drawer(undefined) };
