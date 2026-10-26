// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ComposeModal } from "./compose";
import { SelectedReply } from "./composethread.stories";
import { ThreadBar } from "./composethreadbar";

const meta: Meta = {
  title: "Patterns/Compose mail/Thread bar",
  component: ThreadBar,
};
export default meta;

/** Replying in a thread: the bar names it, and opens the record's threads to
 *  move the draft onto another one or out to a new email. */
export const Change: StoryObj = {
  render: SelectedReply.render,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.click(
      await dialog.findByRole("button", { name: /Change/ }),
    );
    await screen.findByRole("button", { name: "New email" });
  },
};

// A record with a long history: twelve threads on the first page, more behind.
const LONG: components["schemas"]["Activity"][] = Array.from(
  { length: 12 },
  (_, index) => {
    const id = `long-${index}`;
    const subject = `Weekly call ${index + 1}: planning, budget and next steps`;
    const at = `2026-09-${String(28 - index).padStart(2, "0")}T09:00:00Z`;
    return {
      id,
      kind: "email",
      offers_email: false,
      subject,
      body: "Agenda and notes for this week's call.",
      direction: "inbound",
      thread_key: id,
      occurred_at: at,
      created_at: at,
      updated_at: at,
      source: "manual",
      captured_by: "human:u1",
      is_done: false,
      content_state: "available",
      email_summary: {
        activity_id: id,
        subject,
        preview:
          "Agenda and notes for this week's call, with the budget figures.",
        direction: "inbound",
        occurred_at: at,
        counterparty: "Ada Sommer",
        display_status: "team",
        move: index === 0 ? "needs_reply" : "none",
        attachment_count: index % 3,
        version: 1,
      },
    };
  },
);

/** A long record: the list scrolls inside the menu, and the menu itself
 *  scrolls no further than its last control. */
export const LongRecord: StoryObj = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /activities": () =>
        jsonResponse({
          data: LONG,
          page: { has_more: true, next_cursor: "older" },
        }),
    });
    return (
      <StoryProviders>
        <ComposeModal
          entityType="contact"
          entityId="ada"
          open
          onClose={() => undefined}
        />
      </StoryProviders>
    );
  },
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.click(
      await dialog.findByRole("button", { name: /Continue a thread$/ }),
    );
    await screen.findByRole("button", { name: "Show more" });
    const menu = document.querySelector<HTMLElement>(".compose-threadmenu");
    const list = document.querySelector<HTMLElement>(".compose-threadlist");
    // One scroller: the list. The menu has nothing past its last control.
    await expect(
      menu && menu.scrollHeight - menu.clientHeight,
    ).toBeLessThanOrEqual(1);
    await expect(list && list.scrollHeight > list.clientHeight).toBe(true);
  },
};
