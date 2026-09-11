// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ContactMemory } from "./contactmemory";
import { StoryProviders } from "./story-utils";
import "./contact360.css";

// Conversation memory: the threads and meetings a contact is remembered by,
// with the five cuts through them.
//
// The cuts are what this file is for. They sat in the panel's head, where five
// options do not fit on the one band a head is — the strip wrapped, the head
// grew, and this card stood taller than every other card in the stack. They
// are a row in the BODY now, above the rows they narrow, and both frames below
// show that row: with rows under it, and with the empty sentence under it,
// because a filtered list that has come up empty is exactly when a reader
// needs the strip that got them there.
//
// The card is a GLANCE: three conversations, then the footer's way to the
// record's whole chronology. `Truncated` is the frame that shows both halves of
// that — what a talkative contact looks like here, and where the rest went.
//
// EVERY INSTANT IS FIXED. `make fe-clock-drift` runs the suite at +200 days,
// and a row dated relative to today would read as a meeting still to come.

type Contact360 = components["schemas"]["Contact360"];
type Activity = components["schemas"]["Activity"];

const AT = "2026-08-30T09:00:00Z";
const page = { has_more: false, next_cursor: null };

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: AT,
} as const;

const contact: components["schemas"]["Contact"] = {
  id: "3f7c1a90-0000-4000-8000-00000000c001",
  full_name: "Dana Buyer",
  first_name: "Dana",
  ...CAPTURED,
};

// A retained email arrives with the server's own row model beside it, and the
// card hands that to `EmailEntry` rather than drawing its own two lines — so
// the message reads here exactly as it does on the timeline. The preview and
// the body differ on purpose: the card shows the preview and the drawer holds
// the whole thing.
const email: Activity = {
  id: "01a05500-0000-7000-8000-00000000ee01",
  kind: "email",
  subject: "Re: the renewal quote",
  body:
    "Can you hold the price until Friday? " +
    "I have to take it past procurement first and they meet on Thursday.",
  direction: "inbound",
  content_state: "available",
  occurred_at: "2026-08-29T09:15:00Z",
  version: 4,
  is_done: false,
  email_summary: {
    activity_id: "01a05500-0000-7000-8000-00000000ee01",
    occurred_at: "2026-08-29T09:15:00Z",
    version: 4,
    subject: "Re: the renewal quote",
    preview: "Can you hold the price until Friday?",
    counterparty: "Dana Buyer",
    direction: "inbound",
    display_status: "team",
    move: "needs_reply",
    attachment_count: 0,
  },
  ...CAPTURED,
};

const meeting: Activity = {
  id: "01a05500-0000-7000-8000-00000000ee02",
  kind: "meeting",
  subject: "Depot walkthrough",
  body: "Walked the Hamburg depot with Dana and her workshop lead.",
  content_state: "available",
  occurred_at: "2026-08-27T08:00:00Z",
  version: 1,
  is_done: false,
  ...CAPTURED,
};

const note: Activity = {
  id: "01a05500-0000-7000-8000-00000000ee03",
  kind: "note",
  subject: "Call prep",
  body: "Wants the fleet numbers before we talk pricing.",
  content_state: "available",
  occurred_at: "2026-08-26T11:00:00Z",
  version: 1,
  is_done: false,
  ...CAPTURED,
};

// A contact who talks a lot: more captured than the card draws, which is the
// only way to see the cut and the footer under it.
const olderNotes: Activity[] = [
  "Left a voicemail about the depot slot",
  "Sent the fleet numbers over",
  "Asked for the renewal terms in writing",
].map((subject, index) => ({
  ...note,
  id: `01a05500-0000-7000-8000-0000000000${20 + index}`,
  subject,
  body: subject,
  occurred_at: `2026-08-${25 - index}T09:00:00Z`,
}));

function view(activities: readonly Activity[]): Contact360 {
  return {
    as_of: AT,
    contact,
    sections_omitted: [],
    last_outbound_at: "2026-08-30T07:00:00Z",
    activities: { data: [...activities], page },
  };
}

// The card sits in the record's right-hand pair, so it is drawn at that measure
// rather than at the page's: the strip and the title share a narrow column, and
// how much room the cuts have is the thing worth looking at here.
function card(activities: readonly Activity[]) {
  return () => (
    <StoryProviders>
      <div style={{ maxWidth: 560 }}>
        <ContactMemory view={view(activities)} onOpenEmail={() => {}} />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof ContactMemory> = {
  title: "Records/Contact record/Conversation memory",
  component: ContactMemory,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ContactMemory>;

/** Three kinds under the strip: a retained email, a meeting and a note. */
export const WithRows: Story = { render: card([email, meeting, note]) };

/**
 * More than the glance holds. Three conversations are drawn and the footer
 * carries the reader to the Timeline tab for the rest — the same place the
 * rail's recent-activity glance leads, so one record has one ledger.
 */
export const Truncated: Story = {
  render: card([email, meeting, note, ...olderNotes]),
};

/**
 * Nothing captured. The strip stays — it is how the reader got here, and a
 * card that took its cuts away when a cut came up empty would strand them.
 */
export const Empty: Story = { render: card([]) };

/**
 * Dark. The strip's track and its pressed option are a `--bgChip` pair tuned
 * per theme, and this is the first surface where a body-mounted row of them
 * has to hold its own against the panel's ground.
 */
export const WithRowsDark: Story = {
  ...WithRows,
  globals: { theme: "dark" },
};
