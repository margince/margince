// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meetingRow, readingsDay } from "./brief.fixtures";
import { PromisesPanel, SchedulePanel } from "./brief.schedule";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";
import type { WorklistItem } from "./worklist.queries";

// The two rail panels the morning is read alongside: what the day is booked
// with, and what this rep owes.
//
// THE EMPTY FRAMES ARE THE POINT OF THIS FILE. A panel with nothing in it draws
// nothing at all — no band, no hairline, no grey sentence — and the rail's own
// quiet panel carries one line per silent source instead (see
// `Shell/Brief rail`). Every OTHER state keeps the box: a read in flight and a
// read that failed are facts about the request, and collapsing them would tell
// a rep their morning was clear on the strength of an answer nobody received.
//
// So a story below that renders an empty frame is passing. Read each one in
// BOTH themes with the toolbar's Theme control — every colour here is a
// `color-mix()` of a canonical token, so a surface can be correct in light and
// wrong in dark.

/** A row the task lane would send: what this rep owes, with no time on it. */
function taskRow(id: string, title: string): WorklistItem {
  return {
    id,
    source: "task",
    level: 2,
    category: "tasks",
    title,
    because: [],
    consequence: "task_slips",
    actions: ["complete", "open"],
  };
}

/** One panel, in a rail-width column, with nothing reachable but the session. */
function panel(node: React.ReactNode) {
  return () => {
    installFetchStub({ "GET /me": meRoute({}) });
    return (
      <StoryProviders>
        <div className="brief-rail" style={{ maxWidth: 320 }}>
          {node}
        </div>
      </StoryProviders>
    );
  };
}

const BOOKED = readingsDay({}, [
  meetingRow("m1", false, "2026-08-21T07:30:00Z"),
  meetingRow("m2", true, "2026-08-21T13:00:00Z"),
  taskRow("t1", "Call Alice back"),
]);

const CLEAR = readingsDay({}, []);

const meta: Meta = {
  title: "Shell/Brief schedule",
};
export default meta;
type Story = StoryObj;

// ── Today's schedule ────────────────────────────────────────────────────────

// A meeting the queue carries, at the hour it starts. The time is `due_at`:
// `occurred_at` is when something HAPPENED, which a meeting still ahead of the
// reader has no answer for. The badge rides WITH the title, so a meeting whose
// name wraps keeps its warning attached to it.
export const Schedule: Story = {
  render: panel(<SchedulePanel day={BOOKED} state="ready" />),
};

// Nothing booked: no panel. The line that says so is on the rail's quiet panel.
export const ScheduleCollapsed: Story = {
  render: panel(<SchedulePanel day={CLEAR} state="ready" />),
};

// The worklist has not answered yet, so the panel stands and says what it is
// waiting for. A collapse here would read as a clear day.
export const ScheduleLoading: Story = {
  render: panel(<SchedulePanel day={undefined} state="loading" />),
};

// The worklist read failed. Same box, and the failure said out loud — a rep who
// saw nothing would go into the morning believing there was nothing to see.
export const ScheduleRefused: Story = {
  render: panel(<SchedulePanel day={undefined} state="failed" />),
};

// ── Tasks due ───────────────────────────────────────────────────────────────

// What this rep owes today, and nothing else. The panel read "Promises &
// tasks" over a standing line explaining that a promise made in conversation
// reaches nothing; the title is the fix and the disclaimer was the workaround.
export const Tasks: Story = {
  render: panel(<PromisesPanel day={BOOKED} state="ready" />),
};

// Nothing due: no panel.
export const TasksCollapsed: Story = {
  render: panel(<PromisesPanel day={CLEAR} state="ready" />),
};

export const TasksLoading: Story = {
  render: panel(<PromisesPanel day={undefined} state="loading" />),
};

export const TasksRefused: Story = {
  render: panel(<PromisesPanel day={undefined} state="failed" />),
};
