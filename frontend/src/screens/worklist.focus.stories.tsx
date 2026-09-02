// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { StoryProviders } from "./story-utils";
import { FocusCard } from "./worklist.focus";

// The queue's top row, lifted out and drawn as itself — see worklist.focus.tsx
// for why this exists separately from the row it duplicates.

type WorklistItem = components["schemas"]["WorklistItem"];

const meta: Meta<typeof FocusCard> = {
  title: "Records/Worklist/Focus card",
  component: FocusCard,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof FocusCard>;

export const ADealAtRisk: Story = {
  args: {
    item: {
      id: "deal-1",
      source: "deal_at_risk",
      category: "deals_at_risk",
      level: 3,
      consequence: "deal_slips_past_close",
      title: "Acme Expansion",
      overdue: true,
      because: [
        {
          kind: "material",
          value: { kind: "money", minor: 16010000, currency: "EUR" },
        },
        { kind: "quiet_days", value: { kind: "days", days: 83 } },
      ],
      actions: ["open"],
      primary_action: "open",
      subject: {
        type: "deal",
        id: "01a05500-0000-7000-8000-000000000010",
        label: "Acme Expansion",
      },
    } satisfies WorklistItem,
  },
};
