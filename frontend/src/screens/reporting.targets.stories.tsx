import type { Meta, StoryObj } from "@storybook/react-vite";
import { reportingTargets } from "./reporting.scenarios";
import { reportingStoryRoutes } from "./reporting.story-fixtures";
import { ReportingTargets } from "./reporting.targets";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const meta: Meta = { title: "Records/Reports/Analytics/Targets" };
export default meta;
type Story = StoryObj;
export const Default: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/targets": () => jsonResponse({ data: reportingTargets }),
    });
    return (
      <StoryProviders>
        <ReportingTargets />
      </StoryProviders>
    );
  },
};

export const Empty: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <ReportingTargets />
      </StoryProviders>
    );
  },
};

export const RetiredAllocation: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/targets": () =>
        jsonResponse({
          data: reportingTargets.map((target) => ({
            ...target,
            definition: {
              ...target.definition,
              retired: true,
              reason: "Incorrect allocation retired",
            },
          })),
        }),
    });
    return (
      <StoryProviders>
        <ReportingTargets />
      </StoryProviders>
    );
  },
};
