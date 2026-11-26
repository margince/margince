// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";

import type { CalendarChange } from "../lib/calendarchange";
import { StoryProviders } from "../testing/story-utils";
import { Card } from "./atoms";
import { EmailEntry } from "./emailentry";
import { MeetingChange } from "./meetingchange";

// A meeting's later mail, drawn as the meeting. Each story is one thing the
// organizer or a guest can do to it; `InARow` is the box where it lives.

const MOVED: CalendarChange = {
  kind: "updated",
  title: "Pricing review with Anna Weber",
  location: "Office Berlin, Room 4",
  starts_at: "2026-12-03T14:00:00Z",
  ends_at: "2026-12-03T15:00:00Z",
  starts_precision: "instant",
  changed: ["time", "location"],
  before: {
    starts_at: "2026-12-02T09:00:00Z",
    ends_at: "2026-12-02T10:00:00Z",
    starts_precision: "instant",
    location: "Microsoft Teams",
  },
  guests_added: [{ email: "ben.ito@partner.example", name: "Ben Ito" }],
};

function Box({ change }: Readonly<{ change: CalendarChange }>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 560 }}>
        <MeetingChange change={change} fallbackTitle="Updated invitation" />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof Box> = {
  title: "Components/Text and data display/Meeting change",
  component: Box,
};
export default meta;
type Story = StoryObj<typeof Box>;

export const Moved: Story = {
  args: { change: MOVED },
  play: async ({ canvasElement }) => {
    const box = within(canvasElement);
    await expect(box.getByText("Meeting changed")).toBeVisible();
    await expect(box.getByText("Microsoft Teams")).toBeVisible();
    await expect(box.getByText("Added Ben Ito")).toBeVisible();
  },
};

export const Canceled: Story = {
  args: {
    change: {
      kind: "canceled",
      title: "Pricing review with Anna Weber",
      location: "Office Berlin, Room 4",
      starts_at: "2026-12-03T14:00:00Z",
      ends_at: "2026-12-03T15:00:00Z",
      starts_precision: "instant",
      changed: [],
    },
  },
};

export const NewTimeProposed: Story = {
  args: {
    change: {
      kind: "proposed",
      title: "Pricing review with Anna Weber",
      starts_at: "2026-12-04T08:00:00Z",
      ends_at: "2026-12-04T09:00:00Z",
      starts_precision: "instant",
      changed: ["time"],
      before: {
        starts_at: "2026-12-03T14:00:00Z",
        ends_at: "2026-12-03T15:00:00Z",
        starts_precision: "instant",
      },
      by: { email: "anna.weber@partner.example", name: "Anna Weber" },
    },
  },
};

export const GuestDeclined: Story = {
  args: {
    change: {
      kind: "answered",
      title: "Pricing review with Anna Weber",
      starts_at: "2026-12-03T14:00:00Z",
      starts_precision: "instant",
      changed: [],
      by: { email: "anna.weber@partner.example", name: "Anna Weber" },
      answer: "declined",
    },
  },
};

export const AllDayRenamed: Story = {
  args: {
    change: {
      kind: "updated",
      title: "Partner offsite, day two",
      starts_at: "2026-11-20T00:00:00Z",
      starts_precision: "day",
      changed: ["title"],
      before: { title: "Partner offsite" },
    },
  },
};

/** Where the box lives: in the one email row, on a card. */
export const InARow: Story = {
  render: () => (
    <StoryProviders>
      <Card style={{ maxWidth: 560 }}>
        <EmailEntry
          summary={{
            activity_id: "11111111-1111-4111-8111-111111111111",
            occurred_at: "2026-11-11T09:12:00Z",
            occurred_precision: "instant",
            display_status: "team",
            attachment_count: 0,
            move: "none",
            version: 1,
            subject: "Updated invitation: Pricing review with Anna Weber",
            preview: "Anna Weber has updated this event.",
            direction: "inbound",
            counterparty: "Anna Weber",
            calendar_change: MOVED,
          }}
          timestamp="11 Nov 09:12"
          onOpen={() => {}}
        />
      </Card>
    </StoryProviders>
  ),
};
