// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProbabilityRing } from "./probabilityring";

const meta: Meta<typeof ProbabilityRing> = {
  title: "Components/Text and data display/Probability ring",
  component: ProbabilityRing,
  parameters: { layout: "padded" },
};
export default meta;

// Every step a stage commonly sets, so the fill reads the same at each size.
function RingSteps() {
  return (
    <div style={{ display: "flex", gap: "var(--space-4)", flexWrap: "wrap" }}>
      {[0, 10, 25, 40, 50, 75, 90, 100].map((percent) => (
        <ProbabilityRing key={percent} percent={percent} />
      ))}
    </div>
  );
}

export const Steps: StoryObj = { render: () => <RingSteps /> };
export const StepsDark: StoryObj = {
  globals: { theme: "dark" },
  render: () => <RingSteps />,
};

// In a details row: the figure in the row's own type, beside a body value.
export const InRow: StoryObj = {
  render: () => (
    <p>
      Proposal · <ProbabilityRing percent={35} value />
    </p>
  ),
};
