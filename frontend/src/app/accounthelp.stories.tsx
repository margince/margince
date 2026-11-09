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

// Help documents, licence notices and keyboard shortcuts stay available to every seat.
const meta: Meta<typeof HelpSubmenu> = {
  title: "Shell/Account help and feedback",
  component: HelpSubmenu,
  parameters: { layout: "fullscreen" },
  play: async (context) => {
    await openHelpFlyout(context);
    const link = within(document.body).getByRole("menuitem", {
      name: /^Web app licenses/,
    });
    await expect(link).toHaveAttribute(
      "href",
      "/licenses/THIRD-PARTY-NOTICES.txt",
    );
  },
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
