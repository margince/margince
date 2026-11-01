/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { stubClipboard } from "../../design-system/clipboard-testing";
import { meFixture } from "../../testing/mefixture";
import { defaultOperations } from "../../testing/operationsettings.fixtures";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { SystemHealthPage } from "./settings.systemhealth";
import { attentionReport, healthReport } from "./systemhealth.fixtures";

type Report = components["schemas"]["SystemHealth"];
function stub(report: Report = healthReport) {
  const fetcher = vi.fn(
    async (input: RequestInfo | URL, _init?: RequestInit) => {
      const path = new URL(
        input instanceof Request ? input.url : String(input),
        "https://test.local",
      ).pathname.replace(/^\/v1/, "");
      if (path === "/me")
        return jsonResponse(
          meFixture({
            roles: ["admin"],
            allow: {
              job_health: ["read"],
              embedding_reindex: ["read"],
              data_task: ["read"],
              installation_settings: ["read", "update"],
            },
          }),
        );
      if (path === "/admin/system-health") return jsonResponse(report);
      if (path === "/admin/job-health")
        return jsonResponse({ generated_at: report.generated_at, kinds: [] });
      if (path === "/admin/job-health/history")
        return jsonResponse({
          coverage: "retained_only",
          groups: [],
          runs: [
            {
              job_id: 4711,
              kind: "capture_classify",
              state: "discarded",
              attempt: 3,
              max_attempts: 3,
              failure_class: "provider_unavailable",
              reason: "Provider unavailable",
            },
          ],
          has_more: false,
        });
      if (path === "/installation/settings")
        return jsonResponse({ operations: defaultOperations });
      return jsonResponse({});
    },
  );
  vi.stubGlobal("fetch", fetcher);
  return fetcher;
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("loads only the compact overview and leaves detail reads lazy", async () => {
  const fetcher = stub();
  render(<SystemHealthPage />);
  expect(
    await screen.findByRole("heading", {
      name: "No action needed",
    }),
  ).toBeInTheDocument();
  const paths = fetcher.mock.calls.map(([request]) =>
    String(request instanceof Request ? request.url : request),
  );
  expect(paths.some((path) => path.includes("/admin/system-health"))).toBe(
    true,
  );
  expect(
    paths.some((path) =>
      /job-health|capture-health|automatic-work|recovery-health|installation\/settings/.test(
        path,
      ),
    ),
  ).toBe(false);
});

it("shows actionable findings and keeps supporting facts behind a click", async () => {
  const user = userEvent.setup();
  stub(attentionReport);
  render(<SystemHealthPage />);
  expect(
    await screen.findByRole("heading", { name: "Attention needed" }),
  ).toBeInTheDocument();
  expect(screen.queryByText(/Some evidence is missing/)).toBeNull();
  const panel = screen
    .getByRole("heading", {
      name: "Background work is delayed",
    })
    .closest("section");
  expect(panel).not.toBeNull();
  if (!panel) throw new Error("finding panel missing");
  expect(within(panel).queryByText("12 runs")).not.toBeVisible();
  await user.click(within(panel).getByText("Technical details"));
  expect(within(panel).getByText("12 runs")).toBeInTheDocument();
  expect(within(panel).getByText("Technical operator")).toBeInTheDocument();
  expect(
    within(panel).getByRole("button", { name: "Copy technical report" }),
  ).toBeInTheDocument();
});

it("opens configuration without requesting a health report", async () => {
  const fetcher = stub();
  render(<SystemHealthPage view="configuration" />);
  expect(
    await screen.findByRole("heading", { name: "Background schedules" }),
  ).toBeInTheDocument();
  expect(
    fetcher.mock.calls.some(([r]) =>
      String(r instanceof Request ? r.url : r).includes("/admin/system-health"),
    ),
  ).toBe(false);
});

it("copies safe evidence without sending a message", async () => {
  const user = userEvent.setup();
  const clipboard = stubClipboard("accepts");
  const fetcher = stub(attentionReport);
  render(<SystemHealthPage />);
  await user.click(await screen.findByText("Technical details"));
  await user.click(
    screen.getByRole("button", { name: "Copy technical report" }),
  );
  await waitFor(() => expect(clipboard.written).toHaveLength(1));
  const copied = JSON.parse(clipboard.written[0]);
  expect(copied.source).toBe("jobs");
  expect(copied.queues).toEqual([]);
  expect(copied.history.runs[0]).toMatchObject({
    job_id: 4711,
    attempt: 3,
    failure_class: "provider_unavailable",
  });
  expect(
    fetcher.mock.calls.every(
      ([, init]) => !init?.method || init.method === "GET",
    ),
  ).toBe(true);
});

it("withholds source conditions and counts without permission", async () => {
  const user = userEvent.setup();
  stub({
    ...healthReport,
    sources: [
      {
        id: "jobs",
        permission: "job_health:read",
        scope: "workspace",
        condition: "unknown",
        code: "withheld",
        availability: "withheld",
      },
    ],
  });
  render(<SystemHealthPage />);
  await screen.findByRole("heading", { name: "No action needed" });
  await user.click(screen.getByText("All checks"));
  expect(screen.queryByText(/Permission required/)).toBeNull();
  expect(screen.queryByText(/Background jobs/)).toBeNull();
});

it("leaves automatic retries out of the action list", async () => {
  stub({
    ...healthReport,
    findings: [
      {
        ...attentionReport.findings[0],
        id: "normal-retry",
        source: "jobs",
        code: "queue_retrying",
        condition: "retrying",
        requires_action: false,
      },
    ],
  });
  render(<SystemHealthPage />);
  await screen.findByRole("heading", { name: "No action needed" });
  expect(
    screen.queryByRole("heading", {
      name: "Failed work is waiting for another attempt",
    }),
  ).toBeNull();
  expect(
    screen.queryByRole("button", { name: "See problem and next step" }),
  ).toBeNull();
});

it("explains the installation effect of stopped mail sorting without claiming mail delivery stopped", async () => {
  stub({
    ...attentionReport,
    findings: [
      {
        ...attentionReport.findings[0],
        id: "sorting",
        source: "jobs",
        code: "queue_unverified",
        condition: "unverified",
        requires_action: true,
        job_kinds: ["capture_classify"],
      },
    ],
  });
  render(<SystemHealthPage />);
  await screen.findByRole("heading", {
    name: "Mail sorting: failed runs need review",
  });
  expect(
    screen.getByText(
      /In Worklist, emails that need a reply or a meeting follow-up/,
    ),
  ).toBeVisible();
  expect(screen.getByText(/Check your inbox manually/)).toBeVisible();
  expect(
    screen.getByText(/You can still receive and read email/),
  ).toBeVisible();
});
it("names the missing check instead of a vague evidence status", async () => {
  const user = userEvent.setup();
  stub();
  render(<SystemHealthPage />);
  await user.click(await screen.findByText("All checks"));
  expect(screen.getAllByText("No restore test recorded")[0]).toBeVisible();
  expect(screen.queryByText("Not enough evidence")).toBeNull();
});

it("limits a partial capture check to the categories it actually read", async () => {
  const user = userEvent.setup();
  stub({
    ...healthReport,
    sources: [
      {
        id: "capture",
        permission: "job_health:read",
        scope: "workspace",
        availability: "unknown",
        condition: "clear",
        code: "capture_checked",
        observed_at: healthReport.generated_at,
      },
    ],
  });
  render(<SystemHealthPage />);
  await user.click(await screen.findByText("All checks"));
  expect(screen.getByText(/Mail capture/)).toHaveTextContent(
    "No capture backlog detected in checked categories",
  );
  expect(screen.queryByText("Working normally")).toBeNull();
});

it("shows what a successful refusal check established about connector activity", async () => {
  const user = userEvent.setup();
  stub({
    ...healthReport,
    sources: [
      {
        id: "intake",
        permission: "job_health:read",
        scope: "workspace",
        availability: "available",
        condition: "clear",
        code: "intake_no_refusals",
        observed_at: healthReport.generated_at,
      },
    ],
  });
  render(<SystemHealthPage />);
  await user.click(await screen.findByText("All checks"));
  expect(screen.getByText(/Connector intake/)).toHaveTextContent(
    "activity is not verified",
  );
  expect(screen.queryByText("Working normally")).toBeNull();
});

it("does not assume rejected imports are still missing after a possible later success", async () => {
  stub({
    ...attentionReport,
    action_count: 1,
    sources: healthReport.sources.map((source) =>
      source.id === "intake"
        ? { ...source, condition: "technical", code: "intake_refused" }
        : source,
    ),
    findings: [
      {
        ...attentionReport.findings[0],
        id: "intake:refused",
        source: "intake",
        code: "intake_refused",
        condition: "technical",
        requires_action: true,
        count: 3,
        unit: "records",
      },
    ],
  });
  render(<SystemHealthPage />);
  expect(
    await screen.findByText(
      /CRM records and reports may be missing information/,
    ),
  ).toBeVisible();
  expect(
    screen.getByText(/imported successfully later before resubmitting/),
  ).toBeVisible();
});
