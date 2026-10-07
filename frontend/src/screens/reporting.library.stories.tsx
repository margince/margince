import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReportingLibrary } from "./reporting.library";
import { reportingStoryRoutes } from "./reporting.story-fixtures";
import { installFetchStub, StoryProviders } from "./story-utils";

const meta: Meta = { title: "Records/Reports/Analytics/Saved reports" };
export default meta;
type Story = StoryObj;
export const Default: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <ReportingLibrary />
      </StoryProviders>
    );
  },
};
