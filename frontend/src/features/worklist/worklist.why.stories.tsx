// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { en } from "../../i18n/en";
import { StoryProviders, stubWithSession } from "../../testing/story-utils";
import type { WorklistItem } from "./worklist.queries";
import { WorklistWhy } from "./worklist.why";

// A row's Details: what it asks and every fact behind it.
const meta: Meta<typeof WorklistWhy> = {
  title: "Records/Worklist/Why",
  component: WorklistWhy,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => {
      stubWithSession({}, { activity: ["read", "update"] });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};
export default meta;
type Story = StoryObj<typeof WorklistWhy>;

// Two facts, and two reasons that only rank the row against the queue.
const TASK: WorklistItem = {
  id: "task-1",
  source: "task",
  category: "tasks",
  level: 2,
  consequence: "task_slips",
  occurred_precision: "instant",
  because: [
    { kind: "due_today" },
    { kind: "material" },
    { kind: "unassigned" },
    { kind: "routine" },
  ],
  actions: [],
};

/** The facts are listed; the rank comparisons are left out. */
export const FactReasons: Story = {
  args: { item: TASK },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText(en["worklist.why.title"])).toBeVisible();
    const reasons = canvas.getByRole("list", {
      name: en["worklist.why.reasons"],
    });
    await expect(
      within(reasons)
        .getAllByRole("listitem")
        .map((reason) => reason.textContent),
    ).toEqual([
      en["worklist.because.due_today"],
      en["worklist.because.unassigned"],
    ]);
  },
};

export const FactReasonsDark: Story = {
  ...FactReasons,
  globals: { theme: "dark" },
};

/** Only rank comparisons: no list, and the ask still says what to do. */
export const OnlyRanked: Story = {
  args: {
    item: { ...TASK, because: [{ kind: "material" }, { kind: "routine" }] },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText(en["worklist.ask.task"])).toBeVisible();
    await expect(
      canvas.queryByRole("list", { name: en["worklist.why.reasons"] }),
    ).toBeNull();
  },
};

/** A request reminder Margince cannot check says why, and names our reply. */
export const RequestNotChecked: Story = {
  args: {
    item: {
      ...TASK,
      request_check: {
        state: "text_not_checked",
        our_last_reply: {
          activity_id: "reply-1",
          occurred_at: "2026-09-05T10:00:00Z",
          occurred_precision: "day",
        },
      },
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(en["worklist.ask.task_unchecked"]),
    ).toBeVisible();
    await expect(canvas.getByText(/^Latest reply sent: /)).toBeVisible();
  },
};
