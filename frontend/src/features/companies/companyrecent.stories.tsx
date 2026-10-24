// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { LatestCard } from "./companyoverviewlatest";

// The Overview's Latest card: the newest exchanges, who, kind and day
// under each title, and "Unanswered" where an email waits on us.

type Activity = components["schemas"]["Activity"];
type Company360 = components["schemas"]["Company360"];

const email: Activity = {
  id: "a-1",
  kind: "email",
  subject: "Re: retrofit timeline",
  occurred_at: "2026-08-01T12:15:00Z",
  is_done: false,
  direction: "inbound",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-08-01T12:15:00Z",
  updated_at: "2026-08-01T12:15:00Z",
  email_summary: {
    activity_id: "a-1",
    occurred_at: "2026-08-01T12:15:00Z",
    version: 1,
    subject: "Re: retrofit timeline",
    preview: "Can we push the fleet retrofit review back a week?",
    counterparty: "Dana Buyer",
    direction: "inbound",
    display_status: "team",
    move: "needs_reply",
    attachment_count: 0,
  },
};

const note: Activity = {
  ...email,
  id: "a-2",
  kind: "note",
  subject: "Site visit notes",
  occurred_at: "2026-07-20T10:00:00Z",
  direction: null,
  email_summary: null,
};

function view(activities: Activity[]): Company360 {
  return {
    as_of: "2026-08-13T09:00:00Z",
    company: {
      id: "o-1",
      display_name: "Brandt Automotive GmbH",
      source: "manual",
      captured_by: "human:u1",
      created_at: "2026-06-01T08:00:00Z",
      updated_at: "2026-06-01T08:00:00Z",
    },
    sections_omitted: [],
    activities: {
      data: activities,
      page: { has_more: false, next_cursor: null },
    },
  };
}

function Card({ activities }: Readonly<{ activities: Activity[] }>) {
  installFetchStub({
    "GET /users/u1": () =>
      jsonResponse({ id: "u1", display_name: "Lena Fischer" }),
  });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 420 }}>
        <LatestCard
          view={view(activities)}
          onOpenTab={() => {}}
          onOpenEmail={() => {}}
        />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof Card> = {
  title: "Records/Company 360/Latest",
  component: Card,
};
export default meta;
type Story = StoryObj<typeof Card>;

export const Realistic: Story = { args: { activities: [email, note] } };

export const Empty: Story = { args: { activities: [] } };
