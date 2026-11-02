// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, within } from "storybook/test";
import { UTC_ZONE } from "../format/timezone";
import { en } from "../i18n/en";
import { StoryProviders } from "../testing/story-utils";
import {
  type OpenMeeting,
  RecordOfMeetingQuestion,
} from "./logactivity.openmeeting";

// The question the Log form asks when a booked meeting on the record still owes
// its outcome. A calendar meeting adds why its notes become a note.

const meta: Meta = {
  title: "Patterns/Log activity/Record of meeting",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

const MEETING: OpenMeeting = {
  id: "m-1",
  subject: "Quarterly review",
  occurred_at: "2026-07-06T09:00:00Z",
  occurred_precision: "instant",
  calendar_managed: false,
  version: 4,
  body: null,
};

function Question({ meeting }: Readonly<{ meeting: OpenMeeting }>) {
  const [recordsIt, setRecordsIt] = useState(true);
  return (
    <StoryProviders>
      <RecordOfMeetingQuestion
        meeting={meeting}
        value={recordsIt}
        onChange={setRecordsIt}
        zone={UTC_ZONE}
      />
    </StoryProviders>
  );
}

export const Booked: Story = {
  render: () => <Question meeting={MEETING} />,
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("radio", {
        name: en["log.recordOfMeetingYes"],
      }),
    ).toBeChecked();
  },
};

export const CalendarManaged: Story = {
  render: () => <Question meeting={{ ...MEETING, calendar_managed: true }} />,
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText(en["log.recordOfMeetingManaged"]);
  },
};

export const CalendarManagedDark: Story = {
  ...CalendarManaged,
  globals: { theme: "dark" },
};
