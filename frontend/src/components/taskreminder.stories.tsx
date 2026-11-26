// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { type ReminderTask, TaskRemindMenu } from "./taskreminder";
import { useTaskUpdate } from "./taskwrite";

const READER = "00000000-0000-4000-8000-000000000001";
const TASK = "01a05500-0000-7000-8000-000000000d01";

const meta: Meta<typeof TaskRemindMenu> = {
  title: "Patterns/Task reminder",
  component: TaskRemindMenu,
};
export default meta;
type Story = StoryObj<typeof meta>;

function Menu({ task }: Readonly<{ task: ReminderTask }>) {
  const update = useTaskUpdate([["tasks"]]);
  return <TaskRemindMenu task={task} update={update} />;
}

function draw(task: Partial<ReminderTask>) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "update"] }),
    [`PATCH /activities/${TASK}`]: () => jsonResponse({ id: TASK, version: 2 }),
  });
  return (
    <StoryProviders>
      <Menu task={{ id: TASK, version: 1, assignee_id: READER, ...task }} />
    </StoryProviders>
  );
}

// A task due at a time of day: all three quick choices, and no Remove yet.
export const DueAtATime: Story = {
  render: () =>
    draw({
      due_at: new Date(Date.now() + 6 * 60 * 60 * 1000).toISOString(),
      due_has_time: true,
    }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Remind me" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await expect(
      page.getByRole("button", { name: "1 hour before it is due" }),
    ).toBeVisible();
    await expect(page.queryByRole("button", { name: "Remove reminder" })).toBe(
      null,
    );
  },
};

// A reminder already set: the menu says when, and offers to remove it.
export const AlreadySet: Story = {
  render: () =>
    draw({
      remind_at: new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString(),
    }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Remind me" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    // The chip names the reminder, and the menu says it again over its lines.
    await expect(page.getAllByText(/^Reminder set for /)).toHaveLength(2);
    await expect(
      page.getByRole("button", { name: "Remove reminder" }),
    ).toBeVisible();
  },
};
