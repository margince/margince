// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { ViewerZoneProvider } from "../lib/viewerzone";
import { meFixture } from "../testing/mefixture";
import { type ReminderTask, TaskRemindMenu } from "./taskreminder";
import { useTaskUpdate } from "./taskwrite";

const ZONE = "Europe/Berlin";
const TASK = "01a05500-0000-7000-8000-000000000b01";
const ME = meFixture({}).user.id;
const COLLEAGUE = "00000000-0000-4000-8000-0000000000c2";
// 14:00 in Berlin on 3 March 2026.
const NOW = new Date("2026-03-03T13:00:00.000Z");

let sent: { url: string; method: string; body: unknown }[] = [];

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(NOW);
  sent = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : undefined;
      const text = request
        ? await request.clone().text()
        : String(init?.body ?? "");
      sent.push({
        url: String(request ? request.url : input),
        method: request?.method ?? init?.method ?? "GET",
        body: text ? JSON.parse(text) : undefined,
      });
      return new Response(JSON.stringify({ id: TASK, version: 5 }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function renderMenu(task: Partial<ReminderTask> = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], meFixture({}));
  function Harness() {
    const update = useTaskUpdate([["tasks"]]);
    return (
      <TaskRemindMenu
        task={{ id: TASK, version: 4, assignee_id: ME, ...task }}
        update={update}
      />
    );
  }
  return render(
    <QueryClientProvider client={client}>
      <ViewerZoneProvider zone={ZONE}>
        <LocaleProvider initial="en">
          <ToastProvider>
            <Harness />
            <ToastRegion />
          </ToastProvider>
        </LocaleProvider>
      </ViewerZoneProvider>
    </QueryClientProvider>,
  );
}

const patches = () => sent.filter((call) => call.method === "PATCH");

describe("TaskRemindMenu", () => {
  it("sets the reminder an hour from now, pinned to the task's version", async () => {
    const user = userEvent.setup();
    renderMenu();
    await user.click(screen.getByRole("button", { name: "Remind me" }));
    await user.click(screen.getByRole("button", { name: "In 1 hour" }));
    await waitFor(() => expect(patches()).toHaveLength(1));
    expect(patches()[0]?.body).toEqual({
      remind_at: "2026-03-03T14:00:00.000Z",
    });
    expect(await screen.findByText(/^Reminder set for/)).toBeTruthy();
  });

  it("offers Remove only on a task that has a reminder, and clears it", async () => {
    const user = userEvent.setup();
    renderMenu({ remind_at: "2026-03-04T08:00:00.000Z" });
    await user.click(screen.getByRole("button", { name: "Remind me" }));
    await user.click(screen.getByRole("button", { name: "Remove reminder" }));
    await waitFor(() => expect(patches()).toHaveLength(1));
    expect(patches()[0]?.body).toEqual({ remind_at: null });
    expect(await screen.findByText("Reminder removed.")).toBeTruthy();
  });

  it("draws no Remove on a task without a reminder", async () => {
    const user = userEvent.setup();
    renderMenu();
    await user.click(screen.getByRole("button", { name: "Remind me" }));
    expect(screen.queryByRole("button", { name: "Remove reminder" })).toBe(
      null,
    );
  });

  it("is not offered on a colleague's task, whose reminder reaches only them", () => {
    renderMenu({ assignee_id: COLLEAGUE });
    expect(screen.queryByRole("button", { name: "Remind me" })).toBe(null);
  });

  it("refuses a picked time that has passed and sends a later one with its wall clock", async () => {
    const user = userEvent.setup();
    renderMenu();
    await user.click(screen.getByRole("button", { name: "Remind me" }));
    await user.click(screen.getByRole("button", { name: "Pick a time" }));
    const dialog = await screen.findByRole("dialog");
    const day = screen.getByRole("textbox", { name: "Day" });
    await user.type(day, "2026-03-03{Enter}");
    const time = screen.getByRole("textbox", { name: "Time" });
    await user.type(time, "09:00{Enter}");
    await user.click(screen.getByRole("button", { name: "Set reminder" }));
    expect(
      await screen.findByText("Pick a time that is still ahead."),
    ).toBeTruthy();
    expect(patches()).toHaveLength(0);

    await user.clear(time);
    await user.type(time, "16:30{Enter}");
    await user.click(screen.getByRole("button", { name: "Set reminder" }));
    await waitFor(() => expect(patches()).toHaveLength(1));
    expect(patches()[0]?.body).toEqual({
      remind_at: "2026-03-03T15:30:00.000Z",
      remind_intent: { wall_clock: "2026-03-03T16:30", timezone: ZONE },
    });
    await waitFor(() => expect(dialog.isConnected).toBe(false));
  });
});
