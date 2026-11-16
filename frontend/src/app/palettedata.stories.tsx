// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  reportingCatalogClient,
  reportingLeadsEvaluation,
} from "../features/analytics/reporting.story-fixtures";
import { StoryProviders } from "../testing/story-utils";
import { PaletteDataAnswer } from "./palettedata";

const meta: Meta<typeof PaletteDataAnswer> = {
  title: "Shell/Data answer",
  component: PaletteDataAnswer,
};
export default meta;
type Story = StoryObj<typeof PaletteDataAnswer>;

export const Metric: Story = {
  render: () => (
    <StoryProviders client={reportingCatalogClient()}>
      <PaletteDataAnswer
        answer={{
          outcome: "answered",
          source: JSON.stringify(reportingLeadsEvaluation.selection),
          evaluation: reportingLeadsEvaluation,
          count: false,
        }}
        onLeave={() => {}}
      />
    </StoryProviders>
  ),
};
export const MetricDark: Story = { ...Metric, globals: { theme: "dark" } };
export const EmptyRecords: Story = {
  render: () => (
    <StoryProviders>
      <PaletteDataAnswer
        answer={{
          outcome: "answered",
          source: '{"plan":{"target":"deal"}}',
          records: {
            rows: [],
            coverage: "complete_exact",
            executed_plan: "Open deals closing this month.",
            limit: 50,
            notes: [],
          },
        }}
        onLeave={() => {}}
      />
    </StoryProviders>
  ),
};
export const EmptyRecordsDark: Story = {
  ...EmptyRecords,
  globals: { theme: "dark" },
};
