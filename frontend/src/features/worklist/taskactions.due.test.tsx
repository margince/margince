/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { en } from "../../i18n/en";
import { ViewerZoneProvider } from "../../lib/viewerzone";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { TaskDetailModal } from "./taskdetailmodal";

afterEach(cleanup);

// When a task's details say it was logged and is due. The reader's zone is
// named, so the day and hour do not move with the machine the suite runs on.

function openTask() {
  return render(
    <StoryProviders>
      <ViewerZoneProvider zone="Europe/Berlin">
        <TaskDetailModal
          activityId="task-1"
          readOnly={false}
          onClose={() => {}}
          update={
            {
              mutate: () => {},
              isPending: false,
            } as unknown as Parameters<typeof TaskDetailModal>[0]["update"]
          }
        />
      </ViewerZoneProvider>
    </StoryProviders>,
  );
}

test("a task logged with its date alone says when without a time", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: "Send the signed order",
        occurred_at: "2026-09-08T10:00:00Z",
        occurred_precision: "day",
        is_done: false,
        captured_by: "human:u-1",
      }),
  });
  openTask();

  const logged = await screen.findByText(new RegExp(`^${en["tasks.logged"]}`));
  expect(logged.textContent).toMatch(/08\/09\/2026/);
  expect(logged.textContent).not.toMatch(/\d{1,2}:\d{2}/);
});

// The record zone here is the fallback, Berlin. The flag decides whether the
// caption carries a time; the clock reading does not.
function stubDue(dueAt: string, dueHasTime: boolean) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: "Call back the buyer",
        due_at: dueAt,
        due_has_time: dueHasTime,
        occurred_at: "2026-09-08T10:00:00Z",
        is_done: false,
        captured_by: "human:u-1",
      }),
  });
}

test("a task with a callback time says when it is due to the hour", async () => {
  stubDue("2026-09-11T13:00:00Z", true);
  openTask();

  const due = await screen.findByText(/^Due /);
  expect(due.textContent).toMatch(/11\/09\/2026/);
  expect(due.textContent).toMatch(/15:00/);
});

test("a task due by its day says the date alone, whatever its stored hour", async () => {
  stubDue("2026-09-11T13:00:00Z", false);
  openTask();

  const due = await screen.findByText(/^Due /);
  expect(due.textContent).toMatch(/11\/09\/2026/);
  expect(due.textContent).not.toMatch(/\d{1,2}:\d{2}/);
});

test("a task due by the end of its day says the date alone", async () => {
  stubDue("2026-09-11T21:59:59Z", false);
  openTask();

  const due = await screen.findByText(/^Due /);
  expect(due.textContent).toMatch(/11\/09\/2026/);
  expect(due.textContent).not.toMatch(/\d{1,2}:\d{2}/);
});
