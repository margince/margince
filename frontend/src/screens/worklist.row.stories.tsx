// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import { jsonResponse, StoryProviders } from "./story-utils";
import { WorklistRow } from "./worklist.row";

// One row of the queue, standing on its own — the unit worklist.stories.tsx
// exercises through the whole screen. Covered here directly because fe-uat
// (frontend/AGENTS.md) credits a story only for the file it imports, and
// nothing else in the tree imports WorklistRow.

type WorklistItem = components["schemas"]["WorklistItem"];

function noticeItem(): WorklistItem {
  return {
    id: "n1",
    source: "notice",
    category: "tasks",
    level: 4,
    consequence: "task_slips",
    title: "A deal you own changed stage",
    detail: "Acme Renewal moved to a new pipeline stage.",
    because: [],
    actions: ["acknowledge"],
  };
}

function stubRow(readPending: boolean) {
  globalThis.fetch = (async (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.includes("/notices/n1/read")) {
      return readPending
        ? new Promise(() => {})
        : new Response(null, { status: 204 });
    }
    return jsonResponse({ data: [] });
  }) as typeof fetch;
}

const meta: Meta<typeof WorklistRow> = {
  title: "Records/Worklist/Row",
  component: WorklistRow,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Panel title="What to do next">
          <ol className="worklist-list">
            <li>
              <Story />
            </li>
          </ol>
        </Panel>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof WorklistRow>;

const baseArgs = {
  position: 1,
  owner: "",
  selected: false,
  onSelect: () => undefined,
  onReview: () => undefined,
};

// A notice, and the one verb the row draws inline rather than as a link — see
// NoticeAcknowledge in worklist.row.tsx for why.
export const ANoticeToSettle: Story = {
  args: { ...baseArgs, item: noticeItem() },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// Pending: "Got it" is clicked and the read call never resolves.
export const ANoticeSettling: Story = {
  args: { ...baseArgs, item: noticeItem() },
  render: (args) => {
    stubRow(true);
    return <WorklistRow {...args} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Got it" }),
    );
  },
};

// A meeting, drawn with the clock a rep is racing.
//
// The state this row could not reach before: it said "starting shortly" whether
// the meeting began in four minutes or in fifty, so the one row that has to be
// opened BEFORE a wall-clock time was the row that would not say the time.
//
// Dated deliberately far enough out that the story is stable whenever it is
// looked at — a fixture pinned to "today" renders differently every day and the
// snapshot argues with itself.
export const AMeetingWithItsStartTime: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "m1",
      source: "meeting",
      category: "meetings",
      level: 1,
      consequence: "meeting_unprepared",
      title: "Quarterly review with Turbinenbau",
      kind: "unprepared",
      due_at: "2026-09-02T13:00:00Z",
      because: [{ kind: "meeting_soon" }, { kind: "meeting_unprepared" }],
      actions: [],
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// A task nobody has taken, with the date it is due and how it is put down.
//
// Three states the row gained at once: the due moment drawn, "nobody owns it"
// said out loud, and the spans behind "For how long" — opened here, because a
// popover closed is a story that shows nothing about what it holds.
export const ATaskNobodyOwnsBeingPutDown: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "01a05500-0000-7000-8000-0000000000a1",
      source: "task",
      category: "tasks",
      level: 3,
      consequence: "task_slips",
      title: "Send the retrofit quote",
      due_at: "2026-09-02T15:00:00Z",
      because: [{ kind: "due_today" }, { kind: "unassigned" }],
      actions: [],
      dispositions: ["snooze", "not_mine"],
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "For how long" }),
    );
  },
};

// The same row on a phone.
//
// The state the old mobile test could not see: above this breakpoint the row is
// rank, text and verbs on one line, and the verbs never yield width — so at
// 390px the title column was squeezed to a few characters while three buttons
// held their full size beside it. Here the text takes the line and the verbs
// drop below it at a real target height.
//
// Worth a story of its own rather than a note on the desktop one: this is a
// different layout, and the two are checked by looking at both.
export const ATaskOnAPhone: Story = {
  ...ATaskNobodyOwnsBeingPutDown,
  globals: { viewport: { value: "phone" } },
  play: undefined,
};
