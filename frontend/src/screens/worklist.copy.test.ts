// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// comparisonText's `waiting_days` tie-break: when the server's own comparator
// falls back from a bucketed day count to the exact instant two items
// occurred (margince#3316), the line under "how many days" must still read as
// a day count rather than as two clock times a reader cannot reconcile with
// its own heading.

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

  it("leaves a same-day tie-break at zero rather than a negative or fractional count", () => {
    const now = new Date("2026-08-30T23:00:00.000Z");
    const comparison: WorklistComparison = {
      comparator: "waiting_days",
      mine: { kind: "date", date: "2026-08-30T21:52:00.000Z" },
      theirs: { kind: "date", date: "2026-08-30T22:24:00.000Z" },
    };
    expect(comparisonText(comparison, t, "en", zone, now)).toBe(
      "Above the next: 0 against 0.",
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
