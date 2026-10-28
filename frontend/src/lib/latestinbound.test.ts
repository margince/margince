// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { latestInbound } from "./latestinbound";

type Activity = components["schemas"]["Activity"];

function row(id: string, at: string, fields: Partial<Activity>): Activity {
  return {
    id,
    kind: "note",
    occurred_at: at,
    occurred_precision: "instant",
    source: "manual",
    captured_by: "human:x",
    created_at: at,
    updated_at: at,
    offers_email: false,
    is_done: false,
    ...fields,
  };
}

const form = row("form", "2026-10-27T08:00:00Z", { source: "web_form" });
const mailIn = row("mail-in", "2026-10-27T09:00:00Z", {
  kind: "email",
  direction: "inbound",
});

describe("the latest thing they sent", () => {
  it("is nothing on a record nobody wrote to", () => {
    expect(latestInbound([], [])).toBeUndefined();
  });

  it("is the message the server says is waiting for a reply", () => {
    expect(latestInbound([mailIn], [mailIn])).toEqual({
      activity: mailIn,
      kind: "email",
    });
  });

  it("is a website form while nothing from our side came after it", () => {
    const note = row("ours", "2026-10-27T10:00:00Z", {});
    expect(latestInbound([], [note, form])?.kind).toBe("web_form");
  });

  it.each([
    ["a call", { kind: "call" }],
    ["a held meeting", { kind: "meeting", meeting_status: "held" }],
    ["a sent email", { kind: "email", direction: "outbound" }],
  ] as const)("drops the form once %s answered it", (_, fields) => {
    const answer = row("answer", "2026-10-27T10:00:00Z", fields);
    expect(latestInbound([], [answer, form])).toBeUndefined();
  });

  it("keeps the form when the meeting after it was canceled", () => {
    const canceled = row("m", "2026-10-27T10:00:00Z", {
      kind: "meeting",
      meeting_status: "canceled",
    });
    expect(latestInbound([], [canceled, form])?.kind).toBe("web_form");
  });

  it("keeps the form when the call came before it", () => {
    const call = row("call", "2026-10-26T10:00:00Z", { kind: "call" });
    expect(latestInbound([], [form, call])?.kind).toBe("web_form");
  });

  it("is the newer of an open form and a waiting message", () => {
    const laterForm = row("form2", "2026-10-27T11:00:00Z", {
      source: "web_form",
    });
    expect(latestInbound([mailIn], [laterForm, mailIn])?.activity.id).toBe(
      "form2",
    );
    expect(latestInbound([mailIn], [mailIn, form])?.activity.id).toBe(
      "mail-in",
    );
  });

  it("passes over a skipped item to the next one still open", () => {
    const laterForm = row("form2", "2026-10-27T11:00:00Z", {
      source: "web_form",
    });
    const olderMail = row("mail-old", "2026-10-26T09:00:00Z", {
      kind: "email",
      direction: "inbound",
    });
    const skipped = new Set(["form2", "mail-in"]);
    expect(
      latestInbound([mailIn, olderMail], [laterForm, mailIn, olderMail], (r) =>
        skipped.has(r.id),
      )?.activity.id,
    ).toBe("mail-old");
  });
});
