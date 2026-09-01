// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// comparisonText's `waiting_days` tie-break: when the server's own comparator
// falls back from a bucketed day count to the exact instant two items
// occurred, the line under "how many days" must still read as a day count
// rather than as two clock times a reader cannot reconcile with its own
// heading.

import { describe, expect, it } from "vitest";
import { viewerZone } from "../format/timezone";
import type { Translator } from "../i18n";
import { translate } from "../i18n";
import { comparisonText } from "./worklist.copy";
import type { WorklistComparison } from "./worklist.queries";

const t: Translator = (key, params) => translate("en", key, params);
// The elapsed-days math under test (calendarDaysBetween) does not take a
// zone at all, and the one case that renders a date only asserts it contains
// a year — so the viewer's own zone stands in rather than a pinned literal
// (format/zone-by-purpose.test.ts).
const zone = viewerZone();

describe("comparisonText — waiting_days tie-break", () => {
  it("renders raw days-kind values as-is (the ordinary, non-tied case)", () => {
    const comparison: WorklistComparison = {
      comparator: "waiting_days",
      mine: { kind: "days", days: 12 },
      theirs: { kind: "days", days: 30 },
    };
    expect(comparisonText(comparison, t, "en", zone)).toBe(
      "Above the next: 12 against 30.",
    );
  });

  it("converts a date-kind tie-break value to elapsed days, not a timestamp", () => {
    const now = new Date("2026-09-01T00:00:00.000Z");
    const comparison: WorklistComparison = {
      comparator: "waiting_days",
      mine: { kind: "date", date: "2026-08-30T21:52:00.000Z" },
      theirs: { kind: "date", date: "2026-08-29T22:24:00.000Z" },
    };
    const line = comparisonText(comparison, t, "en", zone, now);
    expect(line).toBe("Above the next: 2 against 3.");
    expect(line).not.toContain(":52");
    expect(line).not.toContain(":24");
  });

  it("floors at zero rather than printing a negative count on clock skew", () => {
    // `now` is the server's own snapshot instant in production
    // (Worklist.as_of), so an occurred_at reading later than it means the
    // two clocks disagree at the margin, never that the row is from the
    // future. A negative count under "how many days" is the same dishonest
    // line this function exists to remove, just spelled with a minus sign.
    const now = new Date("2026-08-30T00:00:00.000Z");
    const comparison: WorklistComparison = {
      comparator: "waiting_days",
      mine: { kind: "date", date: "2026-08-31T09:00:00.000Z" },
      theirs: { kind: "date", date: "2026-08-29T09:00:00.000Z" },
    };
    expect(comparisonText(comparison, t, "en", zone, now)).toBe(
      "Above the next: 0 against 1.",
    );
  });

  it("falls back to the bare sentence when a same-day tie-break would print equal numbers", () => {
    // Two different instants that still round to the same calendar day.
    // Printing "0 against 0" would claim they tied, when the comparator's
    // whole reason for firing is that they did not — so this reads as the
    // plain "how long it has waited" sentence instead, the same call the
    // backend's own same-minute guard makes one level finer.
    const now = new Date("2026-08-31T23:00:00.000Z");
    const comparison: WorklistComparison = {
      comparator: "waiting_days",
      mine: { kind: "date", date: "2026-08-31T21:52:00.000Z" },
      theirs: { kind: "date", date: "2026-08-31T22:24:00.000Z" },
    };
    expect(comparisonText(comparison, t, "en", zone, now)).toBe(
      "Above the next on how long it has waited.",
    );
  });

  it("still renders a full date for a non-waiting_days comparator's date value", () => {
    const comparison: WorklistComparison = {
      comparator: "deadline",
      mine: { kind: "date", date: "2026-08-30T21:52:00.000Z" },
      theirs: { kind: "date", date: "2026-08-29T22:24:00.000Z" },
    };
    expect(comparisonText(comparison, t, "en", zone)).toContain("2026");
  });

  it("draws nothing for order (every comparator tied, ids broke it)", () => {
    expect(comparisonText({ comparator: "order" }, t, "en", zone)).toBeNull();
  });
});
