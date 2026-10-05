// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../story-utils";
import { DealWatchList } from "./dealwatchcard";

// What the customer committed to that a deal waits on. The states worth seeing
// are a dated and an undated commitment side by side, and a list the reader
// sees only part of, which must not read as "nothing owed".

const meta: Meta<typeof DealWatchList> = {
  title: "Records/Deal 360/Watch",
  component: DealWatchList,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div style={{ maxWidth: 560 }}>
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof DealWatchList>;

const row = {
  contact_id: "01a02e25-a5ac-7099-8099-581cbf001a03",
  contact_name: "Ines Huber",
  occurred_at: "2026-09-01T08:00:00Z",
};

export const Watched: Story = {
  args: {
    onOpenEmail: () => {},
    commitments: {
      complete: true,
      has_more: false,
      data: [
        {
          ...row,
          id: "01a02e25-a5ac-7099-8099-581cbf001a02",
          body: "Send the purchase order",
          source_quote: "We will send the purchase order by Friday.",
          source_activity_id: "01a02e25-a5ac-7099-8099-581cbf001a01",
          source_kind: "email",
          due_at: "2026-09-04T21:59:59Z",
        },
        {
          ...row,
          id: "01a02e25-a5ac-7099-8099-581cbf001a05",
          body: "Confirm the rollout date with IT",
          source_quote: "I'll check the rollout date with our IT team.",
          source_activity_id: "01a02e25-a5ac-7099-8099-581cbf001a04",
          source_kind: "meeting",
          due_at: null,
        },
      ],
    },
  },
};

export const WatchedDark: Story = { ...Watched, globals: { theme: "dark" } };

export const PartlyHidden: Story = {
  args: { commitments: { complete: false, has_more: false, data: [] } },
};

export const MoreThanShown: Story = {
  args: {
    ...Watched.args,
    commitments: {
      // biome-ignore lint/style/noNonNullAssertion: Watched sets it above.
      ...Watched.args!.commitments!,
      has_more: true,
    },
  },
};
