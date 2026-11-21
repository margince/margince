// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Card } from "../design-system/atoms";
import { parseHash } from "../lib/router";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { SettingsRail } from "./shellrail";
import { SettingsPageTitle, SettingsTopBar } from "./shelltitle";

// A settings page in the frame the shell draws. The rail shows the settings
// level, the top bar names the section and the page, and the page heads itself.
const meta: Meta<typeof SettingsTopBar> = {
  title: "Shell/Settings page title",
  component: SettingsTopBar,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof SettingsTopBar>;

const ACCOUNT = parseHash("#/settings/account");

function SettingsFrame({ held = false }: Readonly<{ held?: boolean }>) {
  installFetchStub({ "GET /me": meRoute({}) });
  return (
    <StoryProviders>
      <div className="app railexpanded">
        <SettingsRail route={ACCOUNT} collapsed={false} />
        <main className="main">
          <SettingsTopBar
            route={ACCOUNT}
            collapsed={false}
            onToggle={() => undefined}
            onOpenSearch={() => undefined}
            held={held}
          />
          <div className="scroll">
            <SettingsPageTitle route={ACCOUNT} actionSlot={() => undefined} />
            <div className="wrap">
              <Card as="div">The page, under the heading that names it.</Card>
            </div>
          </div>
        </main>
      </div>
    </StoryProviders>
  );
}

export const AccountPage: Story = {
  name: "a settings page — the trail and the heading name it",
  render: () => <SettingsFrame />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("heading", { level: 1, name: "Account" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("button", { name: /^Notifications/ }),
    ).toBeVisible();
  },
};

/** The onboarding gate holds the reader: no outbox and no bell in the bar. */
export const HeldOnOnboarding: Story = {
  name: "a settings page while onboarding holds the reader",
  render: () => <SettingsFrame held />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("heading", { level: 1, name: "Account" });
    await expect(
      canvas.queryByRole("button", { name: /^Notifications/ }),
    ).toBeNull();
  },
};
