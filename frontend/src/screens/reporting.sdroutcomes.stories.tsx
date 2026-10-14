import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../testing/story-utils";
import { sdrEvaluation } from "./reporting.scenarios";
import { SdrOutcomes } from "./reporting.sdroutcomes";
export default { title: "Records/Reports/Analytics/Sdr chart" } satisfies Meta;
const [chart] = sdrEvaluation.charts;
if (!chart) {
  throw new Error("the SDR evaluation fixture carries no chart");
}
export const Default: StoryObj = {
  render: () => (
    <StoryProviders>
      <SdrOutcomes
        chart={chart}
        evaluation={sdrEvaluation}
        onEvidence={() => {}}
      />
    </StoryProviders>
  ),
};
export const Dark: StoryObj = { ...Default, globals: { theme: "dark" } };
