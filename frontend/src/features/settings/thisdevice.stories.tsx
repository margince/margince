// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { SettingList } from "../../design-system/settingrow";
import { en } from "../../i18n/en";
import type { InstallState } from "../../lib/pwa";
import { StoryProviders } from "../../testing/story-utils";
import { InstallSettingRow } from "./thisdevice";

const AVAILABLE: InstallState = {
  kind: "available",
  prompt: () => new Promise(() => undefined),
};

function story(install: InstallState) {
  return () => (
    <StoryProviders>
      <SettingList>
        <InstallSettingRow install={install} />
      </SettingList>
    </StoryProviders>
  );
}

const meta: Meta<typeof InstallSettingRow> = {
  title: "Settings/You/Account/This browser",
  component: InstallSettingRow,
};
export default meta;
type Story = StoryObj<typeof InstallSettingRow>;

// Chromium kept its install offer for the page to present.
export const Available: Story = { render: story(AVAILABLE) };

// The browser's dialog is open: Install keeps focus and waits for the answer.
export const Pending: Story = {
  render: story(AVAILABLE),
  play: async ({ canvasElement }) => {
    const install = await within(canvasElement).findByRole("button", {
      name: en["settings.installAppAction"],
    });
    await userEvent.click(install);
    await expect(install).toHaveAttribute("aria-busy", "true");
  },
};

// The reader turned the offer down; the browser can still install it.
export const Dismissed: Story = { render: story({ kind: "dismissed" }) };

// iPhone and iPad browsers install by hand from the share sheet.
export const Manual: Story = { render: story({ kind: "manual-ios" }) };

export const Installed: Story = { render: story({ kind: "installed" }) };

export const Phone: Story = {
  tags: ["uat-phone"],
  render: story(AVAILABLE),
};
