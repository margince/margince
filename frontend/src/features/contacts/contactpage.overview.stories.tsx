// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../../testing/mefixture";
import {
  ContactCommitmentsCard,
  ContactDealCard,
  ContactUnderstandingCard,
} from "./contactcards";
import { ContactMemory } from "./contactmemory";
import "./contact360.css";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import {
  chatConversation,
  page,
  populated,
  type View,
  withheld,
} from "./contactpage.fixtures";

// The contact page's overview cards, from the page gallery
// (contactpage.stories.tsx). Its records come from contactpage.fixtures.ts.

const meta: Meta = {
  title: "Records/Contact 360/Page overview",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// --- Overview cards: the four the design draws, stacked ----------------------

const none = () => undefined;

export const OverviewPanels: Story = {
  render: () => {
    stubWithSession({}, {});
    return (
      <StoryProviders>
        <div className="record-stack" style={{ maxWidth: 720 }}>
          <ContactCommitmentsCard
            view={populated}
            firstName="Dana"
            onAllTasks={none}
          />
          <ContactDealCard
            view={populated}
            firstName="Dana"
            onOpenDeals={none}
          />
          <ContactMemory
            view={populated}
            onOpenEmail={none}
            onFullHistory={none}
          />
          <ContactUnderstandingCard
            view={populated}
            firstName="Dana"
            onCorrect={none}
          />
        </div>
      </StoryProviders>
    );
  },
};

export const OverviewChannelConversation: Story = {
  render: () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow: {} })),
      "GET /channel-providers": () =>
        jsonResponse({
          data: [
            {
              provider: "zalo_oa",
              label: "Zalo OA",
              credential_model: "workspace_bot",
              supplies_transport: true,
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <div className="record-stack" style={{ maxWidth: 720 }}>
          <ContactMemory
            view={chatConversation}
            onOpenEmail={none}
            onFullHistory={none}
          />
        </div>
      </StoryProviders>
    );
  },
};

// A deal with money, stage and a close date, and more than one stakeholder in
// the room. This is the populated end of ContactDealCard, which
// `populated` never reaches (its own commercial section carries no deal).
const dealWithCommittee: View = {
  ...populated,
  commercial: {
    role: "champion",
    deal: {
      deal_id: "d-1",
      title: "Fleet retrofit, 40 vehicles",
      stage: "negotiation",
      amount_minor: 9_500_000,
      currency: "EUR",
      close_date: "2026-09-30",
    },
    committee: [
      { contact_id: "p-2", full_name: "Mika Voss", role: "economic_buyer" },
      { contact_id: "p-3", full_name: "Jonas Reiter", role: "influencer" },
    ],
  },
};

// One of every loop kind ContactCommitmentsCard renders, plus a `done` row:
// `populated` carries only a single `commitment_ours`, so neither
// `commitment_theirs`, `open_question` nor the done state ever appear.
const richLoops: View = {
  ...populated,
  claims: [
    ...(populated.claims ?? []),
    {
      id: "c-3",
      kind: "commitment_theirs",
      body: "send the fleet utilization numbers",
      source_activity_id: "a-1",
      source_quote: "I'll get you the utilization numbers by Friday.",
      source_label: "Re: retrofit timeline",
      occurred_at: "2026-08-01T10:15:00Z",
      status: "open",
      needs_review: false,
    },
    {
      id: "c-4",
      kind: "open_question",
      body: "whether the depot needs a second charger bank",
      source_activity_id: "a-1",
      source_quote: "Not sure yet if one charger bank covers the whole depot.",
      source_label: "Re: retrofit timeline",
      occurred_at: "2026-08-01T10:15:00Z",
      status: "open",
      needs_review: false,
    },
    {
      id: "c-5",
      kind: "commitment_ours",
      body: "confirm the installation window",
      source_activity_id: "a-1",
      source_quote: "We'll confirm the installation window by Monday.",
      source_label: "Re: retrofit timeline",
      occurred_at: "2026-07-28T10:15:00Z",
      status: "done",
      needs_review: false,
    },
  ],
};

// Every channel ContactMemory labels (meeting, call, note, beside the email
// entries every other fixture carries) and a `replied` status. The single
// email entry on `populated` reaches none of these.
const richMemory: View = {
  ...populated,
  conversation_memory: [
    ...(populated.conversation_memory ?? []),
    {
      key: "thread-2",
      channel: "meeting",
      direction: "internal",
      title: "Fleet retrofit kickoff",
      summary: "Walked through the retrofit scope and the depot's constraints.",
      generated_by: "deterministic",
      occurred_at: "2026-07-15T13:00:00Z",
      activity_count: 1,
      status: null,
      linked_deal_id: null,
      first_activity_id: "a-4",
    },
    {
      key: "thread-3",
      channel: "call",
      direction: "outbound",
      title: "Check-in call",
      summary: "Confirmed the depot's offline window with Dana.",
      generated_by: "deterministic",
      occurred_at: "2026-07-22T09:30:00Z",
      activity_count: 1,
      status: "replied",
      linked_deal_id: null,
      first_activity_id: "a-5",
    },
    {
      key: "thread-4",
      channel: "note",
      direction: "internal",
      title: "Internal note",
      summary:
        "Sam flagged Dana as the sole technical contact on this account.",
      generated_by: "deterministic",
      occurred_at: "2026-07-10T09:00:00Z",
      activity_count: 1,
      status: null,
      linked_deal_id: null,
      first_activity_id: "a-6",
    },
  ],
};

// No thread, no meeting, no captured activity to fold: the true empty
// state, distinct from `populated`'s one email entry.
const emptyMemory: View = {
  ...populated,
  conversation_memory: [],
  activities: { data: [], page },
};

// The other path through the card, which the thread fixtures above cannot
// reach. With no conversation_memory the card folds the captured activities.
// A retained email there draws the canonical row: mail icon, access badge,
// the server's preview, openable.
//
// Both access states, because one component draws them. A readable-only story
// would not show what a limited message looks like: the second row keeps its
// shape and loses its words. The note is the control: every other kind still
// reads as the card's own two lines.
const foldedActivities: View = {
  ...populated,
  conversation_memory: [],
  activities: {
    data: [
      {
        id: "a-40",
        kind: "email",
        occurred_at: "2026-08-29T09:15:00Z",
        subject: "Re: the renewal quote",
        body: "Can you hold the price until Friday? I have to take it past procurement first.",
        direction: "inbound",
        content_state: "available",
        source: "manual",
        captured_by: "human:u-1",
        created_at: "2026-08-29T09:15:00Z",
        updated_at: "2026-08-29T09:15:00Z",
        version: 4,
        is_done: false,
        email_summary: {
          activity_id: "a-40",
          occurred_at: "2026-08-29T09:15:00Z",
          version: 4,
          subject: "Re: the renewal quote",
          preview: "Can you hold the price until Friday?",
          counterparty: "Dana Buyer",
          direction: "inbound",
          display_status: "team",
          move: "needs_reply",
          attachment_count: 2,
        },
      },
      {
        id: "a-41",
        kind: "email",
        occurred_at: "2026-08-27T16:40:00Z",
        content_state: "withheld",
        source: "manual",
        captured_by: "human:u-2",
        created_at: "2026-08-27T16:40:00Z",
        updated_at: "2026-08-27T16:40:00Z",
        version: 2,
        is_done: false,
        email_summary: {
          activity_id: "a-41",
          occurred_at: "2026-08-27T16:40:00Z",
          version: 2,
          display_status: "withheld",
          move: "none",
          attachment_count: 0,
        },
      },
      {
        id: "a-42",
        kind: "note",
        occurred_at: "2026-08-26T11:00:00Z",
        subject: "Call prep",
        body: "Wants the fleet numbers before we talk pricing.",
        content_state: "available",
        source: "manual",
        captured_by: "human:u-1",
        created_at: "2026-08-26T11:00:00Z",
        updated_at: "2026-08-26T11:00:00Z",
        version: 1,
        is_done: false,
      },
    ],
    page,
  },
};

// The gaps `OverviewPanels` above leaves. The withheld deal card, and the one
// with a deal and a committee. Every commitments loop kind, including a done
// row. The What was said card's full channel set and its empty state.
export const OverviewGaps: Story = {
  render: () => {
    stubWithSession({}, {});
    return (
      <StoryProviders>
        <div className="record-stack" style={{ maxWidth: 720 }}>
          <ContactDealCard
            view={withheld}
            firstName="Dana"
            onOpenDeals={none}
          />
          <ContactDealCard
            view={dealWithCommittee}
            firstName="Dana"
            onOpenDeals={none}
          />
          <ContactCommitmentsCard
            view={richLoops}
            firstName="Dana"
            onAllTasks={none}
          />
          <ContactMemory
            view={richMemory}
            onOpenEmail={none}
            onFullHistory={none}
          />
          <ContactMemory
            view={foldedActivities}
            onOpenEmail={none}
            onFullHistory={none}
          />
          <ContactMemory
            view={emptyMemory}
            onOpenEmail={none}
            onFullHistory={none}
          />
        </div>
      </StoryProviders>
    );
  },
};
