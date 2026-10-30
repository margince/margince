// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { reportingStoryCatalog } from "../../testing/reporting.fixtures";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { PREPARED_REPORTS, preparedSelection } from "./reporting.prepared";
import { PreparedReports } from "./reporting.preparedpanel";
import {
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

const weekly = PREPARED_REPORTS.find(
  (report) => report.id === "weekly-sales-review",
);

it("asks a prepared question only in the metrics and charts the reader may read", () => {
  if (weekly?.kind !== "question") throw new Error("weekly is a question");
  const onlyWon = {
    ...reportingStoryCatalog,
    metrics: reportingStoryCatalog.metrics.filter(
      (metric) => metric.id === "bookings_won",
    ),
  };
  const selection = preparedSelection(
    weekly,
    onlyWon,
    reportingStoryScope,
    "p-1",
  );
  expect(selection?.metrics).toEqual(["bookings_won"]);
  for (const block of selection?.blocks ?? []) {
    expect(onlyWon.metrics[0]?.blocks).toContain(block);
  }
  expect(selection?.pipeline_id).toBeUndefined();
  expect(
    preparedSelection(weekly, reportingStoryCatalog, reportingStoryScope, "p-1")
      ?.pipeline_id,
  ).toBe("p-1");
  expect(
    preparedSelection(
      weekly,
      { ...onlyWon, metrics: [] },
      reportingStoryScope,
      undefined,
    ),
  ).toBeNull();
});

function draw(ownLens = false) {
  const asked: string[] = [];
  installFetchStub(reportingStoryRoutes());
  const stub = globalThis.fetch;
  globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    asked.push(input instanceof Request ? input.url : String(input));
    return stub(input, init);
  }) as typeof fetch;
  render(
    <StoryProviders>
      <PreparedReports scope={reportingStoryScope} ownLens={ownLens} />
    </StoryProviders>,
  );
  return asked;
}

it("previews a prepared report with its own question and opens a save for a copy", async () => {
  const asked = draw();
  const user = userEvent.setup();
  const row = (await screen.findByText("Weekly sales review")).closest(
    ".panel-row, [class*='row']",
  ) as HTMLElement;
  await user.click(within(row).getByRole("button", { name: "Preview" }));
  await screen.findAllByText(/Sales won/);
  const evaluate = asked.find((url) => url.includes("/analytics/evaluate"));
  expect(evaluate).toContain("period=last_week");
  expect(evaluate).toContain("bookings_won");

  await user.click(within(row).getByRole("button", { name: "Save a copy" }));
  expect(await screen.findByRole("dialog")).toBeInTheDocument();
});

it("opens the section that already holds a prepared report", async () => {
  draw();
  const row = (await screen.findByText("Delivery review")).closest(
    ".panel-row, [class*='row']",
  ) as HTMLElement;
  await userEvent
    .setup()
    .click(within(row).getByRole("button", { name: "Open" }));
  expect(window.location.hash).toMatch(/^#\/analytics\/delivery/);
});

it("names a pipeline only for a question with stage charts", () => {
  const qualification = PREPARED_REPORTS.find(
    (report) => report.id === "qualification-review",
  );
  if (qualification?.kind !== "question") throw new Error("a question");
  const selection = preparedSelection(
    qualification,
    reportingStoryCatalog,
    reportingStoryScope,
    "p-1",
  );
  expect(selection?.pipeline_id).toBeUndefined();
});

it("offers personal outcomes only to a reader whose lens is their own records", async () => {
  draw(false);
  await screen.findByText("Delivery review");
  expect(screen.queryByText("Personal outcomes")).toBeNull();
  cleanup();
  draw(true);
  expect(await screen.findByText("Personal outcomes")).toBeInTheDocument();
});

// The prospecting template asks what the catalog's sdr preset asks, whatever
// that preset holds today.
it("asks the prospecting template from the catalog's sdr preset", () => {
  const prospecting = PREPARED_REPORTS.find(
    (report) => report.id === "my-prospecting-progress",
  );
  if (prospecting?.kind !== "question") throw new Error("it is a question");
  const meetingsOnly = {
    ...reportingStoryCatalog,
    presets: reportingStoryCatalog.presets.map((preset) =>
      preset.id === "sdr"
        ? {
            ...preset,
            metrics: ["meetings_held" as const],
            blocks: ["sdr_outcomes" as const],
          }
        : preset,
    ),
  };
  const selection = preparedSelection(
    prospecting,
    meetingsOnly,
    reportingStoryScope,
    "p-1",
  );
  expect(selection?.metrics).toEqual(["meetings_held"]);
  expect(selection?.blocks).toEqual(["sdr_outcomes"]);
  expect(selection?.period).toBe("this_week");
  expect(
    preparedSelection(
      prospecting,
      { ...meetingsOnly, presets: [] },
      reportingStoryScope,
      "p-1",
    ),
  ).toBeNull();
});

// The team template asks what the catalog's team preset asks.
it("asks the team prospecting template from the catalog's team preset", () => {
  const team = PREPARED_REPORTS.find(
    (report) => report.id === "team-prospecting-review",
  );
  if (team?.kind !== "question") throw new Error("it is a question");
  const withTeam = {
    ...reportingStoryCatalog,
    presets: [
      ...reportingStoryCatalog.presets,
      {
        id: "team" as const,
        metrics: ["meetings_held" as const, "accepted_opportunities" as const],
        blocks: ["sdr_outcomes" as const],
      },
    ],
  };
  const selection = preparedSelection(
    team,
    withTeam,
    reportingStoryScope,
    "p-1",
  );
  expect(selection?.metrics).toEqual([
    "meetings_held",
    "accepted_opportunities",
  ]);
  expect(selection?.blocks).toEqual(["sdr_outcomes"]);
  expect(selection?.period).toBe("this_week");
  expect(
    preparedSelection(team, reportingStoryCatalog, reportingStoryScope, "p-1"),
  ).toBeNull();
});
