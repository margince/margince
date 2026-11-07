// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import {
  AccountMenuFrame,
  openMenu,
  openThemeFlyout,
  stubSession,
} from "../testing/accountmenu-story";
import { PORTRAIT, WHITE_PORTRAIT } from "../testing/seatphotos";
import { AccountMenu } from "./account";
import { AccountPanel } from "./accountpanel";
import { ThemeSubmenu } from "./accounttheme";

/**
 * The seat's chip at the end of the top bar, and the menu it opens.
 * `fullscreen`, because the menu places itself against the real bar; canvas
 * padding would draw a geometry the product does not have.
 */
const meta: Meta<typeof AccountMenu> = {
  title: "Shell/Account block",
  component: AccountMenu,
  subcomponents: { AccountPanel, ThemeSubmenu },
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof AccountMenu>;

/**
 * The top bar: the seat's chip alone, a circle filling the bar's square inside
 * a control-edge ring. The initials are hidden, so the button's name is "Account".
 */
export const TopBar: Story = {
  name: "Top bar trigger",
  play: async ({ canvasElement }) => {
    const trigger = await within(canvasElement).findByRole("button", {
      name: "Account",
    });
    const face = await waitFor(() => {
      const chip = trigger.querySelector(".avatar");
      if (!chip) throw new Error("the account trigger draws no chip");
      return chip;
    });
    const height =
      getComputedStyle(trigger).getPropertyValue("--controlHeight");
    await expect(face.getBoundingClientRect().width).toBe(
      Number.parseFloat(height),
    );
    await expect(face.closest("[aria-hidden='true']")).not.toBeNull();
  },
  render: () => {
    stubSession();
    return <AccountMenuFrame />;
  },
};

/** The same trigger for a seat with a picture: it covers the whole circle. */
export const TopBarPhoto: Story = {
  name: "Top bar trigger with a picture",
  play: async ({ canvasElement }) => {
    const trigger = await within(canvasElement).findByRole("button", {
      name: "Account",
    });
    await waitFor(() =>
      expect(trigger.querySelector(".avatar-photo img")).not.toBeNull(),
    );
  },
  render: () => {
    stubSession({ avatar_url: PORTRAIT });
    return <AccountMenuFrame />;
  },
};

/** A white picture keeps its edge against the bar, by the ring alone. */
export const TopBarWhitePhoto: Story = {
  name: "Top bar trigger with a white picture",
  render: () => {
    stubSession({ avatar_url: WHITE_PORTRAIT });
    return <AccountMenuFrame />;
  },
};

export const TopBarWhitePhotoDark: Story = {
  ...TopBarWhitePhoto,
  name: "Top bar trigger with a white picture (dark)",
  globals: { theme: "dark" },
};

export const TopBarPhotoPhone: Story = {
  ...TopBarPhoto,
  name: "Top bar trigger with a picture (phone)",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/**
 * The panel, open: who you are, with the chip before the name and address. The
 * menu is named "Account", like the trigger, and focus lands on that head row.
 * Then settings, theme, the booking link, help and feedback, and sign out under
 * its own rule. Only Sign out has a leading glyph, because nobody wants to press
 * it by accident. A chevron says a layer is behind its row.
 */
export const TopBarMenu: Story = {
  name: "Top bar menu",
  play: async (context) => {
    await openMenu(context);
    const who = document.querySelector(".acctmenuwho");
    const chip = who?.querySelector(".avatar");
    const lines = who?.querySelector(".acctwho");
    if (!chip || !lines)
      throw new Error("the menu head lost its chip or lines");
    const middle = (box: DOMRect) => box.top + box.height / 2;
    const chipBox = chip.getBoundingClientRect();
    const linesBox = lines.getBoundingClientRect();
    await expect(chipBox.right).toBeLessThan(linesBox.left);
    await expect(middle(chipBox)).toBeCloseTo(middle(linesBox), 0);
    // Named as the trigger is; the head, which names the seat, takes focus.
    const menu = within(document.body).getByRole("menu", { name: "Account" });
    await expect(menu.contains(who)).toBe(true);
    await expect(document.activeElement).toBe(who);
  },
  render: () => {
    stubSession();
    return <AccountMenuFrame />;
  },
};

/** The menu's head for a seat with a picture: the same picture as the trigger. */
export const TopBarMenuPhoto: Story = {
  name: "Top bar menu with a picture",
  play: async (context) => {
    await openMenu(context);
    const head = document.querySelector(".acctmenuwho");
    await expect(head?.querySelector(".avatar-photo img")).not.toBeNull();
  },
  render: () => {
    stubSession({ avatar_url: PORTRAIT });
    return <AccountMenuFrame />;
  },
};

/**
 * The appearance flyout, open, opening LEFT.
 *
 * That direction is the point of the frame: the menu is anchored to a trigger at
 * the viewport's right edge, so a flyout that opened outward would open past the
 * window. The three answers are the product's own radios, so the tick on the
 * standing one is the control's state rather than an ornament beside it.
 */
export const TopBarThemeFlyout: Story = {
  name: "Top bar theme flyout",
  play: openThemeFlyout,
  render: () => {
    stubSession();
    return <AccountMenuFrame />;
  },
};

/**
 * The same flyout in dark, because the surfaces it stacks are three token layers
 * deep — the page ground, the menu's elevated card, and the flyout's own card
 * over it — and a shadow that separates them in light can vanish in dark.
 */
export const TopBarThemeFlyoutDark: Story = {
  name: "Top bar theme flyout (dark)",
  globals: { theme: "dark" },
  play: openThemeFlyout,
  render: () => {
    stubSession();
    return <AccountMenuFrame />;
  },
};

/**
 * On a phone, open. The trigger's box and the menu both end at the gutter.
 */
export const TopBarMenuPhone: Story = {
  name: "Top bar menu (phone)",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async (context) => {
    await openMenu(context);
    const bar = context.canvasElement.querySelector<HTMLElement>(".topbar");
    if (!bar) throw new Error("the top bar is missing");
    const trigger = within(bar).getByRole("button", { name: /Account$/ });
    const menu = await within(document.body).findByRole("menu");
    const inner =
      bar.getBoundingClientRect().right -
      Number.parseFloat(getComputedStyle(bar).paddingRight);
    await expect(trigger.getBoundingClientRect().right).toBe(inner);
    await expect(menu.getBoundingClientRect().right).toBe(inner);
  },
  render: () => {
    stubSession();
    return <AccountMenuFrame />;
  },
};
