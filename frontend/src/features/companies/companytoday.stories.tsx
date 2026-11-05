// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { company360 } from "../../testing/company.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import type { AccountScan } from "./accountscan";
import { NeedsList, useTodayReading } from "./companytoday";

// What needs a contact on the account, under the Overview's cards: a booked
// meeting, a manual draft, and whatever the account's own suggestions advise.
//
// No seeded demo account can reach the withheld story. Every one grants the
// viewer full RBAC, so `state_strip` and `contacts` (the best route in) are
// never omitted on a live session. A role scoped away from either still reads
// the rest of the list. It says so for the two it cannot answer instead of
// dropping them silently.

const meta: Meta = {
  title: "Records/Company 360/Needs list",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type View = components["schemas"]["Company360"];
type Company = components["schemas"]["Company"];

const page = { has_more: false, next_cursor: null };

const company: Company = {
  id: "o-1",
  workspace_id: "w-1",
  display_name: "Brandt Automotive GmbH",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

// THE STRIP ON ITS OWN, so a story that changes one reading of it spreads
// something that is definitely there. Read back off `populated` it is
// `View["state_strip"]` — optional on the contract — and the spread then makes
// `account` optional in a place the type requires it.
const strip: NonNullable<View["state_strip"]> = {
  account: { lifecycle: "customer", relationship_types: ["customer"] },
  engagement: {
    state: "waiting_on_us",
    last_inbound_at: "2026-07-11T09:00:00Z",
    last_outbound_at: null,
  },
  signal: {
    kind: "stalled_deal",
    severity: "warn",
    summary: "Depot pilot has had no activity in 18 days.",
  },
};

const populated: View = {
  ...company360,
  as_of: "2026-07-13T09:00:00Z",
  company,
  sections_omitted: [],
  state_strip: strip,
  contacts: {
    data: [
      {
        contact_id: "p-1",
        full_name: "Dana Buyer",
        title: "Head of Fleet",
        deal_roles: [],
        consent: { marketing_email: "granted" },
        routes: {
          top: [
            {
              user_id: "u-1",
              display_name: "Mira Voss",
              strength_bucket: "strong",
            },
          ],
          untried: false,
          remainder: 0,
        },
        strength: {
          score: 71,
          bucket: "strong",
          factors: {
            recency: 0.9,
            frequency: 0.6,
            reciprocity: 0.8,
            direction: 0.8,
          },
        },
      },
    ],
    page,
  },
  next_meeting: {
    activity_id: "a-1",
    starts_at: "2026-07-14T09:00:00Z",
    occurred_precision: "instant",
    subject: "Renewal review",
    participants: [{ contact_id: "p-1", display_name: "Dana Buyer" }],
  },
  suggestions: [
    {
      kind: "no_reply",
      fingerprint: "f-1",
      reason: "You reached out 11 days ago and nobody has come back.",
      // The receipt the rule carries: the message's own subject and opening
      // words, the day it went out, and the channel — what the chip opens.
      evidence: [
        {
          entity_type: "activity",
          entity_id: "a-2",
          name: "Renewal terms for 2027",
          quote: "Hi Dana, attached are the renewal terms we discussed.",
          at: "2026-07-27T09:00:00Z",
          origin: "Email you sent",
        },
      ],
    },
  ],
  next_steps: {
    data: [
      {
        activity_id: "a-3",
        subject: "Send the renewal paperwork",
        due_at: "2026-07-01T09:00:00Z",
        overdue: true,
      },
    ],
    page,
  },
};

// state_strip and contacts withheld — the two readings no seeded demo account
// ever omits, so this is the only place the brief's own withheld path for
// either one renders.
const withheld: View = {
  ...company360,
  ...populated,
  state_strip: undefined,
  contacts: undefined,
  next_meeting: undefined,
  sections_omitted: ["state_strip", "contacts", "next_meeting"],
};

function Brief({
  view,
  loading = false,
  failed = false,
  scan,
}: Readonly<{
  view?: View;
  loading?: boolean;
  failed?: boolean;
  scan?: AccountScan;
}>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <Needs view={view} loading={loading} failed={failed} scan={scan} />
      </div>
    </StoryProviders>
  );
}

// Inside the providers, which the hook reads: `Brief` renders them, so it
// cannot call the hook itself.
function Needs({
  view,
  loading,
  failed,
  scan,
}: Readonly<{
  view?: View;
  loading: boolean;
  failed: boolean;
  scan?: AccountScan;
}>) {
  const reading = useTodayReading({
    companyId: "o-1",
    view,
    loading,
    failed,
    scan,
  });
  return <NeedsList reading={reading} />;
}

export const Populated: Story = { render: () => <Brief view={populated} /> };

// Advice resting on a message rather than on a quoted receipt: the rule's
// evidence carries the server's own row model, so the basis is drawn as an
// EmailEntry — subject, sender and preview — instead of a chip. The preview
// is one long unbreakable line on purpose: the row owes the card an ellipsis
// at the card's measure, and a basis sized to its own words rather than to
// the card runs straight out of it.
const restingOnAMail: View = {
  ...company360,
  ...populated,
  suggestions: [
    {
      kind: "no_reply",
      fingerprint: "f-2",
      title: "Confirm the server booking",
      reason:
        "You asked the client to book the server, but nothing says it was done or followed up on.",
      evidence: [
        {
          entity_type: "activity",
          entity_id: "a-4",
          email_summary: {
            activity_id: "a-4",
            occurred_at: "2026-07-11T14:17:00Z",
            occurred_precision: "instant",
            version: 1,
            subject: "Re: Scheduling tool: project start + setup",
            preview:
              "You would then still need to book the server once more On Fri, Jul 11, 2026 at 4:17 PM Dana Buyer <dana@brandt.example> wrote: thanks for the walkthrough, we will sort the booking out on our side next week",
            counterparty: null,
            direction: "outbound",
            display_status: "team",
            move: "waiting_for_them",
            attachment_count: 0,
          },
        },
      ],
    },
  ],
};

export const AdviceRestingOnAMail: Story = {
  render: () => <Brief view={restingOnAMail} />,
};

// Margince reading the account: the rules' rows stand, and the pending row
// above them says more is coming rather than that this is everything.
export const BeingRead: Story = {
  render: () => (
    <Brief
      view={populated}
      scan={{
        company_id: "o-1",
        state: "running",
        findings: populated.suggestions ?? [],
        findings_dropped: 0,
      }}
    />
  ),
};

// The read has answered: the model's finding beside the rule's row, each
// with its receipt, and the foot saying who wrote what from how much.
export const Scanned: Story = {
  render: () => (
    <Brief
      view={populated}
      scan={{
        company_id: "o-1",
        state: "done",
        generated_at: "2026-08-07T08:58:00Z",
        generated_by: "model",
        read: { exchanges: 14, deals: 2, window_days: 180 },
        findings: [
          ...(populated.suggestions ?? []),
          {
            kind: "question_unanswered",
            fingerprint: "f-2",
            title: "Confirm the depot installation",
            reason:
              "Dana asked whether installation can happen at their Kassel depot, and nothing of yours since answers it.",
            written_by: "model",
            due_at: "2026-08-03T14:20:00Z",
            evidence: [
              {
                entity_type: "activity",
                entity_id: "a-3",
                name: "Re: Fleet retrofit 2026 — proposal",
                quote:
                  "can you confirm whether the installation can be done at our depot in Kassel rather than at your site?",
                at: "2026-08-03T14:20:00Z",
                origin: "Email they sent",
              },
            ],
            action: { kind: "draft_reply", activity_id: "a-3" },
          },
        ],
        findings_dropped: 0,
      }}
    />
  ),
};

export const SectionWithheld: Story = {
  render: () => <Brief view={withheld} />,
};

// The advice withheld and nothing else to list: the note says what was not
// read, and no quiet sentence claims the day over it.
export const AdviceWithheldNothingListed: Story = {
  render: () => (
    <Brief
      view={{
        ...company360,
        suggestions: undefined,
        sections_omitted: ["suggestions"],
      }}
    />
  ),
};

export const Loading: Story = { render: () => <Brief loading /> };

export const Failed: Story = { render: () => <Brief failed /> };
