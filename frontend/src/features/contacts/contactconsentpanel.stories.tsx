// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactConsentPanel } from "./contactconsentpanel";
import { populated } from "./contactpage.fixtures";

const meta = {
  title: "Records/Contact 360/Consent panel",
  component: ContactConsentPanel,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof ContactConsentPanel>;
export default meta;
type Story = StoryObj<typeof meta>;
export const ReadOnly: Story = {
  args: {
    view: { ...populated, contact: { ...populated.contact, writable: false } },
  },
  render: (args) => {
    installFetchStub({
      "GET /me": meRoute(
        { contact: ["read"] },
        { roles: ["rep"], seat: "read" },
      ),
      "GET /contacts/p-1/consent/guard": () => jsonResponse({ entries: [] }),
      "GET /contacts/p-1/consent/stops": () => jsonResponse({ stops: [] }),
    });
    return <ContactConsentPanel {...args} />;
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).findByRole("button", {
        name: "Manage consent and proof history",
      }),
    ).resolves.toBeVisible();
  },
};
export const ReadOnlyDark: Story = { ...ReadOnly, globals: { theme: "dark" } };
