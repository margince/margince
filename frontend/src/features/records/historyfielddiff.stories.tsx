// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { HistoryFieldDiff } from "./historyfielddiff";

// An activity time's change, each side read at its own precision. A side
// that knows only its day names that day and no clock.

const VALUES = { currency: null, locale: "en", zone: "Europe/Berlin" } as const;

const meta = {
  title: "Records/Record 360/Record history/Activity time change",
  component: HistoryFieldDiff,
  parameters: { layout: "padded" },
  render: (args) => (
    <StoryProviders>
      <HistoryFieldDiff {...args} />
    </StoryProviders>
  ),
} satisfies Meta<typeof HistoryFieldDiff>;
export default meta;

type Story = StoryObj<typeof meta>;

export const BothAtATime: Story = {
  args: {
    field: "occurred_at",
    oldValue: "2026-09-01T09:15:00Z",
    newValue: "2026-09-03T14:30:00Z",
    oldPrecision: "instant",
    newPrecision: "instant",
    values: VALUES,
  },
};

export const MovedToADay: Story = {
  args: {
    ...BothAtATime.args,
    newValue: "2026-09-03T12:00:00Z",
    newPrecision: "day",
  },
  play: async () => {
    const day = await screen.findByText(/2026/, {
      selector: ".field-diff-to",
    });
    await expect(day.textContent).not.toMatch(/\d{1,2}:\d{2}/);
    const time = document.querySelector(".field-diff-from");
    await expect(time?.textContent).toMatch(/\d{1,2}:\d{2}/);
  },
};
