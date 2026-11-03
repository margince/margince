// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../../testing/story-utils";
import { DealViabilityView } from "./dealviabilitycard";

// Whether the customer still supports a deal. The state worth seeing is a
// review whose task is done and which is still open: a done task is not an
// answer, so the card keeps the review.

const meta: Meta<typeof DealViabilityView> = {
  title: "Records/Deal 360/Customer support",
  component: DealViabilityView,
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

type Story = StoryObj<typeof DealViabilityView>;

const base = {
  deal_id: "d-1",
  qualification_mode: "enforcing",
  disposition: "none",
  complete: true,
  assessed_at: "2026-11-02T08:00:00Z",
  close_date_provisional: false,
  close_date_overdue: false,
  stage_probability: 60,
} as const;

export const Supported: Story = {
  args: { viability: { ...base, qualification: "eligible" } },
};

export const ReviewOpenTaskDone: Story = {
  args: {
    viability: {
      ...base,
      qualification: "excluded",
      disposition: "open_review",
      review: {
        id: "r-1",
        opened_at: "2026-11-01T08:00:00Z",
        episode: 1,
        delivery_state: "delivered",
        task: { id: "t-1", done: true },
      },
    },
  },
};

export const NotChecked: Story = {
  args: {
    viability: {
      ...base,
      qualification_mode: "off",
      qualification: "unknown",
      complete: false,
    },
  },
};
