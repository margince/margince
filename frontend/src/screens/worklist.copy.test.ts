// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// comparisonText and reasonText: the sentences the Worklist draws for why one
// row beat the next, and for each fact behind an item's rank.

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
// The "still renders a full date" case below only asserts the year, so the
// viewer's own zone stands in rather than a pinned literal
// (format/zone-by-purpose.test.ts).
const zone = viewerZone();

describe("comparisonText", () => {
  it("renders a waiting_days pair as day counts", () => {
    const comparison: WorklistComparison = {
      comparator: "waiting_days",
      mine: { kind: "days", days: 12 },
      theirs: { kind: "days", days: 30 },
    };
    expect(comparisonText(comparison, t, "en", zone)).toBe(
      "Above the next: 12 against 30.",
    );
  });

  it("falls back to the bare sentence when the server sends no values", () => {
    // The occurrence step withholds both values when they would print the
    // same day count — a comparator that decided but has nothing a reader
    // could check draws the plain sentence rather than a false tie.
    const comparison: WorklistComparison = { comparator: "waiting_days" };
    expect(comparisonText(comparison, t, "en", zone)).toBe(
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

  // The comparator this build emits and could not name.
  //
  // `crowded` is the anti-monopoly rule: the row BELOW was held back so one
  // lane could not own the page. It carries no values on purpose — "8th
  // against 9th" describes the lane, not either row — and the client's
  // known-comparator guard, written to drop values from NEWER servers, was
  // dropping one this same build sends. The row whose position is hardest to
  // explain was the one row with no explanation.
  it("names crowded, which this build's own server emits", () => {
    expect(comparisonText({ comparator: "crowded" }, t, "en", zone)).toBe(
      "Above the next because that one is one of many of its kind.",
    );
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

// The lead's deadline is a MOMENT, and the sentence has to put it somewhere.
//
// Both halves of this live on opposite sides of the wire: the backend attaches
// the date (attention/lead.go leadStanding), and VALUED_REASONS here decides
// whether it reaches a sentence at all. A value can travel for a reason whose
// copy has nowhere to put it, and that combination renders the plain phrase
// while the figure is silently dropped — so the two are one change, and this is
// the half that fails if only the backend lands.
describe("reasonText — the lead's own deadline", () => {
  it("names when the reply is due", () => {
    const reason: WorklistReason = {
      kind: "response_due_soon",
      value: { kind: "date", date: "2026-09-03T14:30:00Z" },
    };
    const got = reasonText(reason, t, "en", zone);
    expect(got).not.toBeNull();
    // The formatted moment, not the raw ISO string: valueText renders it in the
    // reader's locale and zone, and asserting the literal would pin a format
    // this file does not own.
    expect(got).not.toBe("reply due soon");
    expect(got).toContain("reply due by");
  });

  it("falls back to the plain phrase when no deadline travelled", () => {
    const reason: WorklistReason = { kind: "response_due_soon" };
    expect(reasonText(reason, t, "en", zone)).toBe("reply due soon");
  });
});
