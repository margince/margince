// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { PromotedLeadPanel } from "./leads.promotion";

const meta: Meta<typeof PromotedLeadPanel> = {
  title: "Records/Leads/Promotion outcome",
  component: PromotedLeadPanel,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /contacts/c-1": () =>
          jsonResponse({ id: "c-1", full_name: "Dana Buyer" }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
  args: {
    lead: {
      id: "l-1",
      full_name: "Dana Buyer",
      status: "promoted",
      score: 72,
      source: "manual",
      captured_by: "human:u-1",
      version: 2,
      created_at: "2026-06-01T08:00:00Z",
      updated_at: "2026-06-20T08:00:00Z",
      promoted_at: "2026-06-20T08:00:00Z",
      promoted_contact_id: "c-1",
    },
    promotion: {
      outcome: "merged",
      trigger: "inbound_reply",
      evidenceNote: "Asked for a proposal.",
      pending: false,
      failed: false,
    },
  },
};
export default meta;
type Story = StoryObj<typeof PromotedLeadPanel>;

export const InboundReply: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText(/Inbound reply/)).toBeVisible();
    await expect(await canvas.findByText("Dana Buyer")).toBeVisible();
    await expect(canvas.queryByText(/inbound_reply/)).not.toBeInTheDocument();
  },
};
export const MeetingBooked: Story = {
  args: {
    promotion: {
      outcome: "created",
      trigger: "meeting_booked",
      pending: false,
      failed: false,
    },
  },
};
export const MeetingHeld: Story = {
  args: {
    promotion: {
      outcome: "created",
      trigger: "meeting_held",
      pending: false,
      failed: false,
    },
  },
};
export const QualifiedManually: Story = {
  args: {
    promotion: {
      outcome: "created",
      trigger: "human_qualify",
      pending: false,
      failed: false,
    },
  },
};
export const UnknownTrigger: Story = {
  args: {
    promotion: {
      outcome: "unknown",
      trigger: "future_trigger",
      pending: false,
      failed: false,
    },
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).queryByText(/future_trigger/),
    ).not.toBeInTheDocument();
  },
};
