import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { reportingTargets } from "./reporting.scenarios";
import { reportingStoryRoutes } from "./reporting.story-fixtures";
import { ReportingTargets } from "./reporting.targets";

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

// A seat that reads targets but cannot set them, and holds none yet: who sets
// them, and no verb to add one.
export const ReaderEmpty: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes(undefined, true));
    return (
      <StoryProviders>
        <ReportingTargets />
      </StoryProviders>
    );
  },
};

export const RetiredAllocation: Story = {
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", {
        name: "Target retired",
      }),
    );
  },
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
