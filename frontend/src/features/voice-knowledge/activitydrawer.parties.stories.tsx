// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { AttendeesLine } from "./activitydrawer.parties";
import "../../design-system/emaildetail.css";

const meta: Meta<typeof AttendeesLine> = {
  title: "Records/Record 360/Meeting attendees",
  component: AttendeesLine,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: {
    attendees: [],
    answers: new Map(),
    guests: [
      {
        address: "pat@example.test",
        display_name: "Pat",
        response: "accepted",
      },
      {
        address: "lee@example.test",
        display_name: "Lee",
        response: "declined",
      },
      {
        address: "sam@example.test",
        display_name: "Sam",
        response: "tentative",
      },
      {
        address: "jo@example.test",
        display_name: "Jo",
        response: "needs_action",
      },
    ],
  },
};
export default meta;
type Story = StoryObj<typeof AttendeesLine>;

export const Responses: Story = {
  play: async ({ canvasElement }) => {
    const page = within(canvasElement);
    await expect(page.getByText("Declined")).toBeVisible();
    await expect(page.getByText("Maybe")).toBeVisible();
    await expect(page.getByText("No answer")).toBeVisible();
    await expect(page.queryByText("Accepted")).not.toBeInTheDocument();
  },
};

export const MoreGuests: Story = { args: { leftOut: 3 } };
