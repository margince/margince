// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  AccountMenuFrame,
  openThemeFlyout,
  stubSession,
} from "../testing/accountmenu-story";
import { FlyoutRow } from "./accountflyout";

/**
 * The theme row with its flyout open at phone width. There the flyout folds
 * into the menu under its row. The desktop frames are under "Shell/Account
 * block" and "Shell/Account help and feedback".
 */
const meta: Meta<typeof FlyoutRow> = {
  title: "Shell/Account flyout row",
  component: FlyoutRow,
  parameters: { layout: "fullscreen" },
  play: openThemeFlyout,
  render: () => {
    stubSession();
    return <AccountMenuFrame />;
  },
};
export default meta;
type Story = StoryObj<typeof FlyoutRow>;

export const OpenPhone: Story = {
  name: "Theme open (phone)",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const OpenPhoneDark: Story = {
  name: "Theme open (phone, dark)",
  globals: { viewport: { value: "phone" }, theme: "dark" },
  tags: ["uat-phone"],
};
