// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { meetingRow, readingsDay } from "../../testing/brief.fixtures";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import type { WorklistItem } from "../worklist/worklist.queries";
import { SchedulePanel } from "./brief.schedule";

// External meetings, the rail's list of today's meetings with somebody from
// outside, each saying who it is with. A read in flight and a read that failed
// are facts about the request, so they keep the box and claim nothing.
//
// Read each story in both themes with the toolbar's Theme control. Every colour
// here is a `color-mix()` of a canonical token. So a surface can be correct in
// light and wrong in dark.

/** A row the task lane would send: what this rep owes, with no time on it. */
function taskRow(id: string, title: string): WorklistItem {
  return {
    occurred_precision: "instant",
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
        <div className="brief-rail" style={{ maxWidth: 360 }}>
          {node}
        </div>
      </StoryProviders>
    );
  };
}

/** A meeting whose row names whose page its brief is read on, and who it is with. */
function withBrief(item: WorklistItem): WorklistItem {
  return {
    ...item,
    with_contact: "c-1",
    move: { action: "open_meeting_brief", activity_id: item.id },
    contact: {
      id: "c-1",
      label: "Anna Weber",
      employer: { company_id: "co-1", company_name: "Weber GmbH" },
    },
    room: { participants: [{ contact_id: "c-1", full_name: "Anna Weber" }] },
  };
}

const BOOKED = readingsDay({}, [
  withBrief(meetingRow("m1", false, "2026-08-21T07:30:00Z")),
  withBrief(meetingRow("m2", true, "2026-08-21T13:00:00Z")),
  taskRow("t1", "Call Alice back"),
]);

/** The last of `rows` ends inside the panel, so nothing is cut at its edge. */
async function expectNoRowClipped(canvasElement: HTMLElement) {
  const panel = canvasElement.querySelector(".rail-schedule");
  const rows = canvasElement.querySelectorAll(".rail-schedule-row");
  const last = rows.item(rows.length - 1);
  await expect(panel).not.toBeNull();
  await expect(last).not.toBeNull();
  await expect(last.getBoundingClientRect().bottom).toBeLessThanOrEqual(
    (panel as HTMLElement).getBoundingClientRect().bottom,
  );
  await expect((panel as HTMLElement).scrollHeight).toBe(
    (panel as HTMLElement).clientHeight,
  );
}

const CLEAR = readingsDay({}, []);

// Early on the day of BOOKED, before either meeting.
const NOW = Date.parse("2026-08-21T06:50:00Z");

const meta: Meta = {
  title: "Shell/Home/Schedule",
};
export default meta;
type Story = StoryObj;

// ── Today's schedule ────────────────────────────────────────────────────────

// Two external meetings at the hours they start, each naming the account and
// the contact. The time is `due_at`: `occurred_at` is when something HAPPENED,
// which a meeting still ahead of the reader has no answer for. The next one
// adds how soon it starts and its brief, and otherwise looks like the other.
export const Schedule: Story = {
  render: panel(
    <SchedulePanel
      day={BOOKED}
      state="ready"
      now={NOW}
      onPrepareMeeting={() => undefined}
    />,
  ),
  play: ({ canvasElement }) => expectNoRowClipped(canvasElement),
};

// Mid-morning: the first meeting is under way and the second is the next one,
// with how soon it starts.
export const ScheduleMidDay: Story = {
  render: panel(
    <SchedulePanel
      day={BOOKED}
      state="ready"
      now={Date.parse("2026-08-21T11:40:00Z")}
      onPrepareMeeting={() => undefined}
    />,
  ),
};

// A meeting known only by its date: its row draws no start time.
export const ScheduleDateOnlyMeeting: Story = {
  render: panel(
    <SchedulePanel
      day={readingsDay({}, [
        {
          ...meetingRow("m3", false, "2026-08-21T10:00:00Z"),
          occurred_precision: "day",
        },
      ])}
      state="ready"
    />,
  ),
  play: async ({ canvasElement }) => {
    const title = await within(canvasElement).findByText(
      "Weber GmbH · quarterly review",
    );
    const row = title.closest(".rail-schedule-row");
    await expect(row).not.toBeNull();
    await expect(row?.textContent).not.toMatch(/\d{1,2}:\d{2}/);
  },
};

// Nothing external booked: the panel says so, so the box never leaves the rail.
// A day of meetings among colleagues reads the same, because the server's
// meetings lane sends only meetings with somebody from outside on them. An
// internal call and a colleague's booking through the reader's page never
// arrive as rows.
export const ScheduleClearDay: Story = {
  render: panel(
    <SchedulePanel
      day={CLEAR}
      state="ready"
      now={NOW}
      onPrepareMeeting={() => undefined}
    />,
  ),
};

