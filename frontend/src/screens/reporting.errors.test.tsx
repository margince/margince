/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { ReportingComparison } from "./reporting.comparison";
import { ReportingDefinitions } from "./reporting.definitions";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { ReportingExecutions } from "./reporting.executions";
import { ReportingExportButton } from "./reporting.export";
import { REPORTING_FIXTURE_ZONE } from "./reporting.fixtures";
import { ReportingForecastGraphs } from "./reporting.forecast";
import { ReportingLibrary } from "./reporting.library";
import { ReportingReportDetail } from "./reporting.report";
import { reportingEditions, reportingSchedule } from "./reporting.scenarios";
import { ReportingScheduleDialog } from "./reporting.schedule";
import {
  reportingStoryEvaluation,
  reportingStoryReport,
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";
import { ReportingTargets } from "./reporting.targets";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
  window.location.hash = "";
});
const denied = () =>
  jsonResponse({ status: 403, detail: "Reporting access has changed" }, 403);
const cases: { name: string; route: string; view: ReactNode }[] = [
  {
    name: "target list",
    route: "GET /analytics/targets",
    view: <ReportingTargets />,
  },
  {
    name: "metric definitions",
    route: "GET /analytics/metrics",
    view: <ReportingDefinitions />,
  },
  {
    name: "report library",
    route: "GET /analytics/reports",
    view: <ReportingLibrary />,
  },
  {
    name: "saved definition",
    route: "GET /analytics/reports/report",
    view: <ReportingReportDetail reportId="report" />,
  },
  {
    name: "live report",
    route: "GET /analytics/reports/report/evaluation",
    view: <ReportingReportDetail reportId="report" />,
  },
  {
    name: "edition list",
    route: "GET /analytics/reports/report/editions",
    view: <ReportingReportDetail reportId="report" />,
  },
  {
    name: "schedule list",
    route: "GET /analytics/reports/report/schedules",
    view: <ReportingReportDetail reportId="report" />,
  },
  {
    name: "frozen edition",
    route: "GET /analytics/editions/missing",
    view: <ReportingReportDetail reportId="report" editionId="missing" />,
  },
  {
    name: "execution history",
    route: "GET /analytics/reports/report/executions",
    view: <ReportingExecutions reportId="report" canRetry={false} />,
  },
  {
    name: "comparison",
    route: "GET /analytics/editions/compare",
    view: (
      <ReportingComparison editions={reportingEditions} onClose={() => {}} />
    ),
  },
  {
    name: "forecast",
    route: "GET /analytics/evaluate",
    view: <ReportingForecastGraphs scope={reportingStoryScope} />,
  },
];
it.each(cases)(
  "shows the refusal instead of a successful empty $name",
  async ({ route, view }) => {
    installFetchStub({ ...reportingStoryRoutes(), [route]: denied });
    render(<StoryProviders>{view}</StoryProviders>);
    expect(
      await screen.findByText("Reporting access has changed"),
    ).toBeVisible();
  },
);

it("keeps a rejected schedule edit open with the entered cadence", async () => {
  const user = userEvent.setup({ delay: null });
  const close = vi.fn();
  installFetchStub({
    ...reportingStoryRoutes(),
    "PATCH /analytics/schedules/schedule": () =>
      jsonResponse(
        { status: 409, detail: "Refresh the schedule before editing it" },
        409,
      ),
  });
  render(
    <StoryProviders>
      <ReportingScheduleDialog
        report={reportingStoryReport}
        schedule={reportingSchedule}
        timezone={REPORTING_FIXTURE_ZONE}
        onClose={close}
      />
    </StoryProviders>,
  );
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Activate schedule" }),
    ).toBeEnabled(),
  );
  await user.click(screen.getByRole("button", { name: "Activate schedule" }));
  expect(
    await screen.findByText("Refresh the schedule before editing it"),
  ).toBeVisible();
  expect(screen.getByRole("combobox", { name: "Frequency" })).toHaveTextContent(
    "Monthly",
  );
  expect(close).not.toHaveBeenCalled();
});

