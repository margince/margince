import { REPORTING_FIXTURE_ZONE } from "../../testing/reporting.fixtures";
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
import { pickOption } from "../../design-system/select-testing";
import { RecordZoneProvider } from "../../lib/recordzone";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { layoutOf } from "./reporting.layout";
import { ReportingOverview } from "./reporting.overview";
import { ReportingReportDetail } from "./reporting.report";
import { SaveReportingDialog } from "./reporting.save";
import {
  reportingStoryEvaluation,
  reportingStoryReport,
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
  window.location.hash = "";
});
it("saves the full selected graph layout and context", async () => {
  const user = userEvent.setup({ delay: null });
  const saved = vi.fn();
  const write = vi.fn(() => jsonResponse(reportingStoryReport));
  installFetchStub({
    ...reportingStoryRoutes(),
    "POST /analytics/reports": write,
  });
  render(
    <StoryProviders>
      <SaveReportingDialog
        selection={reportingStoryEvaluation.selection}
        onClose={() => {}}
        onSaved={saved}
      />
    </StoryProviders>,
  );
  await user.type(
    await screen.findByRole("textbox", { name: "Report name" }),
    "Friday review",
  );
  await user.type(
    screen.getByRole("textbox", { name: "Purpose" }),
    "For the Friday call.",
  );
  expect(
    screen.queryByRole("combobox", { name: "Metrics" }),
  ).not.toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Customize report" }),
  ).toHaveAttribute("aria-expanded", "false");
  await user.click(screen.getByRole("button", { name: "Save report" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(write).toHaveBeenCalledWith(
    expect.objectContaining({
      name: "Friday review",
      description: "For the Friday call.",
      audience: "private",
      selection: {
        ...reportingStoryEvaluation.selection,
        layout: layoutOf(reportingStoryEvaluation.selection),
      },
    }),
  );
});
it("draws a saved layout's sections under their titles", async () => {
  const laidOutEvaluation = {
    ...reportingStoryEvaluation,
    selection: {
      ...reportingStoryEvaluation.selection,
      layout: [
        {
          title: "Where deals stand",
          cards: [
            {
              id: "stages",
              kind: "stage_distribution" as const,
              width: "half" as const,
            },
          ],
        },
      ],
    },
  };
  installFetchStub(reportingStoryRoutes(laidOutEvaluation, true));
  render(
    <StoryProviders>
      <ReportingReportDetail reportId="report" />
    </StoryProviders>,
  );
  expect(
    await screen.findByRole("heading", { name: "Where deals stand" }),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("heading", { name: "Sales won over time" }),
  ).not.toBeInTheDocument();
});
it("previews one chart before it is added, saving nothing", async () => {
  const user = userEvent.setup({ delay: null });
  const write = vi.fn(() => jsonResponse(reportingStoryReport));
  installFetchStub({
    ...reportingStoryRoutes(),
    "PATCH /analytics/reports/report": write,
  });
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <SaveReportingDialog
        selection={reportingStoryReport.selection}
        report={reportingStoryReport}
        onClose={() => {}}
        onSaved={() => {}}
      />
    </StoryProviders>,
  );
  await user.click(
    screen.getByText("Charts and order", { selector: ".disclosure-label" }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Remove Time in current stage" }),
  );
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Chart to add" }),
    "Time in current stage",
  );
  await user.click(screen.getByRole("button", { name: "Preview" }));
  await waitFor(() =>
    expect(
      fetch.mock.calls.some(([input]) => {
        const url = input instanceof Request ? input.url : String(input);
        return (
          url.includes("/analytics/evaluate") &&
          url.includes("blocks=stage_age")
        );
      }),
    ).toBe(true),
  );
  expect(write).not.toHaveBeenCalled();
});
it("previews the report as a role for a seat that may read roles", async () => {
  const user = userEvent.setup({ delay: null });
  const routes = reportingStoryRoutes(reportingStoryEvaluation, true);
  routes["GET /me"] = meRoute(
    {
      report_definition: ["read", "update"],
      deal: ["read"],
      pipeline: ["read"],
      role_admin: ["read"],
    },
    { roles: ["admin"], seat: "full" },
  );
  routes["GET /roles"] = () =>
    jsonResponse({ roles: [{ key: "rep", name: "Rep" }] });
  const preview = vi.fn(() => jsonResponse(reportingStoryEvaluation));
  routes["GET /analytics/reports/report/reader-preview"] = preview;
  installFetchStub(routes);
  render(
    <StoryProviders>
      <ReportingReportDetail reportId="report" />
    </StoryProviders>,
  );
  await pickOption(
    user,
    await screen.findByRole("combobox", { name: "Preview as" }),
    "Rep",
  );
  expect(
    await screen.findByText(/Preview as Rep: what a seat with this role/),
  ).toBeInTheDocument();
  expect(preview).toHaveBeenCalled();
  expect(
    screen.queryByRole("button", { name: /Export CSV/ }),
  ).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "End preview" }));
  expect(
    await screen.findByRole("button", { name: /Export CSV/ }),
  ).toBeInTheDocument();
  expect(screen.queryByText(/Preview as Rep/)).not.toBeInTheDocument();
});
it("offers no role preview to a seat that may not read roles", async () => {
  installFetchStub(reportingStoryRoutes(reportingStoryEvaluation, true));
  render(
    <StoryProviders>
      <ReportingReportDetail reportId="report" />
    </StoryProviders>,
  );
  await screen.findByRole("heading", { name: "DACH operating review" });
  expect(
    screen.queryByRole("combobox", { name: "Preview as" }),
  ).not.toBeInTheDocument();
});
it("shows the report's purpose under its name", async () => {
  const routes = reportingStoryRoutes(reportingStoryEvaluation, true);
  routes["GET /analytics/reports/report"] = () =>
    jsonResponse({
      ...reportingStoryReport,
      description: "What the board reads on Mondays.",
    });
  installFetchStub(routes);
  render(
    <StoryProviders>
      <ReportingReportDetail reportId="report" />
    </StoryProviders>,
  );
  await screen.findByRole("heading", { name: "DACH operating review" });
  expect(
    screen.getByText("What the board reads on Mondays."),
  ).toBeInTheDocument();
});
it("opens keyboard-selected evidence with its receipt and restores focus", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub(reportingStoryRoutes());
  const fetch = vi.spyOn(globalThis, "fetch");
  const { container } = render(
    <StoryProviders>
      <ReportingOverview scope={reportingStoryScope} />
    </StoryProviders>,
  );
  await screen.findByRole("heading", { name: "Sales won over time" });
  const point = container.querySelector<HTMLButtonElement>(
    ".report-chart-point",
  );
  if (!point) throw new Error("No chart observation");
  point.focus();
  await user.keyboard("{Enter}");
  const drawer = await screen.findByRole("dialog");
  expect(await within(drawer).findByText("Northstar rollout")).toBeVisible();
  const request = fetch.mock.calls
    .map(([input]) => (input instanceof Request ? input.url : String(input)))
    .find((url) => url.includes("/analytics/evidence?"));
  expect(request).toContain("evaluation_key=fixture-evaluation");
  expect(request).toContain("framework_revision=1");
  await user.keyboard("{Escape}");
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  expect(point).toHaveFocus();
});
it("uses local inclusive custom dates without querying an incomplete range", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub(reportingStoryRoutes());
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <RecordZoneProvider zone={REPORTING_FIXTURE_ZONE}>
        <ReportingOverview scope={reportingStoryScope} />
      </RecordZoneProvider>
    </StoryProviders>,
  );
  await pickOption(
    user,
    await screen.findByRole("combobox", { name: "Date range" }),
    "Custom interval",
  );
  const before = fetch.mock.calls.length;
  await user.type(screen.getByLabelText("From"), "28/03/2026");
  await user.tab();
  expect(fetch.mock.calls).toHaveLength(before);
  await user.type(screen.getByLabelText("Through"), "29/03/2026");
  await user.tab();
  await waitFor(() => {
    const requests = fetch.mock.calls.map(
      ([input]) =>
        new URL(
          input instanceof Request ? input.url : String(input),
          window.location.origin,
        ),
    );
    const query = requests.find(
      (url) => url.searchParams.get("period") === "custom",
    );
    expect(query?.searchParams.get("start_at")).toBe(
      "2026-03-27T23:00:00.000Z",
    );
    expect(query?.searchParams.get("end_at")).toBe("2026-03-29T22:00:00.000Z");
  });
});
it("does not request editions or schedules without their read grants", async () => {
  const routes = reportingStoryRoutes(reportingStoryEvaluation, true);
  routes["GET /me"] = meRoute(
    { report_definition: ["read"], deal: ["read"], pipeline: ["read"] },
    { roles: ["rep"], seat: "read" },
  );
  installFetchStub(routes);
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <ReportingReportDetail reportId="report" />
    </StoryProviders>,
  );
  await screen.findByRole("heading", { name: "DACH operating review" });
  expect(
    fetch.mock.calls.some(([input]) =>
      /\/(schedules|editions)(\?|$)/.test(
        input instanceof Request ? input.url : String(input),
      ),
    ),
  ).toBe(false);
  expect(
    screen.queryByRole("button", { name: "Save snapshot" }),
  ).not.toBeInTheDocument();
});
it("preserves the sales pipeline after changing the SDR date range", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub(reportingStoryRoutes());
  render(
    <StoryProviders>
      <ReportingOverview scope={reportingStoryScope} />
    </StoryProviders>,
  );
  const pipeline = await screen.findByRole("combobox", { name: "Pipeline" });
  expect(pipeline).toHaveTextContent("Sales");
  await user.click(screen.getByRole("button", { name: "SDR outcomes" }));
  expect(
    screen.queryByRole("combobox", { name: "Pipeline" }),
  ).not.toBeInTheDocument();
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Date range" }),
    "Last month",
  );
  await user.click(screen.getByRole("button", { name: "Sales" }));
  expect(
    screen.getByRole("combobox", { name: "Date range" }),
  ).toHaveTextContent("Last month");
  expect(screen.getByRole("combobox", { name: "Pipeline" })).toHaveTextContent(
    "Sales",
  );
});

