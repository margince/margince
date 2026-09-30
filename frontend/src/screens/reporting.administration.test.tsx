/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  act,
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
import { ReportingDefinitions } from "./reporting.definitions";
import { REPORTING_FIXTURE_ZONE } from "./reporting.fixtures";
import { reportingSchedule, reportingTargets } from "./reporting.scenarios";
import { ReportingScheduleDialog } from "./reporting.schedule";
import {
  reportingStoryFramework,
  reportingStoryReport,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";
import { ReportingTargets } from "./reporting.targets";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

it("creates a monetary commitment in minor units and refuses an incomplete target", async () => {
  const user = userEvent.setup({ delay: null });
  const write = vi.fn(() => jsonResponse(reportingTargets[0]));
  installFetchStub({
    ...reportingStoryRoutes(),
    "POST /analytics/targets": write,
  });
  render(
    <StoryProviders>
      <ReportingTargets />
    </StoryProviders>,
  );
  await user.click(await screen.findByRole("button", { name: "Set target" }));
  const dialog = within(await screen.findByRole("dialog"));
  expect(dialog.getByRole("button", { name: "Set target" })).toBeDisabled();
  await dialog.findByRole("combobox", { name: "Metrics" });
  fireEvent.change(dialog.getByLabelText(/Year/), {
    target: { value: "2026" },
  });
  await pickOption(
    user,
    dialog.getByRole("combobox", { name: /Month starting/ }),
    "October",
  );
  await user.type(
    dialog.getByRole("spinbutton", { name: /^Target/ }),
    "1250.25",
  );
  await user.type(dialog.getByLabelText(/Reason/), "Agreed October commitment");
  await user.click(dialog.getByRole("button", { name: "Set target" }));
  await waitFor(() =>
    expect(write).toHaveBeenCalledWith(
      expect.objectContaining({
        value: 125025,
        metric: "bookings_won",
        period_start: "2026-10-01",
        reason: "Agreed October commitment",
      }),
    ),
  );
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
});

it("revises a count target without rescaling it or changing its identity", async () => {
  const user = userEvent.setup({ delay: null });
  const write = vi.fn(() => jsonResponse(reportingTargets[2]));
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/targets": () => jsonResponse({ data: reportingTargets }),
    "PATCH /analytics/targets/target-2": write,
  });
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <ReportingTargets />
    </StoryProviders>,
  );
  const row = (
    await screen.findByRole("cell", { name: /Meetings held/ })
  ).closest("tr");
  if (!row) throw new Error("Missing commitment row");
  await user.click(within(row).getByRole("button", { name: "Revise target" }));
  const dialog = within(await screen.findByRole("dialog"));
  expect(dialog.getByLabelText(/Year/)).toBeDisabled();
  expect(dialog.getByRole("combobox", { name: "Pipeline" })).toBeDisabled();
  expect(dialog.getByRole("spinbutton", { name: /^Target/ })).toHaveValue(40);
  fireEvent.change(dialog.getByRole("spinbutton", { name: /^Target/ }), {
    target: { value: "45" },
  });
  await user.type(dialog.getByLabelText(/Reason/), "Additional capacity");
  await user.click(dialog.getByRole("button", { name: "Revise target" }));
  await waitFor(() =>
    expect(write).toHaveBeenCalledWith(
      expect.objectContaining({
        value: 45,
        metric: "meetings_held",
        period_start: "2026-09-01",
      }),
    ),
  );
  const request = fetch.mock.calls
    .map(([input]) => input)
    .find((input) => input instanceof Request && input.method === "PATCH");
  expect(request instanceof Request && request.headers.get("If-Match")).toBe(
    "2",
  );
});

