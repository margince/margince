// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { Panel } from "../../design-system/panel";
import { company360 } from "../company.fixtures";
import { StoryProviders } from "../story-utils";
import { RecordSpine } from "./spine";

// The account's story as a thread. The stories are the shapes a real account
// takes: one that has gone quiet, one in live conversation, one nobody has
// spoken to at all, and one with more history than the thread draws. The gap
// stop separates the first three, and it is the only stop drawn from an
// ABSENCE rather than from a record.

type View = components["schemas"]["Company360"];

const page = { has_more: false, next_cursor: null };
const AS_OF = "2026-08-25T09:00:00Z";

type Logged = NonNullable<View["activities"]>["data"][number];

// THE AUDIT FIELDS EVERY ACTIVITY ROW CARRIES ON THE WIRE, written once.
//
// They are identical in all eleven rows below and say nothing about the shape
// any one story exists to show, so spelling them per row buried the two or
// three fields that actually differ — which is the only thing a reader of a
// story is looking for.
const logged = (
  one: Omit<
    Logged,
    "is_done" | "source" | "captured_by" | "created_at" | "updated_at"
  >,
): Logged => ({
  ...one,
  is_done: true,
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-08-18T09:00:00Z",
  updated_at: "2026-08-18T09:00:00Z",
});

// One summary for the long-subject rows: they differ in the subject line and
// in nothing else, and the summary is not what that story is about.
const teamThread = {
  activity_id: "m-invoice",
  display_status: "team",
  occurred_at: "2026-08-18T09:00:00Z",
  attachment_count: 0,
  move: "none",
  version: 1,
} satisfies Logged["email_summary"];

const base: View = {
  ...company360,
  as_of: AS_OF,
  company: {
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
      logged({
        id: "a-1",
        kind: "meeting",
        direction: "outbound",
        subject: "Erstgespräch Matthias Ortner — Plaud-Transkript",
        occurred_at: "2026-08-18T09:00:00Z",
        links: [],
      }),
    ],
    page,
  },
  next_steps: { data: [], page },
};

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
const goneQuiet: View = {
  ...company360,
  ...base,
  last_outbound_at: "2026-08-18T09:00:00Z",
  last_inbound_at: null,
  health: { last_meeting_at: "2026-08-18T09:00:00Z", single_threaded: true },
  state_strip: {
    account: { status: "opportunity", relationship_types: [] },
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
};

export const GoneQuiet: Story = { render: () => <Card view={goneQuiet} /> };

// They wrote back after the meeting, so there is no silence to draw: a
// conversation in progress is not a gap, and drawing one would tell a reader
// to chase somebody who has already answered.
const inConversation: View = {
  ...company360,
  ...goneQuiet,
  last_inbound_at: "2026-08-24T09:00:00Z",
};

export const InConversation: Story = {
  render: () => <Card view={inConversation} />,
};

// Nothing has been said at all. The thread has only what is dated ahead, and
// no stop is drawn for a silence that has no conversation to start from.
const neverSpoken: View = {
  ...company360,
  ...base,
  last_outbound_at: null,
  last_inbound_at: null,
  activities: { data: [], page },
  state_strip: {
    account: { status: "target", relationship_types: [] },
    commercial: {
      open_count: 1,
      stalled_count: 0,
      priced_count: 0,
      converted_count: 0,
      next_close_on: "2026-10-20",
    },
  },
};

export const NeverSpoken: Story = { render: () => <Card view={neverSpoken} /> };

// An account with a past. Two conversations ran here before the invoice
// question, and the roll-up says how many the thread did not draw — the shape
// a reader who does not remember the account actually arrives at.
const withHistory: View = {
  ...company360,
  ...goneQuiet,
  activities: {
    data: [
      logged({
        id: "m-6",
        kind: "email",
        direction: "outbound",
        subject: "Re: Rechnung Position 3",
        occurred_at: "2026-08-18T09:00:00Z",
        thread_key: "t-invoice",
        links: [],
      }),
      logged({
        id: "m-5",
        kind: "email",
        direction: "inbound",
        subject: "Rechnung Position 3",
        occurred_at: "2026-08-16T09:00:00Z",
        thread_key: "t-invoice",
        links: [],
      }),
      logged({
        id: "m-4",
        kind: "meeting",
        direction: "outbound",
        subject: "Kickoff Kugellager-online.de",
        occurred_at: "2026-07-26T09:00:00Z",
        thread_key: "t-kickoff",
        links: [],
      }),
      logged({
        id: "m-3",
        kind: "email",
        direction: "inbound",
        subject: "Re: Kickoff Kugellager-online.de",
        occurred_at: "2026-07-21T09:00:00Z",
        thread_key: "t-kickoff",
        links: [],
      }),
      logged({
        id: "m-2",
        kind: "email",
        direction: "outbound",
        subject: "Angebot Lagerhaltung",
        occurred_at: "2026-06-30T09:00:00Z",
        thread_key: "t-quote",
        links: [],
      }),
      logged({
        id: "m-1",
        kind: "email",
        direction: "outbound",
        subject: "Erstkontakt über die Messe",
        occurred_at: "2026-06-02T09:00:00Z",
        thread_key: "t-intro",
        links: [],
      }),
    ],
    page,
  },
};

export const WithHistory: Story = { render: () => <Card view={withHistory} /> };

// Real mail, which does not write subject lines to fit a column: a receipt
// carrying a tracking reference with no space in it, and an invoice subject
// three times the width of the stop it belongs to. Every stop stays inside its
// own stretch of the axis — the state this exists to hold, because a subject
// that escaped its column printed straight across the stops beside it and the
// thread became one smear of overlapping words.
const longSubjects: View = {
  ...company360,
  ...goneQuiet,
  activities: {
    data: [
      logged({
        id: "m-3",
        kind: "email",
        direction: "inbound",
        subject:
          "Apartment Management Services — Invoice for June, July and August 2026",
        occurred_at: "2026-08-18T09:00:00Z",
        thread_key: "t-invoice",
        email_summary: teamThread,
        links: [],
      }),
      logged({
        id: "m-2",
        kind: "email",
        direction: "inbound",
        subject: "YoSC-eSNpTHBr7_A0H0t_p16FBCWDD5389B22k64593Hmlet",
        occurred_at: "2026-08-14T09:00:00Z",
        thread_key: "t-receipt",
        email_summary: teamThread,
        links: [],
      }),
      logged({
        id: "m-1",
        kind: "email",
        direction: "inbound",
        subject: "Receipt for Lars Jankowsky (Re:Fly)",
        occurred_at: "2026-08-06T09:00:00Z",
        thread_key: "t-fly",
        email_summary: teamThread,
        links: [],
      }),
    ],
    page: { has_more: true, next_cursor: null },
  },
};

export const LongSubjects: Story = {
  render: () => <Card view={longSubjects} onOpenEmail={() => {}} />,
};

const meta: Meta<typeof RecordSpine> = {
  title: "Records/Company 360/Spine",
  component: RecordSpine,
};
export default meta;
type Story = StoryObj<typeof RecordSpine>;
