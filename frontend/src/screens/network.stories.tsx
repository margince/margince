// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { PersonNetworkCard } from "./network";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The relationship-graph card (ADR-0078). It fetches its own data, so the
// shared fetch stub carries the fixture — chosen to show the reading that is
// easy to get wrong: a colleague on the record nobody has ever spoken to.
//
// The deal-coverage frames that used to sit beside these are in
// deal360/dealcommittee.stories.tsx, on the card that actually ships. Its
// `Withheld` and `Empty` are the pair to open when changing that surface: they
// differ only in `sections_omitted`, and a reviewer comparing the two frames is
// the check that no refactor collapses "we could not look" into "nothing is
// wrong".
const meta: Meta = {
  title: "Records/Network",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const network = {
  person_id: "p-1",
  colleagues: [
    {
      user_id: "u-1",
      display_name: "Anna Weber",
      strength: 81,
      strength_bucket: "strong",
      interactions_90d: 24,
      last_at: "2026-07-28T09:00:00Z",
    },
    {
      user_id: "u-2",
      display_name: "Jonas Bach",
      strength: 44,
      strength_bucket: "moderate",
      interactions_90d: 6,
      last_at: "2026-06-11T16:30:00Z",
    },
    // A colleague on the record with no exchange behind it: the band is `none`
    // and NO number is drawn, because never-spoken and gone-cold are different
    // facts and a zero renders them identically.
    {
      user_id: "u-3",
      display_name: "Mira Falk",
      strength: null,
      strength_bucket: "none",
      interactions_90d: 0,
      last_at: null,
    },
  ],
};

export const WhoKnowsThem: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /people/p-1/network": () => jsonResponse(network),
    });
    return (
      <StoryProviders>
        <PersonNetworkCard id="p-1" />
      </StoryProviders>
    );
  },
};

export const NobodyKnowsThem: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /people/p-1/network": () =>
        jsonResponse({ person_id: "p-1", colleagues: [] }),
    });
    return (
      <StoryProviders>
        <PersonNetworkCard id="p-1" />
      </StoryProviders>
    );
  },
};
