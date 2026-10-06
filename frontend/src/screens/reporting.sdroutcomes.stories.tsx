import type { Meta, StoryObj } from "@storybook/react-vite";
import { sdrEvaluation } from "./reporting.scenarios";
import { SdrOutcomes } from "./reporting.sdroutcomes";
import { StoryProviders } from "./story-utils";
export default { title: "Records/Reports/Analytics/Sdr chart" } satisfies Meta;
export const Default: StoryObj = {
  render: () => (
    <StoryProviders>
      <SdrOutcomes
        chart={sdrEvaluation.charts[0]}
        evaluation={sdrEvaluation}
        onEvidence={() => {}}
      />
    </StoryProviders>
  ),
};
export const Dark: StoryObj = { ...Default, globals: { theme: "dark" } };
