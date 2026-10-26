// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { UTC_ZONE } from "../format/timezone";
import { en } from "../i18n/en";
import { activityMarks } from "./standingmarks";

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
  asOf: "2026-08-13T09:00:00Z",
} as const;

describe("activityMarks", () => {
  it("names the way a message went and with whom, from the row alone", () => {
    const [mark] = activityMarks(
      [
        row({
          id: "a",
          kind: "email",
          occurred_at: "2026-08-01T10:00:00Z",
          subject: "Scope question",
          direction: "inbound",
          email_summary: {
            counterparty: "Dana Reyes",
            direction: "inbound",
          } as Activity["email_summary"],
        }),
      ],
      words,
    );

    expect(mark).toMatchObject({
      key: "a",
      kind: "email",
      direction: "inbound",
      what: "Email",
      title: "Scope question",
      detail: "From Dana Reyes",
      when: "1 Aug",
    });
  });

  it("names the recorded author of a list row that carries no counterparty", () => {
    const [mark] = activityMarks(
      [
        row({
          id: "a",
          kind: "email",
          occurred_at: "2026-08-01T10:00:00Z",
          direction: "inbound",
          voice: {
            author: {
              kind: "contact_record",
              display_name: "Pia Sender",
              is_reader: false,
            },
            contact_role: "cc",
          },
        }),
      ],
      words,
    );

    expect(mark?.detail).toBe("From Pia Sender");
  });

  it("says only the kind for a row whose text is withheld, and invents no name", () => {
    const [mark] = activityMarks(
      [
        row({
          id: "w",
          kind: "call",
          occurred_at: "2026-08-01T10:00:00Z",
          subject: null,
        }),
      ],
      words,
    );

    expect(mark?.title).toBe("Call");
    expect(mark?.detail).toBeUndefined();
  });

  it("leaves out tasks, meetings not held, archived rows and anything after the read", () => {
    const marks = activityMarks(
      [
        row({ id: "t", kind: "task", occurred_at: "2026-08-01T10:00:00Z" }),
        row({
          id: "b",
          kind: "meeting",
          meeting_status: "booked",
          occurred_at: "2026-08-01T10:00:00Z",
        }),
        row({
          id: "c",
          kind: "meeting",
          meeting_status: "canceled",
          occurred_at: "2026-08-01T10:00:00Z",
        }),
        row({
          id: "x",
          kind: "note",
          archived_at: "2026-08-02T10:00:00Z",
          occurred_at: "2026-08-01T10:00:00Z",
        }),
        row({ id: "f", kind: "email", occurred_at: "2026-09-01T10:00:00Z" }),
        row({
          id: "ok",
          kind: "meeting",
          meeting_status: "held",
          occurred_at: "2026-08-01T10:00:00Z",
        }),
      ],
      words,
    );

    expect(marks.map((mark) => mark.key)).toEqual(["ok"]);
  });
});
