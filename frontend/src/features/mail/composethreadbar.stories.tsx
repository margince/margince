// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { SelectedReply } from "./composethread.stories";
import { ThreadBar } from "./composethreadbar";

const meta: Meta = {
  title: "Patterns/Compose mail/Thread bar",
  component: ThreadBar,
};
export default meta;

/** Replying in a thread: the bar names it, and opens the record's threads to
 *  move the draft onto another one or out to a new email. */
export const Change: StoryObj = {
  render: SelectedReply.render,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.click(
      await dialog.findByRole("button", { name: /Change/ }),
    );
    await screen.findByRole("button", { name: "New email" });
  },
};
