// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { EmailAccessEditor } from "./emailaccesseditor";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Who reads this message, under its subject, and the verb that changes it.
//
// What these pictures are for is the LINE: the mark, the reason a captured
// message is held, and the verb immediately after them. The verb is the text
// affordance rather than a filled box, because the fact is what the line is
// about and a bordered control beside a badge reads as the louder of the two.
//
// The server decides which write is on offer (`change_mode`) and whether this
// reader may make it (`can_change`), so the states below are access blocks
// rather than roles: nothing here re-derives the control from the row.

type EmailPresentation = components["schemas"]["EmailPresentation"];
type EmailAccess = components["schemas"]["EmailAccess"];

const meta: Meta = {
  title: "Records/Email box/Message access",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const ACTIVITY = "01a05500-0000-7000-8000-0000000000a1";
const ANA = "01a05500-0000-7000-8000-0000000000c1";
const BAO = "01a05500-0000-7000-8000-0000000000c2";

const page = { has_more: false, next_cursor: null };

const roster = [
  { id: ANA, display_name: "Ana Fischer", email: "ana@demo.test" },
  { id: BAO, display_name: "Bao Nguyen", email: "bao@demo.test" },
];

/**
 * One presentation, with the access block the story is about.
 *
 * Typed as the generated `EmailPresentation` for the reason the unit tests give
 * it the same treatment: a fixture that drifts from the contract fails the
 * build instead of drawing a shape the server never sends.
 */
function presentation(access: Partial<EmailAccess>): EmailPresentation {
  return {
    id: ACTIVITY,
    lifecycle: "delivered",
    occurred_at: "2026-09-01T09:15:00Z",
    summary: {
      activity_id: ACTIVITY,
      occurred_at: "2026-09-01T09:15:00Z",
      version: 3,
      subject: "Re: Meeting Drafts/Links",
      preview: "Attached, as agreed.",
      display_status: "team",
      move: "none",
      attachment_count: 0,
    },
    body: "Attached, as agreed.",
    thread_key: "t1",
    from: [],
    to: [],
    cc: [],
    bcc: [],
    bcc_withheld: false,
    attachments: [],
    links: [],
    thread: { members: [], next_cursor: null },
    can_reply: true,
    can_relink: false,
    version: 3,
    access: {
      content_state: "available",
      display_status: "team",
      audience: "workspace",
      can_change: false,
      change_mode: "none",
      ...access,
    },
  };
}

function Access({ access }: Readonly<{ access: Partial<EmailAccess> }>) {
  // The roster is a paginated walk behind `useAudienceCandidates`, and only the
  // named-members story reaches it. Routing both pages here rather than per
  // story keeps every picture reading off one set of seats.
  installFetchStub({
    "GET /me": meRoute({ activity: ["update"] }),
    "GET /users": () => jsonResponse({ data: roster, page }),
    "GET /teams": () => jsonResponse({ data: [], page }),
  });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 620 }}>
        <EmailAccessEditor presentation={presentation(access)} />
      </div>
    </StoryProviders>
  );
}

/**
 * A captured thread this reader has shared with the company, which is the
 * drawer's most common heading. "Make private" sits immediately after the word
 * it flips, and the caption under it says what the press REACHES — this
 * reader's side of the whole thread, not this one message.
 */
export const SharedThread: Story = {
  render: () => (
    <Access
      access={{
        display_status: "team",
        audience: "workspace",
        can_change: true,
        change_mode: "thread_contribution",
        change_scope: "thread",
      }}
    />
  ),
};

/**
 * The same thread held to its participants: the mark is filled, the verb names
 * where the message GOES rather than where it is, and the reason the server
 * gave stays on the badge's side of the line.
 */
export const HeldThread: Story = {
  render: () => (
    <Access
      access={{
        display_status: "participants",
        audience: "participants",
        explanation: "explicitly_confidential",
        can_change: true,
        change_mode: "thread_contribution",
        change_scope: "thread",
      }}
    />
  ),
};

/**
 * A hand-logged message, whose audience somebody set and which takes the direct
 * write. One control in the same slot, because the reader is answering one
 * question — and the set already on the message is named under it.
 */
export const HandLoggedSet: Story = {
  render: () => (
    <Access
      access={{
        display_status: "selected",
        audience: "selected",
        selected_members: [
          { subject_type: "user", subject_id: ANA },
          { subject_type: "user", subject_id: BAO },
        ],
        can_change: true,
        change_mode: "message_audience",
      }}
    />
  ),
};

/**
 * A reader with no standing to widen it. The mark and the sentence are still
 * drawn — who reads a message is a fact about it, like its date — and no empty
 * slot is left where the verb would be.
 */
export const NothingToPress: Story = {
  render: () => (
    <Access
      access={{
        display_status: "participants",
        audience: "participants",
        can_change: false,
        change_mode: "none",
      }}
    />
  ),
};
