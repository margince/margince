// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "../design-system/atoms";
import { StoryProviders } from "../testing/story-utils";
import { GoneNotice } from "./gonenotice";

// What a shared link, saved report or booking link draws when its read answers
// 404, 410 or 422. It shows the screen's own sentence, and its way on if it has one.
const meta: Meta<typeof GoneNotice> = {
  title: "Patterns/Link not there",
  component: GoneNotice,
  parameters: { layout: "padded" },
  render: (args) => (
    <StoryProviders>
      <div className="wrap">
        <GoneNotice {...args} />
      </div>
    </StoryProviders>
  ),
};
export default meta;

type Story = StoryObj<typeof GoneNotice>;

export const BookingLink: Story = { args: { messageKey: "book.linkGone" } };

export const SavedReport: Story = {
  args: {
    messageKey: "reporting.reportGone",
    action: <Button>Back to Analytics</Button>,
  },
};
