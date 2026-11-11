/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { en } from "../../i18n/en";
import { OpenSourceProvider } from "../../lib/opensource";
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
  occurred_precision: "instant",
  because: [{ kind: "due_today" }, { kind: "unassigned" }],
  actions: ["complete", "snooze"],
};

function openTask(
  task: Record<string, unknown>,
  item?: WorklistItem,
  openSource: (id: string) => void = () => {},
) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "update"] }, { seat: "full" }),
    "GET /activities/task-1": () => jsonResponse(task),
  });
  return render(
    <StoryProviders>
      <OpenSourceProvider openSource={openSource}>
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
      </OpenSourceProvider>
    </StoryProviders>,
  );
}

test("a reminder an automation wrote says which one and what it asks", async () => {
  openTask(QUIET_REMINDER, ROW);

  expect(
    await screen.findByText(en["tasks.origin.no_activity_reminder"]),
  ).toBeTruthy();
  expect(screen.getByText(en["worklist.why.title"])).toBeTruthy();
  const reasons = screen.getByRole("list", {
    name: en["worklist.why.reasons"],
  });
  expect(
    within(reasons)
      .getAllByRole("listitem")
      .map((reason) => reason.textContent),
  ).toEqual([
    en["worklist.because.due_today"],
    en["worklist.because.unassigned"],
  ]);
});

test("the details leave out how the row ranks against the queue", async () => {
  openTask(QUIET_REMINDER, {
    ...ROW,
    because: [{ kind: "due_today" }, { kind: "material" }, { kind: "routine" }],
  });

  const reasons = await screen.findByRole("list", {
    name: en["worklist.why.reasons"],
  });
  expect(
    within(reasons)
      .getAllByRole("listitem")
      .map((reason) => reason.textContent),
  ).toEqual([en["worklist.because.due_today"]]);
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

test("a row with no reasons still says what it asks", async () => {
  openTask(QUIET_REMINDER, { ...ROW, because: [], consequence: "none" });

  expect(await screen.findByText(en["worklist.ask.task"])).toBeTruthy();
  expect(
    screen.queryByRole("list", { name: en["worklist.why.reasons"] }),
  ).toBeNull();
});

const REQUEST_REMINDER = {
  ...QUIET_REMINDER,
  subject: "Please send the offer",
  source_system: "email_request",
};

test("a reminder Margince cannot check says so and opens our last reply", async () => {
  const opened: string[] = [];
  openTask(
    REQUEST_REMINDER,
    {
      ...ROW,
      request_check: {
        state: "text_not_checked",
        our_last_reply: {
          activity_id: "reply-1",
          occurred_at: "2026-09-05T10:00:00Z",
          occurred_precision: "day",
        },
      },
    },
    (id) => opened.push(id),
  );

  expect(
    await screen.findByText(en["worklist.ask.task_unchecked"]),
  ).toBeTruthy();
  expect(screen.queryByText(en["worklist.ask.task"])).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: /^Latest reply sent: / }));
  expect(opened).toEqual(["reply-1"]);
});

test("a reminder Margince cannot check names no reply the reader cannot open", async () => {
  openTask(REQUEST_REMINDER, {
    ...ROW,
    request_check: { state: "text_not_checked" },
  });

  expect(
    await screen.findByText(en["worklist.ask.task_unchecked"]),
  ).toBeTruthy();
  expect(screen.queryByText(/^Latest reply sent: /)).toBeNull();
});
