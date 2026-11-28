// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { TimelineList } from "./timeline";
import { ChatSummaryFlag } from "./timelinechat";

const meta = {
  title: "Components/Text and data display/Chat recaps",
  component: ChatSummaryFlag,
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /me": meRoute({ activity: ["read"] }),
        ...Object.fromEntries(
          ["dispact", "slack", "teams"].map((provider) => [
            `GET /activities/${provider}/chat-source`,
            () =>
              jsonResponse({
                available: false,
                user_id: "00000000-0000-0000-0000-000000000001",
              }),
          ]),
        ),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
} satisfies Meta<typeof ChatSummaryFlag>;
export default meta;
type Story = StoryObj<typeof meta>;
export const ProviderSources: Story = {
  args: { entry: {} },
  render: () => (
    <TimelineList
      zone="Europe/Berlin"
      entries={(["dispact", "slack", "teams"] as const).map((provider) => ({
        id: provider,
        kind: "note",
        title: "Pilot still depends on Legal approval",
        body: "The buyer requested a Monday start. Legal must approve the data processing agreement first. Jordan will ask Legal for a review; the start date remains unconfirmed.",
        atIso: "2026-06-01T10:00:00Z",
        atPrecision: "instant",
        provenance: { kind: "agent", agent: "chat_links" },
        chatEpisode: {
          provider,
          channel_name: "Berlin pilot",
          source_changed: provider === "slack",
          message_count: 8,
          ended_at: "2026-06-01T11:00:00Z",
          url: "https://chat.example.test/pilot",
        },
      }))}
    />
  ),
};
