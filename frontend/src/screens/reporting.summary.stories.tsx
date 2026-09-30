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
