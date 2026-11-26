// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { TaskDetailMeta } from "./taskdetailmeta";
import { useTaskUpdate } from "./taskwrite";

// Check: one line of chips. Where the reader may act, the due chip opens
// Snooze and Move to, and Remind me closes the line.

const meta: Meta = {
  title: "Patterns/Task detail meta",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

const READER = "00000000-0000-4000-8000-000000000001";

const TASK: components["schemas"]["Activity"] = {
  occurred_precision: "instant",
  offers_email: false,
  id: "task-1",
  kind: "task",
  version: 2,
  subject: "Send Alice the renewal terms",
  occurred_at: "2026-09-08T09:00:00Z",
  due_at: "2027-09-01T13:00:00Z",
  due_has_time: true,
  is_done: false,
  task_type: "reach_out",
  assignee_id: READER,
  links: [{ entity_type: "company", entity_id: "o-1" }],
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-09-08T09:00:00Z",
  updated_at: "2026-09-08T09:00:00Z",
};

function ChipLine({
  task,
  canAct,
}: Readonly<{ task: components["schemas"]["Activity"]; canAct: boolean }>) {
  const update = useTaskUpdate([["tasks"]]);
  return <TaskDetailMeta task={task} update={update} canAct={canAct} />;
}

function draw(task: components["schemas"]["Activity"], canAct: boolean) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "update"] }),
    "GET /companies/o-1": () =>
      jsonResponse({ id: "o-1", display_name: "Brandt Automotive" }),
    "PATCH /activities/task-1": () =>
      jsonResponse({ id: "task-1", version: 3 }),
  });
  return (
    <StoryProviders>
      <ChipLine task={task} canAct={canAct} />
    </StoryProviders>
  );
}

export const Writable: Story = {
  render: () => draw(TASK, true),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", { name: /^Due / }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await expect(
      page.getByRole("button", { name: en["tasks.snooze"] }),
    ).toBeVisible();
  },
};

// A done task reads its facts only: the due date is a plain chip.
export const Done: Story = {
  render: () => draw({ ...TASK, is_done: true }, false),
};