it("requires confirmation before archiving and explains the schedule effect", async () => {
  const user = userEvent.setup({ delay: null });
  const archive = vi.fn(() => new Response(null, { status: 204 }));
  installFetchStub({
    ...reportingStoryRoutes(),
    "DELETE /analytics/reports/report": archive,
  });
  render(
    <StoryProviders>
      <ReportingReportDetail reportId="report" />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Report actions" }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Archive report" }),
  );
  const dialog = await screen.findByRole("dialog");
  expect(
    within(dialog).getByText(
      "Archive this report and pause its schedules? Saved snapshots remain available.",
    ),
  ).toBeVisible();
  expect(archive).not.toHaveBeenCalled();
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  expect(archive).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Report actions" }));
  await user.click(
    await screen.findByRole("button", { name: "Archive report" }),
  );
  await user.click(
    within(await screen.findByRole("dialog")).getByRole("button", {
      name: "Archive report",
    }),
  );
  await waitFor(() => expect(archive).toHaveBeenCalledOnce());
});

it("restores the selected period and close window after reopening performance", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub(reportingStoryRoutes());
  const first = render(
    <StoryProviders>
      <ReportingOverview scope={reportingStoryScope} />
    </StoryProviders>,
  );
  await pickOption(
    user,
    await screen.findByRole("combobox", { name: "Date range" }),
    "Last month",
  );
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Expected close window" }),
    "Fiscal quarter",
  );
  first.unmount();
  render(
    <StoryProviders>
      <ReportingOverview scope={reportingStoryScope} />
    </StoryProviders>,
  );
  expect(
    await screen.findByRole("combobox", { name: "Date range" }),
  ).toHaveTextContent("Last month");
  expect(
    screen.getByRole("combobox", { name: "Expected close window" }),
  ).toHaveTextContent("Fiscal quarter");
});

