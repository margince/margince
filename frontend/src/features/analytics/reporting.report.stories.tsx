import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ReportingReportDetail } from "./reporting.report";
import {
  reportingEditions,
  reportingExecutions,
  reportingSchedule,
} from "./reporting.scenarios";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

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
        ...reportingEditions[0]?.evaluation,
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

export const CapturingAndFailures: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/reports/report/schedules": () =>
        jsonResponse({ data: [reportingSchedule] }),
      "GET /analytics/reports/report/executions": () =>
        jsonResponse({
          data: [
            {
              ...reportingExecutions[0],
              id: "queued",
              status: "pending",
              edition_id: undefined,
            },
            ...reportingExecutions,
          ],
        }),
    });
    return (
      <StoryProviders>
        <ReportingReportDetail reportId="report" />
      </StoryProviders>
    );
  },
};

// A failed history read opens the section once, and the reader may close it.
export const HistoryReadFailed: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/reports/report/editions": () =>
        jsonResponse({ title: "Server error", status: 500 }, 500),
    });
    return (
      <StoryProviders>
        <ReportingReportDetail reportId="report" />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const name = await within(canvasElement).findByText(
      "History and schedules",
    );
    const summary = name.closest("summary");
    if (summary === null) throw new Error("the history has no summary");
    await waitFor(() =>
      expect(summary).toHaveAttribute("aria-expanded", "true"),
    );
    await userEvent.click(summary);
    await expect(summary).toHaveAttribute("aria-expanded", "false");
  },
};

export const ArchiveConfirmation: Story = {
  ...Default,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Report actions",
      }),
    );
    await userEvent.click(
      await screen.findByRole("button", {
        name: "Archive report",
      }),
    );
  },
};
