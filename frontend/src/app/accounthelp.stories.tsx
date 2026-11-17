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

// The places off the app come first, then the rows that open a dialog. The
// build, its license and the notices sit behind About Margince.
const meta: Meta<typeof HelpSubmenu> = {
  title: "Shell/Account help and feedback",
  component: HelpSubmenu,
  parameters: { layout: "fullscreen" },
  play: async (context) => {
    await openHelpFlyout(context);
    const help = within(document.body).getByRole("menu", {
      name: "Help and feedback",
    });
    const items = within(help).getAllByRole("menuitem");
    await expect(items.at(-2)).toHaveTextContent("Keyboard shortcuts");
    await expect(items.at(-1)).toHaveTextContent("About Margince");
    await expect(items.at(-1)).toHaveAttribute("aria-haspopup", "dialog");
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
