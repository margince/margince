// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Card, PendingBody } from "./atoms";

const meta = {
  title: "Components/Loading/Pending body",
  component: PendingBody,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <Card>
        <Story />
      </Card>
    ),
  ],
} satisfies Meta<typeof PendingBody>;
export default meta;

type Story = StoryObj<typeof meta>;

// Not `delayMs`: it renders nothing until the delay passes, and an empty frame
// pictures no state at all.

export const Spoken: Story = {
  args: { label: "Loading the review queue…" },
};

export const Visible: Story = {
  args: { label: "Loading the review queue…", visible: true },
};

// Eight is the ceiling: a wait that needs more room is a shape, not more bars.
export const FullReservation: Story = {
  args: { label: "Loading the contract terms…", lines: 8 },
};
