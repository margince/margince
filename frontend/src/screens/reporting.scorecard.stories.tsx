import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReportingScorecard } from "./reporting.scorecard";
import { reportingStoryEvaluation } from "./reporting.story-fixtures";
import { StoryProviders } from "./story-utils";
export default {
  title: "Records/Reports/Analytics/Owner scorecard",
} satisfies Meta;
export const Default: StoryObj = {
  render: () => (
    <StoryProviders>
      <ReportingScorecard
        evaluation={reportingStoryEvaluation}
        onEvidence={() => {}}
      />
    </StoryProviders>
  ),
};