it("derives the target period from the business period despite an old bookmark", async () => {
  window.location.hash =
    "#/analytics/performance?period=this_quarter&target=month";
  installFetchStub(reportingStoryRoutes());
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <ReportingOverview scope={reportingStoryScope} />
    </StoryProviders>,
  );
  await screen.findByRole("heading", { name: "Sales won over time" });
  const requests = fetch.mock.calls.flatMap(([input]) =>
    input instanceof Request && input.url.includes("/analytics/evaluate?")
      ? [new URL(input.url)]
      : [],
  );
  expect(requests.length).toBeGreaterThan(0);
  expect(requests.at(-1)?.searchParams.get("target_basis")).toBe(
    "fiscal_quarter",
  );
});

it("labels a frozen pipeline drill-down with its historical state date", async () => {
  installFetchStub(reportingStoryRoutes());
  const evaluation = {
    ...reportingStoryEvaluation,
    context: {
      ...reportingStoryEvaluation.context,
      state_at: "2025-01-10T12:00:00Z",
    },
  };
  render(
    <StoryProviders>
      <ReportingEvidenceDrawer
        evaluation={evaluation}
        reference={{ metric: "open_pipeline", context_id: "state" }}
        editionId="old-snapshot"
        onClose={() => {}}
      />
    </StoryProviders>,
  );
  expect(screen.getByText("10/01/2025, 13:00")).toBeVisible();
  expect(screen.queryByText("Open pipeline today")).not.toBeInTheDocument();
});
