// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { TelegramConnectorsPanel } from "./connectors.telegram";

// The workspace's Telegram bot on Integrations. Every story but the read-only
// one draws an administrator's view, so the verbs its layout is judged with show.
function stubAsBotManager(routes: RouteMap) {
  stubWithSession(routes, {
    channel_connection: ["read", "create", "update", "delete"],
  });
}

const meta: Meta<typeof TelegramConnectorsPanel> = {
  title: "Settings/Data/Integrations/Telegram bot",
  component: TelegramConnectorsPanel,
};
export default meta;
type Story = StoryObj<typeof TelegramConnectorsPanel>;

// Three states. No bot yet shows the connect verb. Live bots get a row each. A
// deployment with no credential store to seal a token in answers 503, a
// feature-off state and not an error.
function telegramStory(
  channels: unknown[] | null,
  stub: (routes: RouteMap) => void = stubAsBotManager,
) {
  return routeStory(
    () =>
      channels === null
        ? jsonResponse({ code: "channel_credentials_not_configured" }, 503)
        : jsonResponse({ data: channels }),
    stub,
  );
}

function routeStory(
  answer: RouteMap[string],
  stub: (routes: RouteMap) => void = stubAsBotManager,
) {
  return () => {
    stub({ "GET /channel-connections": answer });
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

export const TelegramNoBotYet: Story = { render: telegramStory([]) };

// Two live bots is the state a send refuses outright in. This panel is the only
// surface that can end it, so both have to be here, each with its own
// Disconnect.
export const TelegramTwoBots: Story = {
  render: telegramStory([
    salesBot,
    {
      ...salesBot,
      id: "018f3a1b-0000-7000-8000-0000000000d2",
      channelId: "555000222",
      channelLabel: "acme_support_bot",
      status: "pending",
    },
  ]),
};

export const TelegramNotConfigured: Story = { render: telegramStory(null) };

// The seeded rep reads the bot and may change none of it: the roster without
// a verb, and one line naming who can act instead.
export const TelegramReadOnly: Story = {
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
    await expect(
      row.queryByRole("button", { name: "Replace token" }),
    ).toBeNull();
    await expect(row.queryByRole("button", { name: "Disconnect" })).toBeNull();
  },
};

// `--textMuted` (the description) against `--textMeta` (the bot handle) is
// the pair most likely to collapse into one grey under the dark accent lift.
export const TelegramDark: Story = {
  globals: { theme: "dark" },
  render: telegramStory([salesBot]),
};

// The read still in flight: the panel's own skeleton under its intro.
export const TelegramLoading: Story = {
  render: routeStory(() => new Promise(() => {})),
};

// The read refused: the shared failure plate with its retry, not a callout.
export const TelegramLoadFailed: Story = {
  render: routeStory(() => jsonResponse({ code: "internal" }, 500)),
};
