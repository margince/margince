// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  leadRow,
  meetingRow,
  readingsDay,
  wholeLeads,
  wholeMeetings,
} from "../../testing/brief.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { BriefReadingsStrip } from "./brief.readings";
import "./brief.css";

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
        occurred_precision: "instant",
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

export const FlaggedValueWithUnpricedDeals: Story = {
  args: {
    day: readingsDay({
      revenue_at_risk_minor: 12500000,
      revenue_currency: "USD",
      unpriced_deals: 2,
    }),
  },
};
export const FlaggedValueWithUnpricedDealsDark: Story = {
  ...FlaggedValueWithUnpricedDeals,
  globals: { theme: "dark" },
};

// The strip as Home's Numbers rail draws it: rows with an arrow and rows
// without one, whose figures end on one edge.
export const InTheRail: Story = {
  args: { day: { ...readingsDay({}, [], []), calendar: "not_connected" } },
  decorators: [
    (Story) => (
      <div className="brief-rail" style={{ inlineSize: 360 }}>
        <Story />
      </div>
    ),
  ],
};

// Home's Numbers panel whole: a quiet day with one prepared meeting, no flagged
// deal, and zeros that open nothing.
export const NumbersPanel: Story = {
  args: {
    day: {
      ...readingsDay(
        { prospecting: 0, review: 0 },
        [meetingRow("m1", true)],
        [wholeMeetings(1)],
      ),
      calendar: "connected",
    },
  },
  decorators: [
    (Story) => (
      <div className="brief-rail" style={{ inlineSize: 360 }}>
        <Panel title="Numbers" flatHead className="brief-overview">
          <PanelBody>
            <Story />
          </PanelBody>
        </Panel>
      </div>
    ),
  ],
};
export const NumbersPanelDark: Story = {
  ...NumbersPanel,
  globals: { theme: "dark" },
};
