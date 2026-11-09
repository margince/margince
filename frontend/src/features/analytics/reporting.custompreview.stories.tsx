import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { holdCustomReport } from "./reporting.custombuilder";
import { CustomReportPreview } from "./reporting.custompreview";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const meta: Meta = {
  title: "Records/Reports/Analytics/Custom report preview",
  beforeEach: () => {
    installFetchStub(reportingStoryRoutes());
    holdCustomReport(meFixture().user.id, {
      name: "Sales review",
      description: "Quarterly report",
      audience: "private",
      selection: reportingStoryEvaluation.selection,
    });
  },
};
export default meta;
type Story = StoryObj;
export const Default: Story = {
  render: () => (
    <StoryProviders>
      <CustomReportPreview />
    </StoryProviders>
  ),
};
export const Dark: Story = { ...Default, globals: { theme: "dark" } };
