// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { ChannelHistoryProfile } from "./channelhistoryprofile";

const meta = {
  title: "Records/Team chat/Slack history profile",
  component: ChannelHistoryProfile,
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
} satisfies Meta<typeof ChannelHistoryProfile>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Internal: Story = {
  args: {
    profile: {
      distribution: "internal",
      history_per_minute: 50,
      replies_per_minute: 50,
      history_page_size: 200,
      replies_page_size: 100,
      source: "admin_declared",
      recorded_at: "2026-11-01T09:00:00Z",
      canonical_supported: true,
    },
  },
};
export const RestrictedCommercialDistribution: Story = {
  args: {
    profile: {
      distribution: "non_marketplace",
      history_per_minute: 1,
      replies_per_minute: 1,
      history_page_size: 15,
      replies_page_size: 15,
      source: "admin_declared",
      recorded_at: "2026-11-01T09:00:00Z",
      canonical_supported: false,
    },
  },
};
export const Unconfirmed: Story = { args: {} };
