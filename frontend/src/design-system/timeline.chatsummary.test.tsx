// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { browserZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
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

function renderRow(row: Activity) {
  const [entry] = activityTimeline([row], undefined);
  if (!entry) throw new Error("the adapter dropped the row");
  render(
    <LocaleProvider initial="en">
      <ul>
        <TimelineRow entry={entry} zone={browserZone()} />
      </ul>
    </LocaleProvider>,
  );
}

const episode = {
  message_count: 3,
  ended_at: "2026-11-12T09:10:00Z",
  url: "https://chat.example.com/channels/acme?seq=41",
};

afterEach(cleanup);

describe("a note the AI summarised from a chat channel", () => {
  it("says it is an AI summary from Dispact and links to the talk", () => {
    renderRow(note({ chat_episode: episode }));
    expect(screen.getByText(en["timeline.chatSummary.badge"])).toBeVisible();
    expect(screen.getByText("From Dispact")).toBeVisible();
    expect(screen.getByText(/3 messages, until/)).toBeVisible();
    expect(
      screen.getByRole("link", { name: /Open in Dispact/ }),
    ).toHaveAttribute("href", episode.url);
  });

  it("shows none of it on a note with no chat talk behind it", () => {
    renderRow(note({ source_system: null }));
    expect(screen.queryByText(en["timeline.chatSummary.badge"])).toBeNull();
    expect(screen.queryByRole("link", { name: /Open in Dispact/ })).toBeNull();
  });

  it("shows none of it when the reader may not read the note", () => {
    renderRow(note({ chat_episode: episode, content_state: "withheld" }));
    expect(screen.queryByText(en["timeline.chatSummary.badge"])).toBeNull();
  });
});
