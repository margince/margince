// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  type CalendarChange,
  changeBadge,
  changeSentence,
  meetingWhen,
} from "./calendarchange";
import { eyebrowKeyFor } from "./worklisteyebrow";
import type { WorklistItem } from "./worklisttypes";

const t = (key: MessageKey, values?: Record<string, string>) =>
  translate("en", key, values);
const ZONE = "Europe/Berlin";

function change(over: Partial<CalendarChange>): CalendarChange {
  return {
    kind: "updated",
    title: "Pricing review",
    starts_at: "2026-12-03T14:00:00Z",
    ends_at: "2026-12-03T15:00:00Z",
    starts_precision: "instant",
    changed: [],
    ...over,
  };
}

describe("what a meeting's later mail says in one line", () => {
  it("names where a moved meeting was and where it is now", () => {
    const moved = change({
      changed: ["time", "location"],
      location: "Office Berlin",
      before: {
        starts_at: "2026-12-02T09:00:00Z",
        ends_at: "2026-12-02T10:00:00Z",
        starts_precision: "instant",
      },
    });
    const line = changeSentence(moved, t, "en", ZONE);
    expect(line).toMatch(/^Moved from .*2 Dec.*10:00.* to .*3 Dec.*15:00/);
  });

  it("names the new place, or that there is none", () => {
    expect(
      changeSentence(
        change({ changed: ["location"], location: "Room 4" }),
        t,
        "en",
        ZONE,
      ),
    ).toBe("Now at Room 4");
    expect(
      changeSentence(change({ changed: ["location"] }), t, "en", ZONE),
    ).toBe("No longer has a place");
  });

  it("names the guests added when nothing else moved", () => {
    const added = change({
      guests_added: [
        { email: "ben@partner.example", name: "Ben Ito" },
        { email: "cara@partner.example" },
      ],
    });
    expect(changeSentence(added, t, "en", ZONE)).toBe(
      "Added Ben Ito, cara@partner.example",
    );
  });

  it("says when a canceled meeting was set for, and who proposed or answered", () => {
    expect(changeSentence(change({ kind: "canceled" }), t, "en", ZONE)).toMatch(
      /^Was set for .*3 Dec.*15:00/,
    );
    const by = { email: "anna@partner.example", name: "Anna Weber" };
    expect(
      changeSentence(change({ kind: "proposed", by }), t, "en", ZONE),
    ).toMatch(/^Anna Weber proposes .*3 Dec/);
    expect(
      changeSentence(
        change({ kind: "answered", by, answer: "declined" }),
        t,
        "en",
        ZONE,
      ),
    ).toBe("Anna Weber declined");
  });

  it("draws an all-day meeting by its date alone", () => {
    const allDay = meetingWhen(
      { starts_at: "2026-11-20T00:00:00Z", starts_precision: "day" },
      "en",
      ZONE,
    );
    expect(allDay).toMatch(/20 Nov/);
    expect(allDay).not.toMatch(/\d\d:\d\d/);
  });
});

describe("the badge a meeting's later mail wears", () => {
  it("is the change's word on the worklist row as well", () => {
    const row = {
      id: "i1",
      source: "customer_waiting",
      category: "customer_waiting",
      actions: [],
      email_summary: { calendar_change: change({ kind: "canceled" }) },
    } as unknown as WorklistItem;
    expect(eyebrowKeyFor(row)).toBe("meetingChange.badge.canceled");
    expect(
      changeBadge(change({ kind: "answered", answer: "accepted" })),
    ).toEqual({ key: "meetingChange.badge.accepted", tone: "success" });
  });
});
