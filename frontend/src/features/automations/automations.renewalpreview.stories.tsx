// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { RenewalPreviewDetails } from "./automations.renewalpreview";

const meta: Meta<typeof RenewalPreviewDetails> = {
  title: "Settings/Automations/Renewal preview",
  component: RenewalPreviewDetails,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof RenewalPreviewDetails>;
const renewalDates = {
  missing_dates: 2,
  recurs_yearly: false,
  samples: [
    {
      entity_id: "customer-1",
      occurrence_date: "2027-01-05",
      reminder_date: "2026-12-06",
    },
  ],
};

export const OneDate: Story = {
  args: { renewal: renewalDates },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText("2 visible records have no renewal date"),
    ).toBeVisible();
    await expect(within(canvasElement).getByText(/^Remind on /)).toBeVisible();
  },
};
export const OneDateDark: Story = { ...OneDate, globals: { theme: "dark" } };
export const Annual: Story = {
  ...OneDate,
  args: { renewal: { ...renewalDates, recurs_yearly: true } },
};
export const AnnualDark: Story = { ...Annual, globals: { theme: "dark" } };
export const NoDates: Story = {
  args: { renewal: { missing_dates: 4, recurs_yearly: false, samples: [] } },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText("4 visible records have no renewal date"),
    ).toBeVisible();
  },
};
export const NoDatesDark: Story = { ...NoDates, globals: { theme: "dark" } };
