// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { CompanyRecentList } from "./companyrecent";
import { StoryProviders } from "./story-utils";

// "What happened lately" (co360's own row-per-exchange reading): the deal a
// row is ABOUT, named when `nameOf` can resolve it and left the honest
// unnamed phrase when it cannot. The neutral kind chip never borrows the
// AI-provenance indigo for a human's own correspondence.

type Activity = components["schemas"]["Activity"];

const base: Activity = {
  id: "a-1",
  kind: "email",
  subject: "Renewal terms",
  occurred_at: "2026-08-20T09:00:00Z",
  is_done: false,
  direction: "inbound",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-08-20T09:00:00Z",
  updated_at: "2026-08-20T09:00:00Z",
  // A retained email carries the server's own row model, and drawing it is
  // what makes these stories show the canonical row rather than the fallback
  // every other kind takes.
  email_summary: {
    activity_id: "a-1",
    occurred_at: "2026-08-20T09:00:00Z",
    version: 1,
    subject: "Renewal terms",
    preview: "Sending the revised terms across for Thursday.",
    counterparty: "Dana Buyer",
    direction: "inbound",
    display_status: "team",
    move: "needs_reply",
    attachment_count: 1,
  },
};

function List({ activities }: Readonly<{ activities: Activity[] }>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 640 }}>
        <CompanyRecentList
          activities={activities}
          nameOf={(entityType, entityId) =>
            entityType === "deal" && entityId === "d-1"
              ? "Acme Expansion"
              : undefined
          }
          onOpenRecord={() => {}}
        />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof List> = {
  title: "Records/Company 360/What happened lately",
  component: List,
};
export default meta;
type Story = StoryObj<typeof List>;

// A row about a deal `nameOf` can resolve: "on Acme Expansion" is a fact a
// rep acts on, not a click to find out.
export const DealResolved: Story = {
  render: () => (
    <List
      activities={[
        {
          ...base,
          links: [{ entity_type: "deal", entity_id: "d-1" }],
        },
      ]}
    />
  ),
};

// The same shape of row, but the reading this list was built from no longer
// carries that deal (a closed deal, a capped list). The fallback phrase, not
// a blank line where a name would have gone.
export const DealNameMissing: Story = {
  render: () => (
    <List
      activities={[
        {
          ...base,
          id: "a-2",
          links: [{ entity_type: "deal", entity_id: "d-missing" }],
        },
      ]}
    />
  ),
};

// Every kind the row draws, each with its own neutral chip and its own
// direction phrase: an email they wrote, a call we made, a task with no
// direction at all, and a note that states none either.
export const MixedKinds: Story = {
  render: () => (
    <List
      activities={[
        {
          ...base,
          id: "a-3",
          kind: "email",
          subject: "Re: Renewal terms",
          direction: "inbound",
          occurred_at: "2026-08-20T09:00:00Z",
          email_summary: {
            ...base.email_summary,
            activity_id: "a-3",
            occurred_at: "2026-08-20T09:00:00Z",
            version: 1,
            subject: "Re: Renewal terms",
            display_status: "team",
            move: "needs_reply",
            attachment_count: 1,
          },
        },
        // Every kind below drops the summary `base` carries. Only an email has
        // one, so a call spreading it would draw as a message and this story
        // would be describing a record the server never sends.
        {
          ...base,
          id: "a-4",
          kind: "call",
          subject: undefined,
          direction: "outbound",
          duration_seconds: 900,
          occurred_at: "2026-08-19T14:00:00Z",
          email_summary: undefined,
        },
        {
          ...base,
          id: "a-5",
          kind: "task",
          subject: "Send updated MSA",
          direction: undefined,
          occurred_at: "2026-08-18T11:00:00Z",
          email_summary: undefined,
        },
        {
          ...base,
          id: "a-6",
          kind: "note",
          subject: "Prefers async updates",
          direction: undefined,
          occurred_at: "2026-08-17T16:00:00Z",
          email_summary: undefined,
        },
      ]}
    />
  ),
};

// The same four kinds in dark: each chip is neutral by rule, never the
// AI-provenance indigo, so this is the story that shows whether that
// neutrality still holds once the panel itself is dark.
export const MixedKindsDark: Story = {
  ...MixedKinds,
  globals: { theme: "dark" },
};

// A message this reader may not open, beside one they may.
//
// The withheld row keeps its shape and loses its words — no subject, no
// counterparty, no preview. Drawing it as absent would report a quieter
// account than the one on file; drawing it as empty would say there was
// nothing to read, which is a claim about the message rather than about them.
export const WithheldBesideReadable: Story = {
  render: () => (
    <List
      activities={[
        base,
        {
          ...base,
          id: "a-7",
          subject: undefined,
          occurred_at: "2026-08-19T08:30:00Z",
          content_state: "withheld",
          email_summary: {
            activity_id: "a-7",
            occurred_at: "2026-08-19T08:30:00Z",
            version: 1,
            display_status: "withheld",
            move: "none",
            attachment_count: 0,
          },
        },
      ]}
    />
  ),
};
