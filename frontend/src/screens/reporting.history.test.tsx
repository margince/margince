/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { pickOption } from "../design-system/select-testing";
import { ReportingComparison } from "./reporting.comparison";
import { ReportingExecutions } from "./reporting.executions";
import { ReportingLibrary } from "./reporting.library";
import { ReportingReportDetail } from "./reporting.report";
import { SaveReportingDialog } from "./reporting.save";
import {
  reportingEditions,
  reportingExecutions,
  reportingSchedule,
} from "./reporting.scenarios";
import {
  reportingStoryReport,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
  window.location.hash = "";
});

it("filters saved reports by schedules and opens the persisted layout", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/reports": () =>
      jsonResponse({
        data: [
          {
            ...reportingStoryReport,
            edition_count: 2,
            latest_captured_at: "2026-10-01T07:00:00Z",
            cadence: "weekly, monthly",
            next_due_at: "2026-10-08T07:00:00Z",
            last_status: "succeeded",
          },
        ],
      }),
  });
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <ReportingLibrary />
    </StoryProviders>,
  );
  expect(await screen.findByText("Weekly, Monthly")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Scheduled" }));
  await waitFor(() =>
    expect(
      fetch.mock.calls.some(
        ([input]) =>
          input instanceof Request && input.url.includes("scheduled=true"),
      ),
    ).toBe(true),
  );
  await user.click(
    screen.getByRole("button", { name: "DACH operating review" }),
  );
  await screen.findByRole("heading", { name: "DACH operating review" });
  expect(window.location.hash).toContain("report");
});

it("compares frozen editions and opens evidence from the selected side", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/editions/compare": () =>
      jsonResponse({
        left: reportingEditions[1],
        right: reportingEditions[0],
        compatible: true,
        deltas: [{ metric: "bookings_won", absolute: 4320000, percentage: 25 }],
      }),
    "GET /analytics/editions/edition-august/evidence": () =>
      jsonResponse({
        context: reportingEditions[1].evaluation.context,
        metric: "bookings_won",
        rows: [
          {
            key: "deal",
            label: "August purchase order",
            value: 17280000,
            restricted: false,
          },
        ],
        truncated: false,
      }),
  });
  render(
    <StoryProviders>
      <ReportingComparison editions={reportingEditions} onClose={() => {}} />
    </StoryProviders>,
  );
  await screen.findByRole("heading", { name: "Bookings won" });
  expect(screen.getByText(/25%/)).toBeVisible();
  const bar = document.querySelector<HTMLButtonElement>(".report-chart-column");
  if (!bar) throw new Error("Missing comparison observation");
  await user.click(bar);
  expect(await screen.findByText("August purchase order")).toBeVisible();
});

it("shows incompatible populations without a misleading delta", async () => {
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/editions/compare": () =>
      jsonResponse({
        left: reportingEditions[1],
        right: reportingEditions[0],
        compatible: false,
        reason: "Team membership changed",
        deltas: [],
      }),
  });
  render(
    <StoryProviders>
      <ReportingComparison editions={reportingEditions} onClose={() => {}} />
    </StoryProviders>,
  );
  expect(await screen.findByText("Team membership changed")).toBeVisible();
  expect(screen.queryByText(/25%/)).not.toBeInTheDocument();
});

it("renders captured editions without requesting the current live evaluation", async () => {
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/editions/edition-september": () =>
      jsonResponse({ ...reportingEditions[0], redacted: true }),
    "GET /analytics/reports/report/editions": () =>
      jsonResponse({ data: reportingEditions }),
    "GET /analytics/reports/report/schedules": () =>
      jsonResponse({ data: [reportingSchedule] }),
  });
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <ReportingReportDetail reportId="report" editionId="edition-september" />
    </StoryProviders>,
  );
  await screen.findByRole("heading", { name: "Bookings progress" });
  expect(
    await screen.findByRole("button", { name: "Compare editions" }),
  ).toBeEnabled();
  expect(
    screen.getByRole("button", { name: "Monthly · Revision 3" }),
  ).toBeVisible();
  expect(
    fetch.mock.calls.some(
      ([input]) =>
        input instanceof Request && input.url.includes("/report/evaluation"),
    ),
  ).toBe(false);
  expect(
    screen.getByText(/Frozen edition.*Privacy redaction applied/i),
  ).toBeVisible();
});

it("duplicates a shared report into a private definition", async () => {
  const user = userEvent.setup({ delay: null });
  const duplicate = vi.fn(() =>
    jsonResponse({ ...reportingStoryReport, id: "copy" }),
  );
  installFetchStub({
    ...reportingStoryRoutes(),
    "POST /analytics/reports": duplicate,
  });
  render(
    <StoryProviders>
      <ReportingReportDetail reportId="report" />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Duplicate privately" }),
  );
  await waitFor(() =>
    expect(duplicate).toHaveBeenCalledWith({
      name: reportingStoryReport.name,
      audience: "private",
      selection: reportingStoryReport.selection,
    }),
  );
  await waitFor(() => expect(window.location.hash).toContain("copy"));
});

