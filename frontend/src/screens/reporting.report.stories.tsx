import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ReportingReportDetail } from "./reporting.report";
import {
  reportingEditions,
  reportingExecutions,
  reportingSchedule,
} from "./reporting.scenarios";
import { reportingStoryRoutes } from "./reporting.story-fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const meta: Meta = { title: "Records/Reports/Analytics/Saved report detail" };
export default meta;
type Story = StoryObj;
export const Default: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <ReportingReportDetail reportId="report" />
      </StoryProviders>
    );
  },
};

export const FrozenHistory: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/reports/report/editions": () =>
        jsonResponse({ data: reportingEditions }),
      "GET /analytics/editions/edition-september": () =>
        jsonResponse(reportingEditions[0]),
    });
    return (
      <StoryProviders>
        <ReportingReportDetail
          reportId="report"
          editionId="edition-september"
        />
      </StoryProviders>
    );
  },
};
export const Expired: Story = {
  render: () => {
    const expired = {
      ...reportingEditions[0],
      expired: true,
      redacted: true,
      withheld: true,
      evaluation: {
        ...reportingEditions[0].evaluation,
        charts: [],
        metrics: [],
      },
    };
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/editions/edition-september": () => jsonResponse(expired),
    });
    return (
      <StoryProviders>
        <ReportingReportDetail
          reportId="report"
          editionId="edition-september"
        />
      </StoryProviders>
    );
  },
};

export const RunFailures: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/reports/report/schedules": () =>
        jsonResponse({ data: [reportingSchedule] }),
      "GET /analytics/reports/report/executions": () =>
        jsonResponse({ data: reportingExecutions }),
    });
    return (
      <StoryProviders>
        <ReportingReportDetail reportId="report" />
      </StoryProviders>
    );
  },
};

export const ArchiveConfirmation: Story = {
  ...Default,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Archive report",
      }),
    );
  },
};
