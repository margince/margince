/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { activityTimeline } from "./activitytimeline";
import { GroupedTimelineList } from "./timeline";
import { sideOf, threadSides } from "./timelineentry";

// History names who WROTE a message by the shared rule (lib/messagevoice.ts
// correspondenceSide, the twin of activities.WroteItOurSide), never by raw
// transport direction. These rows go through the production adapter,
// so a regression to `entry.direction` as authorship fails here.

type Activity = components["schemas"]["Activity"];
type Author = components["schemas"]["ActivityAuthor"];

afterEach(cleanup);

const render = (ui: ReactNode) =>
  rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);

const casey: Author = {
  kind: "contact_record",
  display_name: "Casey Example",
  address: "casey@customer.example",
  is_reader: false,
};
const alex: Author = {
  kind: "colleague",
  display_name: "Alex Seller",
  address: "alex@ours.example",
  is_reader: false,
};

function mail(
  id: string,
  at: string,
  direction: Activity["direction"],
  author: Author,
  contactRole?: NonNullable<Activity["voice"]>["contact_role"],
): Activity {
  return {
    id,
    kind: "email",
    subject: "Renewal",
    body: `Message ${id}`,
    direction,
    thread_key: "renewal-thread",
    is_done: false,
    offers_email: false,
    occurred_at: at,
    occurred_precision: "instant",
    source: "gmail",
    captured_by: "connector:gmail:00000000-0000-4000-8000-000000000001",
    created_at: at,
    updated_at: at,
    voice: { author, contact_role: contactRole },
  };
}

// Two colleague-authored messages and one contact-authored reply. The legacy
// transport directions conflict with the author on two of the three rows: the
// colleague's inbound copy and Casey's reply stored as outbound.
const thread: Activity[] = [
  mail("reply", "2026-10-30T10:00:00Z", "outbound", casey, "author"),
  mail("copy", "2026-10-20T10:00:00Z", "inbound", alex, "cc"),
  mail("first", "2026-10-04T10:00:00Z", "outbound", alex, "addressee"),
];

describe("History authorship", () => {
  it("credits each message to its author, not to its transport direction", () => {
    const entries = activityTimeline(thread);
    expect(entries.map(sideOf)).toEqual(["theirs", "ours", "ours"]);
    expect(threadSides(entries)).toEqual({ theirs: 1, ours: 2 });
  });

  it("never credits a contact who was only copied as the sender", () => {
    // Casey is on the colleague's mail as Cc. The author is the colleague.
    const [copy] = activityTimeline([
      mail("cc-only", "2026-10-20T10:00:00Z", "inbound", alex, "cc"),
    ]);
    expect(copy && sideOf(copy)).toBe("ours");
  });

  it("falls back to transport direction only when no author names a side", () => {
    const unresolved: Author = { kind: "unresolved", is_reader: false };
    const [inbound] = activityTimeline([
      mail("u", "2026-10-20T10:00:00Z", "inbound", unresolved),
    ]);
    expect(inbound && sideOf(inbound)).toBe("theirs");
    // A row with no direction at all claims no side.
    const [bare] = activityTimeline([
      mail("n", "2026-10-20T10:00:00Z", undefined, casey),
    ]);
    expect(bare && sideOf(bare)).toBeUndefined();
  });

  it("counts the thread card's sides from the same rule", () => {
    const entries = activityTimeline(thread);
    render(
      <GroupedTimelineList
        zone="UTC"
        groups={[
          {
            id: "renewal",
            kind: "thread",
            entries,
            partial: false,
          },
        ]}
      />,
    );
    expect(
      screen.getByText("3 messages · 1 from them, 2 from your team"),
    ).toBeTruthy();
  });
});
