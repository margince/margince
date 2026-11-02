// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  AccountMenuFrame,
  openHelpFlyout,
  stubSession,
} from "../testing/accountmenu-story";
import { HelpSubmenu } from "./accounthelp";

/**
 * Help and feedback, open from the account menu and opening LEFT like the theme
 * flyout. Four places off the app, each marked as a new tab, the keyboard
 * shortcuts, and the build under its own rule.
 *
 * Only the unstamped build is drawn: the releases are compiled into the bundle
 * from the build's environment, so a story cannot set them. The unit tests
 * cover the stamped and instance builds (accounthelp.test.tsx).
 */
const meta: Meta<typeof HelpSubmenu> = {
  title: "Shell/Account help and feedback",
  component: HelpSubmenu,
  parameters: { layout: "fullscreen" },
  play: openHelpFlyout,
  render: () => {
    stubSession();
    return <AccountMenuFrame />;
  },
};
export default meta;
type Story = StoryObj<typeof HelpSubmenu>;

export const DevelopmentBuild: Story = {
  name: "Development build",
};

export const DevelopmentBuildDark: Story = {
  name: "Development build (dark)",
  globals: { theme: "dark" },
};

/** At phone width the flyout folds into the menu as an indented group. */
export const DevelopmentBuildPhone: Story = {
  name: "Development build (phone)",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
