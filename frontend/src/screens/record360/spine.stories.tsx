// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { Panel } from "../../design-system/panel";
import { StoryProviders } from "../story-utils";
import { RecordSpine } from "./spine";

// The account's story as a thread. The stories are the shapes a real account
// takes: one that has gone quiet, one in live conversation, one nobody has
// spoken to at all, and one with more history than the thread draws. The gap
// stop separates the first three, and it is the only stop drawn from an
// ABSENCE rather than from a record.

type View = components["schemas"]["Organization360"];

const page = { has_more: false, next_cursor: null };
const AS_OF = "2026-08-25T09:00:00Z";

const base = {
  as_of: AS_OF,
  organization: {
    id: "o-1",
    display_name: "Kugellager-online.de",
    captured_by: "human:u1",
    source: "manual",
    version: 1,
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
  },
  sections_omitted: [],
  activities: {
    data: [
      {
        id: "a-1",
        kind: "meeting",
        direction: "outbound",
        subject: "Erstgespräch Matthias Ortner — Plaud-Transkript",
        occurred_at: "2026-08-18T09:00:00Z",
        links: [],
      },
    ],
    page,
  },
  next_steps: { data: [], page },
} as unknown as View;

function Card({
  view,
  onOpenEmail,
}: Readonly<{ view: View; onOpenEmail?: (activityId: string) => void }>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <Panel title="Company 360" tone="accent">
          <RecordSpine
            source={view}
            commercial={view?.state_strip?.commercial}
            onOpenEmail={onOpenEmail}
          />
        </Panel>
      </div>
    </StoryProviders>
  );
}

// The shape this component exists for: one conversation, then nothing. The
// day count is the largest thing on the card because the silence is the fact
// a reader must not skim past.
const goneQuiet = {
  ...base,
  last_outbound_at: "2026-08-18T09:00:00Z",
  last_inbound_at: null,
  health: { last_meeting_at: "2026-08-18T09:00:00Z", single_threaded: true },
  state_strip: {
    account: { lifecycle: "opportunity", relationship_types: [] },
    commercial: {
      open_count: 1,
      stalled_count: 0,
      priced_count: 1,
      converted_count: 0,
      open_pipeline_minor_base: 4_800_000,
      base_currency: "EUR",
      next_close_on: "2026-10-20",
    },
  },
  next_steps: {
    data: [
      {
        activity_id: "s-1",
        subject: "NDA finalisieren und unterzeichnen",
        due_at: "2026-09-01T09:00:00Z",
        overdue: false,
      },
      {
        activity_id: "s-2",
        subject: "Rainer auf Matthias Ortner ansetzen",
        due_at: "2026-08-20T09:00:00Z",
        overdue: true,
      },
    ],
    page,
  },
} as unknown as View;

export const GoneQuiet: Story = { render: () => <Card view={goneQuiet} /> };

// They wrote back after the meeting, so there is no silence to draw: a
// conversation in progress is not a gap, and drawing one would tell a reader
// to chase somebody who has already answered.
const inConversation = {
  ...goneQuiet,
  last_inbound_at: "2026-08-24T09:00:00Z",
} as unknown as View;

export const InConversation: Story = {
  render: () => <Card view={inConversation} />,
};

// Nothing has been said at all. The thread has only what is dated ahead, and
// no stop is drawn for a silence that has no conversation to start from.
const neverSpoken = {
  ...base,
  last_outbound_at: null,
  last_inbound_at: null,
  activities: { data: [], page },
  state_strip: {
    account: { lifecycle: "target", relationship_types: [] },
    commercial: {
      open_count: 1,
      stalled_count: 0,
      priced_count: 0,
      converted_count: 0,
      next_close_on: "2026-10-20",
    },
  },
} as unknown as View;

export const NeverSpoken: Story = { render: () => <Card view={neverSpoken} /> };

// An account with a past. Two conversations ran here before the invoice
// question, and the roll-up says how many the thread did not draw — the shape
// a reader who does not remember the account actually arrives at.
const withHistory = {
  ...goneQuiet,
  activities: {
    data: [
      {
        id: "m-6",
        kind: "email",
        direction: "outbound",
        subject: "Re: Rechnung Position 3",
        occurred_at: "2026-08-18T09:00:00Z",
        thread_key: "t-invoice",
        links: [],
      },
      {
        id: "m-5",
        kind: "email",
        direction: "inbound",
        subject: "Rechnung Position 3",
        occurred_at: "2026-08-16T09:00:00Z",
        thread_key: "t-invoice",
        links: [],
      },
      {
        id: "m-4",
        kind: "meeting",
        direction: "outbound",
        subject: "Kickoff Kugellager-online.de",
        occurred_at: "2026-07-26T09:00:00Z",
        thread_key: "t-kickoff",
        links: [],
      },
      {
        id: "m-3",
        kind: "email",
        direction: "inbound",
        subject: "Re: Kickoff Kugellager-online.de",
        occurred_at: "2026-07-21T09:00:00Z",
        thread_key: "t-kickoff",
        links: [],
      },
      {
        id: "m-2",
        kind: "email",
        direction: "outbound",
        subject: "Angebot Lagerhaltung",
        occurred_at: "2026-06-30T09:00:00Z",
        thread_key: "t-quote",
        links: [],
      },
      {
        id: "m-1",
        kind: "email",
        direction: "outbound",
        subject: "Erstkontakt über die Messe",
        occurred_at: "2026-06-02T09:00:00Z",
        thread_key: "t-intro",
        links: [],
      },
    ],
    page,
  },
} as unknown as View;

export const WithHistory: Story = { render: () => <Card view={withHistory} /> };

// Real mail, which does not write subject lines to fit a column: a receipt
// carrying a tracking reference with no space in it, and an invoice subject
// three times the width of the stop it belongs to. Every stop stays inside its
// own stretch of the axis — the state this exists to hold, because a subject
// that escaped its column printed straight across the stops beside it and the
// thread became one smear of overlapping words.
const longSubjects = {
  ...goneQuiet,
  activities: {
    data: [
      {
        id: "m-3",
        kind: "email",
        direction: "inbound",
        subject:
          "Apartment Management Services — Invoice for June, July and August 2026",
        occurred_at: "2026-08-18T09:00:00Z",
        thread_key: "t-invoice",
        email_summary: { display_status: "visible" },
        links: [],
      },
      {
        id: "m-2",
        kind: "email",
        direction: "inbound",
        subject: "YoSC-eSNpTHBr7_A0H0t_p16FBCWDD5389B22k64593Hmlet",
        occurred_at: "2026-08-14T09:00:00Z",
        thread_key: "t-receipt",
        email_summary: { display_status: "visible" },
        links: [],
      },
      {
        id: "m-1",
        kind: "email",
        direction: "inbound",
        subject: "Receipt for Lars Jankowsky (Re:Fly)",
        occurred_at: "2026-08-06T09:00:00Z",
        thread_key: "t-fly",
        email_summary: { display_status: "visible" },
        links: [],
      },
    ],
    page: { has_more: true, next_cursor: null },
  },
} as unknown as View;

export const LongSubjects: Story = {
  render: () => <Card view={longSubjects} onOpenEmail={() => {}} />,
};

const meta: Meta<typeof RecordSpine> = {
  title: "Records/Company 360/Spine",
  component: RecordSpine,
};
export default meta;
type Story = StoryObj<typeof RecordSpine>;
