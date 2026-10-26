// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";

import type { components } from "../api/schema";
import { StoryProviders } from "../testing/story-utils";
import { EmailThread } from "./emailthread";

// The opened message of a thread, once for each answer the server gives about
// how much of its text was read. Every note sits above the body; a complete
// body carries none.

type EmailPresentation = components["schemas"]["EmailPresentation"];
type BodyCompleteness = components["schemas"]["BodyCompleteness"];

const BASE: EmailPresentation = {
  id: "11111111-1111-4111-8111-111111111111",
  lifecycle: "delivered",
  occurred_at: "2026-09-01T09:12:00Z",
  occurred_precision: "instant",
  summary: {
    activity_id: "11111111-1111-4111-8111-111111111111",
    occurred_at: "2026-09-01T09:12:00Z",
    occurred_precision: "instant",
    display_status: "team",
    attachment_count: 0,
    move: "none",
    version: 3,
    subject: "Angebot Q4",
    direction: "inbound",
    counterparty: "Ana Sommer",
  },
  body: "Können wir Dienstag kurz sprechen? Die Laufzeit passt uns.\n\nViele Grüße\nAna Sommer",
  body_completeness: "complete",
  from: [{ address: "ana@brandt.example", display_name: "Ana Sommer" }],
  to: [{ address: "lars@margince.example", display_name: "Lars J." }],
  cc: [],
  bcc: [],
  bcc_withheld: false,
  attachments: [],
  links: [],
  thread: { members: [], next_cursor: null },
  access: {
    content_state: "available",
    display_status: "team",
    can_change: false,
    change_mode: "none",
  },
  can_reply: true,
  can_relink: false,
  version: 3,
};

const HEADER_ONLY = "From: ana@brandt.example\nTo: lars@margince.example";

function Thread({
  completeness,
  body,
}: Readonly<{ completeness: BodyCompleteness; body?: string }>) {
  return (
    <StoryProviders>
      <EmailThread
        presentation={{
          ...BASE,
          body: body ?? BASE.body,
          body_completeness: completeness,
        }}
        formatWhen={() => "1 Sep 09:12"}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof Thread> = {
  title: "Components/Text and data display/Email thread",
  component: Thread,
};
export default meta;

type Story = StoryObj<typeof Thread>;

/** Read in full: no note. */
export const Complete: Story = { args: { completeness: "complete" } };

/** A text part was not read in full; the start is shown. */
export const Partial: Story = {
  args: {
    completeness: "partial",
    body: `${HEADER_ONLY}\n\nKönnen wir Dienstag`,
  },
};

/** No text could be read; only the From and To lines are stored. */
export const Unreadable: Story = {
  args: { completeness: "unreadable", body: HEADER_ONLY },
};

/** The mail added nothing of its own; the text is the forwarded original's. */
export const Forwarded: Story = { args: { completeness: "forwarded" } };

/** Stored before Margince checked this, and not checked yet. */
export const Unassessed: Story = {
  args: { completeness: "unassessed", body: HEADER_ONLY },
};
