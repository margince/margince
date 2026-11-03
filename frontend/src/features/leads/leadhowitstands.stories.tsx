// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import type { RecordTimeline } from "../../design-system/recordtimeline";
import type { Locale } from "../../i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadHowItStands } from "./leadhowitstands";
import type { Lead } from "./leads";
import { useLeadPatch } from "./leads.patch";

type Activity = components["schemas"]["Activity"];

const AS_OF = "2026-06-20T15:00:00Z";

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 72,
  source: "manual",
  captured_by: "human:u-1",
  writable: true,
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
};

function meeting(over: Partial<Activity>): Activity {
  return {
    id: "a-meeting",
    kind: "meeting",
    subject: "Depot site visit",
    meeting_status: "booked",
    occurred_at: "2026-06-24T09:00:00Z",
    occurred_precision: "instant",
    offers_email: false,
    source: "manual",
    captured_by: "human:u-1",
    is_done: false,
    created_at: "2026-06-18T08:00:00Z",
    updated_at: "2026-06-18T08:00:00Z",
    ...over,
  };
}

function thread(activities: Activity[]): RecordTimeline {
  return {
    activities,
    hasNextPage: false,
    isFetchingNextPage: false,
    fetchNextPage: () => undefined,
    isPending: false,
    isSuccess: true,
    isError: false,
    refetch: () => undefined,
  };
}

function Standing({
  activities,
  record = lead,
}: Readonly<{ activities: Activity[]; record?: Lead }>) {
  const writer = useLeadPatch(record, record.id, () => undefined);
  return (
    <LeadHowItStands
      lead={record}
      writer={writer}
      terminalReasonId="lead-terminal"
      thread={thread(activities)}
      asOf={AS_OF}
      onQualify={() => undefined}
      onDisqualify={() => undefined}
    />
  );
}

function renderStanding(
  activities: Activity[],
  record: Lead = lead,
  locale: Locale = "en",
) {
  installFetchStub({
    "GET /me": meRoute({ lead: ["read", "update"] }),
    "GET /lead-disqualify-reasons": () => jsonResponse({ data: REASONS }),
  });
  return (
    <StoryProviders locale={locale}>
      <Standing activities={activities} record={record} />
    </StoryProviders>
  );
}

const REASONS: components["schemas"]["LeadDisqualifyReason"][] = [
  {
    id: "r-budget",
    label: "No budget",
    label_key: "leadReasons.builtin.no_budget",
    sort_order: 30,
    active: true,
    system: true,
    lead_count: 1,
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
];

const meta = {
  title: "Records/Leads/How it stands",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;
type Story = StoryObj;

async function nextMeetingCard(canvasElement: HTMLElement) {
  const label = await within(canvasElement).findByText("Next meeting", {
    selector: ".stat-card-label-text",
  });
  return label.closest(".stat-card");
}

export const BookedMeeting: Story = {
  render: () => renderStanding([meeting({})]),
  play: async ({ canvasElement }) => {
    const card = await nextMeetingCard(canvasElement);
    await expect(card?.textContent).toContain("Depot site visit");
  },
};

// A meeting known only by its day is still ahead for the whole of that day.
// The figure keeps naming it after noon UTC has passed.
export const MeetingOnlyItsDayKnown: Story = {
  render: () =>
    renderStanding([
      meeting({
        occurred_at: "2026-06-20T12:00:00Z",
        occurred_precision: "day",
      }),
    ]),
  play: async ({ canvasElement }) => {
    const card = await nextMeetingCard(canvasElement);
    await expect(card?.textContent).toContain("Depot site visit");
    await expect(card?.textContent).not.toMatch(/\d{1,2}:\d{2}/);
  },
};

// The server sends the reason's stored English label. A German reader sees
// the built-in reason in German.
export const ClosedForABuiltInReason: Story = {
  render: () =>
    renderStanding(
      [],
      {
        ...lead,
        status: "disqualified",
        disqualify_reason_id: "r-budget",
        disqualify_reason: "No budget",
      },
      "de",
    ),
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText(/Geschlossen: Kein Budget\./),
    ).toBeInTheDocument();
  },
};
