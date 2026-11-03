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
 * ONE chrome: the avatar at the end of the strip's trail, with the menu dropping
 * out of it and the theme flyout opening LEFT because there is no room on the
 * right. The block had a sidebar form once — a row at the foot of the column,
 * menu rising, flyout mirrored — and it went with the foot it stood in; the
 * sidebar carries the installation's entitlement there now (Shell/Navigation
 * shell). Nothing here photographs an arrangement a reader cannot reach.
 *
 * The chip is the design system's `Avatar` rather than a mark of this block's
 * own: the monogram is taken the same way everywhere, and the mesh is keyed on
 * the seat's USER ID, so the reader carries one colour from the chrome to their
 * own account page, and neither a rename nor a new address moves them to
 * another one.
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
 * The top bar: the avatar alone, 32px, at the end of the trail.
 *
 * Nothing beside it is drawn — no name, no address, no chevron — so the mark is
 * the whole of the affordance, and the sentence it stands for reaches a screen
 * reader through the clipped line instead.
 */
export const TopBar: Story = {
  name: "Top bar trigger",
  render: () => {
    stubSession();
    return <AccountMenuFrame />;
  },
};

/**
 * The panel, open: who you are, settings, theme, the booking link, help and
 * feedback, and sign out under its own rule. Only Sign out has a leading glyph,
 * because it is the row nobody wants to press by accident. A chevron says a
 * layer is behind its row.
 */
export const TopBarMenu: Story = {
  name: "Top bar menu",
  play: openMenu,
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
 * On a phone, open. The avatar's own edge meets the page gutter while its
 * square target reaches past it, and the menu hangs at that same gutter.
 */
export const TopBarMenuPhone: Story = {
  name: "Top bar menu (phone)",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async (context) => {
    await openMenu(context);
    const bar = context.canvasElement.querySelector<HTMLElement>(".topbar");
    const avatar = bar?.querySelector(".account .avatar");
    const menu = await within(document.body).findByRole("menu");
    if (!bar || !avatar) {
      throw new Error("the top bar or its avatar is missing");
    }
    const inner =
      bar.getBoundingClientRect().right -
      Number.parseFloat(getComputedStyle(bar).paddingRight);
    await expect(avatar.getBoundingClientRect().right).toBe(inner);
    await expect(menu.getBoundingClientRect().right).toBe(inner);
  },
  render: () => {
    stubSession();
    return <AccountMenuFrame />;
  },
};
