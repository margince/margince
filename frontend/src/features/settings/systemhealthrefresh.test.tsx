/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { meFixture } from "../../testing/mefixture";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { SystemHealthPage } from "./settings.systemhealth";
import { attentionReport } from "./systemhealth.fixtures";
import { useJobHistory } from "./systemhealth.queries";

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-14T09:00:00Z"));
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
async function settle(ms = 10) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}
function serve(read: () => Promise<Response>, history?: unknown) {
  const calls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request =
        input instanceof Request ? input : new Request(String(input));
      const path = new URL(request.url, "https://test.local").pathname;
      calls.push(path);
      if (path.endsWith("/me"))
        return jsonResponse(
          meFixture({ roles: ["admin"], allow: { job_health: ["read"] } }),
        );
      if (path.endsWith("/admin/system-health")) return read();
      if (path.endsWith("/admin/job-health/history"))
        return jsonResponse(history);
      return jsonResponse({});
    }),
  );
  return calls;
}
it("starts no summary poll while hidden and refreshes on return", async () => {
  const visibility = vi
    .spyOn(document, "visibilityState", "get")
    .mockReturnValue("hidden");
  const calls = serve(async () => jsonResponse(attentionReport));
  render(<SystemHealthPage />);
  await settle(120_000);
  expect(calls.filter((p) => p.endsWith("/admin/system-health"))).toHaveLength(
    0,
  );
  visibility.mockReturnValue("visible");
  act(() => document.dispatchEvent(new Event("visibilitychange")));
  await settle();
  expect(
    screen.getByRole("heading", { name: "Attention needed" }),
  ).toBeInTheDocument();
  const before = calls.length;
  visibility.mockReturnValue("hidden");
  act(() => document.dispatchEvent(new Event("visibilitychange")));
  await settle(180_000);
  expect(calls).toHaveLength(before);
});
it("keeps last-known findings visible after bounded read failures", async () => {
  let failed = false;
  serve(async () => {
    if (failed) throw new Error("synthetic source outage");
    return jsonResponse(attentionReport);
  });
  render(<SystemHealthPage />);
  await settle();
  const refresh = screen.getByRole("button", { name: "Refresh" });
  refresh.focus();
  failed = true;
  fireEvent.click(refresh);
  await settle(4_000);
  expect(
    screen.getByText("Last known evidence; the latest check did not complete."),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("heading", {
      name: "Background work is delayed",
    }),
  ).toBeInTheDocument();
  expect(document.activeElement).toBe(refresh);
});
it("coalesces automatic polling with an in-flight explicit refresh", async () => {
  let count = 0;
  let finish: ((response: Response) => void) | undefined;
  serve(async () => {
    count++;
    if (count === 1) return jsonResponse(attentionReport);
    return new Promise<Response>((resolve) => {
      finish = resolve;
    });
  });
  render(<SystemHealthPage />);
  await settle();
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await settle(60_000);
  expect(count).toBe(2);
  if (!finish) throw new Error("refresh did not start");
  await act(async () => {
    finish?.(jsonResponse(attentionReport));
  });
  await settle();
  expect(
    screen.getByRole("heading", { name: "Attention needed" }),
  ).toBeInTheDocument();
});

it("rejects an all-clear payload that omits its available source observation", async () => {
  serve(async () =>
    jsonResponse({
      ...attentionReport,
      assessment: "checked",
      complete: true,
      findings: [],
      sources: [
        {
          id: "jobs",
          permission: "job_health:read",
          scope: "workspace",
          availability: "available",
          condition: "clear",
          code: "queue_checked",
        },
      ],
    }),
  );
  render(<SystemHealthPage />);
  await settle(4_000);
  expect(screen.getByRole("alert")).toBeInTheDocument();
  expect(
    screen.queryByRole("heading", {
      name: "No issues detected in checked services",
    }),
  ).toBeNull();
});

