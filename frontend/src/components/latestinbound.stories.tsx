// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { StoryProviders } from "../testing/story-utils";
import { LatestInboundLine, LatestInboundPanel } from "./latestinbound";

type Activity = components["schemas"]["Activity"];

// The newest unanswered thing on a record, as the panel leading its Overview
// and as the line in its head on every other tab. The times are an hour or so
// before now, so the label reads the way it does on a live record.
const hoursAgo = (hours: number) =>
  new Date(Date.now() - hours * 3_600_000).toISOString();

function activity(id: string, fields: Partial<Activity>): Activity {
  const at = fields.occurred_at ?? hoursAgo(2);
  return {
    id,
    kind: "email",
    occurred_at: at,
    occurred_precision: "instant",
    source: "email",
    captured_by: "connector:email",
    created_at: at,
    updated_at: at,
    offers_email: true,
    is_done: false,
    ...fields,
  };
}

const mail = activity("mail", {
  direction: "inbound",
  subject: "Re: Pricing for 40 seats",
  email_summary: {
    activity_id: "mail",
    occurred_at: hoursAgo(2),
    occurred_precision: "instant",
    display_status: "workspace",
    attachment_count: 0,
    move: "needs_reply",
    version: 1,
    subject: "Re: Pricing for 40 seats",
    preview:
      "Thanks, the numbers look fine. Can you send the order form today? I want it signed before our board meets on Friday.",
    counterparty: "Jonas Weber",
    counterparty_contact_id: "p-jonas",
  },
});

const form = activity("form", {
  kind: "note",
  source: "web_form",
  occurred_at: hoursAgo(0.2),
  subject: "Website form: Request a demo",
  body: "We run 45 field reps on spreadsheets and lose track of follow-ups.\n\nName: Marta Kühn\nCompany: Nordlicht Logistik\nTeam size: 40-60\nPage: https://example.com/demo",
});

const chat = activity("chat", {
  kind: "message",
  direction: "inbound",
  source: "connector",
  occurred_at: hoursAgo(0.7),
  body: "I uploaded the cleaned contact CSV to the shared folder. Can you start the import tomorrow?",
});

// Stands in for the record's own row verbs, which each page hands in.
const verbs = () => <Button variant="primary">Reply</Button>;

const meta: Meta<typeof LatestInboundPanel> = {
  title: "Records/Just in",
  component: LatestInboundPanel,
};
export default meta;
type Story = StoryObj<typeof LatestInboundPanel>;

export const Email: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <LatestInboundPanel
          latest={{ activity: mail, kind: "email" }}
          actions={verbs}
        />
      </div>
    </StoryProviders>
  ),
};

export const WebForm: Story = {
  name: "Web form",
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <LatestInboundPanel
          latest={{ activity: form, kind: "web_form" }}
          actions={verbs}
        />
      </div>
    </StoryProviders>
  ),
};

export const Message: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <LatestInboundPanel
          latest={{ activity: chat, kind: "message" }}
          actions={verbs}
        />
      </div>
    </StoryProviders>
  ),
};

export const Withheld: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <LatestInboundPanel
          latest={{
            activity: { ...mail, content_state: "withheld" },
            kind: "email",
          }}
        />
      </div>
    </StoryProviders>
  ),
};

export const HeadLine: Story = {
  name: "Line in the record's head",
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 300 }}>
        <LatestInboundLine
          latest={{ activity: mail, kind: "email" }}
          onOpen={() => {}}
        />
      </div>
    </StoryProviders>
  ),
};
