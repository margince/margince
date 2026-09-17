// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import type { AccountScan } from "./accountscan";
import { company360 } from "./company.fixtures";
import {
  Company360Call,
  TodayOnThisAccount,
  useTodayReading,
} from "./companytoday";
import { StoryProviders } from "./story-utils";

// The daily brief, above the tab bar: a context band read off the account's
// dated readings, then the moves — a booked meeting, a manual draft, and
// whatever the account's own suggestions advise.
//
// The withheld story is the one no seeded demo account can reach: every one
// grants the viewer full RBAC, so `state_strip` (whose move, the open risk)
// and `contacts` (the best route in) are never omitted on a live session. A
// role scoped away from either still reads the rest of the brief; it just
// says so for the two it cannot answer, rather than silently dropping them.

const meta: Meta = {
  title: "Records/Company 360/Today on this account",
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
  account: { status: "customer", relationship_types: ["customer"] },
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
  company: company,
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

// Two of the three dimensions rated, each with the reading its rating was made
// from: what the chips' asides carry.
const rated: View = {
  ...company360,
  ...populated,
  health: {
    relationship: {
      rating: "strong",
      reason: "They last wrote 2 days ago, and started the thread.",
    },
    commercial: {
      rating: "at_risk",
      reason: "The depot pilot has not moved in 18 days.",
    },
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
        <TodayOnThisAccount
          companyId="o-1"
          view={view}
          loading={loading}
          failed={failed}
          scan={scan}
        />
      </div>
    </StoryProviders>
  );
}

export const Populated: Story = { render: () => <Brief view={populated} /> };

// The account card fires on owed promises and nothing else. Here nothing is
// owed while the rules still have something to say, so the quiet card is NOT
// drawn: "nothing needs you today" over a row asking for an answer is the
// panel disagreeing with itself, and the row is the half a reader can check.
const nothingOwed: View = {
  ...company360,
  ...populated,
  moment: {
    claim_key: "moment:nothing_needed",
    evidence_fingerprint: "quiet",
    rule: "nothing_needed",
    headline: "Nothing is owed to this account",
    why_now: "No promise to this account is open or coming due.",
    confidence: "observed_fact",
    evidence: [],
    recommended_action: {
      kind: "log_activity",
      label: "Log something",
      state: "will_confirm",
    },
  },
};

export const NothingOwedButAdviceStands: Story = {
  render: () => <Brief view={nothingOwed} />,
};

// The same card when it IS the whole answer: with no advice and nothing
// scheduled it leads the list, keeping the reason and the verb that the
// panel's own bare sentence has nowhere to put.
export const NothingOwedAndNothingAdvised: Story = {
  render: () => (
    <Brief
      view={{
        ...company360,
        ...nothingOwed,
        suggestions: [],
        next_meeting: undefined,
        next_steps: { data: [], page },
      }}
    />
  ),
};

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
        read: { exchanges: 14, deals: 2 },
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

// The call, with the three rated dimensions under it. Payment is unrated here
// on purpose: the finance read the rating comes from is not stubbed, which is
// the state a fresh account is actually in — and it is the chip whose aside
// has a definition and no working to show.
function Call({ view }: Readonly<{ view: View }>) {
  const reading = useTodayReading({
    companyId: "o-1",
    view,
    loading: false,
    failed: false,
  });
  return <Company360Call reading={reading} name={company.display_name} />;
}

export const Reading: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <Call view={rated} />
      </div>
    </StoryProviders>
  ),
};

export const SectionWithheld: Story = {
  render: () => <Brief view={withheld} />,
};

// waiting_on_them is one of only two engagement states ENGAGEMENT_TONE
// (company360.tsx) colours "warn" — the ball is in their court, not ours —
// and the one state that also draws a silence note (companytoday.tsx's own
// `silenceNote`, gated on this exact state plus a `last_outbound_at` to
// count from). populated above never reaches either: its engagement is
// waiting_on_us, the one state that carries no tone at all.
export const WaitingOnThem: Story = {
  render: () => (
    <Brief
      view={{
        ...company360,
        ...populated,
        state_strip: {
          ...strip,
          engagement: {
            state: "waiting_on_them",
            last_inbound_at: null,
            last_outbound_at: "2026-06-25T09:00:00Z",
          },
        },
      }}
    />
  ),
};

// dormant: the other warn-toned engagement state, but the one that carries no
// silence note of its own — silenceNote only ever fires for waiting_on_them,
// so a dormant account's warning is the strip's tone and nothing else.
export const Dormant: Story = {
  render: () => (
    <Brief
      view={{
        ...company360,
        ...populated,
        state_strip: {
          ...strip,
          engagement: {
            state: "dormant",
            last_inbound_at: "2026-04-02T09:00:00Z",
            last_outbound_at: "2026-04-10T09:00:00Z",
          },
        },
      }}
    />
  ),
};

export const Loading: Story = { render: () => <Brief loading /> };

export const Failed: Story = { render: () => <Brief failed /> };
