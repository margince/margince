// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, within } from "storybook/test";
import { AiDraftBand } from "./composeaiband";
import { SelectedReply } from "./composethread.stories";

const meta: Meta = {
  title: "Patterns/Compose mail/AI line",
  component: AiDraftBand,
};
export default meta;

/** The ask for a draft, one line above the words of a reply. */
export const Asking: StoryObj = {
  render: SelectedReply.render,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByRole("button", { name: "Draft reply" });
  },
};
