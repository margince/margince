// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { InstallState } from "../app/pwa";
import { StoryProviders } from "./story-utils";
import { InstallPanel } from "./thisdevice";

const AVAILABLE: InstallState = {
  kind: "available",
  prompt: () => new Promise(() => undefined),
};

function story(install: InstallState) {
  return () => (
    <StoryProviders>
      <InstallPanel install={install} />
    </StoryProviders>
  );
}

const meta: Meta<typeof InstallPanel> = {
  title: "Settings/You/Account/This device",
  component: InstallPanel,
};
export default meta;
type Story = StoryObj<typeof InstallPanel>;

// Chromium kept its install offer for the page to present.
export const Available: Story = { render: story(AVAILABLE) };

// The reader turned the offer down; the browser can still install it.
export const Dismissed: Story = { render: story({ kind: "dismissed" }) };

// iPhone and iPad browsers install by hand from the share sheet.
export const Manual: Story = { render: story({ kind: "manual-ios" }) };

export const Installed: Story = { render: story({ kind: "installed" }) };

export const Phone: Story = {
  tags: ["uat-phone"],
  render: story(AVAILABLE),
};
