// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  AccountMenuFrame,
  openHelpFlyout,
  stubSession,
} from "../testing/accountmenu-story";
import { HelpSubmenu } from "./accounthelp";

// Help documents and keyboard shortcuts stay available to every seat; the
// build, its license and the notices sit behind About Margince.
const meta: Meta<typeof HelpSubmenu> = {
  title: "Shell/Account help and feedback",
  component: HelpSubmenu,
  parameters: { layout: "fullscreen" },
  play: async (context) => {
    await openHelpFlyout(context);
    const about = within(document.body).getByRole("menuitem", {
      name: "About Margince",
    });
    await expect(about).toHaveAttribute("aria-haspopup", "dialog");
  },
  render: () => {
    stubSession();
    return <AccountMenuFrame />;
  },
};
export default meta;
type Story = StoryObj<typeof HelpSubmenu>;

export const Open: Story = {};

export const OpenDark: Story = {
  name: "Open (dark)",
  globals: { theme: "dark" },
};

/** At phone width the flyout folds into the menu as an indented group. */
export const OpenPhone: Story = {
  name: "Open (phone)",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
