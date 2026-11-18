/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test } from "vitest";
import {
  TaskDetailDialog,
  TaskQuickActions,
  useTaskUpdate,
} from "../../components/taskactions";
import { en } from "../../i18n/en";
import { taskWriteKeys } from "../../lib/activitykeys";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";

afterEach(cleanup);

function Dialog() {
  const update = useTaskUpdate(taskWriteKeys("contact", "c-1"));
  return (
    <TaskDetailDialog
      activityId="task-1"
      readOnly={false}
      onClose={() => {}}
      update={update}
      openerFor={() => undefined}
    />
  );
}

function stubWith(refusal: { status: number; code: string }) {
  let reads = 0;
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "update"] }, { seat: "full" }),
    "GET /activities/task-1": () => {
      reads += 1;
      return jsonResponse({
        id: "task-1",
        kind: "task",
        writable: true,
        subject: "Send the comparison sheet",
        due_at: "2099-09-11T21:59:59Z",
        occurred_at: "2026-09-08T09:00:00Z",
        is_done: false,
        captured_by: "human:u-1",
        version: 3,
      });
    },
    "PATCH /activities/task-1": () =>
      jsonResponse(
        {
          type: "about:blank",
          title: "Refused",
          status: refusal.status,
          code: refusal.code,
        },
        refusal.status,
      ),
  });
  return () => reads;
}

function open() {
  render(
    <StoryProviders>
      <Dialog />
    </StoryProviders>,
  );
}

test("Done on a task that changed elsewhere says so and offers a reload", async () => {
  const user = userEvent.setup();
  const reads = stubWith({ status: 409, code: "version_skew" });
  open();

  await user.click(await screen.findByRole("button", { name: "Done" }));

  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain(en["common.versionSkew"]);
  const before = reads();
  await user.click(screen.getByRole("button", { name: "Reload" }));
  await screen.findByRole("button", { name: "Snooze 1 day" });
  expect(reads()).toBeGreaterThan(before);
  expect(screen.queryByRole("alert")).toBeNull();
});

test("Snooze 1 day on a task that changed elsewhere says so", async () => {
  const user = userEvent.setup();
  stubWith({ status: 409, code: "version_skew" });
  open();

  await user.click(await screen.findByRole("button", { name: "Snooze 1 day" }));

  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain(en["common.versionSkew"]);
});

test("Done on a task that is gone says it was refused instead of staying silent", async () => {
  const user = userEvent.setup();
  stubWith({ status: 404, code: "not_found" });
  open();

  await user.click(await screen.findByRole("button", { name: "Done" }));

  expect((await screen.findByRole("alert")).textContent).not.toBe("");
  expect(screen.queryByRole("button", { name: "Reload" })).toBeNull();
});

test("while one task write is in flight, the verbs of every task on the page are held", () => {
  const update = {
    mutate: () => {},
    isPending: true,
    variables: { id: "task-other" },
  } as unknown as Parameters<typeof TaskQuickActions>[0]["update"];
  render(
    <StoryProviders>
      <TaskQuickActions
        activityId="task-1"
        version={3}
        dueAt="2099-09-11T21:59:59Z"
        update={update}
      />
    </StoryProviders>,
  );

  expect(
    screen.getByRole("button", { name: "Done" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(
    screen
      .getByRole("button", { name: "Snooze 1 day" })
      .hasAttribute("disabled"),
  ).toBe(true);
});
