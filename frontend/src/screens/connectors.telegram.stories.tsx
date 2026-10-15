// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "../testing/story-utils";
import { TelegramConnectorsPanel } from "./connectors.telegram";
import "./connectors.css";

// The workspace's bot: none yet, and live bots with a row each. A deployment
// with no credential store answers with a calm feature-off state.

function asBotManager(routes: RouteMap) {
  stubWithSession(routes, {
    channel_connection: ["read", "create", "update", "delete"],
  });
}

function telegramStory(
  channels: unknown[] | null,
  stub: (routes: RouteMap) => void = asBotManager,
) {
  return () => {
    stub({
      "GET /channel-connections": () =>
        channels === null
          ? jsonResponse({ code: "channel_credentials_not_configured" }, 503)
          : jsonResponse({ data: channels }),
    });
    return (
      <StoryProviders>
        <TelegramConnectorsPanel />
      </StoryProviders>
    );
  };
}

const salesBot = {
  id: "018f3a1b-0000-7000-8000-0000000000d1",
  provider: "telegram",
  channelId: "555000111",
  channelLabel: "acme_sales_bot",
  status: "connected",
  version: 1,
};

const supportBotPending = {
  ...salesBot,
  id: "018f3a1b-0000-7000-8000-0000000000d2",
  channelId: "555000222",
  channelLabel: "acme_support_bot",
  status: "pending",
};

const meta: Meta<typeof TelegramConnectorsPanel> = {
  title: "Settings/You/Connections/Telegram bot",
  component: TelegramConnectorsPanel,
};
export default meta;
type Story = StoryObj<typeof TelegramConnectorsPanel>;

// No bot: one row says so and carries the connect verb.
export const NoBotYet: Story = { render: telegramStory([]) };

export const NoBotYetPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: telegramStory([]),
};

// A send refuses outright while two bots are live. This panel is the only
// surface that can end that, so each bot has its own Disconnect.
export const TwoBots: Story = {
  render: telegramStory([salesBot, supportBotPending]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findAllByTestId("telegram-connection"),
    ).toHaveLength(2);
    await expect(
      canvas.queryByText(/One bot receives and sends messages/),
    ).toBeNull();
  },
};

export const TwoBotsDark: Story = {
  globals: { theme: "dark" },
  render: telegramStory([salesBot, supportBotPending]),
};

export const TwoBotsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: telegramStory([salesBot, supportBotPending]),
};

export const BotMenu: Story = {
  render: telegramStory([salesBot]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Actions for @acme_sales_bot",
      }),
    );
    await within(canvasElement.ownerDocument.body).findByRole("button", {
      name: "Replace token",
    });
  },
};

export const NotConfigured: Story = { render: telegramStory(null) };

// The seeded rep reads the bot and may change none of it: the roster without
// a menu, and one line naming who can act instead.
export const ReadOnly: Story = {
  render: telegramStory([salesBot], (routes) =>
    stubWithSession(
      routes,
      { channel_connection: ["read"] },
      { roles: ["rep"] },
    ),
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText(
      "Only an administrator or operations user can connect or change the bot.",
    );
    const row = within(canvas.getByTestId("telegram-connection"));
    await expect(row.queryByRole("button")).toBeNull();
  },
};
