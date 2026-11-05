// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { en } from "../../i18n/en";
import { StoryProviders } from "../../testing/story-utils";
import { WorklistReturnLink } from "./worklist.return";

// "Back to Worklist" on a record the reader opened from the Worklist panel. The
// link carries the panel's view and scope, so going back lands where they were.

const meta: Meta<typeof WorklistReturnLink> = {
  title: "Records/Worklist/Return link",
  component: WorklistReturnLink,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof WorklistReturnLink>;

function at(hash: string) {
  return () => {
    const prior = globalThis.location.hash;
    globalThis.location.hash = hash;
    return () => {
      globalThis.location.hash = prior;
    };
  };
}

export const OpenedFromTheWorklist: Story = {
  beforeEach: at("#/contacts/c1?from=worklist&queue_scope=team"),
  render: () => (
    <StoryProviders>
      <WorklistReturnLink />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const link = await within(canvasElement).findByRole("link", {
      name: en["brief.queue.back"],
    });
    await expect(link.getAttribute("href")).toContain("queue_scope=team");
  },
};

export const OpenedFromElsewhere: Story = {
  beforeEach: at("#/contacts/c1"),
  render: () => (
    <StoryProviders>
      <WorklistReturnLink />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).queryByRole("link")).toBeNull();
  },
};
