// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
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
const meta: Meta = { title: "Patterns/Composer conversation" };
export default meta;
export const SelectAndPreview: StoryObj = { render: () => <Conversation /> };
export const SelectedReply: StoryObj = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /activities": () =>
        jsonResponse({ data: MESSAGES, page: { has_more: false } }),
      "GET /activities/message-0": () => jsonResponse(MESSAGES[0]),
      "GET /activities/message-1": () => jsonResponse(MESSAGES[1]),
      "GET /activities/message-0/reply-recipient": () =>
        jsonResponse({ address: "ada@example.test", mailbox_user_ids: [] }),
      "GET /activities/message-1/reply-recipient": () =>
        jsonResponse({ address: "ada@example.test", mailbox_user_ids: [] }),
    });
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
  },
};
