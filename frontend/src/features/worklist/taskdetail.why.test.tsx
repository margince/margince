/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { TaskDetailModal } from "./taskdetailmodal";
import type { WorklistItem } from "./worklist.queries";

afterEach(cleanup);

// A task opened from the list says what made it and why it is on the list.
//
// A reminder an automation wrote has a title, a record and a date, and no body.
// A drawer showing only those three cannot say what the reminder wants. Nor
// can it say why the reminder outranks the rest of the day.

const QUIET_REMINDER = {
  id: "task-1",
  kind: "task",
  writable: true,
  subject: "Check in — no activity since 2026-06-30",
  body: null,
  due_at: "2026-09-10T21:59:59Z",
  occurred_at: "2026-09-06T13:42:00Z",
  is_done: false,
  source_system: "no_activity_reminder",
  captured_by: "system:automation",
};

const ROW: WorklistItem = {
  id: "task-1",
  source: "task",
  category: "tasks",
  level: 2,
  consequence: "task_slips",
  because: [{ kind: "due_today" }, { kind: "unassigned" }],
  actions: ["complete", "snooze"],
};

function openTask(task: Record<string, unknown>, item?: WorklistItem) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "update"] }, { seat: "full" }),
    "GET /activities/task-1": () => jsonResponse(task),
  });
  return render(
    <StoryProviders>
      <TaskDetailModal
        activityId="task-1"
        item={item}
        readOnly={false}
        onClose={() => {}}
        update={
          {
            mutate: () => {},
            isPending: false,
          } as unknown as Parameters<typeof TaskDetailModal>[0]["update"]
        }
      />
    </StoryProviders>,
  );
}

test("a reminder an automation wrote says which one and what it asks", async () => {
  openTask(QUIET_REMINDER, ROW);

  expect(
    await screen.findByText(en["tasks.origin.no_activity_reminder"]),
  ).toBeTruthy();
  expect(screen.getByText(en["worklist.why.title"])).toBeTruthy();
  expect(
    screen.getByText(
      `${en["worklist.because.due_today"]} · ${en["worklist.because.unassigned"]}`,
    ),
  ).toBeTruthy();
  expect(screen.getByText(en["worklist.consequence.task_slips"])).toBeTruthy();
});

test("an automation with no family of its own still says it was one", async () => {
  openTask({ ...QUIET_REMINDER, source_system: null });

  expect(await screen.findByText(en["tasks.origin.system"])).toBeTruthy();
});

test("a task a colleague typed claims no automation", async () => {
  openTask({
    ...QUIET_REMINDER,
    subject: "Send the renewal paperwork",
    source_system: null,
    captured_by: "human:u-1",
  });

  // The positive control: the task has drawn before anything is ruled out.
  await screen.findByText("Send the renewal paperwork");
  expect(screen.queryByText(en["tasks.origin.system"])).toBeNull();
  expect(
    screen.queryByText(en["tasks.origin.no_activity_reminder"]),
  ).toBeNull();
});

test("a task opened from a record page has no list to explain it", async () => {
  openTask(QUIET_REMINDER);

  await screen.findByText(en["tasks.origin.no_activity_reminder"]);
  expect(screen.queryByText(en["worklist.why.title"])).toBeNull();
});

test("a row with nothing to say draws no empty panel", async () => {
  openTask(QUIET_REMINDER, { ...ROW, because: [], consequence: "none" });

  await screen.findByText(en["tasks.origin.no_activity_reminder"]);
  expect(screen.queryByText(en["worklist.why.title"])).toBeNull();
});