// More contacts than the row names: the account, the first names as links, an
// outside address, and how many more there are.
export const MoreNamesThanFit: Story = {
  render: panel(
    <SchedulePanel
      day={readingsDay({}, [
        {
          ...withBrief(meetingRow("m1", false, "2026-08-21T07:30:00Z")),
          room: {
            participants: [
              { contact_id: "c-1", full_name: "Anna Weber" },
              { contact_id: "c-2", full_name: "Charlotte Nguyen" },
              { contact_id: "c-3", full_name: "Lena Hartmann" },
            ],
            other_participants: [
              { kind: "colleague", name: "Rita Rep" },
              { kind: "address", name: "procurement@weber.example" },
            ],
            participants_more: 4,
          },
        },
      ])}
      state="ready"
      now={NOW}
    />,
  ),
  play: async ({ canvasElement }) => {
    const who = canvasElement.querySelector(".rail-schedule-attendees");
    await expect(who?.textContent).toContain("Weber GmbH · Anna Weber");
    // A colleague is on every meeting here, so the row leaves them out.
    await expect(who?.textContent).not.toContain("Rita Rep");
  },
};

// A full day: every row keeps the same look, and the last one, badge and all,
// ends inside the panel.
export const LongList: Story = {
  render: panel(
    <SchedulePanel
      day={readingsDay(
        {},
        [
          "2026-08-21T07:30:00Z",
          "2026-08-21T09:00:00Z",
          "2026-08-21T10:30:00Z",
          "2026-08-21T12:00:00Z",
          "2026-08-21T14:00:00Z",
          "2026-08-21T16:30:00Z",
        ].map((at, index) =>
          withBrief(meetingRow(`m${index}`, index % 2 === 0, at)),
        ),
      )}
      state="ready"
      now={NOW}
      onPrepareMeeting={() => undefined}
    />,
  ),
  play: ({ canvasElement }) => expectNoRowClipped(canvasElement),
};

// A meeting booked further out, which the CRM holds whatever the calendar
// connection says.
const NEXT_MEETING = {
  activity_id: "a-next",
  starts_at: "2026-08-25T08:00:00Z",
  occurred_precision: "instant" as const,
  subject: "Weber GmbH · kickoff",
  linked_deal_id: "d-1",
  participants: [{ contact_id: "c-1", full_name: "Anna Weber" }],
};

// Nothing left today, and the next booked meeting named on the day's line with
// a date where today's rows carry a time.
export const QuietDayNextMeeting: Story = {
  render: panel(
    <SchedulePanel
      day={{ ...CLEAR, calendar: "connected", next_meeting: NEXT_MEETING }}
      state="ready"
    />,
  ),
};

// A title longer than the rail, and a crowded invite. The title wraps beside its
// stop, and the attendees wrap as one sentence clear of the date gutter.
const LONG_TITLE =
  "Gradion – | 2nd demo for Document Management with the procurement team";

export const LongTitleAndAttendees: Story = {
  render: panel(
    <>
      <SchedulePanel
        day={readingsDay({}, [
          {
            ...withBrief(meetingRow("m1", false, "2026-08-21T07:30:00Z")),
            title: LONG_TITLE,
          },
        ])}
        state="ready"
        now={NOW}
        onPrepareMeeting={() => undefined}
        onOpenMeeting={() => undefined}
      />
      <SchedulePanel
        day={{
          ...CLEAR,
          calendar: "connected",
          next_meeting: {
            ...NEXT_MEETING,
            subject: LONG_TITLE,
            participants: [
              { contact_id: "c-1", full_name: "Anna Weber" },
              { contact_id: "c-2", full_name: "Charlotte Nguyen" },
              { contact_id: "c-3", full_name: "Lena Hartmann" },
              { contact_id: "c-4", full_name: "Marcus Lindqvist" },
            ],
            other_participants: [
              { kind: "address", name: "procurement@weber.example" },
            ],
            participants_more: 3,
          },
        }}
        state="ready"
        onOpenMeeting={() => undefined}
      />
    </>,
  ),
};

// No calendar connected: the zero measured nothing, so the panel stays, leads
// to where one connects, and still names the meeting booked by hand.
export const CalendarNotConnected: Story = {
  render: panel(
    <SchedulePanel
      day={{ ...CLEAR, calendar: "not_connected", next_meeting: NEXT_MEETING }}
      state="ready"
    />,
  ),
};

// A connection that stopped syncing: the same box, the way to reconnect, and
// the next meeting the CRM already holds.
export const CalendarNotSyncing: Story = {
  render: panel(
    <SchedulePanel
      day={{ ...CLEAR, calendar: "unreadable", next_meeting: NEXT_MEETING }}
      state="ready"
    />,
  ),
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
