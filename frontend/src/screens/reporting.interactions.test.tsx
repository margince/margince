import { REPORTING_FIXTURE_ZONE } from "./reporting.fixtures";
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
import { RecordZoneProvider } from "../app/recordzone";
import { pickOption } from "../design-system/select-testing";
import { ReportingOverview } from "./reporting.overview";
import { ReportingReportDetail } from "./reporting.report";
import { SaveReportingDialog } from "./reporting.save";
import {
  reportingStoryEvaluation,
  reportingStoryReport,
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

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
  await user.click(screen.getByRole("button", { name: "Save report" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(write).toHaveBeenCalledWith(
    expect.objectContaining({
      name: "Friday review",
      audience: "private",
      selection: reportingStoryEvaluation.selection,
    }),
  );
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
  await screen.findByRole("heading", { name: "Bookings progress" });
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
    await screen.findByRole("combobox", { name: "Event period" }),
    "Custom interval",
  );
  const before = fetch.mock.calls.length;
  fireEvent.change(screen.getByLabelText("From"), {
    target: { value: "2026-03-28" },
  });
  expect(fetch.mock.calls).toHaveLength(before);
  fireEvent.change(screen.getByLabelText("Through"), {
    target: { value: "2026-03-29" },
  });
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
    screen.queryByRole("button", { name: "Capture edition" }),
  ).not.toBeInTheDocument();
});
it("preserves the sales pipeline when returning from SDR outcomes", async () => {
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
  await user.click(screen.getByRole("button", { name: "Sales" }));
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
    await screen.findByRole("button", { name: "Archive report" }),
  );
  const dialog = await screen.findByRole("dialog");
  expect(
    within(dialog).getByText(
      "Archive this report and pause its schedules? Saved editions remain available.",
    ),
  ).toBeVisible();
  expect(archive).not.toHaveBeenCalled();
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  expect(archive).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Archive report" }));
  await user.click(
    within(await screen.findByRole("dialog")).getByRole("button", {
      name: "Archive report",
    }),
  );
  await waitFor(() => expect(archive).toHaveBeenCalledOnce());
});
