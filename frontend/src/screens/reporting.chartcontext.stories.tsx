import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChartContext } from "./reporting.chartcontext";
import { reportingStoryEvaluation } from "./reporting.story-fixtures";
import { StoryProviders } from "./story-utils";
export default {
  title: "Records/Reports/Analytics/Chart context",
} satisfies Meta;
export const PartialCoverage: StoryObj = {
  render: () => (
    <StoryProviders>
      <ChartContext
        chart={{
          ...reportingStoryEvaluation.charts[0],
          coverage: {
            status: "partial",
            withheld: false,
            reason: "Some records have no recorded owner.",
          },
        }}
        evaluation={reportingStoryEvaluation}
      />
    </StoryProviders>
  ),
};
