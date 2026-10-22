// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, within } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { AiDraftToggle } from "./composeaiband";
import { SelectedReply } from "./composethread.stories";

const meta: Meta<typeof AiDraftToggle> = {
  title: "Patterns/Compose mail/AI band",
  component: AiDraftToggle,
};
export default meta;

function Toggle() {
  const [open, setOpen] = useState(false);
  return (
    <StoryProviders>
      <AiDraftToggle open={open} onToggle={() => setOpen((was) => !was)} />
    </StoryProviders>
  );
}

/** The editor's own AI verb, which opens the ask for a draft under the bar. */
export const Verb: StoryObj = {
  render: () => <Toggle />,
  play: async () => {
    const verb = await screen.findByRole("button", { name: "Draft with AI" });
    await userEvent.click(verb);
    await expect(verb).toHaveAttribute("aria-expanded", "true");
  },
};

/** The ask opened inside the composer: what the reply should say, and what a
 *  draft is written from. */
export const Asking: StoryObj = {
  render: SelectedReply.render,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.click(
      await dialog.findByRole("button", { name: "Draft with AI" }),
    );
    await dialog.findByText(/Nothing is sent\./);
  },
};
