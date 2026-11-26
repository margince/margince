// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { TimelineActions } from "./timelineactions";

type Activity = components["schemas"]["Activity"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const chat: Activity = {
  id: "chat-one",
  kind: "message",
  channel_provider: "whatsapp",
  occurred_at: "2026-09-01T09:00:00Z",
  occurred_precision: "instant",
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
  captured_by: "connector:whatsapp:owner",
  source: "whatsapp",
  is_done: false,
  offers_email: false,
  audience: "workspace",
  audience_reason: null,
  content_state: "available",
  thread_key: "chat-thread",
  mailbox_imported: true,
  thread_audience_yours: true,
  version: 1,
};

async function openMenu(activity: Activity, posts: unknown[] = []) {
  stubWithSession(
    {
      "POST /activities/threads/chat-thread/audience": (body) => {
        posts.push(body);
        return jsonResponse({ messages: 2, shared: false, held_by_others: 0 });
      },
    },
    { activity: ["read", "update"] },
  );
  render(
    <StoryProviders>
      <TimelineActions
        activity={activity}
        entityType="contact"
        entityId="contact-one"
      />
    </StoryProviders>,
  );
  const user = userEvent.setup();
  await user.click(
    await screen.findByRole("button", { name: /more actions/i }),
  );
  return user;
}

it("narrows a captured chat thread from its row", async () => {
  const posts: unknown[] = [];
  const user = await openMenu(chat, posts);
  await user.click(await screen.findByRole("button", { name: "Visibility" }));
  expect(
    screen.getByText(/whole conversation, not one message/i),
  ).toBeInTheDocument();
  await user.click(screen.getByLabelText(/only me and the people/i));
  await user.click(screen.getByRole("button", { name: "Save visibility" }));
  await vi.waitFor(() => expect(posts).toEqual([{ share: false }]));
});

it("offers no thread control on a chat row the caller did not import", async () => {
  await openMenu({ ...chat, thread_audience_yours: false });
  expect(screen.queryByRole("button", { name: "Visibility" })).toBeNull();
});

it("offers no thread control on an email", async () => {
  await openMenu({ ...chat, kind: "email" });
  expect(screen.queryByRole("button", { name: "Visibility" })).toBeNull();
});

it("keeps the per-message control on a connector row no import row backs", async () => {
  await openMenu({
    ...chat,
    mailbox_imported: false,
    thread_audience_yours: false,
    writable: true,
  });
  expect(
    screen.getByRole("button", { name: "Change visibility" }),
  ).toBeInTheDocument();
});
