// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import type { components } from "../../api/schema";
import {
  TaskCompleteCheck,
  TaskQuickActions,
  useTaskUpdate,
} from "../../components/taskactions";
import { Panel } from "../../design-system/panel";
import { company360, emptySection } from "../../testing/company.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { CompanyTaskRows } from "./companytaskrows";

const meta: Meta<typeof CompanyTaskRows> = {
  title: "Records/Company 360/Task rows",
  component: CompanyTaskRows,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof CompanyTaskRows>;

type Company360 = components["schemas"]["Company360"];

type NextStep = NonNullable<Company360["next_steps"]>["data"][number];

// One late task, one due within the week and one with no date. They are read
// against the 360's own day, so the badges never move with the clock.
const steps: NextStep[] = [
  {
    activity_id: "a-1",
    subject: "Send Dana the renewal paperwork",
    body: "Dana asked for the signed renewal before their board meets on 30 July.",
    due_at: "2026-07-25T12:00:00Z",
    overdue: true,
    task_type: "reach_out",
    linked_contact_id: "p-1",
    linked_deal_id: "d-1",
  },
  {
    activity_id: "a-2",
    subject: "Prepare the walkthrough brief",
    due_at: "2026-08-19T12:00:00Z",
    overdue: false,
    task_type: "meet",
  },
  {
    activity_id: "a-3",
    subject: "Find a second contact at Brandt",
    overdue: false,
    task_type: "do",
  },
];

const tasks: Company360 = {
  ...company360,
  as_of: "2026-08-13T09:00:00Z",
  next_steps: { ...emptySection, data: steps },
};

export const LateDueAndUndated: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 880 }}>
        <Panel title="Tasks">
          <CompanyTaskRows view={tasks} />
        </Panel>
      </div>
    </StoryProviders>
  ),
};

export const LateDueAndUndatedDark: Story = {
  ...LateDueAndUndated,
  globals: { theme: "dark" },
};

// A promise task we may have kept in a later mail asks the Worklist's
// question under its subject, where the list offers the tick.
export const PromiseKeptByMail: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 880 }}>
        <Panel title="Tasks">
          <CompanyTaskRows
            view={{
              ...tasks,
              next_steps: {
                ...emptySection,
                data: [
                  {
                    activity_id: "a-1",
                    subject: "Send Dana the renewal paperwork",
                    due_at: "2026-07-25T12:00:00Z",
                    overdue: true,
                    may_be_done: {
                      contact_id: "p-1",
                      email_activity_id: "e-1",
                      wrote_at: "2026-08-12T10:00:00Z",
                      wrote_precision: "instant",
                      claim_key: "moment:may_be_done:a-1",
                      evidence_fingerprint: "fp-1",
                    },
                  },
                ],
              },
            }}
            renderTick={() => null}
          />
        </Panel>
      </div>
    </StoryProviders>
  ),
};

export const NoOpenTasks: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 880 }}>
        <Panel title="Tasks">
          <CompanyTaskRows view={company360} />
        </Panel>
      </div>
    </StoryProviders>
  ),
};

// The Tasks tab's own row: the tick before the name, a snooze opposite.
function TasksTabRows({ view }: Readonly<{ view: Company360 }>) {
  const update = useTaskUpdate([]);
  return (
    <Panel title="Tasks">
      <CompanyTaskRows
        view={view}
        onOpenTask={() => undefined}
        renderTick={(step) => (
          <TaskCompleteCheck
            activityId={step.activity_id}
            subject={step.subject}
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
    </Panel>
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

// At a phone's width the snooze moves under the task's name. The name reads as
// whole words, never one letter per line.
export const TasksTabOnPhone: Story = {
  render: () => (
    <StoryProviders>
      <TasksTabRows
        view={{
          ...tasks,
          next_steps: {
            ...emptySection,
            data: steps.map((step) => ({
              ...step,
              subject: `Call Alice back about the retrofit: ${step.subject}`,
            })),
          },
        }}
      />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const titles = canvasElement.querySelectorAll(".co-taskrow-title");
    await expect(titles.length).toBe(3);
    for (const title of titles) {
      await expect(brokenWords(title)).toEqual([]);
    }
  },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
