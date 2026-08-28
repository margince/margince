// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ConfirmDetailsScreen } from "./confirm";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The public, anonymous confirm-your-details page: no session, no rail, and a
// token in the URL as the whole capability. The order on screen is the point —
// the Art. 14 disclosure card first, the marketing ask second, and neither
// answer pre-selected — so the resting story is what the catalog reviews.
//
// Unknown, expired and already-spent tokens all read as absent (404), which is
// why the refused story routes a 404 rather than a message of its own.

type ConfirmDetails = components["schemas"]["ConfirmDetails"];

const TOKEN = "cfm-7f3a";
const DETAILS_ROUTE = `GET /public/confirm/${TOKEN}`;

const held: ConfirmDetails = {
  full_name: "Dana Buyer",
  title: "Head of Procurement",
  company: "Brandt Automotive GmbH",
  email: "dana.buyer@brandt.example",
  phone: "+49 30 555 0142",
  marketing_state: "unknown",
  provenance: [
    {
      field: "title",
      source: "Email signature",
      recorded_at: "2026-06-14",
    },
    { field: "phone", source: "Web form", recorded_at: "2026-05-02" },
  ],
};

const meta: Meta<typeof ConfirmDetailsScreen> = {
  title: "Signed out/Confirm your details",
  component: ConfirmDetailsScreen,
};
export default meta;

type Story = StoryObj<typeof ConfirmDetailsScreen>;

function page(routes: Parameters<typeof installFetchStub>[0]) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <ConfirmDetailsScreen token={TOKEN} />
      </StoryProviders>
    );
  };
}

const heldRoutes = { [DETAILS_ROUTE]: () => jsonResponse(held) };

// The card as the contact meets it: four correctable fields, the employer they
// cannot change here, and the ask underneath with neither answer chosen.
export const Held: Story = { render: page(heldRoutes) };

// Dark, on the same frame: the fields, the card and the page ground are three
// elevations a darker palette compresses toward each other.
export const HeldDark: Story = {
  globals: { theme: "dark" },
  render: page(heldRoutes),
};

// A token the server will not resolve. The page says the link cannot be used
// and nothing about WHY — unknown, expired and spent must read alike.
export const LinkRefused: Story = {
  render: page({
    [DETAILS_ROUTE]: () =>
      jsonResponse({ title: "Not found", status: 404, code: "not_found" }, 404),
  }),
};

// No token at all, which is the shape of a link that lost its query on the way
// — the screen refuses before it asks the server anything.
export const NoToken: Story = {
  render: () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <ConfirmDetailsScreen />
      </StoryProviders>
    );
  },
};