it("retries only failed or suspended runs and refreshes their status", async () => {
  const user = userEvent.setup({ delay: null });
  const retry = vi.fn(() =>
    jsonResponse({ ...reportingExecutions[0], status: "pending" }),
  );
  const list = vi.fn(() => jsonResponse({ data: reportingExecutions }));
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/reports/report/executions": list,
    "POST /analytics/executions/run-suspended/retry": retry,
  });
  render(
    <StoryProviders>
      <ReportingExecutions reportId="report" canRetry />
    </StoryProviders>,
  );
  const buttons = await screen.findAllByRole("button", { name: "Retry" });
  expect(buttons).toHaveLength(2);
  await user.click(buttons[0]);
  await waitFor(() => expect(retry).toHaveBeenCalledOnce());
  await waitFor(() => expect(list.mock.calls.length).toBeGreaterThan(1));
});

it("saves edited layout order, metric selection and team audience", async () => {
  const user = userEvent.setup({ delay: null });
  const saved = vi.fn();
  const write = vi.fn(() => jsonResponse(reportingStoryReport));
  installFetchStub({
    ...reportingStoryRoutes(),
    "PATCH /analytics/reports/report": write,
  });
  render(
    <StoryProviders>
      <SaveReportingDialog
        selection={reportingStoryReport.selection}
        report={reportingStoryReport}
        onClose={() => {}}
        onSaved={saved}
      />
    </StoryProviders>,
  );
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Audience" }),
    "Selected team",
  );
  await user.click(
    await screen.findByRole("button", { name: "Move down: Bookings progress" }),
  );
  await user.click(
    within(screen.getByRole("group", { name: "Charts and order" })).getByRole(
      "checkbox",
      { name: "Where are deals slowing down?" },
    ),
  );
  await user.click(screen.getByRole("button", { name: "Save report" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(write).toHaveBeenCalledWith(
    expect.objectContaining({
      audience: "team",
      audience_team_id: reportingStoryReport.selection.scope.id,
      selection: expect.objectContaining({
        blocks: ["stage_distribution", "bookings_trend", "owner_attainment"],
      }),
    }),
  );
});

it("disables comparison when both selectors name the same edition", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/editions/compare": () =>
      jsonResponse({
        left: reportingEditions[1],
        right: reportingEditions[0],
        compatible: true,
        deltas: [],
      }),
  });
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <ReportingComparison editions={reportingEditions} onClose={() => {}} />
    </StoryProviders>,
  );
  await screen.findByRole("heading", { name: "Bookings won" });
  const selectors = screen.getAllByRole("combobox");
  const rightLabel = selectors[1].textContent;
  if (!rightLabel) throw new Error("Missing edition date label");
  const count = fetch.mock.calls.length;
  await pickOption(user, selectors[0], rightLabel);
  expect(
    screen.queryByRole("heading", { name: "Bookings won" }),
  ).not.toBeInTheDocument();
  expect(fetch.mock.calls).toHaveLength(count);
});

it("keeps incompatible currencies separate and binds evidence to each edition", async () => {
  const user = userEvent.setup({ delay: null });
  const before = {
    ...reportingEditions[1],
    evaluation: {
      ...reportingEditions[1].evaluation,
      metrics: [
        { ...reportingEditions[1].evaluation.metrics[0], unit: "money" },
      ],
    },
  };
  const after = {
    ...reportingEditions[0],
    evaluation: {
      ...reportingEditions[0].evaluation,
      context: { ...reportingEditions[0].evaluation.context, currency: "USD" },
      metrics: [
        { ...reportingEditions[0].evaluation.metrics[0], unit: "money" },
      ],
    },
  };
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/editions/compare": () =>
      jsonResponse({
        left: before,
        right: after,
        compatible: false,
        reason: "Reporting currencies differ",
        deltas: [],
      }),
    "GET /analytics/editions/edition-september/evidence": () =>
      jsonResponse({
        context: after.evaluation.context,
        metric: "bookings_won",
        rows: [
          {
            key: "usd",
            label: "Dollar-denominated order",
            value: 21600000,
            restricted: false,
          },
        ],
        truncated: false,
      }),
  });
  render(
    <StoryProviders>
      <ReportingComparison editions={[after, before]} onClose={() => {}} />
    </StoryProviders>,
  );
  await screen.findByText("Reporting currencies differ");
  expect(document.querySelector(".report-chart-column")).toBeNull();
  await user.click(screen.getByRole("button", { name: /216,000/ }));
  expect(await screen.findByText("Dollar-denominated order")).toBeVisible();
  await user.click(
    within(screen.getAllByRole("dialog")[1]).getByRole("button", {
      name: "Close",
    }),
  );
  await waitFor(() => expect(screen.getAllByRole("dialog")).toHaveLength(1));
});

it.each([
  { expired: true, withheld: false, message: /retention/i },
  { expired: false, withheld: true, message: /current permissions/i },
])(
  "explains retained edition availability: %s",
  async ({ expired, withheld, message }) => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/editions/edition-september": () =>
        jsonResponse({
          ...reportingEditions[0],
          expired,
          withheld,
          evaluation: {
            ...reportingEditions[0].evaluation,
            charts: [],
            metrics: [],
          },
        }),
    });
    render(
      <StoryProviders>
        <ReportingReportDetail
          reportId="report"
          editionId="edition-september"
        />
      </StoryProviders>,
    );
    const banner = await screen.findByText(/Frozen edition/);
    expect(banner).toHaveTextContent(message);
    expect(
      screen.queryByRole("heading", { name: "Bookings progress" }),
    ).not.toBeInTheDocument();
  },
);
