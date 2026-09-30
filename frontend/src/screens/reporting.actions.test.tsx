/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { pickOption } from "../design-system/select-testing";
import { downloadBytes } from "./download";
import { ReportingDefinitions } from "./reporting.definitions";
import { ReportingOverview } from "./reporting.overview";
import { ReportingReportDetail } from "./reporting.report";
import {
  reportingEditions,
  reportingSchedule,
  reportingTargets,
} from "./reporting.scenarios";
import { ReportingScorecard } from "./reporting.scorecard";
import {
  reportingStoryCatalog,
  reportingStoryEvaluation,
  reportingStoryFramework,
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";
import { ReportingTargets } from "./reporting.targets";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

vi.mock("./download", () => ({ downloadBytes: vi.fn() }));
const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
  vi.clearAllMocks();
  window.location.hash = "";
});

it("sends independent pipeline, target-period and expected-close filters and opens saving", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub(reportingStoryRoutes());
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <ReportingOverview scope={reportingStoryScope} />
    </StoryProviders>,
  );
  await screen.findByRole("heading", { name: "Bookings progress" });
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Pipeline" }),
    "All pipelines",
  );
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Target period" }),
    "Fiscal quarter",
  );
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Expected close window" }),
    "All open deals",
  );
  await waitFor(() =>
    expect(
      fetch.mock.calls.some(([input]) => {
        if (!(input instanceof Request)) return false;
        const url = new URL(input.url);
        return (
          url.pathname.endsWith("/evaluate") &&
          url.searchParams.get("target_basis") === "fiscal_quarter" &&
          url.searchParams.get("close_window") === "all_open" &&
          !url.searchParams.has("pipeline_id")
        );
      }),
    ).toBe(true),
  );
  await user.click(screen.getByRole("button", { name: "Save report" }));
  expect(
    await screen.findByRole("dialog", { name: "Save report" }),
  ).toBeVisible();
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }),
  );
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("captures the saved revision with an idempotency key and exposes report editing and schedules", async () => {
  const user = userEvent.setup({ delay: null });
  const freeze = vi.fn(() =>
    jsonResponse({ id: "run", status: "pending" }, 202),
  );
  installFetchStub({
    ...reportingStoryRoutes(),
    "POST /analytics/reports/report/editions": freeze,
    "GET /analytics/reports/report/schedules": () =>
      jsonResponse({ data: [reportingSchedule] }),
    "GET /analytics/reports/report/editions": () =>
      jsonResponse({ data: reportingEditions }),
  });
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <ReportingReportDetail reportId="report" />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Capture edition" }),
  );
  await waitFor(() => expect(freeze).toHaveBeenCalledOnce());
  const request = fetch.mock.calls
    .map(([input]) => input)
    .find(
      (input) =>
        input instanceof Request &&
        input.method === "POST" &&
        input.url.endsWith("/editions"),
    );
  expect(
    request instanceof Request && request.headers.get("Idempotency-Key"),
  ).toMatch(/^[\da-f-]{36}$/);
  await user.click(screen.getByRole("button", { name: "Edit report" }));
  expect(
    await screen.findByRole("dialog", { name: "Edit report" }),
  ).toBeVisible();
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }),
  );
  await user.click(screen.getByRole("button", { name: "Schedule" }));
  expect(
    await screen.findByRole("combobox", { name: "Frequency" }),
  ).toHaveTextContent("Weekly");
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }),
  );
  await user.click(
    screen.getByRole("button", { name: "Monthly · Revision 3" }),
  );
  expect(
    await screen.findByRole("combobox", { name: "Frequency" }),
  ).toHaveTextContent("Monthly");
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }),
  );
  await user.click(screen.getByRole("button", { name: "Live preview" }));
  expect(window.location.hash).toContain("report");
});

