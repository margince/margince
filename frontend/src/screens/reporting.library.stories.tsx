import type { Meta, StoryObj } from "@storybook/react-vite";
import { installFetchStub, StoryProviders } from "../testing/story-utils";
import { ReportingLibrary } from "./reporting.library";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

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
