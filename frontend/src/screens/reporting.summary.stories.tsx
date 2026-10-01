import type { Meta, StoryObj } from "@storybook/react-vite";
import { reportingStoryEvaluation } from "./reporting.story-fixtures";
import { ResultsSummary } from "./reporting.summary";
import { StoryProviders } from "./story-utils";

const meta: Meta = { title: "Records/Reports/Analytics/Outcome summary" };
export default meta;
type Story = StoryObj;
export const Sales: Story = {
  render: () => (
    <StoryProviders>
      <ResultsSummary
        evaluation={reportingStoryEvaluation}
        onEvidence={() => {}}
      />
    </StoryProviders>
  ),
};
export const Dark: Story = { ...Sales, globals: { theme: "dark" } };

export const NoSalesThisPeriod: Story = {
  render: () => (
    <StoryProviders>
      <ResultsSummary
        evaluation={{
          ...reportingStoryEvaluation,
          metrics: reportingStoryEvaluation.metrics.map((metric) => ({
            ...metric,
            value: 0,
            coverage: {
              ...metric.coverage,
              status: "no_data",
              eligible_count: 0,
            },
            target: undefined,
            target_actual: undefined,
          })),
        }}
        onEvidence={() => {}}
      />
    </StoryProviders>
  ),
};

export const NoSalesThisPeriodDark: Story = {
  ...NoSalesThisPeriod,
  globals: { theme: "dark" },
};
