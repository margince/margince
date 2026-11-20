// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
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
}: Readonly<{ startCollapsed: boolean }>) {
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
        <TopBar route={ACCOUNT} onOpenSearch={openSearch} />
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
  startCollapsed,
}: Readonly<{ startCollapsed: boolean }>) {
  installFetchStub({ ...SHELL_ROUTES, "GET /me": meRoute({}) });
  return (
    <StoryProviders>
      <SeedInstallation>
        <SettingsChrome startCollapsed={startCollapsed} />
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
  render: () => <SettingsStory startCollapsed={false} />,
};

/** The same chrome with the rail folded to 56px. */
export const Collapsed: Story = {
  render: () => <SettingsStory startCollapsed />,
};