it("keeps target authoring unavailable to a read seat", async () => {
  installFetchStub({
    ...reportingStoryRoutes(undefined, true),
    "GET /analytics/targets": () => jsonResponse({ data: reportingTargets }),
  });
  render(
    <StoryProviders>
      <ReportingTargets />
    </StoryProviders>,
  );
  await screen.findByRole("cell", { name: /Meetings held/ });
  expect(
    screen.queryByRole("button", { name: "Set target" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Revise target" }),
  ).not.toBeInTheDocument();
});

it("publishes stage qualification and capture contexts with a reason and version", async () => {
  const user = userEvent.setup({ delay: null });
  const publish = vi.fn(() => jsonResponse(reportingStoryFramework));
  installFetchStub({
    ...reportingStoryRoutes(),
    "PUT /analytics/framework": publish,
    "GET /pipelines": () =>
      jsonResponse({
        data: [
          {
            id: "sales",
            name: "Sales",
            stages: [
              { id: "qualified", name: "Qualified" },
              { id: "proposal", name: "Proposal" },
            ],
          },
        ],
      }),
  });
  render(
    <StoryProviders>
      <ReportingDefinitions />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Reporting setup" }),
  );
  await user.click(await screen.findByRole("checkbox", { name: "Qualified" }));
  await user.click(screen.getByRole("checkbox", { name: "Proposal" }));
  await user.click(screen.getByRole("checkbox", { name: "Proposal" }));
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Default view" }),
    "SDR outcomes",
  );
  await user.click(screen.getByRole("button", { name: "Add capture" }));
  const pipelinePickers = screen.getAllByRole("combobox", { name: "Pipeline" });
  await pickOption(user, pipelinePickers[pipelinePickers.length - 1], "Sales");
  expect(
    screen.getByRole("button", { name: "Save reporting settings" }),
  ).toBeDisabled();
  await user.type(
    screen.getByLabelText(/Reason/),
    "Align qualification across markets",
  );
  await user.click(
    screen.getByRole("button", { name: "Save reporting settings" }),
  );
  await waitFor(() =>
    expect(publish).toHaveBeenCalledWith(
      expect.objectContaining({
        template: "sdr",
        reason: "Align qualification across markets",
        qualification: [{ pipeline_id: "sales", stage_ids: ["qualified"] }],
        capture_contexts: expect.arrayContaining([
          expect.objectContaining({ pipeline_id: "sales" }),
        ]),
      }),
    ),
  );
});

it("does not expose framework publication to read-only readers", async () => {
  installFetchStub(reportingStoryRoutes(undefined, true));
  render(
    <StoryProviders>
      <ReportingDefinitions />
    </StoryProviders>,
  );
  await screen.findByRole("heading", { name: "Metric definitions" });
  expect(
    screen.queryByRole("button", { name: "Reporting setup" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Save reporting settings" }),
  ).not.toBeInTheDocument();
});

it("creates a month-end schedule pinned to the selected report revision", async () => {
  const user = userEvent.setup({ delay: null });
  const close = vi.fn();
  const write = vi.fn(() => jsonResponse(reportingSchedule));
  installFetchStub({
    ...reportingStoryRoutes(),
    "POST /analytics/reports/report/schedules": write,
  });
  render(
    <StoryProviders>
      <ReportingScheduleDialog
        report={reportingStoryReport}
        timezone={REPORTING_FIXTURE_ZONE}
        onClose={close}
      />
    </StoryProviders>,
  );
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Frequency" }),
    "Monthly",
  );
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Run day" }),
    "31",
  );
  fireEvent.change(screen.getByLabelText(/Local time/), {
    target: { value: "17:30" },
  });
  await user.click(screen.getByRole("button", { name: "Activate schedule" }));
  await waitFor(() =>
    expect(write).toHaveBeenCalledWith({
      report_revision: reportingStoryReport.revision,
      frequency: "monthly",
      day: 31,
      local_time: "17:30",
      enabled: true,
    }),
  );
  expect(close).toHaveBeenCalledOnce();
});

it("resets an edited schedule's day when its cadence changes and can resume it", async () => {
  const user = userEvent.setup({ delay: null });
  const close = vi.fn();
  const write = vi.fn(() => jsonResponse(reportingSchedule));
  installFetchStub({
    ...reportingStoryRoutes(),
    "PATCH /analytics/schedules/schedule": write,
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
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Frequency" }),
    "Weekly",
  );
  expect(screen.getByRole("combobox", { name: "Run day" })).toHaveTextContent(
    "Monday",
  );
  await user.click(screen.getByRole("button", { name: "Activate schedule" }));
  await waitFor(() =>
    expect(write).toHaveBeenCalledWith({
      report_revision: 3,
      frequency: "weekly",
      day: 1,
      local_time: "09:00",
      enabled: true,
    }),
  );
  expect(close).toHaveBeenCalledOnce();
});

it("lets an author pause an enabled schedule after retention becomes unavailable", async () => {
  const user = userEvent.setup({ delay: null });
  const write = vi.fn(() => jsonResponse(reportingSchedule));
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/metrics": () =>
      jsonResponse({ metrics: [], schedule_ready: false }),
    [`PATCH /analytics/schedules/${reportingSchedule.id}`]: write,
  });
  render(
    <StoryProviders>
      <ReportingScheduleDialog
        report={reportingStoryReport}
        schedule={{
          ...reportingSchedule,
          definition: { ...reportingSchedule.definition, enabled: true },
        }}
        timezone={REPORTING_FIXTURE_ZONE}
        onClose={() => {}}
      />
    </StoryProviders>,
  );
  await user.click(screen.getByRole("button", { name: "Save paused" }));
  await waitFor(() =>
    expect(write).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: false }),
    ),
  );
});