it("keeps finding order during a poll and applies priorities on explicit refresh", async () => {
  const first = attentionReport.findings[0];
  if (!first) throw new Error("missing fixture finding");
  const other = {
    ...first,
    id: "capture:exhausted",
    source: "capture",
    code: "capture_exhausted",
  };
  let next = false;
  serve(async () =>
    jsonResponse({
      ...attentionReport,
      findings: next ? [other, first] : [first, other],
    }),
  );
  render(<SystemHealthPage />);
  await settle();
  const headings = () =>
    screen.getAllByRole("heading").map((h) => h.textContent);
  const before = headings();
  const refresh = screen.getByRole("button", { name: "Refresh" });
  refresh.focus();
  next = true;
  await settle(60_000);
  expect(headings()).toEqual(before);
  expect(document.activeElement).toBe(refresh);
  fireEvent.click(refresh);
  await settle();
  expect(headings()).not.toEqual(before);
  expect(headings()[1]).toBe(before[2]);
});

it("clears withdrawn evidence when the reader's grants change", async () => {
  serve(async () => jsonResponse(attentionReport));
  const { client } = render(<SystemHealthPage />);
  await settle();
  const oldKey = client
    .getQueryCache()
    .findAll({ queryKey: ["system-health"] })
    .find((entry) => entry.state.data !== undefined)?.queryKey;
  if (!oldKey) throw new Error("summary cache missing");
  expect(client.getQueryData(oldKey)).toMatchObject({
    assessment: "attention",
  });
  act(() =>
    client.setQueryData(
      ["me"],
      meFixture({ roles: ["rep"], allow: { embedding_reindex: ["read"] } }),
    ),
  );
  await settle();
  expect(client.getQueryData(oldKey)).toBeUndefined();
});

it("rejects an unknown unit before it can render a misleading count", async () => {
  const first = attentionReport.findings[0];
  if (!first) throw new Error("missing finding fixture");
  serve(async () =>
    jsonResponse({
      ...attentionReport,
      findings: [{ ...first, unit: "unknown_entity" }],
    }),
  );
  render(<SystemHealthPage />);
  await settle(4_000);
  expect(screen.getByRole("alert")).toBeInTheDocument();
  expect(
    screen.queryByRole("heading", { name: "Background work is delayed" }),
  ).toBeNull();
});

it("keeps the priority notice quiet when a refresh also contains routine findings", async () => {
  const action = attentionReport.findings[0];
  if (!action) throw new Error("missing finding fixture");
  serve(async () =>
    jsonResponse({
      ...attentionReport,
      findings: [
        {
          ...action,
          id: "routine",
          requires_action: false,
          condition: "retrying",
          code: "queue_retrying",
        },
        action,
      ],
    }),
  );
  render(<SystemHealthPage />);
  await settle();
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await settle();
  expect(
    screen.queryByText("Priorities changed. Refresh to apply the new order."),
  ).toBeNull();
});

it.each(["first_observed_at", "window_start", "window_end"])(
  "rejects a malformed %s before formatting it",
  async (field) => {
    const first = attentionReport.findings[0];
    if (!first) throw new Error("missing finding fixture");
    serve(async () =>
      jsonResponse({
        ...attentionReport,
        findings: [{ ...first, [field]: "invalid-date" }],
      }),
    );
    render(<SystemHealthPage />);
    await settle(4_000);
    expect(screen.getByRole("alert")).toBeInTheDocument();
  },
);

function HistoryProbe() {
  const query = useJobHistory({ days: 7 });
  return query.isError ? (
    <p role="alert">History unreadable</p>
  ) : (
    <p>{query.data ? "History loaded" : "Waiting"}</p>
  );
}
it.each([
  { state: "future-state" },
  { failed_at: "invalid-date" },
  { first_failed_at: "invalid-date" },
  { attempt: -1 },
  { max_attempts: 1.5 },
  { reason: { private_field: "not-an-operator-message" } },
])("rejects malformed retained-run fields: %j", async (override) => {
  serve(async () => jsonResponse(attentionReport), {
    observed_at: attentionReport.generated_at,
    cutoff: attentionReport.generated_at,
    window_start: attentionReport.generated_at,
    coverage: "retained_only",
    has_more: false,
    population_may_have_changed: false,
    groups: [],
    runs: [
      {
        job_id: 4711,
        kind: "capture_classify",
        state: "discarded",
        attempt: 3,
        max_attempts: 3,
        failed_at: attentionReport.generated_at,
        reason: "The job could not finish.",
        ...override,
      },
    ],
  });
  render(<HistoryProbe />);
  await settle(4_000);
  expect(screen.getByRole("alert")).toHaveTextContent("History unreadable");
  expect(screen.queryByText("History loaded")).toBeNull();
});
