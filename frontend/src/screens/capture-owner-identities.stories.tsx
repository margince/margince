// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { OwnerIdentitiesCard } from "./capture-owner-identities";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// A seat's own other addresses. Unlike the exclusions card beside it there is no
// scope to tell apart — every row here is the reader's own, and a colleague's
// identities are never listed — so the states worth drawing are what the list
// looks like with entries and what it says when there are none.
const ALIAS = {
  id: "oi-1",
  kind: "address",
  value: "l.jankowfsky@privat.example",
  source: "user",
  created_at: "2026-09-01T09:00:00Z",
};
const OWN_DOMAIN = {
  id: "oi-2",
  kind: "domain",
  value: "privat.example",
  source: "user",
  created_at: "2026-09-01T09:05:00Z",
};

function story(identities: Record<string, unknown>[]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /capture/owner-identities": () => jsonResponse({ data: identities }),
    });
    return (
      <StoryProviders>
        <OwnerIdentitiesCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof OwnerIdentitiesCard> = {
  title: "Settings/Admin settings/Capture/Your other addresses",
  component: OwnerIdentitiesCard,
};
export default meta;
type Story = StoryObj<typeof OwnerIdentitiesCard>;

// Both kinds at once: one address, and a whole domain the same person reads.
// The kind is the row's answer, because "l.jankowfsky@privat.example" and
// "privat.example" bind very different amounts of mail and the row has to say
// which it is.
export const Populated: Story = { render: story([ALIAS, OWN_DOMAIN]) };

// Nothing declared, which is the ordinary state and has to read as a fact rather
// than as a list that failed to draw — "I have told it about no other addresses"
// is what a reader opens this card to confirm.
export const Empty: Story = { render: story([]) };
