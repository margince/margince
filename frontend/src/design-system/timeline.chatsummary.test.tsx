// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { browserZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  refuseUnrouted,
} from "../testing/story-utils";
import { activityTimeline } from "./activitytimeline";
import { TimelineRow } from "./timeline";

type Activity = components["schemas"]["Activity"];

function note(fields: Partial<Activity>): Activity {
  return {
    id: "a-note",
    kind: "note",
    subject: "Discount agreed",
    body: "The team agreed 12% if Acme signs this week.",
    is_done: false,
    offers_email: false,
    occurred_at: "2026-11-12T09:00:00Z",
    occurred_precision: "instant",
    source: "chat_links",
    source_system: "dispact",
    captured_by: "system:chat_links",
    created_at: "2026-11-12T09:40:00Z",
    updated_at: "2026-11-12T09:40:00Z",
    ...fields,
  };
}

const me = meFixture({ allow: { activity: ["read"] } });
function sourceStub(available = true, user = me.user.id) {
  const source = vi.fn(() =>
    jsonResponse({
      available,
      user_id: user,
      url: available ? "https://chat.example.test/verified-root" : undefined,
      valid_until: "2099-01-01T00:00:00Z",
    }),
  );
  installFetchStub(
    {
      "GET /me": meRoute({ activity: ["read"] }),
      "GET /activities/a-note/chat-source": source,
    },
    refuseUnrouted,
  );
  return source;
}
function renderRow(row: Activity) {
  const [entry] = activityTimeline([row], undefined);
  if (!entry) throw new Error("the adapter dropped the row");
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ul>
          <TimelineRow entry={entry} zone={browserZone()} />
        </ul>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const episode = {
  channel_name: "Berlin pilot",
  message_count: 3,
  ended_at: "2026-11-12T09:10:00Z",
  url: "https://chat.example.com/channels/acme",
};

afterEach(cleanup);

describe("a note the AI summarised from a chat channel", () => {
  it("marks preserved human text whose source changed and shows its update time", () => {
    sourceStub();
    renderRow(note({ chat_episode: { ...episode, source_changed: true } }));
    expect(
      screen.getByText(en["timeline.chatSummary.sourceChanged"]),
    ).toBeVisible();
    expect(screen.getByText(/Updated /)).toBeVisible();
    expect(
      screen.getByText("The team agreed 12% if Acme signs this week."),
    ).toBeVisible();
  });

  it("opens a source only after verifying the current reader", async () => {
    const source = sourceStub();
    renderRow(note({ chat_episode: episode }));
    expect(screen.getByText(en["timeline.chatSummary.badge"])).toBeVisible();
    expect(screen.getByText("From Dispact")).toBeVisible();
    expect(screen.getByText(/Berlin pilot/)).toBeVisible();
    expect(screen.getByText(/3 messages, until/)).toBeVisible();
    expect(source).not.toHaveBeenCalled();
    expect(screen.queryByRole("link", { name: /Open in Dispact/ })).toBeNull();
    const check = screen.getByRole("button", {
      name: en["timeline.chatSummary.sourceCheck"],
    });
    await waitFor(() => expect(check).toBeEnabled());
    fireEvent.click(check);
    expect(
      await screen.findByRole("link", { name: /Open in Dispact/ }),
    ).toHaveAttribute("href", "https://chat.example.test/verified-root");
    expect(source).toHaveBeenCalledTimes(1);
  });

  it.each(["slack", "teams"] as const)(
    "names %s from the verified recap source",
    (provider) => {
      sourceStub();
      renderRow(note({ chat_episode: { ...episode, provider } }));
      const name = provider === "slack" ? "Slack" : "Teams";
      expect(screen.getByText(`From ${name}`)).toBeVisible();
      expect(
        screen.queryByRole("link", { name: `Open in ${name}` }),
      ).toBeNull();
      expect(screen.queryByText("From Dispact")).toBeNull();
    },
  );

  it("shows none of it on a note with no chat talk behind it", () => {
    renderRow(note({ source_system: null }));
    expect(screen.queryByText(en["timeline.chatSummary.badge"])).toBeNull();
    expect(screen.queryByRole("link", { name: /Open in Dispact/ })).toBeNull();
  });

  it("shows none of it when the reader may not read the note", () => {
    const source = sourceStub();
    renderRow(note({ chat_episode: episode, content_state: "withheld" }));
    expect(screen.queryByText(en["timeline.chatSummary.badge"])).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
    expect(source).not.toHaveBeenCalled();
  });
  it.each([false, true])(
    "withholds unavailable or another reader's source (available=%s)",
    async (available) => {
      sourceStub(available, available ? "another-reader" : me.user.id);
      renderRow(note({ chat_episode: episode }));
      const check = screen.getByRole("button", {
        name: en["timeline.chatSummary.sourceCheck"],
      });
      await waitFor(() => expect(check).toBeEnabled());
      fireEvent.click(check);
      if (!available)
        expect(
          await screen.findByText(en["timeline.chatSummary.sourceUnavailable"]),
        ).toBeVisible();
      else await waitFor(() => expect(check).toBeEnabled());
      expect(screen.queryByRole("link")).toBeNull();
    },
  );
});
