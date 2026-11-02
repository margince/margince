// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import {
  TaskCompleteCheck,
  TaskQuickActions,
  useTaskUpdate,
} from "../../components/taskactions";
import { company360, emptySection } from "../../testing/company.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { CompanyTaskTable } from "./companytaskstable";

const meta: Meta<typeof CompanyTaskTable> = {
  title: "Records/Company 360/Task table",
  component: CompanyTaskTable,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof CompanyTaskTable>;

// One late task, one due within the week and one with no date. They are read
// against the 360's own day, so the badges never move with the clock.
const tasks = {
  ...company360,
  as_of: "2026-08-13T09:00:00Z",
  next_steps: {
    ...emptySection,
    data: [
      {
        activity_id: "a-1",
        subject: "Send Dana the renewal paperwork",
        due_at: "2026-07-25T12:00:00Z",
        overdue: true,
      },
      {
        activity_id: "a-2",
        subject: "Prepare the walkthrough brief",
        due_at: "2026-08-19T12:00:00Z",
        overdue: false,
      },
      {
        activity_id: "a-3",
        subject: "Find a second contact at Brandt",
        overdue: false,
      },
    ],
  },
};

export const LateDueAndUndated: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 880 }}>
        <CompanyTaskTable view={tasks} />
      </div>
    </StoryProviders>
  ),
};

export const LateDueAndUndatedDark: Story = {
  ...LateDueAndUndated,
  globals: { theme: "dark" },
};

export const NoOpenTasks: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 880 }}>
        <CompanyTaskTable view={company360} />
      </div>
    </StoryProviders>
  ),
};

// The Tasks tab's own row: the tick before the name, a snooze after the badge.
function TasksTabTable({ view }: Readonly<{ view: typeof tasks }>) {
  const update = useTaskUpdate([]);
  return (
    <CompanyTaskTable
      view={view}
      onOpenTask={() => undefined}
      renderTick={(step) => (
        <TaskCompleteCheck
          activityId={step.activity_id}
          version={step.version}
          update={update}
        />
      )}
      renderAction={(step) => (
        <TaskQuickActions
          activityId={step.activity_id}
          version={step.version}
          dueAt={step.due_at}
          update={update}
          showComplete={false}
        />
      )}
    />
  );
}

// Every word of a text node that the layout broke across two lines.
function brokenWords(root: Element): string[] {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const broken: string[] = [];
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    for (const word of (node.textContent ?? "").matchAll(/\S+/g)) {
      const range = document.createRange();
      range.setStart(node, word.index);
      range.setEnd(node, word.index + word[0].length);
      if (range.getClientRects().length > 1) broken.push(word[0]);
    }
  }
  return broken;
}

// At a phone's width the badge and the snooze give way to the task's name, so
// the name reads as whole words instead of one letter per line.
export const TasksTabOnPhone: Story = {
  render: () => (
    <StoryProviders>
      <TasksTabTable
        view={{
          ...tasks,
          next_steps: {
            ...tasks.next_steps,
            data: tasks.next_steps.data.map((step) => ({
              ...step,
              subject: `Call Alice back about the retrofit: ${step.subject}`,
            })),
          },
        }}
      />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const titles = canvasElement.querySelectorAll('[data-fold="title"]');
    await expect(titles.length).toBe(3);
    for (const title of titles) {
      await expect(brokenWords(title)).toEqual([]);
    }
  },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
