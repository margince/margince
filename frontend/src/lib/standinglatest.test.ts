// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { UTC_ZONE } from "../format/timezone";
import { en } from "../i18n/en";
import { latestWord } from "./standinglatest";

type Activity = components["schemas"]["Activity"];

const t = (key: keyof typeof en, params?: Record<string, string>): string =>
  Object.entries(params ?? {}).reduce<string>(
    (text, [name, value]) => text.replace(`{${name}}`, value),
    en[key],
  );

const row = (
  over: Partial<Activity> & Pick<Activity, "id" | "kind" | "occurred_at">,
): Activity => ({ is_done: false, ...over }) as Activity;

const words = {
  t,
  locale: "en",
  zone: UTC_ZONE,
  record: UTC_ZONE,
  asOf: "2026-08-13T09:00:00Z",
} as const;

const margincesNote = row({
  id: "note",
  kind: "note",
  occurred_at: "2026-08-09T11:32:00Z",
  automatic_kind: "product",
  body: "Win probability set to 40%: the customer wants smaller components.",
});

const theirMail = row({
  id: "mail",
  kind: "email",
  occurred_at: "2026-08-05T10:00:00Z",
  direction: "inbound",
  body: "Full text with the quoted thread under it",
  email_summary: {
    counterparty: "Dana Reyes",
    direction: "inbound",
    preview: "We would like to start with parts 1 and 2a.",
  } as Activity["email_summary"],
});

describe("latestWord", () => {
  it("says the reason Margince gave for its newest change, marked as Margince's", () => {
    expect(latestWord([theirMail, margincesNote], words)).toMatchObject({
      byMargince: true,
      kind: "Note",
      when: "9 Aug",
      text: "Win probability set to 40%: the customer wants smaller components.",
    });
  });

  it("quotes a message by its own line and names who wrote it", () => {
    expect(latestWord([theirMail], words)).toMatchObject({
      byMargince: false,
      kind: "Email",
      detail: "From Dana Reyes",
      text: "We would like to start with parts 1 and 2a.",
    });
  });

  it("names the colleague who wrote a note", () => {
    const note = row({
      id: "n",
      kind: "note",
      occurred_at: "2026-08-10T10:00:00Z",
      body: "Budget signed off.",
      created_by: {
        actor_type: "human",
        actor_id: "u1",
        actor_name: "Uwe Morcinietz",
      },
    });
    expect(latestWord([note], words)?.detail).toBe("Uwe Morcinietz");
  });

  it("passes over rows that say nothing a reader may see", () => {
    const silent = [
      row({
        id: "task",
        kind: "task",
        occurred_at: "2026-08-12T10:00:00Z",
        subject: "Call back",
      }),
      row({
        id: "held-back",
        kind: "note",
        occurred_at: "2026-08-12T10:00:00Z",
        body: "x",
        content_state: "withheld",
      }),
      row({
        id: "noise",
        kind: "email",
        occurred_at: "2026-08-12T10:00:00Z",
        body: "x",
        capture_label: "noise",
      }),
      row({
        id: "bulk",
        kind: "email",
        occurred_at: "2026-08-12T10:00:00Z",
        body: "x",
        automatic_kind: "bulk",
      }),
      row({
        id: "booked",
        kind: "meeting",
        occurred_at: "2026-08-12T10:00:00Z",
        body: "Agenda",
        meeting_status: "booked",
      }),
      row({
        id: "ahead",
        kind: "note",
        occurred_at: "2026-08-20T10:00:00Z",
        body: "Not yet said",
      }),
      row({
        id: "empty",
        kind: "note",
        occurred_at: "2026-08-12T10:00:00Z",
        body: "  ",
      }),
    ];
    expect(latestWord([...silent, theirMail], words)?.text).toBe(
      "We would like to start with parts 1 and 2a.",
    );
    expect(latestWord(silent, words)).toBeUndefined();
  });

  it("is nothing on a page with no timeline", () => {
    expect(latestWord(undefined, words)).toBeUndefined();
  });
});
