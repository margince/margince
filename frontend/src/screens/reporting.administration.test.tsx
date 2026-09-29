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
  expect(dialog.getByRole("button", { name: "Targets" })).toBeDisabled();
  await dialog.findByRole("combobox", { name: "Metrics" });
  fireEvent.change(dialog.getByLabelText(/First day of target period/), {
    target: { value: "2026-10-01" },
  });
  await user.type(dialog.getByLabelText(/Target amount or count/), "1250.25");
  await user.type(
    dialog.getByLabelText(/Reason for revision/),
    "Agreed October commitment",
  );
  await user.click(dialog.getByRole("button", { name: "Targets" }));
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
    await screen.findByRole("cell", { name: "Meetings held" })
  ).closest("tr");
  if (!row) throw new Error("Missing commitment row");
  await user.click(within(row).getByRole("button", { name: "Revise target" }));
  const dialog = within(await screen.findByRole("dialog"));
  expect(dialog.getByLabelText(/First day of target period/)).toBeDisabled();
  expect(dialog.getByRole("combobox", { name: "Pipeline" })).toBeDisabled();
  expect(dialog.getByLabelText(/Target amount or count/)).toHaveValue(40);
  fireEvent.change(dialog.getByLabelText(/Target amount or count/), {
    target: { value: "45" },
  });
  await user.type(
    dialog.getByLabelText(/Reason for revision/),
    "Additional capacity",
  );
  await user.click(dialog.getByRole("button", { name: "Targets" }));
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
  await screen.findByRole("cell", { name: "Meetings held" });
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
    screen.getByRole("button", { name: "Publish framework" }),
  ).toBeDisabled();
  await user.type(
    screen.getByLabelText(/Reason for revision/),
    "Align qualification across markets",
  );
  await user.click(screen.getByRole("button", { name: "Publish framework" }));
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
  await screen.findByText("Revision 1 · Sales");
  expect(
    screen.queryByRole("button", { name: "Publish framework" }),
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
  await user.click(screen.getByRole("button", { name: "Schedule" }));
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
  await user.click(screen.getByRole("checkbox", { name: "Schedule enabled" }));
  await user.click(screen.getByRole("button", { name: "Schedule" }));
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
