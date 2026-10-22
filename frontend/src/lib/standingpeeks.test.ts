// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { UTC_ZONE } from "../format/timezone";
import { en } from "../i18n/en";
import { type PeekWords, partialFoot, tasksPeek } from "./standingpeeks";

const fill = (text: string, params?: Record<string, string>) =>
  Object.entries(params ?? {}).reduce(
    (out, [name, value]) => out.replace(`{${name}}`, value),
    text,
  );

const words: PeekWords = {
  t: (key, params) => fill(en[key], params),
  plural: (base, count, params) =>
    fill(
      en[`${base}_${count === 1 ? "one" : "other"}` as keyof typeof en],
      params,
    ),
  locale: "en",
  zone: UTC_ZONE,
};

describe("partialFoot", () => {
  it("says how many of the whole the rows are when they are a page of it", () => {
    expect(partialFoot(3, 11, words)).toBe("Showing 3 of 11.");
  });

  it("says nothing when the rows are the whole", () => {
    expect(partialFoot(4, 4, words)).toBeUndefined();
  });
});

describe("tasksPeek", () => {
  it("lists each task with its date, and marks an overdue or undated one in words", () => {
    const card = tasksPeek(
      [
        { id: "a", subject: "Book the visit", dueAt: "2026-10-28T09:00:00Z" },
        {
          id: "b",
          subject: "Send paperwork",
          dueAt: "2026-10-01T09:00:00Z",
          overdue: true,
        },
        { id: "c", subject: "Tidy the sheet" },
      ],
      words,
      3,
    );

    expect(card?.items.map((item) => item.sub)).toEqual([
      "28 Oct",
      "1 Oct, overdue",
      "No date",
    ]);
    expect(card?.foot).toBeUndefined();
  });

  it("draws no card for no tasks", () => {
    expect(tasksPeek([], words, 0)).toBeUndefined();
  });
});
