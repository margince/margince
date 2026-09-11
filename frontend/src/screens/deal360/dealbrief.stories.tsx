// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../story-utils";
import { DealBrief } from "./dealbrief";

// The human-authored brief on a deal page.
//
// The states worth seeing are the ones a dropped distinction would render the
// same: an ABSENT brief against an empty-string one (both must draw nothing,
// not an empty panel), and a long brief clamped against a short one shown
// whole — a clamp applied to text that already fits offers a "Read more" that
// reveals nothing.

const meta: Meta<typeof DealBrief> = {
  title: "Records/Deal brief",
  component: DealBrief,
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

type Story = StoryObj<typeof DealBrief>;

export const Short: Story = {
  args: {
    brief:
      "They run payroll for about 400 seasonal staff and close the month by hand. They want the close down to two days.",
  },
};

export const Long: Story = {
  args: {
    brief: [
      "They run payroll for about 400 seasonal staff across six sites.",
      "Month-end close is manual and takes nine working days.",
      "The finance lead wants it under three, and has board attention on it.",
      "Their current vendor cannot export line-level data.",
      "Procurement needs two references in the same sector.",
      "Security review is expected to take three weeks.",
      "They asked for a phased rollout starting with one site.",
    ].join("\n"),
  },
};

// Renders NOTHING. An empty panel would say the deal has no brief where the
// truth is that nobody wrote one, and it would sit on every unbriefed deal.
export const Absent: Story = { args: { brief: null } };

// The same, for a stored empty string: whitespace is not a brief.
export const Blank: Story = { args: { brief: "   " } };
