/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

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
import { reportingStoryReport } from "../../testing/reporting.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ReportingReportDetail } from "./reporting.report";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

const revision = (
  number: number,
  state: "draft" | "published" | "earlier",
) => ({
  revision: number,
  state,
  created_at: "2026-09-20T09:00:00Z",
  created_by: "maya",
  selection: reportingStoryEvaluation.selection,
});

// Every request the page sent, as method, path, query and headers.
function sentRequests() {
  const fetch = vi.spyOn(globalThis, "fetch");
  return () =>
    fetch.mock.calls
      .map(([input]) => input)
      .filter((input): input is Request => input instanceof Request)
      .map((request) => ({
        method: request.method,
        url: new URL(request.url),
        ifMatch: request.headers.get("If-Match"),
      }));
}

function renderReport() {
  render(
    <StoryProviders>
      <ReportingReportDetail reportId="report" />
    </StoryProviders>,
  );
}

it("lets an editor preview, then publish, an open draft", async () => {
  const user = userEvent.setup({ delay: null });
  const publish = vi.fn(() =>
    jsonResponse({ ...reportingStoryReport, revision: 4, version: 5 }),
  );
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/reports/report": () =>
      jsonResponse({ ...reportingStoryReport, draft_revision: 4, version: 4 }),
    "GET /analytics/reports/report/revisions": () =>
      jsonResponse({ data: [revision(4, "draft"), revision(3, "published")] }),
    "POST /analytics/reports/report/draft/publish": publish,
  });
  const sent = sentRequests();
  renderReport();

  await user.click(
    await screen.findByRole("button", { name: "Preview draft" }),
  );
  await waitFor(() =>
    expect(
      sent().some(
        (r) =>
          r.url.pathname.endsWith("/evaluation") &&
          r.url.searchParams.get("revision") === "4",
      ),
    ).toBe(true),
  );
  expect(screen.getByRole("button", { name: "Save snapshot" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "Publish draft" }));
  await waitFor(() => expect(publish).toHaveBeenCalledOnce());
  expect(
    sent().find((r) => r.url.pathname.endsWith("/draft/publish"))?.ifMatch,
  ).toContain("4");
});

it("saves an edit as a draft, sending the charts alone", async () => {
  const user = userEvent.setup({ delay: null });
  let body: { selection?: { blocks: string[] } } | undefined;
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/reports/report/revisions": () =>
      jsonResponse({ data: [revision(3, "published")] }),
    "PUT /analytics/reports/report/draft": (sentBody) => {
      body = sentBody as typeof body;
      return jsonResponse({ ...reportingStoryReport, draft_revision: 4 });
    },
  });
  const sent = sentRequests();
  renderReport();
  await user.click(
    await screen.findByRole("button", { name: "Report actions" }),
  );
  await user.click(screen.getByRole("button", { name: "Edit report" }));
  const dialog = await screen.findByRole("dialog", { name: "Edit report" });
  const published = reportingStoryReport.selection.blocks;
  const dropped = within(dialog)
    .getAllByRole("checkbox")
    .filter((box) => (box as HTMLInputElement).checked)
    .at(-1);
  if (!dropped) throw new Error("the dialog shows no chosen chart to drop");
  await user.click(dropped);
  await user.click(
    within(dialog).getByRole("button", { name: "Save as draft" }),
  );
  await waitFor(() => expect(body).toBeDefined());
  expect(Object.keys(body ?? {})).toEqual(["selection"]);
  expect(body?.selection?.blocks).toEqual(published.slice(0, -1));
  expect(sent().find((r) => r.method === "PUT")?.ifMatch).toContain(
    String(reportingStoryReport.version),
  );
});

it("keeps a rename out of a draft, which holds charts alone", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/reports/report/revisions": () =>
      jsonResponse({ data: [revision(3, "published")] }),
  });
  renderReport();
  await user.click(
    await screen.findByRole("button", { name: "Report actions" }),
  );
  await user.click(screen.getByRole("button", { name: "Edit report" }));
  const dialog = await screen.findByRole("dialog", { name: "Edit report" });
  await user.type(
    within(dialog).getByRole("textbox", { name: /report name/i }),
    "!",
  );
  expect(
    within(dialog).getByRole("button", { name: "Save as draft" }),
  ).toBeDisabled();
});

it("restores an earlier revision from the version list", async () => {
  const user = userEvent.setup({ delay: null });
  let restored = false;
  const restore = vi.fn(() => {
    restored = true;
    return jsonResponse({ ...reportingStoryReport, revision: 4, version: 4 });
  });
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/reports/report/revisions": () =>
      jsonResponse({
        data: restored
          ? [
              revision(4, "published"),
              revision(3, "earlier"),
              revision(2, "earlier"),
            ]
          : [revision(3, "published"), revision(2, "earlier")],
      }),
    "POST /analytics/reports/report/revisions/2/restore": restore,
  });
  renderReport();
  await user.click(await screen.findByText("History and schedules"));
  await user.click(
    await screen.findByRole("button", { name: "Restore version 2" }),
  );
  await waitFor(() => expect(restore).toHaveBeenCalledOnce());
  expect(
    await screen.findByRole("button", { name: "Restore version 3" }),
  ).toBeVisible();
});

it("shows a reader no draft and no version list", async () => {
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/reports/report": () =>
      jsonResponse({ ...reportingStoryReport, can_manage: false }),
  });
  const sent = sentRequests();
  renderReport();
  await screen.findByRole("heading", { name: reportingStoryReport.name });
  expect(screen.queryByRole("button", { name: "Preview draft" })).toBeNull();
  expect(sent().some((r) => r.url.pathname.endsWith("/revisions"))).toBe(false);
});
