// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { company360 } from "../../testing/company.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyMeetingsTab } from "./companymeetings";

type Activity = components["schemas"]["Activity"];
type Company360 = components["schemas"]["Company360"];

// Read the morning before the kickoff.
function view(more: Partial<Company360> = {}): Company360 {
  return { ...company360, as_of: "2026-06-01T09:00:00Z", ...more };
}

function meeting(
  id: string,
  occurredAt: string,
  subject: string,
  more: Partial<Activity> = {},
): Activity {
  return {
    occurred_precision: "instant",
    offers_email: false,
    id,
    kind: "meeting",
    subject,
    occurred_at: occurredAt,
    duration_seconds: 2700,
    is_done: false,
    source: "manual",
    captured_by: "human:host",
    content_state: "available",
    created_at: "2026-05-01T09:00:00Z",
    updated_at: "2026-05-01T09:00:00Z",
    ...more,
  };
}

// An account in motion: a kickoff with two of its contacts next, a demo after
// it, and three meetings already held, one of them missed.
const nextId = "0198f011-dddd-7000-8000-000000000001";
const meetings: Activity[] = [
  meeting(
    "0198f011-dddd-7000-8000-000000000002",
    "2026-06-09T13:00:00Z",
    "Platform demo for the plant managers",
    { meeting_status: "booked" },
  ),
  meeting(nextId, "2026-06-03T09:00:00Z", "Rollout kickoff", {
    meeting_status: "booked",
  }),
  meeting(
    "0198f011-dddd-7000-8000-000000000003",
    "2026-05-27T10:00:00Z",
    "Pricing call",
    { meeting_status: "held" },
  ),
  meeting(
    "0198f011-dddd-7000-8000-000000000004",
    "2026-05-19T14:00:00Z",
    "Security review",
    { meeting_status: "no_show" },
  ),
  meeting(
    "0198f011-dddd-7000-8000-000000000005",
    "2026-05-06T08:30:00Z",
    "Discovery workshop",
    { meeting_status: "held" },
  ),
];

function withMeetings(rows: Activity[]): Decorator {
  return (Story) => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }, { seat: "full" }),
      "GET /activities": () =>
        jsonResponse({
          data: rows,
          page: { has_more: false, next_cursor: null },
        }),
    });
    return (
      <StoryProviders>
        <Story />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof CompanyMeetingsTab> = {
  title: "Records/Company 360/Meetings",
  component: CompanyMeetingsTab,
  args: { companyId: "o-1", loading: false, onBriefMeeting: () => {} },
};
export default meta;
type Story = StoryObj<typeof CompanyMeetingsTab>;

export const Busy: Story = {
  decorators: [withMeetings(meetings)],
  args: {
    view: view({
      next_meeting: {
        activity_id: nextId,
        starts_at: "2026-06-03T09:00:00Z",
        occurred_precision: "instant",
        subject: "Rollout kickoff",
        participants: [
          { contact_id: "p-1", display_name: "Nina Weber" },
          { contact_id: "p-2", display_name: "Jonas Brandt" },
        ],
      },
    }),
  },
};
export const BusyDark: Story = { ...Busy, globals: { theme: "dark" } };
export const BusyPhone: Story = { ...Busy, tags: ["uat-phone"] };

export const Empty: Story = {
  decorators: [withMeetings([])],
  args: { view: view() },
};

export const Withheld: Story = {
  decorators: [withMeetings([])],
  args: { view: view({ sections_omitted: ["activities", "next_meeting"] }) },
};
