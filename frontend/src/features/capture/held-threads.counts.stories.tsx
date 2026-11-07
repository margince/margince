// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { HeldCounts } from "./held-threads.counts";

// The held list by status over the whole list, so a stuck classifier backlog
// reads apart from real holds.

const meta: Meta = {
  title: "Settings/You/Capture activity/Held thread counts",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

/** A backlog waiting to be classified beside the threads really held. */
export const ABacklogAndRealHolds: Story = {
  render: () => (
    <StoryProviders>
      <HeldCounts
        counts={{
          pending: 212,
          unsure: 18,
          held: 940,
          held_by_owner: 3,
          stalled: 0,
        }}
      />
    </StoryProviders>
  ),
};
