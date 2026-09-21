// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "./story-utils";
import type { WorklistItem } from "./worklist.queries";
import { QueueBand } from "./worklist.queuebands";

// One band of the queue, and the runs of work due on different days inside it.
//
// The band alone rather than the screen, which fetches: a story mounting the
// whole worklist draws a loading skeleton and says nothing about the headings.

const meta: Meta<typeof QueueBand> = {
  title: "Records/Worklist/Queue band",
  component: QueueBand,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof QueueBand>;

function row(over: Partial<WorklistItem>): WorklistItem {
  return {
    id: "row",
    source: "task",
    category: "tasks",
    level: 4,
    consequence: "task_slips",
    because: [],
    actions: [],
    ...over,
  } as WorklistItem;
}

/** The rows, drawn plainly: this story is about the headings above them. */
function Rows({ items }: Readonly<{ items: readonly WorklistItem[] }>) {
  return (
    <ol className="worklist-list">
      {items.map((item) => (
        <li key={item.id} className="t-body">
          {item.title}
        </li>
      ))}
    </ol>
  );
}

// The case the sub-headings exist for: today's work and next week's in one
// band, told apart without reading a single date.
export const WorkDueLater: Story = {
  args: {
    canReportEmpty: true,
    rows: Rows,
    rowProps: {},
    section: {
      band: "now",
      items: [
        row({
          id: "a",
          due_group: "overdue",
          title: "Send the retrofit quote",
        }),
        row({ id: "b", due_group: "today", title: "Answer the Weber thread" }),
        row({ id: "c", due_group: "tomorrow", title: "Call the architect" }),
        row({
          id: "d",
          due_group: "this_week",
          title: "Prepare the phase-two figures",
        }),
      ],
    },
  },
};

// Everything is owed today, so the band's own heading is the only one. A
// sub-heading here would divide a list that has one part.
export const AllDueToday: Story = {
  args: {
    canReportEmpty: true,
    rows: Rows,
    rowProps: {},
    section: {
      band: "now",
      items: [
        row({
          id: "a",
          due_group: "overdue",
          title: "Send the retrofit quote",
        }),
        row({ id: "b", due_group: "today", title: "Answer the Weber thread" }),
      ],
    },
  },
};

// A band the day declares and no row reaches. It says so in words rather than
// drawing a heading over a gap.
export const NothingInThisBand: Story = {
  args: {
    canReportEmpty: true,
    rows: Rows,
    rowProps: {},
    section: { band: "build_pipeline", items: [] },
  },
};
