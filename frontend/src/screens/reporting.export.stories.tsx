import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReportingExportButton } from "./reporting.export";
import { reportingStoryEvaluation } from "./reporting.story-fixtures";
import { StoryProviders } from "./story-utils";
export default {
  title: "Records/Reports/Analytics/Export report",
} satisfies Meta;
export const Default: StoryObj = {
  render: () => (
    <StoryProviders>
      <ReportingExportButton evaluation={reportingStoryEvaluation} />
    </StoryProviders>
  ),
};
