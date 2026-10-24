// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { requireVersion } from "../../api/version";
import { activityTimeline } from "../../design-system/activitytimeline";
import { useRecordTimeline } from "../../design-system/recordtimeline";
import { TimelineList } from "../../design-system/timeline";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { jsonResponse } from "../../testing/company.fixtures";
import { meFixture } from "../../testing/mefixture";
import { TimelineActions } from "./timelineactions";

type Activity = components["schemas"]["Activity"];
const TASK: Activity = {
  offers_email: false,
  id: "task-1",
  kind: "task",
  subject: "Send the revised quote",
  occurred_at: "2026-06-01T09:00:00Z",
  created_at: "2026-06-01T09:00:00Z",
  updated_at: "2026-06-01T09:00:00Z",
  captured_by: "human:u1",
  source: "manual",
  is_done: false,
  version: 3,
  content_state: "available",
  audience: "workspace",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function History() {
  const zone = useRecordZone();
  const timeline = useRecordTimeline("company", "company-1");
  return (
    <TimelineList
      zone={zone}
      entries={activityTimeline(timeline.activities, undefined, (activity) => (
        <TimelineActions
          activity={activity}
          entityType="company"
          entityId="company-1"
        />
      ))}
    />
  );
}

function show(task: Activity, canUpdate = true) {
  let current = task;
  const writes: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname.replace(/^\/v1/, "");
      if (path === "/me")
        return jsonResponse(
          meFixture({
            allow: { activity: canUpdate ? ["read", "update"] : ["read"] },
          }),
        );
      if (path === "/activities/task-1" && request.method === "PATCH") {
        writes.push(request.clone());
        const body: unknown = await request.json();
        if (
          typeof body !== "object" ||
          body === null ||
          !("is_done" in body) ||
          typeof body.is_done !== "boolean"
        )
          throw new Error("Unexpected task patch");
        current = {
          ...current,
          is_done: body.is_done,
          version: requireVersion(current.version) + 1,
        };
        return jsonResponse(current);
      }
      if (path === "/activities/task-1") return jsonResponse(current);
      if (path === "/activities")
        return jsonResponse({ data: [current], page: { has_more: false } });
      throw new Error(`Unexpected request: ${request.method} ${path}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["company360", "company-1"], {});
  client.setQueryData(["tasks"], {});
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <History />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { writes, client };
}

describe("tasks in record history", () => {
  it("keeps completed tasks labelled and opens their completed detail", async () => {
    const user = userEvent.setup();
    const { writes } = show({ ...TASK, is_done: true });
    expect(await screen.findByText("Completed")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Open task" }));
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText("Completed")).toBeTruthy();
    expect(within(dialog).queryByRole("button", { name: "Done" })).toBeNull();
    expect(writes).toHaveLength(0);
  });

  it("completes and undoes from History, refreshing both the history and task reads", async () => {
    const user = userEvent.setup();
    const { writes, client } = show(TASK);
    expect(await screen.findByText("Open")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Open task" }));
    await user.click(await screen.findByRole("button", { name: "Done" }));
    expect(await screen.findByText("Task completed")).toBeTruthy();
    await waitFor(() => expect(screen.queryByText("Open")).toBeNull());
    expect(writes[0]?.headers.get("If-Match")).toContain("3");
    expect(
      client.getQueryState(["company360", "company-1"])?.isInvalidated,
    ).toBe(true);
    expect(client.getQueryState(["tasks"])?.isInvalidated).toBe(true);
    await user.click(screen.getByRole("button", { name: "Undo" }));
    expect(await screen.findByText("Open")).toBeTruthy();
    expect(writes[1]?.headers.get("If-Match")).toContain("4");
    expect(await writes[1]?.json()).toEqual({ is_done: false });
  });

  it("opens an open task read-only without update permission", async () => {
    const user = userEvent.setup();
    const { writes } = show(TASK, false);
    await user.click(await screen.findByRole("button", { name: "Open task" }));
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText("Send the revised quote");
    expect(within(dialog).queryByRole("button", { name: "Done" })).toBeNull();
    expect(writes).toHaveLength(0);
  });

  it("offers no task panel or status for withheld content", async () => {
    show({ ...TASK, content_state: "withheld", subject: undefined });
    await screen.findByText("Task");
    expect(screen.queryByRole("button", { name: "Open task" })).toBeNull();
    expect(screen.queryByText("Open")).toBeNull();
    expect(screen.queryByText("Completed")).toBeNull();
  });
});
