import { describe, expect, it } from "vitest";
import { translate, translatePlural } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { whenText } from "../lib/worklistcopy";
import { row } from "../testing/worklist.testkit";
import { cardWhen } from "./taskmeta";

const ZONE = "Europe/Berlin";
const NOW = new Date("2026-11-16T09:00:00Z");

const t = ((key: MessageKey, values?: Record<string, string>) =>
  translate("en", key, values)) as Parameters<typeof whenText>[1];

const words = {
  t,
  plural: (
    base: Parameters<typeof translatePlural>[1],
    count: number,
    params?: Record<string, string>,
  ) => translatePlural("en", base, count, params),
  locale: "en" as const,
};

const late = row({
  source: "task",
  due_at: "2026-11-06T22:59:59Z",
  due_has_time: false,
  overdue: true,
});

// A late task's due day is one fact. Home's card and the Worklist row read it
// from one writer, so a reader never compares two spellings of a day.
describe("a late task's due day", () => {
  it("says the day and how late it is", () => {
    expect(whenText(late, t, "en", ZONE, ZONE, NOW)).toBe(
      "6 Nov · 10 days late",
    );
  });

  it("reads the same on Home's card as on the Worklist row", () => {
    expect(cardWhen(late, words, ZONE, ZONE, NOW)).toBe(
      whenText(late, t, "en", ZONE, ZONE, NOW),
    );
  });

  // A timed deadline is an instant. It keeps the reader's own date and clock
  // and the agreed clock beside them, then adds how late it is.
  it("keeps a timed late task on the reader's clock", () => {
    const timed = row({
      source: "task",
      due_at: "2026-11-05T23:30:00Z",
      due_has_time: true,
      due_intent: {
        wall_clock: "2026-11-05T18:30",
        timezone: "America/New_York",
      },
      overdue: true,
    });
    expect(whenText(timed, t, "en", ZONE, "America/New_York", NOW)).toBe(
      "due 06/11/2026, 00:30 (5 Nov, 18:30 America/New_York) · 10 days late",
    );
  });

  it("keeps the due day of a task that is not late", () => {
    const due = { ...late, overdue: false, due_at: "2026-11-20T22:59:59Z" };
    expect(whenText(due, t, "en", ZONE, ZONE, NOW)).toBe("due 20/11/2026");
  });
});
