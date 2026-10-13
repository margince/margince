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

// A seat's own other email addresses: the empty state with its one verb, and
// the table with its Source column telling typed addresses from learned ones.
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
const LEARNED = {
  id: "oi-3",
  kind: "address",
  value: "lars@brandt-automotive.example",
  source: "delivered_to",
  created_at: "2026-09-02T07:30:00Z",
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
  title: "Settings/You/Connections/Your other email addresses",
  component: OwnerIdentitiesCard,
};
export default meta;
type Story = StoryObj<typeof OwnerIdentitiesCard>;

// Both kinds, and one address Margince found in the mailbox.
export const Populated: Story = {
  render: story([ALIAS, OWN_DOMAIN, LEARNED]),
};

export const PopulatedDark: Story = {
  globals: { theme: "dark" },
  render: story([ALIAS, OWN_DOMAIN, LEARNED]),
};

// The approved empty state: what the list is for, and Add address.
export const Empty: Story = { render: story([]) };

export const EmptyDark: Story = {
  globals: { theme: "dark" },
  render: story([]),
};
