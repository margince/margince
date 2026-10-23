// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { UTC_ZONE } from "../../format/timezone";
import { StoryProviders } from "../../testing/story-utils";
import { HistoryFieldDiff } from "./historyfielddiff";

const meta = {
  title: "Records/Record 360/History field values",
  component: HistoryFieldDiff,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: {
    field: "amount_minor",
    oldValue: "2500000",
    newValue: "4150000",
    values: { currency: "EUR", locale: "en", zone: UTC_ZONE },
  },
} satisfies Meta<typeof HistoryFieldDiff>;
export default meta;
type Story = StoryObj<typeof meta>;

export const RecordedCurrency: Story = {};
export const MissingCurrency: Story = {
  args: { values: { currency: null, locale: "en", zone: UTC_ZONE } },
};
export const Timestamp: Story = {
  args: {
    field: "occurred_at",
    oldValue: "2026-10-20T08:30:00Z",
    newValue: "2026-10-20T09:30:00Z",
  },
};
