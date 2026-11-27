// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { ChannelSyncSettings } from "./channelsyncsettings";

const meta = {
  title: "Records/Team chat/Sync settings",
  component: ChannelSyncSettings,
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
} satisfies Meta<typeof ChannelSyncSettings>;
export default meta;
type Story = StoryObj<typeof meta>;
export const InboundOnly: Story = {
  args: {
    channel: {
      id: "018f3a1b-0000-7000-8000-00000000c101",
      provider: "dispact",
      channel_slug: "berlin-pilot",
      channel_name: "Berlin pilot",
      channel_url: "https://chat.example.test/berlin-pilot",
      verbosity: "important",
      status: "active",
      save_channel_updates: true,
      send_crm_updates: false,
      created_at: "2026-06-01T09:00:00Z",
    },
    recordType: "deal",
    recordId: "018f3a1b-0000-7000-8000-00000000c001",
    onClose: () => {},
    onSaved: () => {},
    children: null,
  },
};
