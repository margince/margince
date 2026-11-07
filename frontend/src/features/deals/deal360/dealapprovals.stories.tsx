// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { DealApprovals } from "./dealapprovals";

const meta = {
  title: "Records/Deals/Pending decisions",
  component: DealApprovals,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof DealApprovals>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Pending: Story = {
  args: {
    decide: fn(),
    approvals: [
      {
        id: "approval-1",
        kind: "close_date_correction",
        status: "pending",
        summary: "Confirm the real close date",
        proposed_by: "system:close-date",
        proposed_change: { deal_id: "d1", expected_close_date: "2026-11-15" },
        created_at: "2026-08-20T09:00:00Z",
        target_entity_type: "deal",
        target_entity_id: "d1",
      },
    ],
  },
  render: (args) => {
    installFetchStub({ "GET /me": meRoute({ deal: ["read", "update"] }) });
    return <DealApprovals {...args} />;
  },
  play: async ({ args, canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", { name: "Accept" }),
    );
    await expect(args.decide).toHaveBeenCalledWith({
      approvalId: "approval-1",
      verdict: "approve",
    });
  },
};
export const PendingDark: Story = { ...Pending, globals: { theme: "dark" } };
