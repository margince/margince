import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { screen } from "storybook/test";
import { REPORTING_FIXTURE_ZONE } from "./reporting.fixtures";
import { reportingSchedule } from "./reporting.scenarios";
import { ReportingScheduleDialog } from "./reporting.schedule";
import {
  reportingStoryCatalog,
  reportingStoryReport,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const meta: Meta = { title: "Records/Reports/Analytics/Schedule report" };
export default meta;
type Story = StoryObj;
function Preview() {
  const [open, setOpen] = useState(true);
  return open ? (
    <ReportingScheduleDialog
      report={reportingStoryReport}
      timezone={REPORTING_FIXTURE_ZONE}
      onClose={() => setOpen(false)}
    />
  ) : (
    <p>Dialog closed</p>
  );
}
export const Default: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <Preview />
      </StoryProviders>
    );
  },
  play: async () => {
    await screen.findByRole("dialog");
  },
};

export const MonthlyPaused: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <ReportingScheduleDialog
          report={reportingStoryReport}
          schedule={reportingSchedule}
          timezone={REPORTING_FIXTURE_ZONE}
          onClose={() => {}}
        />
      </StoryProviders>
    );
  },
};

export const RetentionRequired: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/metrics": () =>
        jsonResponse({ ...reportingStoryCatalog, schedule_ready: false }),
    });
    return (
      <StoryProviders>
        <Preview />
      </StoryProviders>
    );
  },
};

export const ReadinessLoading: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/metrics": () => new Promise<Response>(() => undefined),
    });
    return (
      <StoryProviders>
        <Preview />
      </StoryProviders>
    );
  },
};
export const ReadinessLoadingDark: Story = {
  ...ReadinessLoading,
  globals: { theme: "dark" },
};
