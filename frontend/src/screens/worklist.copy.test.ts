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
import {
  comparisonText,
  moveHref,
  moveOpensComposer,
  reasonText,
} from "./worklist.copy";
import type {
  WorklistComparison,
  WorklistItem,
  WorklistReason,
} from "./worklist.queries";

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

// The move that became performable.
//
// The row refused to promise a draft until a route existed to open one — its
// own comment said so. These are about the two halves of that promise moving
// together: where the composer opens, and what the label is allowed to claim.

function replyRow(subject: { type: string; id: string } | undefined) {
  return {
    id: "r1",
    source: "waiting_customer",
    category: "customer_waiting",
    title: "Aster Handel",
    because: [],
    actions: ["open"],
    dispositions: [],
    overdue: false,
    subject,
    move: { action: "draft_reply", activity_id: "a-1" },
  } as unknown as WorklistItem;
}

describe("moveHref — the draft_reply move", () => {
  // The composer lives on the person page and drafts to the person. A link
  // asking for it there is a link that does what its label says.
  it("opens the composer on a person's record", () => {
    const href = moveHref(replyRow({ type: "person", id: "p-1" }));
    expect(href).toContain("#/contacts/p-1");
    expect(href).toContain("compose=reply");
    expect(moveOpensComposer(replyRow({ type: "person", id: "p-1" }))).toBe(
      true,
    );
  });

  // A deal has no composer to open, so a link claiming to draft there would
  // promise what the click cannot do. It reaches the record and says so.
  it("reaches the record, and claims no draft, where there is no composer", () => {
    for (const type of ["deal", "organization"]) {
      const item = replyRow({ type, id: "x-1" });
      expect(moveHref(item)).not.toContain("compose=");
      expect(moveHref(item)).toBeTruthy();
      expect(moveOpensComposer(item)).toBe(false);
    }
  });

  it("offers no move at all where the row names no record", () => {
    expect(moveHref(replyRow(undefined))).toBeUndefined();
  });

  // A row with no move suggests no step, and a control drawn for one would be
  // pressable with nothing behind it.
  it("offers no move where the row suggests no step", () => {
    const noMove = {
      ...replyRow({ type: "person", id: "p-1" }),
      move: undefined,
    };
    expect(moveHref(noMove as unknown as WorklistItem)).toBeUndefined();
    expect(moveOpensComposer(noMove as unknown as WorklistItem)).toBe(false);
  });
});

// A row waiting one day said "waiting 1 days". quiet_days carries the exact
// same day-count value shape through the exact same reasonText path, so it
// gets the exact same fix rather than a patch on one string.
describe("reasonText — day-count plural", () => {
  it("uses the singular day form for a one-day wait", () => {
    const reason: WorklistReason = {
      kind: "waiting_days",
      value: { kind: "days", days: 1 },
    };
    expect(reasonText(reason, t, "en", zone)).toBe("waiting 1 day");
  });

  it("uses the plural day form otherwise", () => {
    const reason: WorklistReason = {
      kind: "waiting_days",
      value: { kind: "days", days: 5 },
    };
    expect(reasonText(reason, t, "en", zone)).toBe("waiting 5 days");
  });

  it("pluralizes the sibling quiet_days reason the same way", () => {
    const one: WorklistReason = {
      kind: "quiet_days",
      value: { kind: "days", days: 1 },
    };
    const many: WorklistReason = {
      kind: "quiet_days",
      value: { kind: "days", days: 14 },
    };
    expect(reasonText(one, t, "en", zone)).toBe("quiet for 1 day");
    expect(reasonText(many, t, "en", zone)).toBe("quiet for 14 days");
  });
});
