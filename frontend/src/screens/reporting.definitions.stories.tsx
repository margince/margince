import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReportingDefinitions } from "./reporting.definitions";
import { reportingStoryRoutes } from "./reporting.story-fixtures";
import { installFetchStub, StoryProviders } from "./story-utils";

const meta: Meta = { title: "Records/Reports/Analytics/Definitions" };
export default meta;
type Story = StoryObj;
export const Default: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <ReportingDefinitions />
      </StoryProviders>
    );
  },
};
