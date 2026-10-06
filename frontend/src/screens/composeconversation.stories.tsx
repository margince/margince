// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { ComposeModal } from "./compose";
import { ConversationFold } from "./composeconversation";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

const MESSAGES: components["schemas"]["Activity"][] = [
  "Delivery window",
  "Re: Delivery window",
].map((subject, index) => ({
  id: `message-${index}`,
  kind: "email",
  subject,
  body: "Hello Ada,\n\nWe can deliver the first phase in September.\n\nBest regards,\nSam",
  direction: index === 0 ? "inbound" : "outbound",
  thread_key: "delivery",
  occurred_at: "2026-09-01T09:00:00Z",
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
  source: "manual",
  captured_by: "human:u1",
  is_done: false,
  content_state: "available",
}));

function Reply() {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /activities": () =>
      jsonResponse({ data: MESSAGES, page: { has_more: false } }),
    "GET /activities/message-0": () => jsonResponse(MESSAGES[0]),
    "GET /activities/message-0/reply-recipient": () =>
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
}

const meta: Meta<typeof ConversationFold> = {
  title: "Patterns/Compose mail/Conversation fold",
  component: ConversationFold,
};
export default meta;

type Story = StoryObj<typeof ConversationFold>;

/** A reading drawer wide enough for both: the thread beside the draft. */
export const Beside: Story = {
  parameters: {
    viewport: {
      options: {
        desktop: {
          name: "Desktop",
          styles: { width: "1024px", height: "720px" },
        },
      },
    },
  },
  globals: { viewport: { value: "desktop" } },
  render: () => <Reply />,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByRole("heading", { name: "This thread" });
    await expect(
      dialog.queryByRole("button", { name: "Show the conversation" }),
    ).toBeNull();
  },
};

/** A phone: the thread waits behind its toggle above the draft, opened here. */
export const Folded: Story = {
  render: () => <Reply />,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.click(
      await dialog.findByRole("button", { name: "Show the conversation" }),
    );
    await dialog.findByRole("heading", { name: "This thread" });
    await dialog.findByRole("button", { name: "Hide the conversation" });
  },
};
