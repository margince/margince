// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { useT } from "../../i18n";
import { availabilityLabel } from "../contactroutes";
import { installFetchStub, meRoute, StoryProviders } from "../story-utils";
import { LeadPanel } from "./leadpanel";
import "../contactnetwork.css";

// The page's one recommendation, drawn alone so its badges can be read side by
// side: the route's standing in the head band, and the two facts under the
// chain. Soft throughout: the head badge is success while the ask is open and
// neutral once it is not, and a fact turns warn only for a one-sided route.
//
// Dates are fixed rather than relative: `make fe-clock-drift` runs the suite at
// +200 days and must reach the same verdict.

type RouteCandidate = components["schemas"]["ContactGraphRouteCandidate"];

const SOFIA = "018f3a1b-0000-7000-8000-000000000021";
const PHILIPP = "018f3a1b-0000-7000-8000-000000000031";

function route(over: Partial<RouteCandidate> = {}): RouteCandidate {
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
    receipts: [
      {
        activity_id: "018f3a1b-0000-7000-8000-0000000000e1",
        subject: "Re: Q4 rollout — site list",
        occurred_at: "2026-08-28T09:00:00Z",
        kind: "email",
      },
    ],
    ...over,
  };
}

// The panel is handed the blocked sentence by its tab, which reads it off the
// route's availability; the story asks the same helper so the label is the one
// the tab would draw.
function Lead({ candidate }: Readonly<{ candidate: RouteCandidate }>) {
  const t = useT();
  return (
    <LeadPanel
      route={candidate}
      targetName="Dana Buyer"
      blocked={availabilityLabel(candidate.availability, t)}
      onAsk={() => undefined}
    />
  );
}

function draw(candidate: RouteCandidate) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        contact: ["read"],
        introduction: ["read", "create"],
      }),
    });
    return (
      <StoryProviders>
        <Lead candidate={candidate} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof LeadPanel> = {
  title: "Records/Contact network/Recommended route",
  component: LeadPanel,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof LeadPanel>;

/** A warm, reciprocal direct route: the head badge in success, both facts
 *  neutral, and the ask under the chain. */
export const StrongDirect: Story = { render: draw(route()) };

/** Only one side writes. The fact badge turns warn, because "they already
 *  write to each other" would be the claim the counts contradict. */
export const OneSided: Story = {
  render: draw(
    route({
      strength_bucket: "moderate",
      evidence: {
        interactions_90d: 9,
        inbound_90d: 0,
        outbound_90d: 9,
        two_way: false,
        last_at: "2026-08-20T09:00:00Z",
        days_since_last: 12,
      },
    }),
  ),
};

/** Through somebody at the contact's company: no receipts, and the second
 *  fact names the hand-off. */
export const ThroughAContact: Story = {
  render: draw(
    route({
      route_id: `through:${SOFIA}:${PHILIPP}`,
      route_type: "through_contact",
      through_contact_id: PHILIPP,
      through_display_name: "Philipp Königs",
      receipts: undefined,
    }),
  ),
};

/** Already asked for. The head badge carries the reason in the neutral tone and
 *  the button is gone, because the server would refuse a second ask. */
export const AlreadyRequested: Story = {
  render: draw(route({ availability: "already_requested" })),
};

export const StrongDirectDark: Story = {
  globals: { theme: "dark" },
  render: draw(route()),
};
