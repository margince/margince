// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";

import type { components } from "../api/schema";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";
import { TimelineActions } from "./timelineactions";

// The verbs a reader gets on one timeline row, and which visibility control
// comes with them.
//
// The state worth a picture is the branch: captured mail offers a THREAD
// decision, because its audience is derived from what every importing mailbox
// asks for and a direct write is refused. Hand-logged mail offers the
// per-message dialog. They are different controls answering different
// questions, and the row has to pick the right one without asking the reader.

type Activity = components["schemas"]["Activity"];

const BASE: Activity = {
  id: "11111111-1111-4111-8111-111111111111",
  kind: "email",
  occurred_at: "2026-09-01T09:12:00Z",
  direction: "inbound",
  source: "manual",
  is_done: false,
  captured_by: "human:u1",
  created_at: "2026-09-01T09:12:00Z",
  updated_at: "2026-09-01T09:12:00Z",
  subject: "Angebot Q4 — Rückfragen",
  body: "Können wir Dienstag sprechen?",
  audience: "workspace",
  content_state: "available",
  version: 3,
};

// Capture writes `connector:<name>:<uuid>`, which is the one signal that says
// a mailbox brought this row in — a thread key alone would misread a rep's own
// threaded reply.
const CAPTURED: Activity = {
  ...BASE,
  id: "22222222-2222-4222-8222-222222222222",
  captured_by: "connector:gmail:u1",
  thread_key: "thread:abc",
  audience: "participants",
  audience_reason: "pending_verdict",
};

// A reader outside the audience sees the row and none of its words, so there
// is nothing to change and no control to offer.
const WITHHELD: Activity = {
  ...BASE,
  id: "33333333-3333-4333-8333-333333333333",
  subject: undefined,
  body: undefined,
  content_state: "withheld",
  audience: "participants",
};

function Frame({ activity }: Readonly<{ activity: Activity }>) {
  installFetchStub({
    "GET /me": meRoute({}),
  });
  return (
    <StoryProviders>
      <div
        style={{
          display: "flex",
          gap: "var(--space-2)",
          alignItems: "center",
        }}
      >
        <TimelineActions
          activity={activity}
          entityType="deal"
          entityId="d1"
          personId="p1"
        />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof Frame> = {
  title: "Records/Timeline actions",
  component: Frame,
};
export default meta;

type Story = StoryObj<typeof Frame>;

/** Hand-logged mail: the per-message visibility dialog. */
export const HandLogged: Story = { args: { activity: BASE } };

/** Captured mail: the thread decision, with the reason it is held. */
export const CapturedThread: Story = { args: { activity: CAPTURED } };

/** Outside the audience: the row keeps its verbs and loses the control. */
export const Withheld: Story = { args: { activity: WITHHELD } };
