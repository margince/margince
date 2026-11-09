import type { Meta, StoryObj } from "@storybook/react-vite";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { sdrEvaluation } from "./reporting.scenarios";
import { SdrOutcomes } from "./reporting.sdroutcomes";
import { reportingStoryRoutes } from "./reporting.story-fixtures";
export default { title: "Records/Reports/Analytics/Sdr chart" } satisfies Meta;
const [chart] = sdrEvaluation.charts;
if (!chart) {
  throw new Error("the SDR evaluation fixture carries no chart");
}
export const Default: StoryObj = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <SdrOutcomes
          chart={chart}
          evaluation={sdrEvaluation}
          onEvidence={() => {}}
        />
      </StoryProviders>
    );
  },
};
export const Dark: StoryObj = { ...Default, globals: { theme: "dark" } };
