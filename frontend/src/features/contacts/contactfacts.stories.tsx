// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { archivedContact } from "./contactarchived.fixtures";
import { ContactFacts } from "./contactfacts";

// The facts strip under a contact's name, read off the row alone. So the
// archived page, which has no 360, draws the same strip as the live one.
const meta: Meta = {
  title: "Records/Contact 360/Header facts",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const live: typeof archivedContact = {
  ...archivedContact,
  archived_at: null,
  writable: true,
  primary_email: "dana@brandt.example",
  emails: [
    {
      id: "pe-1",
      contact_id: "p-1",
      email: "dana@brandt.example",
      email_type: "work",
      is_primary: true,
      position: 0,
      source: "manual",
      captured_by: "human:u-1",
    },
  ],
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

// The row alone: no buying role without the 360, and no address on file.
export const Archived: Story = {
  render: () => <Facts contact={archivedContact} />,
};