it("creates a pipeline-specific fiscal-quarter count target and pages the target list", async () => {
  const user = userEvent.setup({ delay: null });
  const write = vi.fn(() => jsonResponse(reportingTargets[2]));
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/targets": () =>
      jsonResponse({ data: reportingTargets, next_cursor: "next-target" }),
    "GET /analytics/metrics": () =>
      jsonResponse({ metrics: reportingStoryCatalog.metrics }),
    "POST /analytics/targets": write,
  });
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <ReportingTargets />
    </StoryProviders>,
  );
  await user.click(await screen.findByRole("button", { name: "Next page" }));
  await waitFor(() =>
    expect(
      fetch.mock.calls.some(
        ([input]) =>
          input instanceof Request && input.url.includes("cursor=next-target"),
      ),
    ).toBe(true),
  );
  await user.click(screen.getByRole("button", { name: "Set target" }));
  const dialog = within(await screen.findByRole("dialog"));
  await pickOption(
    user,
    await dialog.findByRole("combobox", { name: "Metrics" }),
    "Meetings held",
  );
  await pickOption(
    user,
    dialog.getByRole("combobox", { name: "Pipeline" }),
    "Sales",
  );
  await pickOption(
    user,
    dialog.getByRole("combobox", { name: "Target period" }),
    "Fiscal quarter",
  );
  fireEvent.change(dialog.getByLabelText(/First day of target period/), {
    target: { value: "2026-10-01" },
  });
  fireEvent.change(dialog.getByLabelText(/Target amount or count/), {
    target: { value: "120" },
  });
  await user.type(
    dialog.getByLabelText(/Reason for revision/),
    "Quarter commitment",
  );
  await user.click(dialog.getByRole("button", { name: "Targets" }));
  await waitFor(() =>
    expect(write).toHaveBeenCalledWith(
      expect.objectContaining({
        metric: "meetings_held",
        value: 120,
        period_kind: "fiscal_quarter",
        pipeline_id: "sales",
      }),
    ),
  );
});

it("removes a capture context without leaving an enabled phantom capture", async () => {
  const user = userEvent.setup({ delay: null });
  const publish = vi.fn(() => jsonResponse(reportingStoryFramework));
  installFetchStub({
    ...reportingStoryRoutes(),
    "PUT /analytics/framework": publish,
  });
  render(
    <StoryProviders>
      <ReportingDefinitions />
    </StoryProviders>,
  );
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "Add capture" })).toBeEnabled(),
  );
  await user.click(screen.getByRole("button", { name: "Add capture" }));
  await user.click(screen.getByRole("button", { name: "Remove capture" }));
  expect(
    screen.queryByRole("button", { name: "Remove capture" }),
  ).not.toBeInTheDocument();
  await user.type(
    screen.getByLabelText(/Reason for revision/),
    "Stop unnecessary capture",
  );
  await user.click(screen.getByRole("button", { name: "Publish framework" }));
  await waitFor(() =>
    expect(publish).toHaveBeenCalledWith(
      expect.objectContaining({ capture_contexts: [] }),
    ),
  );
});

it.each([undefined, "edition-september"])(
  "downloads the authorized CSV for %s",
  async (editionId) => {
    const user = userEvent.setup({ delay: null });
    const route = editionId
      ? `GET /analytics/editions/${editionId}/export.csv`
      : "GET /analytics/evaluate.csv";
    installFetchStub({
      ...reportingStoryRoutes(),
      [route]: () =>
        new Response("metric,value\nbookings_won,21600000", {
          headers: { "Content-Type": "text/csv" },
        }),
    });
    render(
      <StoryProviders>
        <ReportingScorecard
          evaluation={reportingStoryEvaluation}
          editionId={editionId}
          onEvidence={() => {}}
        />
      </StoryProviders>,
    );
    await user.click(screen.getByRole("button", { name: "Export CSV" }));
    await waitFor(() =>
      expect(downloadBytes).toHaveBeenCalledWith(
        expect.any(Blob),
        "reporting.csv",
        "text/csv",
      ),
    );
  },
);
