// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { TelegramBusinessPanel } from "./connectors.telegrambusiness";

const meta: Meta<typeof TelegramBusinessPanel> = {
  title: "Settings/Data/Integrations/Telegram business",
  component: TelegramBusinessPanel,
};
export default meta;
type Story = StoryObj<typeof TelegramBusinessPanel>;

const bot = {
  id: "018f3a1b-0000-7000-8000-0000000000d1",
  provider: "telegram",
  channelId: "555000111",
  channelLabel: "acme_sales_bot",
  status: "connected",
  version: 1,
};

function accountsStory(accounts: unknown[]) {
  return () => {
    stubWithSession(
      {
        "GET /channel-connections": () => jsonResponse({ data: [bot] }),
        "GET /teams": () =>
          jsonResponse({
            data: [{ id: TEAM_ID, name: "Sales desk" }],
            page: {},
          }),
        "GET /channel-connections/telegram-business": () =>
          jsonResponse({ data: accounts }),
      },
      { channel_connection: ["read", "create", "update", "delete"] },
    );
    return (
      <StoryProviders>
        <TelegramBusinessPanel />
      </StoryProviders>
    );
  };
}

const TEAM_ID = "018f3a1b-0000-7000-8000-0000000000e1";

const base = {
  accountId: "9001",
  connectedAt: "2026-03-02T10:00:00Z",
};

export const NoAccountLinked: Story = { render: accountsStory([]) };

// One account per status: waiting, live and switched off by its owner.
export const EveryStatus: Story = {
  render: accountsStory([
    {
      ...base,
      id: "018f3a1b-0000-7000-8000-0000000000b1",
      ownerLabel: "Mira Keller",
      status: "pending_approval",
    },
    {
      ...base,
      id: "018f3a1b-0000-7000-8000-0000000000b2",
      ownerLabel: "Jonas Weiss",
      status: "connected",
      approvedAt: "2026-03-03T10:00:00Z",
      seatId: "018f3a1b-0000-7000-8000-0000000000f1",
      readerTeamId: TEAM_ID,
      mailVisibility: "held",
    },
    {
      ...base,
      id: "018f3a1b-0000-7000-8000-0000000000b3",
      ownerLabel: "Lena Brandt",
      status: "disconnected",
    },
  ]),
};

// Approved accounts: one held for its team, one shared, and a member's own.
export const ReaderTeamAndVisibility: Story = {
  render: accountsStory([
    {
      ...base,
      id: "018f3a1b-0000-7000-8000-0000000000b2",
      ownerLabel: "Jonas Weiss",
      status: "connected",
      approvedAt: "2026-03-03T10:00:00Z",
      seatId: "018f3a1b-0000-7000-8000-0000000000f1",
      readerTeamId: TEAM_ID,
      mailVisibility: "held",
    },
    {
      ...base,
      id: "018f3a1b-0000-7000-8000-0000000000b4",
      ownerLabel: "Ada Lovell",
      status: "connected",
      approvedAt: "2026-03-03T10:00:00Z",
      seatId: "018f3a1b-0000-7000-8000-0000000000f2",
      readerTeamId: TEAM_ID,
      mailVisibility: "shared",
    },
    {
      ...base,
      id: "018f3a1b-0000-7000-8000-0000000000b5",
      ownerLabel: "Sam Reed",
      status: "connected",
      approvedAt: "2026-03-03T10:00:00Z",
      seatId: "018f3a1b-0000-7000-8000-0000000000f3",
      readerTeamId: null,
      mailVisibility: "classified",
    },
  ]),
};
