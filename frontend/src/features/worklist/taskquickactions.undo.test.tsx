/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  TaskDueDatePick,
  TaskQuickActions,
  useTaskUpdate,
} from "../../components/taskactions";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { ViewerZoneProvider } from "../../lib/viewerzone";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

type Write = { body: unknown; ifMatch: string | null };

// Every PATCH answers with version 7, the version an Undo must be pinned on.
function recordWrites(): Write[] {
  const writes: Write[] = [];
  installFetchStub({});
  const routed = globalThis.fetch;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.method === "PATCH") {
        writes.push({
          body: await request.clone().json(),
          ifMatch: request.headers.get("If-Match"),
        });
        return jsonResponse({ id: "task-1", version: 7 });
      }
      return routed(input, init);
    }),
  );
  return writes;
}

// Due 1 September 2026 at 15:00 in Berlin, with the agreed clock on record.
const DATED = {
  dueAt: "2026-09-01T13:00:00.000Z",
  dueHasTime: true,
  dueIntent: { wall_clock: "2026-09-01T15:00", timezone: "Europe/Berlin" },
};
const DATED_BACK = {
  due_at: DATED.dueAt,
  due_has_time: true,
  due_intent: DATED.dueIntent,
};

function DatedTask() {
  const update = useTaskUpdate([]);
  const task = { activityId: "task-1", version: 2, update, ...DATED };
  return (
    <>
      <TaskQuickActions {...task} />
      <TaskDueDatePick {...task} />
    </>
  );
}

function renderDated() {
  render(
    <StoryProviders>
      <ViewerZoneProvider zone="Europe/Berlin">
        <ToastProvider>
          <DatedTask />
          <ToastRegion />
        </ToastProvider>
      </ViewerZoneProvider>
    </StoryProviders>,
  );
}

function Row() {
  const update = useTaskUpdate([]);
  return <TaskQuickActions activityId="task-1" version={2} update={update} />;
}

describe("the Done verb beside a task", () => {
  it("confirms with an Undo that reopens the task on the version the write produced", async () => {
    const writes: { body: unknown; ifMatch: string | null }[] = [];
    installFetchStub({});
    const routed = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(String(input), init);
        if (request.method === "PATCH") {
          writes.push({
            body: await request.clone().json(),
            ifMatch: request.headers.get("If-Match"),
          });
          return jsonResponse({ id: "task-1", version: 7 });
        }
        return routed(input, init);
      }),
    );
    render(
      <StoryProviders>
        <ToastProvider>
          <Row />
          <ToastRegion />
        </ToastProvider>
      </StoryProviders>,
    );
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Done" }));
    expect(await screen.findByText("Task completed")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Undo" }));

    await waitFor(() => expect(writes).toHaveLength(2));
    expect(writes[0].body).toEqual({ is_done: true });
    expect(writes[1].body).toEqual({ is_done: false });
    expect(writes[1].ifMatch).toContain("7");
  });
});

describe("moving a task's deadline", () => {
  it("confirms Snooze 1 day with the new deadline, and Undo puts the old one back", async () => {
    vi.setSystemTime(new Date("2026-08-25T08:00:00Z"));
    const writes = recordWrites();
    renderDated();
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Snooze 1 day" }));
    expect(await screen.findByText("Moved to 2 Sept, 15:00")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Undo" }));

    await waitFor(() => expect(writes).toHaveLength(2));
    expect(writes[0].body).toMatchObject({
      due_intent: { wall_clock: "2026-09-02T15:00" },
    });
    expect(writes[0].ifMatch).toContain("2");
    expect(writes[1].body).toEqual(DATED_BACK);
    expect(writes[1].ifMatch).toContain("7");
  });

  it("confirms Move with the day and time it sent, and Undo puts the old deadline back", async () => {
    vi.setSystemTime(new Date("2026-08-25T08:00:00Z"));
    const writes = recordWrites();
    renderDated();
    const user = userEvent.setup();

    const day = screen.getByRole("textbox", { name: "Move to" });
    await user.tripleClick(day);
    await user.keyboard("01/11/2026");
    await user.tab();
    const time = screen.getByRole("textbox", { name: "Time (optional)" });
    await user.clear(time);
    await user.type(time, "07:15");
    await user.tab();
    await user.click(screen.getByRole("button", { name: "Move" }));

    expect(await screen.findByText("Moved to 1 Nov, 07:15")).toBeTruthy();
    expect(writes).toHaveLength(1);
    expect(writes[0].body).toMatchObject({
      due_intent: { wall_clock: "2026-11-01T07:15" },
    });
    await user.click(screen.getByRole("button", { name: "Undo" }));

    await waitFor(() => expect(writes).toHaveLength(2));
    expect(writes[1].body).toEqual(DATED_BACK);
    expect(writes[1].ifMatch).toContain("7");
  });
});
