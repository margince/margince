// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { ComposeModal } from "./compose";
import { ThreadPane } from "./composethread";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

const MESSAGES: components["schemas"]["Activity"][] = [
  "Delivery window",
  "Re: Pricing proposal",
].map((subject, index) => ({
  id: `message-${index}`,
  kind: "email",
  subject,
  body: "Hello Ada,\n\nWe can deliver the first phase in September. The second phase depends on your team's availability.\n\nPlease confirm which week works for you.\n\nBest regards,\nSam",
  direction: index === 0 ? "inbound" : "outbound",
  thread_key: "delivery",
  occurred_at: "2026-09-01T09:00:00Z",
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
  source: "manual",
  captured_by: "human:u1",
  is_done: false,
  content_state: "available",
  email_summary: {
    activity_id: `message-${index}`,
    subject,
    preview: "We can deliver the first phase in September…",
    direction: index === 0 ? "inbound" : "outbound",
    occurred_at: "2026-09-01T09:00:00Z",
    counterparty: "Ada Sommer",
    display_status: "team",
    move: "needs_reply",
    attachment_count: 0,
    version: 1,
  },
}));

function Conversation() {
  const [selected, setSelected] = useState("message-0");
  return (
    <StoryProviders>
      <ThreadPane
        messages={MESSAGES}
        selectedId={selected}
        onSelect={setSelected}
        pending={false}
        failed={false}
        onRetry={() => {}}
        nameOf={() => undefined}
        named
      />
    </StoryProviders>
  );
}
const meta: Meta = { title: "Patterns/Compose mail/Conversation" };
export default meta;
export const SelectAndPreview: StoryObj = { render: () => <Conversation /> };
function selectedReplyRoutes() {
  return {
    "GET /me": meRoute({}),
    "GET /activities": () =>
      jsonResponse({ data: MESSAGES, page: { has_more: false } }),
    "GET /activities/message-0": () => jsonResponse(MESSAGES[0]),
    "GET /activities/message-1": () => jsonResponse(MESSAGES[1]),
    "GET /activities/message-0/reply-recipient": () =>
      jsonResponse({ address: "ada@example.test", mailbox_user_ids: [] }),
    "GET /activities/message-1/reply-recipient": () =>
      jsonResponse({ address: "ada@example.test", mailbox_user_ids: [] }),
  };
}

function SelectedReplyComposer() {
  return (
    <StoryProviders>
      <ComposeModal
        activityId="message-0"
        entityType="contact"
        entityId="ada"
        open
        onClose={() => {}}
      />
    </StoryProviders>
  );
}

export const SelectedReply: StoryObj = {
  render: () => {
    installFetchStub(selectedReplyRoutes());
    return <SelectedReplyComposer />;
  },
};

// A machine-written reply beside the thread it answers. The form column is a
// height-bounded scroller here, and the AI card clips its own overflow — so
// the frame shows the card whole, the steer and its verb included, rather
// than squeezed to its head while the column around it has room to scroll.
export const DraftedBesideThread: StoryObj = {
  render: () => {
    installFetchStub({
      ...selectedReplyRoutes(),
      "POST /activities/message-0/draft-email": () =>
        jsonResponse({
          subject: "Re: Delivery window",
          body: "Thanks, Ada — the week of 14 September works for us.",
          to: ["ada@example.test"],
          ai_generated: true,
          ai_disclosure: "Drafted with AI assistance. Review before sending.",
          voice_profile_version: null,
          draft_ref: null,
        }),
    });
    return <SelectedReplyComposer />;
  },
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.click(
      await dialog.findByRole("button", { name: "Draft reply with AI" }),
    );
    const title = await dialog.findByText("AI-assisted draft");
    // Geometry, not toBeVisible: a control clipped by its card's overflow is
    // still "visible" to the DOM, so only its box can say it is on screen.
    const card = title.closest(".panel")?.getBoundingClientRect();
    const verb = dialog
      .getByRole("button", { name: "Draft reply with AI" })
      .getBoundingClientRect();
    await expect(card?.bottom).toBeGreaterThanOrEqual(verb.bottom);
  },
};
