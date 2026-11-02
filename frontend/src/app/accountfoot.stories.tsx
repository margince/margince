// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  AccountMenuFrame,
  openMenu,
  stubSession,
} from "../testing/accountmenu-story";
import { jsonResponse } from "../testing/story-utils";
import { SignOutRow } from "./accountfoot";

/**
 * The way out, refused: the server's own words in an alert, in the menu the
 * reader asked from, under a Sign out row that is enabled again.
 *
 * `AccountRelease`, the other half of this file, is drawn where it ships, at
 * the foot of "Shell/Account help and feedback".
 */
const meta: Meta<typeof SignOutRow> = {
  title: "Shell/Account sign-out",
  component: SignOutRow,
  parameters: { layout: "fullscreen" },
  play: async (context) => {
    await openMenu(context);
    const canvas = within(context.canvasElement);
    await userEvent.click(canvas.getByRole("menuitem", { name: "Sign out" }));
    await canvas.findByRole("alert");
  },
  render: () => {
    stubSession(
      {},
      {
        "POST /auth/logout": () =>
          jsonResponse(
            { title: "Conflict", detail: "Your session is already gone." },
            409,
          ),
      },
    );
    return <AccountMenuFrame />;
  },
};
export default meta;
type Story = StoryObj<typeof SignOutRow>;

export const Refused: Story = {};

export const RefusedPhone: Story = {
  name: "Refused (phone)",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
