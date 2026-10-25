// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { RecordZoneProvider } from "../../lib/recordzone";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { HeldMeetingsTable } from "./contactmeetings.held";

type Activity = components["schemas"]["Activity"];

// Fourteen hours ahead of UTC, so noon UTC is already the next day here. A
// day entry still reads its own date; an instant reads this zone's.
const RECORD_ZONE = "Pacific/Kiritimati";

const held: Activity = {
  id: "0198f011-dddd-7000-8000-000000000001",
  kind: "meeting",
  subject: "Depot walkthrough",
  occurred_at: "2026-09-24T08:00:00Z",
  occurred_precision: "instant",
  offers_email: false,
  duration_seconds: 1800,
  meeting_status: "held",
  is_done: false,
  source: "manual",
  captured_by: "human:host",
  content_state: "available",
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
};

const meta: Meta<typeof HeldMeetingsTable> = {
  title: "Records/Contact 360/Held meetings",
  component: HeldMeetingsTable,
  args: { verbs: () => null },
  decorators: [
    (Story) => {
      installFetchStub({ "GET /me": meRoute({ activity: ["read"] }) });
      return (
        <StoryProviders>
          <RecordZoneProvider zone={RECORD_ZONE}>
            <Story />
          </RecordZoneProvider>
        </StoryProviders>
      );
    },
  ],
};
export default meta;
type Story = StoryObj<typeof HeldMeetingsTable>;

export const Held: Story = { args: { held: [held] } };

// A visit logged by its date alone sits beside one logged at its hour. The
// day entry shows its own date with no clock, though the zone has moved on.
export const DayOnly: Story = {
  args: {
    held: [
      {
        ...held,
        id: "0198f011-dddd-7000-8000-000000000002",
        subject: "Site visit",
        occurred_at: "2026-09-24T12:00:00Z",
        occurred_precision: "day",
      },
      held,
    ],
  },
  play: async ({ canvasElement }) => {
    const title = await within(canvasElement).findByText("Site visit");
    const row = title.closest("tr");
    await expect(row).not.toBeNull();
    await expect(row?.textContent).toMatch(/24 Sept? 2026/);
    await expect(row?.textContent).not.toMatch(/\d{1,2}:\d{2}/);
  },
};

export const DayOnlyPhone: Story = { ...DayOnly, tags: ["uat-phone"] };