it("refreshes stale live evidence rather than silently displaying new source rows", async () => {
  const user = userEvent.setup({ delay: null });
  const close = vi.fn();
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/evidence": () =>
      jsonResponse(
        {
          status: 409,
          code: "version_skew",
          detail: "Refresh the evaluated reading",
        },
        409,
      ),
  });
  render(
    <StoryProviders>
      <ReportingEvidenceDrawer
        evaluation={reportingStoryEvaluation}
        reference={{ metric: "bookings_won", context_id: "interval" }}
        onClose={close}
      />
    </StoryProviders>,
  );
  expect(await screen.findByText(en["reporting.evidenceStale"])).toBeVisible();
  expect(screen.queryByText("Refresh the evaluated reading")).toBeNull();
  await user.click(screen.getByRole("button", { name: "Retry" }));
  await waitFor(() => expect(close).toHaveBeenCalledOnce());
});

it.each([undefined, "edition-september"])(
  "surfaces export failure for %s without reporting success",
  async (editionId) => {
    const user = userEvent.setup({ delay: null });
    const route = editionId
      ? `GET /analytics/editions/${editionId}/export.csv`
      : "GET /analytics/evaluate.csv";
    installFetchStub({ ...reportingStoryRoutes(), [route]: denied });
    render(
      <StoryProviders>
        <ReportingExportButton
          evaluation={reportingStoryEvaluation}
          editionId={editionId}
        />
      </StoryProviders>,
    );
    await user.click(screen.getByRole("button", { name: "Export CSV" }));
    expect(
      await screen.findByText("Reporting access has changed"),
    ).toBeVisible();
  },
);

it("says a stale export reloads the report, not that a record changed", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/evaluate.csv": () =>
      jsonResponse(
        {
          status: 409,
          code: "version_skew",
          detail: "Refresh the evaluated reading",
        },
        409,
      ),
  });
  render(
    <StoryProviders>
      <ReportingExportButton evaluation={reportingStoryEvaluation} />
    </StoryProviders>,
  );
  await user.click(screen.getByRole("button", { name: "Export CSV" }));
  expect(await screen.findByText(en["reporting.exportStale"])).toBeVisible();
  expect(screen.queryByText(en["common.versionSkew"])).toBeNull();
  await user.click(screen.getByRole("button", { name: "Retry" }));
  await waitFor(() =>
    expect(screen.queryByText(en["reporting.exportStale"])).toBeNull(),
  );
});

it("shows a refused reporting setup instead of an empty editor", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/framework": denied,
  });
  render(
    <StoryProviders>
      <ReportingDefinitions />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Reporting setup" }),
  );
  expect(await screen.findByText("Reporting access has changed")).toBeVisible();
});

it("does not describe an old snapshot with the current ownership rule", async () => {
  const user = userEvent.setup({ delay: null });
  const evaluation = {
    ...reportingStoryEvaluation,
    metrics: reportingStoryEvaluation.metrics.map((metric) => ({
      ...metric,
      version: "1",
    })),
  };
  const routes = reportingStoryRoutes(evaluation);
  installFetchStub({
    ...routes,
    "GET /analytics/editions/old/evidence": routes["GET /analytics/evidence"],
  });
  render(
    <StoryProviders>
      <ReportingEvidenceDrawer
        evaluation={evaluation}
        editionId="old"
        reference={{ metric: "bookings_won", context_id: "interval" }}
        onClose={() => {}}
      />
    </StoryProviders>,
  );
  await screen.findByText("Northstar rollout");
  await user.click(
    screen.getByRole("button", { name: "How this is measured" }),
  );
  expect(
    await screen.findByText(
      "This snapshot uses an earlier metric definition. Its saved figures have not been recalculated.",
    ),
  ).toBeVisible();
  expect(
    screen.queryByText(
      "Won deals grouped by their current owner, in the selected close interval.",
    ),
  ).not.toBeInTheDocument();
});

it("closes the evidence drawer when opening its underlying deal", async () => {
  const user = userEvent.setup({ delay: null });
  const close = vi.fn();
  installFetchStub(reportingStoryRoutes());
  render(
    <StoryProviders>
      <ReportingEvidenceDrawer
        evaluation={reportingStoryEvaluation}
        reference={{ metric: "bookings_won", context_id: "interval" }}
        onClose={close}
      />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", {
      name: "Northstar rollout",
    }),
  );
  expect(close).toHaveBeenCalledOnce();
  expect(window.location.hash).toBe(
    "#/deals/00000000-0000-4000-8000-000000000012",
  );
});
