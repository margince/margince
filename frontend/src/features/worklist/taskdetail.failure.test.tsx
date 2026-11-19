/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, test } from "vitest";
import { TaskDetailDialog, useTaskUpdate } from "../../components/taskactions";
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

function TwoTasksOnePage() {
  const update = useTaskUpdate(taskWriteKeys("contact", "c-1"));
  const [open, setOpen] = useState("task-1");
  return (
    <>
      <button type="button" onClick={() => setOpen("task-2")}>
        switch
      </button>
      <TaskDetailDialog
        key={open}
        activityId={open}
        readOnly={false}
        onClose={() => {}}
        update={update}
        openerFor={() => undefined}
      />
    </>
  );
}

test("while a write on one task is in flight, every write control of another task is held", async () => {
  const user = userEvent.setup();
  let release: (response: Response) => void = () => {};
  const task = (id: string) =>
    jsonResponse({
      id,
      kind: "task",
      writable: true,
      subject: `Task ${id}`,
      due_at: "2099-09-11T21:59:59Z",
      occurred_at: "2026-09-08T09:00:00Z",
      is_done: false,
      captured_by: "human:u-1",
      version: 3,
    });
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "update"] }, { seat: "full" }),
    "GET /activities/task-1": () => task("task-1"),
    "GET /activities/task-2": () => task("task-2"),
    "PATCH /activities/task-1": () =>
      new Promise<Response>((resolve) => {
        release = resolve;
      }),
  });
  render(
    <StoryProviders>
      <TwoTasksOnePage />
    </StoryProviders>,
  );

  await user.click(await screen.findByRole("button", { name: "Done" }));
  await user.click(screen.getByRole("button", { name: "switch" }));
  await screen.findByText("Task task-2");

  for (const name of ["Done", "Snooze 1 day"]) {
    expect(screen.getByRole("button", { name }).hasAttribute("disabled")).toBe(
      true,
    );
  }
  expect(
    screen.getByRole("textbox", { name: "Move to" }).hasAttribute("disabled"),
  ).toBe(true);
  release(jsonResponse({ id: "task-1", version: 4 }));
});
