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
 * A row of the account menu with its flyout open, at phone width, where the
 * flyout stops being a popover and folds into the menu under the row that
 * opened it. The desktop frames are under "Shell/Account block" and
 * "Shell/Account help and feedback".
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
