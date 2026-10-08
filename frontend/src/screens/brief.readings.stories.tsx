// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { leadRow, readingsDay, wholeLeads } from "./brief.fixtures";
import { BriefReadingsStrip } from "./brief.readings";
import { StoryProviders } from "./story-utils";

const meta: Meta<typeof BriefReadingsStrip> = {
  title: "Shell/Home/Prospecting readings",
  component: BriefReadingsStrip,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof BriefReadingsStrip>;

export const PlannedOutreach: Story = {
  args: {
    day: readingsDay(
      { prospecting: 1 },
      [leadRow("lead-1", "2026-08-31T15:00:00Z")],
      [wholeLeads(1)],
    ),
  },
};

export const TaskSourceUnavailable: Story = {
  args: {
    day: {
      ...readingsDay({ prospecting: 0 }, [], []),
      sources_unavailable: [
        { source: "task", category: "tasks", reason: "failed" },
      ],
    },
  },
};

// The meetings slot over a calendar that could not count: a word where the
// zero would be, and a door to where the calendar connects.
export const MeetingsCalendarNotConnected: Story = {
  args: { day: { ...readingsDay({}, [], []), calendar: "not_connected" } },
};

export const MeetingsCalendarNotSyncing: Story = {
  args: { day: { ...readingsDay({}, [], []), calendar: "unreadable" } },
};

// A measured quiet day: the zero stands, and its line says when the next
// conversation is.
export const MeetingsQuietDayNextMeeting: Story = {
  args: {
    day: {
      ...readingsDay({}, [], []),
      calendar: "connected",
      next_meeting: {
        activity_id: "a-next",
        starts_at: "2026-09-04T09:30:00Z",
        subject: "Weber GmbH · kickoff",
      },
    },
  },
};

export const MeetingsCalendarNotConnectedDark: Story = {
  ...MeetingsCalendarNotConnected,
  globals: { theme: "dark" },
};

export const MeetingsQuietDayNextMeetingDark: Story = {
  ...MeetingsQuietDayNextMeeting,
  globals: { theme: "dark" },
};

export const PlannedOutreachDark: Story = {
  ...PlannedOutreach,
  globals: { theme: "dark" },
};

export const TaskSourceUnavailableDark: Story = {
  ...TaskSourceUnavailable,
  globals: { theme: "dark" },
};
