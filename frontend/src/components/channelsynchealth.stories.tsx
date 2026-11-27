// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { ChannelSyncHealth } from "./channelsynchealth";

const meta = {
  title: "Records/Team chat/Sync health",
  component: ChannelSyncHealth,
  decorators: [
    (Story) => {
      installFetchStub({ "GET /me": meRoute({}) });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
} satisfies Meta<typeof ChannelSyncHealth>;
export default meta;
type Story = StoryObj<typeof meta>;

export const RecapsWaiting: Story = {
  args: {
    reading: true,
    health: {
      coverage_at: "2026-11-14T09:00:00Z",
      pending_conversations: 3,
      failed_conversations: 1,
      pending_crm_updates: 0,
      unconfirmed_deliveries: 0,
      last_error: "source_too_long",
    },
  },
};

export const DeliveryUnconfirmed: Story = {
  args: {
    reading: false,
    health: {
      pending_conversations: 0,
      failed_conversations: 0,
      pending_crm_updates: 2,
      unconfirmed_deliveries: 1,
    },
  },
};

export const AudienceChanged: Story = {
  args: {
    reading: true,
    health: {
      pending_conversations: 0,
      failed_conversations: 0,
      pending_crm_updates: 2,
      unconfirmed_deliveries: 0,
      delivery_error: "audience_changed",
    },
  },
};

export const SlackHistoryProfileNeedsReview: Story = {
  args: {
    reading: true,
    health: {
      pending_conversations: 2,
      failed_conversations: 1,
      pending_crm_updates: 0,
      unconfirmed_deliveries: 0,
      last_error: "history_profile",
    },
  },
};
