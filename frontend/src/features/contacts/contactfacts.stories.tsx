// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  callMessengersRoute,
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
  phones: [
    {
      id: "pp-1",
      contact_id: "p-1",
      phone: "+49 89 1234 5678",
      phone_type: "work",
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
  installFetchStub({
    "GET /me": meRoute({ contact: ["read", "update"] }),
    "GET /installation/settings": callMessengersRoute(),
  });
  return (
    <StoryProviders>
      <ContactFacts contact={contact} buyingRole={buyingRole} />
    </StoryProviders>
  );
}

export const Live: Story = {
  render: () => <Facts contact={live} buyingRole="champion" />,
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("link", {
      name: "Open chat in WhatsApp",
    });
  },
};

const LONG_NUMBER = "+849123456789012";

// The longest number a messenger takes, at 390px: its chats wrap under it and
// nothing runs past the edge.
export const LongNumberPhone: Story = {
  render: () => (
    <Facts
      contact={{
        ...live,
        primary_email: "alexandra.konstantinopoulou@brandt-automotive.example",
        phones: live.phones?.map((row) => ({ ...row, phone: LONG_NUMBER })),
      }}
      buyingRole="champion"
    />
  ),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const zalo = await canvas.findByRole("link", { name: "Open chat in Zalo" });
    const number = canvas.getByRole("link", { name: LONG_NUMBER });
    await expect(zalo.getBoundingClientRect().right).toBeLessThanOrEqual(
      document.documentElement.clientWidth,
    );
    await expect(zalo.getBoundingClientRect().top).toBeGreaterThanOrEqual(
      number.getBoundingClientRect().bottom - 0.5,
    );
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
      document.documentElement.clientWidth,
    );
  },
};

// The row alone: no buying role without the 360, and no address on file.
export const Archived: Story = {
  render: () => <Facts contact={archivedContact} />,
};
