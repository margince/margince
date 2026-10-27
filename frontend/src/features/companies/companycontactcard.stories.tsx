// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { StoryProviders } from "../../testing/story-utils";
import { CompanyContactCard } from "./companycontactcard";

type Contact = components["schemas"]["Company360Contact"];

const LAN: Contact = {
  contact_id: "01930000-0000-7000-8000-0000000000c2",
  full_name: "Phạm Thị Lan",
  title: "Trưởng phòng mua hàng",
  primary_email: "lan@minhphat.example",
  strength: {
    score: 72,
    bucket: "strong",
    factors: { recency: 0.8, frequency: 0.6, reciprocity: 0.7, direction: 0.5 },
  },
  deal_roles: [],
  consent: {},
};

const meta = {
  title: "Records/Company 360/Contact card",
  component: CompanyContactCard,
  parameters: { layout: "padded" },
  args: { contact: LAN, companyName: "Minh Phát" },
  render: (args) => (
    <StoryProviders locale="vi">
      <div style={{ maxWidth: 360 }}>
        <CompanyContactCard {...args} />
      </div>
    </StoryProviders>
  ),
} satisfies Meta<typeof CompanyContactCard>;
export default meta;

type Story = StoryObj<typeof meta>;

// The pill takes its words from the shared deal role map, in the reader's
// language.
export const SeatInTheReadersLanguage: Story = {
  args: {
    contact: { ...LAN, deal_roles: [{ deal_id: "d-1", role: "blocker" }] },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("link", { name: "Phạm Thị Lan" });
    await expect(canvas.getByText("Người cản trở")).toBeVisible();
  },
};

// A role the map does not know draws no pill, and a prototype name such as
// `toString` does not print a function.
export const SeatTheMapDoesNotKnow: Story = {
  args: {
    contact: {
      ...LAN,
      deal_roles: [
        { deal_id: "d-1", role: "toString" },
        { deal_id: "d-2", role: "gatekeeper" },
      ],
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("link", { name: "Phạm Thị Lan" });
    await expect(canvasElement.textContent).not.toMatch(
      /function|toString|gatekeeper/,
    );
  },
};
