// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, within } from "storybook/test";
import { Card } from "../design-system/atoms";
import type { Route } from "../lib/router";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import {
  SeedInstallation,
  SHELL_ROUTES,
  usePaletteSeam,
} from "./shell.storykit";
import { SettingsRail } from "./shellrail";
import { SettingsPageTitle } from "./shelltitle";
import { TopBar } from "./topbar";
import "../features/settings/settings.css";

const ACCOUNT: Route = { screen: "settings", id: "account" };

// The settings chrome: its rail, its bar and the page's name in the card.
function SettingsChrome({
  startCollapsed,
  held,
}: Readonly<{ startCollapsed: boolean; held: boolean }>) {
  const [collapsed, setCollapsed] = useState(startCollapsed);
  const { openSearch, palette } = usePaletteSeam();
  return (
    <div className={collapsed ? "app" : "app railexpanded"}>
      <SettingsRail
        route={ACCOUNT}
        collapsed={collapsed}
        onToggle={() => setCollapsed((current) => !current)}
      />
      <main className="main">
        <TopBar route={ACCOUNT} onOpenSearch={openSearch} held={held} />
        <div className="scroll">
          <SettingsPageTitle route={ACCOUNT} actionSlot={() => undefined} />
          <div className="wrap">
            <div className="settings-stack">
              <Card as="div">Content</Card>
            </div>
          </div>
        </div>
      </main>
      {palette}
    </div>
  );
}

function SettingsStory({
  startCollapsed = false,
  held = false,
}: Readonly<{ startCollapsed?: boolean; held?: boolean }>) {
  installFetchStub({ ...SHELL_ROUTES, "GET /me": meRoute({}) });
  return (
    <StoryProviders>
      <SeedInstallation>
        <SettingsChrome startCollapsed={startCollapsed} held={held} />
      </SeedInstallation>
    </StoryProviders>
  );
}

const meta: Meta<typeof SettingsPageTitle> = {
  title: "Shell/Settings chrome",
  component: SettingsPageTitle,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof SettingsPageTitle>;

/** The settings level open, with the page named in the card. */
export const Expanded: Story = {
  render: () => <SettingsStory />,
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

/** The same chrome with the rail folded to 56px. */
export const Collapsed: Story = {
  render: () => <SettingsStory startCollapsed />,
};

/** The onboarding gate holds the reader: Back to setup stands in the bar,
 *  where the setup reminder, the outbox and the bell would. */
export const HeldOnOnboarding: Story = {
  render: () => <SettingsStory held />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("heading", { level: 1, name: "Account" });
    await expect(
      canvas.queryByRole("button", { name: /^Notifications/ }),
    ).toBeNull();
    await expect(
      canvas.getByRole("link", { name: "Back to setup" }),
    ).toBeVisible();
    await expect(
      canvas.queryByRole("button", { name: /^Finish setup/ }),
    ).toBeNull();
  },
};
