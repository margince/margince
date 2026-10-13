// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../screens/story-utils";
import { parseHash } from "./router";
import { TopBar } from "./topbar";

// The top bar's "Finish setup" chip and the checklist behind it, served the
// reads a reader who connected Gmail and skipped the calendar would see.

const gmail = {
  id: "00000000-0000-4000-8000-000000000011",
  provider: "gmail",
  status: "connected",
  scopes: [],
  account_label: "ada@example.test",
};

function reminder() {
  stubWithSession(
    {
      "GET /connectors": () =>
        jsonResponse({
          data: [gmail],
          providers: ["gmail", "graph", "imap"].map((provider) => ({
            provider,
            reason: "ready",
          })),
        }),
      "GET /voice-profiles": () => jsonResponse({ data: [] }),
      "GET /scheduling/profile": () =>
        jsonResponse({
          enabled: false,
          provider: "",
          calendar_id: "",
          duration_minutes: 30,
          notice_minutes: 240,
          horizon_days: 30,
          buffer_minutes: 0,
          title: "Intro call",
          location: "",
        }),
    },
    { voice_profile: ["read", "create", "update"] },
  );
  return (
    <StoryProviders>
      <div className="app">
        <main className="main">
          <TopBar
            route={parseHash("#/settings")}
            collapsed={false}
            onOpenSearch={() => undefined}
          />
        </main>
      </div>
    </StoryProviders>
  );
}

const meta = {
  title: "Shell/Setup reminder",
  render: reminder,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

const openPanel: Story["play"] = async () => {
  await userEvent.click(
    await screen.findByRole("button", { name: /Finish setup/ }),
  );
};

export const Closed: Story = {};
export const ClosedDark: Story = { globals: { theme: "dark" } };
export const Open: Story = { play: openPanel };
export const OpenDark: Story = { play: openPanel, globals: { theme: "dark" } };
export const Phone: Story = {
  play: openPanel,
  globals: { viewport: { value: "phone" } },
};
