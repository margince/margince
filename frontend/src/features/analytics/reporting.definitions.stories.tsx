import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { ReportingDefinitions } from "./reporting.definitions";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

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

export const Setup: Story = {
  ...Default,
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "Reporting setup" }),
    );
  },
};
