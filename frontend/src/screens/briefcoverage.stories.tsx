// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ranked, readingsDay } from "./brief.fixtures";
import { BriefCoverage } from "./briefcoverage";
import { StoryProviders } from "./story-utils";

const meta: Meta<typeof BriefCoverage> = {
  title: "Shell/Home coverage",
  component: BriefCoverage,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof BriefCoverage>;

export const FailedTasks: Story = {
  args: {
    day: {
      ...readingsDay(),
      sources_unavailable: [
        { source: "task", reason: "failed", category: "tasks" },
      ],
    },
  },
};
export const TwoFailedSources: Story = {
  args: {
    day: {
      ...readingsDay(),
      sources_unavailable: [
        { source: "task", reason: "failed", category: "tasks" },
        {
          source: "customer_waiting",
          reason: "failed",
          category: "customer_waiting",
        },
      ],
    },
  },
};
// A grant withheld the source: it is named, and there is no retry, because
// pressing one would never return it.
export const WithheldSource: Story = {
  args: {
    day: {
      ...readingsDay(),
      sources_unavailable: [{ source: "dsr", reason: "withheld" }],
    },
    onRetry: () => {},
  },
};
// A grant withheld the factor, so there is nothing to ask for again and no
// retry beside the line. The failed-source arm below is what shows the two
// absences sharing one list without the button following the wrong one.
export const WithheldFactor: Story = {
  args: {
    day: readingsDay(),
    run: { ...ranked, factors_omitted: ["warmth"] },
  },
};
export const WithheldFactorAndFailedSource: Story = {
  args: {
    day: {
      ...readingsDay(),
      sources_unavailable: [
        { source: "task", reason: "failed", category: "tasks" },
      ],
    },
    run: { ...ranked, factors_omitted: ["warmth"] },
    onRetry: () => {},
  },
};
