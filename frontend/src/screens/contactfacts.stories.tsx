// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { archivedContact } from "./contactarchived.fixtures";
import { ContactFacts } from "./contactfacts";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The facts strip under a contact's name, read off the row alone. So the
// archived page, which has no 360, draws the same strip as the live one.
const meta: Meta = {
  title: "Records/Contact 360/Header facts",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const live = {
  ...archivedContact,
  archived_at: null,
  writable: true,
  social: { linkedin: "https://linkedin.com/in/danabuyer" },
  address: { city: "Munich", country: "DE" },
};

function Facts({
  contact,
  buyingRole,
}: Readonly<{ contact: typeof archivedContact; buyingRole?: string }>) {
  installFetchStub({ "GET /me": meRoute({ contact: ["read", "update"] }) });
  return (
    <StoryProviders>
      <ContactFacts contact={contact} buyingRole={buyingRole} />
    </StoryProviders>
  );
}

export const Live: Story = {
  render: () => <Facts contact={live} buyingRole="champion" />,
};

// No buying role without the 360, and the address reads rather than writes.
export const Archived: Story = {
  render: () => <Facts contact={archivedContact} />,
};
