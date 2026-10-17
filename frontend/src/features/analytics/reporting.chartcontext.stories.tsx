import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { ChartContext } from "./reporting.chartcontext";
import { reportingStoryEvaluation } from "./reporting.story-fixtures";
export default {
  title: "Records/Reports/Analytics/Chart context",
} satisfies Meta;
export const PartialCoverage: StoryObj = {
  render: () => {
    const [chart] = reportingStoryEvaluation.charts;
    if (!chart) throw new Error("the reporting fixture carries no chart");
    return (
      <StoryProviders>
        <ChartContext
          chart={{
            ...chart,
            coverage: {
              status: "partial",
              withheld: false,
              reason: "Some records have no recorded owner.",
            },
          }}
          evaluation={reportingStoryEvaluation}
        />
      </StoryProviders>
    );
  },
};
