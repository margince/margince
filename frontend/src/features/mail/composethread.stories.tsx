// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ComposeModal } from "./compose";
import { ConversationRail } from "./composerail";

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
  const [selected, setSelected] = useState<string | undefined>("message-0");
  const anchor = MESSAGES.find((message) => message.id === selected);
  return (
    <StoryProviders>
      <ConversationRail
        recordName="Ada Sommer"
        conversations={MESSAGES.map((message) => ({
          anchorId: message.id ?? "",
          subject: message.subject ?? "",
          atIso: message.occurred_at ?? "",
          count: 1,
          partial: false,
          summary: message.email_summary ?? undefined,
          memberIds: [message.id ?? ""],
        }))}
        pending={false}
        failed={false}
        onRetry={() => {}}
        answering={selected}
        anchor={anchor}
        anchorPending={false}
        anchorFailed={false}
        onRetryAnchor={() => {}}
        thread={{
          messages: anchor ? [anchor] : [],
          pending: false,
          failed: false,
          retry: () => {},
        }}
        viewerUserId={undefined}
        nameOf={() => undefined}
        onChoose={setSelected}
        onLeave={() => setSelected(undefined)}
        disabled={false}
      />
    </StoryProviders>
  );
}
const meta: Meta = {
  title: "Patterns/Compose mail/Conversation",
  excludeStories: ["selectedReplyRoutes"],
};
export default meta;
export const SelectAndPreview: StoryObj = { render: () => <Conversation /> };
export function selectedReplyRoutes() {
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

// A long draft keeps room for writing while the thread and send controls stay available.
export const DraftedBesideThread: StoryObj = {
  render: () => {
    installFetchStub({
      ...selectedReplyRoutes(),
      "POST /activities/message-0/draft-email": () =>
        jsonResponse({
          subject: "Re: Delivery window",
          body: [
            "Hi Ada,",
            "The week of 14 September works for us. We can start with the planning session on Monday and review the first delivery together on Friday.",
            "For the planning session, please bring the current requirements and a list of reviewers. We will bring the delivery plan and a first estimate for each phase.",
            "Before we confirm the second phase, we should agree on the review dates and who can approve any changes to the scope.",
            "Could you confirm whether Monday morning suits your team?",
          ].join("\n\n"),
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
      await dialog.findByRole("button", { name: "Draft with AI" }),
    );
    await userEvent.click(
      await dialog.findByRole("button", { name: "Draft reply" }),
    );
    await dialog.findByText("AI-assisted draft");
    const editor = dialog.getByRole("textbox", { name: "Body" });
    editor.scrollIntoView({ block: "nearest" });
    const body = editor.getBoundingClientRect();
    const send = dialog
      .getByRole("button", { name: "Send" })
      .getBoundingClientRect();
    await expect(body.height).toBeGreaterThanOrEqual(
      window.innerHeight <= 512 ? 60 : 280,
    );
    await expect(body.top).toBeGreaterThanOrEqual(0);
    const toolbar = dialog
      .getByRole("button", { name: "Bold" })
      .getBoundingClientRect();
    await expect(toolbar.bottom).toBeLessThanOrEqual(body.top);
    await expect(body.bottom).toBeLessThanOrEqual(send.top);
    await expect(send.bottom).toBeLessThanOrEqual(window.innerHeight);
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
      window.innerWidth,
    );
  },
};

export const DraftedBesideThreadDark: StoryObj = {
  ...DraftedBesideThread,
  globals: { theme: "dark" },
};

export const DraftedOnPhone: StoryObj = {
  ...DraftedBesideThread,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const DraftedOnLandscapePhone: StoryObj = {
  ...DraftedBesideThread,
  globals: { viewport: { value: "phoneOnItsSide" } },
  tags: ["uat-phone-on-its-side"],
};