it("waits for readiness before claiming retention setup is needed", async () => {
  let release: () => void = () => {
    throw new Error("Readiness was not requested");
  };
  const response = new Promise<Response>((resolve) => {
    release = () =>
      resolve(jsonResponse({ metrics: [], schedule_ready: true }));
  });
  const readiness = vi.fn(() => response);
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/metrics": readiness,
  });
  render(
    <StoryProviders>
      <ReportingScheduleDialog
        report={reportingStoryReport}
        timezone={REPORTING_FIXTURE_ZONE}
        onClose={() => {}}
      />
    </StoryProviders>,
  );
  await waitFor(() => expect(readiness).toHaveBeenCalledOnce());
  const enabled = screen.getByRole("button", { name: "Activate schedule" });
  expect(enabled).toBeDisabled();
  expect(
    screen.queryByText(/Enable snapshot retention/),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Retention settings" }),
  ).not.toBeInTheDocument();
  await act(async () => {
    release();
  });
  await waitFor(() => expect(enabled).toBeEnabled());
  expect(
    screen.queryByText(/Enable snapshot retention/),
  ).not.toBeInTheDocument();
});

it("offers only fiscal-quarter start months and submits the selected boundary", async () => {
  const user = userEvent.setup({ delay: null });
  const write = vi.fn(() => jsonResponse(reportingTargets[0]));
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /installation/settings": () =>
      jsonResponse({
        fiscal_year_start_month: 2,
        timezone: REPORTING_FIXTURE_ZONE,
      }),
    "POST /analytics/targets": write,
  });
  render(
    <StoryProviders>
      <ReportingTargets />
    </StoryProviders>,
  );
  await user.click(await screen.findByRole("button", { name: "Set target" }));
  const dialog = within(await screen.findByRole("dialog"));
  await pickOption(
    user,
    dialog.getByRole("combobox", { name: "Target period" }),
    "Fiscal quarter",
  );
  await user.click(dialog.getByRole("combobox", { name: /Month starting/ }));
  expect(
    screen.getAllByRole("option").map((option) => option.textContent),
  ).toEqual(["February", "May", "August", "November"]);
  await user.click(screen.getByRole("option", { name: "November" }));
  await user.type(dialog.getByRole("spinbutton", { name: /^Target/ }), "1200");
  await user.type(
    dialog.getByRole("textbox", { name: /Reason/ }),
    "Agreed fiscal-quarter target",
  );
  await user.click(dialog.getByRole("button", { name: "Set target" }));
  await waitFor(() =>
    expect(write).toHaveBeenCalledWith(
      expect.objectContaining({
        period_kind: "fiscal_quarter",
        period_start: "2026-11-01",
        value: 120000,
      }),
    ),
  );
});

it("filters targets on the server and resets a paged result when status changes", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/targets": () =>
      jsonResponse({ data: reportingTargets, next_cursor: "next-page" }),
  });
  const fetch = vi.spyOn(globalThis, "fetch");
  const targetQueries = () =>
    fetch.mock.calls.flatMap(([input]) =>
      input instanceof Request && input.url.includes("/analytics/targets?")
        ? [new URL(input.url).searchParams]
        : [],
    );
  render(
    <StoryProviders>
      <ReportingTargets />
    </StoryProviders>,
  );
  await screen.findByRole("button", { name: "Next page" });
  expect(targetQueries().at(-1)?.get("retired")).toBe("false");
  await user.click(screen.getByRole("button", { name: "Next page" }));
  await waitFor(() =>
    expect(targetQueries().at(-1)?.get("cursor")).toBe("next-page"),
  );
  await user.click(screen.getByRole("button", { name: "Target retired" }));
  await waitFor(() =>
    expect(targetQueries().at(-1)?.get("retired")).toBe("true"),
  );
  expect(targetQueries().at(-1)?.has("cursor")).toBe(false);
  fireEvent.change(screen.getByLabelText("Month starting"), {
    target: { value: "2026-10" },
  });
  await waitFor(() =>
    expect(targetQueries().at(-1)?.get("period_start")).toBe("2026-10-01"),
  );
});

it("saves a new schedule paused without claiming it will run", async () => {
  const user = userEvent.setup({ delay: null });
  const write = vi.fn(() => jsonResponse(reportingSchedule));
  installFetchStub({
    ...reportingStoryRoutes(),
    "POST /analytics/reports/report/schedules": write,
  });
  render(
    <StoryProviders>
      <ReportingScheduleDialog
        report={reportingStoryReport}
        timezone={REPORTING_FIXTURE_ZONE}
        onClose={() => {}}
      />
    </StoryProviders>,
  );
  await user.click(screen.getByRole("button", { name: "Save paused" }));
  await waitFor(() =>
    expect(write).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: false }),
    ),
  );
});
