// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StageStrip, type StripStep } from "./stagestrip";

// A pipeline's shape: open stages shaded by their odds, the ways out at the
// end. The full strip heads a pipeline's page; the compact one rides a row in
// a list of pipelines.

function step(
  key: string,
  name: string,
  probability: number,
  outcome?: "won" | "lost",
): StripStep {
  return { key, name, probability, reading: `${probability}%`, outcome };
}

const sales: readonly StripStep[] = [
  step("q", "Qualified", 10),
  step("d", "Discovery", 25),
  step("p", "Proposal", 50),
  step("n", "Negotiation", 75),
  step("w", "Won", 100, "won"),
  step("l", "Lost", 0, "lost"),
];

const enterprise: readonly StripStep[] = [
  step("q", "Qualified", 5),
  step("t", "Technical validation", 30),
  step("b", "Business case", 45),
  step("p", "Procurement", 75),
  step("g", "Legal review", 90),
  step("w", "Won", 100, "won"),
  step("l", "Lost", 0, "lost"),
];

const meta: Meta<typeof StageStrip> = {
  title: "Components/Status indicators/Stage strip",
  component: StageStrip,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 680 }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof StageStrip>;

/** The full strip: every step named, darkening toward the close. */
export const Full: Story = {
  args: { label: "How a deal moves", steps: sales },
};

/** A long ladder: names give way to an ellipsis before the strip overflows. */
export const LongLadder: Story = {
  args: { label: "How a deal moves", steps: enterprise },
};

/** No open stage yet: the run reads as waiting, the ways out already there. */
export const NoOpenStages: Story = {
  args: {
    label: "How a deal moves",
    empty: "No open stages yet",
    steps: sales.filter((s) => s.outcome !== undefined),
  },
};

/** Compact, as a list of pipelines carries it: two ladders told apart. */
export const Compact: Story = {
  render: () => (
    <div style={{ display: "grid", gap: "var(--space-3)" }}>
      <StageStrip compact label="Sales" steps={sales} />
      <StageStrip compact label="Enterprise" steps={enterprise} />
    </div>
  ),
};

/** Both sizes in dark, where the shading and the outcome plates move. */
export const Dark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <div style={{ display: "grid", gap: "var(--space-4)" }}>
      <StageStrip label="How a deal moves" steps={sales} />
      <StageStrip compact label="Enterprise" steps={enterprise} />
    </div>
  ),
};
