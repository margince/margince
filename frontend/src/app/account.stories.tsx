// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  AccountMenuFrame,
  openMenu,
  openThemeFlyout,
  stubSession,
} from "../testing/accountmenu-story";
import { AccountMenu } from "./account";

/**
 * Who is signed in, and the two things the product now offers nowhere else: the
 * door into settings, and the appearance choice.
 *
 * One chrome: a contact glyph at the end of the strip's trail. The menu drops
 * out of it, and the theme flyout opens left because the right has no room.
 *
 * The menu's head carries the chip, the design system's `Avatar`. Its mesh is
 * keyed on the seat's user id, so a rename or a new address keeps its colour.
 *
 * fullscreen: the block measures itself against the container it sits in — the
 * strip's trail is flush to the content column's right edge, and the menu's drop
 * offset is computed from the strip's own height — so every story renders the
 * real chrome. Storybook's default canvas padding would frame a geometry the
 * product does not use.
 */
const meta: Meta<typeof AccountMenu> = {
  title: "Shell/Account block",
  component: AccountMenu,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof AccountMenu>;

/**
 * The top bar: a contact glyph alone, at the size the bar gives every glyph.
 *
 * Nothing beside it is drawn — no name, no address, no chevron — so the mark is
 * the whole of the affordance, and the sentence it stands for reaches a screen
 * reader through the clipped line instead.
 */
export const TopBar: Story = {
  name: "Top bar trigger",
  play: async ({ canvasElement }) => {
    const trigger = await within(canvasElement).findByRole("button", {
      name: /Account$/,
    });
    const icon = trigger.querySelector("svg");
    if (!icon) throw new Error("the account trigger draws no glyph");
    const size = getComputedStyle(trigger).getPropertyValue("--controlIcon");
    await expect(icon.getBoundingClientRect().width).toBe(
      Number.parseFloat(size),
    );
    await expect(trigger.querySelector(".avatar")).toBeNull();
  },
  render: () => {
    stubSession();
    return <AccountMenuFrame />;
  },
};

/**
 * The panel, open: who you are, with the chip before the name and address.
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
  },
  render: () => {
    stubSession();
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
 * On a phone, open. The glyph's edge meets the gutter as far as its target can
 * reach without leaving the bar. The menu hangs at the gutter.
 */
export const TopBarMenuPhone: Story = {
  name: "Top bar menu (phone)",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async (context) => {
    await openMenu(context);
    const bar = context.canvasElement.querySelector<HTMLElement>(".topbar");
    const trigger = bar?.querySelector(".account .user");
    const glyph = trigger?.querySelector("svg");
    const menu = await within(document.body).findByRole("menu");
    if (!bar || !trigger || !glyph) {
      throw new Error("the top bar or its account glyph is missing");
    }
    const edge = bar.getBoundingClientRect().right;
    const inner = edge - Number.parseFloat(getComputedStyle(bar).paddingRight);
    const box = trigger.getBoundingClientRect();
    const inset = (box.width - glyph.getBoundingClientRect().width) / 2;
    await expect(box.right).toBeLessThanOrEqual(edge);
    await expect(glyph.getBoundingClientRect().right).toBe(
      Math.min(inner, edge - inset),
    );
    await expect(menu.getBoundingClientRect().right).toBe(inner);
  },
  render: () => {
    stubSession();
    return <AccountMenuFrame />;
  },
};
