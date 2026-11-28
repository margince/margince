/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { stubClipboard } from "../../design-system/clipboard-testing";
import { en as englishCopy } from "../../i18n/en";
import { replaceDial } from "../../lib/urlstate";
import { meFixture } from "../../testing/mefixture";
import { defaultOperations } from "../../testing/operationsettings.fixtures";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { SystemHealthPage } from "./settings.systemhealth";
import {
  attentionReport,
  healthReport,
  informationReport,
  overdueFinding,
} from "./systemhealth.fixtures";

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
  window.location.hash = "";
  vi.unstubAllGlobals();
});

it("loads only the compact overview and leaves detail reads lazy", async () => {
  const fetcher = stub();
  render(<SystemHealthPage />);
  expect(
    await screen.findByRole("heading", {
      name: englishCopy["health.incomplete"],
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
    await screen.findByRole("heading", {
      name: englishCopy["health.attention"],
    }),
  ).toBeInTheDocument();
  expect(screen.queryByText(/Some evidence is missing/)).toBeNull();
  const panel = screen
    .getByRole("heading", {
      name: "Mail sorting: delayed",
    })
    .closest("section");
  expect(panel).not.toBeNull();
  if (!panel) throw new Error("finding panel missing");
  expect(within(panel).getByText(/12 runs · Technical operator/)).toBeVisible();
  await user.click(
    within(panel).getByText(englishCopy["health.evidenceDetails"]),
  );
  expect(within(panel).getByText(/12 runs · Technical operator/)).toBeVisible();
  expect(
    within(panel).getByText(englishCopy["health.needsFix"]).closest(".badge"),
  ).toHaveClass("badge-danger");
  expect(
    within(panel).getByRole("button", { name: englishCopy["health.copy"] }),
  ).toBeInTheDocument();
});

it("opens configuration without requesting a health report", async () => {
  const fetcher = stub();
  render(<SystemHealthPage view="configuration" />);
  expect(
    await screen.findByRole("heading", {
      name: englishCopy["operations.schedules.title"],
    }),
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
  await user.click(
    await screen.findByText(englishCopy["health.evidenceDetails"]),
  );
  await user.click(
    screen.getByRole("button", { name: englishCopy["health.copy"] }),
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
  await screen.findByRole("heading", {
    name: englishCopy["health.incomplete"],
  });
  expect(
    await screen.findByRole("heading", { name: "All checks" }),
  ).toBeVisible();
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
  await screen.findByRole("heading", {
    name: englishCopy["health.incomplete"],
  });
  expect(
    screen.queryByRole("heading", {
      name: englishCopy["health.code.queue_retrying"],
    }),
  ).toBeNull();
  expect(
    screen.queryByRole("button", { name: englishCopy["health.inspect"] }),
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
    name: "Mail sorting: failed runs",
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
  stub();
  render(<SystemHealthPage />);
  expect(
    await screen.findByRole("heading", { name: "All checks" }),
  ).toBeVisible();
  expect(
    screen.getAllByText(englishCopy["health.code.recovery_missing"])[0],
  ).toBeVisible();
  expect(screen.queryByText("Not enough evidence")).toBeNull();
});

it("limits a partial capture check to the categories it actually read", async () => {
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
  expect(
    await screen.findByRole("heading", { name: "All checks" }),
  ).toBeVisible();
  expect(
    screen.getByText(/Mail capture/, { selector: ".disclosure-label" }),
  ).toHaveTextContent(englishCopy["health.code.capture_checked"]);
  expect(screen.queryByText("Working normally")).toBeNull();
});

it("shows what a successful refusal check established about connector activity", async () => {
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
  expect(
    await screen.findByRole("heading", { name: "All checks" }),
  ).toBeVisible();
  expect(
    screen.getByText(/Connector intake/, { selector: ".disclosure-label" }),
  ).toHaveTextContent("activity is not verified");
  expect(screen.queryByText("Working normally")).toBeNull();
});

it("files the changes Home leaves out under For information, never Passing", async () => {
  stub(informationReport);
  render(<SystemHealthPage />);
  const group = await screen.findByRole("list", {
    name: englishCopy["health.group.information"],
  });
  expect(
    within(group).getByText(englishCopy["health.source.unplaced"]),
  ).toBeVisible();
  expect(
    within(group).getByText("Information").closest(".badge"),
  ).not.toHaveClass("badge-success");
  const passing = screen.getByRole("list", { name: "Passing" });
  expect(
    within(passing).queryByText(englishCopy["health.source.unplaced"]),
  ).toBeNull();
  expect(screen.queryByText(/Problems? needing action/)).toBeNull();
  expect(screen.getByText("5 passing")).toBeVisible();

  expect(
    await screen.findByRole("heading", { name: "All checks" }),
  ).toBeVisible();
  const value = (term: string) =>
    screen.getByText(term, { selector: "dt" }).nextElementSibling;
  expect(value("notice · update")).toHaveTextContent("3");
  expect(value("booking_page · update")).toHaveTextContent("2");
  const chip = screen
    .getByText(/Changes Home leaves out/, { selector: ".disclosure-label" })
    .querySelector(".badge");
  expect(chip).toHaveClass("badge");
  expect(chip).not.toHaveClass("badge-success");
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

it("shows one actionable sender review instead of the duplicate mailbox summary", async () => {
  const user = userEvent.setup();
  const report: Report = {
    ...attentionReport,
    action_count: 2,
    findings: ["capture_owner_review", "capture_uncertain"].map(
      (code, index) => ({
        ...overdueFinding,
        id: `capture:${code}`,
        source: "capture",
        code,
        condition: "waiting_human",
        responsible: "mailbox_owner",
        count: index + 1,
        unit: index === 0 ? "mailboxes" : "records",
        job_kinds: undefined,
      }),
    ),
  };
  stub(report);
  render(<SystemHealthPage />);
  const group = await screen.findByRole("region", {
    name: englishCopy["health.code.capture_uncertain"],
  });
  expect(within(group).getByText(/2 records · Mailbox owner/)).toBeVisible();
  expect(screen.queryByText(/1 mailbox · Mailbox owner/)).toBeNull();
  expect(screen.getByText("Problem needing action: 1")).toBeVisible();
  await user.click(
    within(group).getByRole("button", {
      name: englishCopy["health.code.capture_owner_review"],
    }),
  );
  expect(
    await screen.findByRole("dialog", {
      name: englishCopy["health.mailReviewOwners"],
    }),
  ).toBeVisible();
});

it("records diagnostics category changes and clears the previous finding", async () => {
  const user = userEvent.setup();
  window.location.hash =
    "#/settings/system-health/diagnostics?section=settings.jobs&finding=jobs%3Aqueue_overdue&kind=capture_classify&state=discarded";
  stub(attentionReport);
  render(<SystemHealthPage view="diagnostics" />);
  await user.click(
    await screen.findByRole("button", {
      name: englishCopy["health.source.updates"],
    }),
  );
  await waitFor(() =>
    expect(window.location.hash).toContain("section=dataTasks.title"),
  );
  expect(window.location.hash).not.toMatch(/finding=|kind=|state=/);
  window.location.hash = "";
});

it("makes a focused diagnostics filter visible and removable", async () => {
  const user = userEvent.setup();
  const second = {
    ...overdueFinding,
    id: "jobs:second",
    code: "queue_unverified",
    condition: "unverified",
  } satisfies Report["findings"][number];
  stub({
    ...attentionReport,
    action_count: 2,
    findings: [overdueFinding, second],
  });
  window.location.hash =
    "#/settings/system-health/diagnostics?section=settings.jobs&finding=jobs%3Aqueue_overdue";
  render(<SystemHealthPage view="diagnostics" />);
  expect(await screen.findByText("Showing 1 of 2 problems")).toBeVisible();
  expect(screen.queryByRole("heading", { name: /failed runs/ })).toBeNull();
  await user.click(
    screen.getByRole("button", { name: englishCopy["health.showAllFindings"] }),
  );
  expect(
    await screen.findByRole("heading", { name: /failed runs/ }),
  ).toBeVisible();
  expect(window.location.hash).not.toMatch(/finding=|kind=|state=/);
});

it("keeps the outgoing-link check discoverable under All checks", async () => {
  stub({
    ...healthReport,
    public_origin: { origin: "https://crm.example.test", reachable: true },
  });
  render(<SystemHealthPage />);
  expect(
    await screen.findByRole("heading", { name: "All checks" }),
  ).toBeVisible();
  expect(
    screen
      .getByText(englishCopy["connectors.originReachable"])
      .closest(".badge"),
  ).not.toHaveClass("badge-success");
});

it("explains which company-page details a failed lookup can leave outdated", async () => {
  stub({
    ...attentionReport,
    findings: [{ ...overdueFinding, job_kinds: ["technical_enrich_company"] }],
  });
  render(<SystemHealthPage />);
  expect(
    await screen.findByText(
      /On company pages, automatically collected mail provider, email security and hosting details/,
    ),
  ).toBeVisible();
  expect(
    screen.getByRole("button", { name: englishCopy["health.inspectWork"] }),
  ).toBeVisible();
});

// Every history read, as the query it asked; the first page offers a second.
function stubHistory(): URLSearchParams[] {
  const fetcher = stub(attentionReport);
  const others = fetcher.getMockImplementation();
  const asked: URLSearchParams[] = [];
  fetcher.mockImplementation(async (input, init) => {
    const url = input instanceof Request ? input.url : String(input);
    if (!url.includes("/admin/job-health/history"))
      return others ? others(input, init) : jsonResponse({});
    const query = new URL(url, "https://test.local").searchParams;
    asked.push(query);
    const first = !query.has("cursor");
    return jsonResponse({
      coverage: "retained_only",
      observed_at: attentionReport.generated_at,
      cutoff: attentionReport.generated_at,
      window_start: attentionReport.generated_at,
      groups: [],
      runs: [],
      has_more: first,
      ...(first ? { next_cursor: "page-2" } : {}),
      population_may_have_changed: false,
    });
  });
  return asked;
}

const JOBS = "#/settings/system-health/diagnostics?section=settings.jobs";
const pill = async (group: string, name: string) =>
  within(await screen.findByRole("group", { name: group })).getByRole(
    "button",
    { name },
  );

it("keeps the failure history's state filter in the address, both ways", async () => {
  const user = userEvent.setup();
  const asked = stubHistory();
  window.location.hash = JOBS;
  render(<SystemHealthPage view="diagnostics" />);
  await user.click(await pill("State", "Retrying"));
  expect(window.location.hash).toContain("state=retryable");
  // Back or Forward to an entry that names another state.
  act(() => replaceDial("state", "cancelled"));
  expect(await pill("State", "Canceled")).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await waitFor(() =>
    expect(asked.map((query) => query.get("state"))).toContain("cancelled"),
  );
});

it("keeps the pressed pill and the window when a state filter moves the history", async () => {
  const user = userEvent.setup();
  stubHistory();
  window.location.hash = JOBS;
  render(<SystemHealthPage view="diagnostics" />);
  await user.click(await pill("Reporting window", "Last day"));
  // Discarded runs lead the section, so this one also moves the history.
  const discarded = await pill("State", "Discarded");
  await user.click(discarded);
  expect(window.location.hash).toContain("state=discarded");
  await waitFor(() => expect(document.activeElement).toBe(discarded));
  expect(await pill("Reporting window", "Last day")).toHaveAttribute(
    "aria-pressed",
    "true",
  );
});

it("starts a changed filter on its first page", async () => {
  const user = userEvent.setup();
  const asked = stubHistory();
  window.location.hash = JOBS;
  render(<SystemHealthPage view="diagnostics" />);
  await user.click(await screen.findByRole("button", { name: "Next page" }));
  await waitFor(() => expect(asked.at(-1)?.get("cursor")).toBe("page-2"));
  await user.click(await pill("State", "Retrying"));
  await waitFor(() => expect(asked.at(-1)?.get("state")).toBe("retryable"));
  expect(asked.at(-1)?.has("cursor")).toBe(false);
});
