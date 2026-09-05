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
  itemTitle,
  KNOWN_SOURCES,
  moveHref,
  moveLabel,
  moveOpensComposer,
  reasonText,
  sourceName,
  sourceUnavailableText,
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

// The wider vocabulary.
//
// The move slot carried one verb while one lane produced one; the deal card
// decides five, and the row now reads them. These are about the row refusing to
// draw a control it cannot honour, and about the label naming the verb the
// SERVER chose rather than the one that used to be the only option.

// The shapes the server really sends, not a bare action. `draft_reply` always
// names the message it answers — that is what makes it a reply — and the other
// verbs name whatever their own operand is, or nothing.
function movingRow(action: string, activityId?: string) {
  const row = replyRow({ type: "person", id: "p-1" }) as unknown as {
    move: unknown;
  };
  const move =
    action === "draft_reply"
      ? { action, activity_id: activityId ?? "a-1" }
      : { action, ...(activityId ? { activity_id: activityId } : {}) };
  return { ...row, move } as unknown as WorklistItem;
}

describe("the verbs the row can and cannot take a reader to", () => {
  // A first outreach reaches the same composer a reply does — the composer
  // picks its own transport from the person, so the address is the same.
  it("opens the composer for a fresh message too", () => {
    const item = movingRow("draft_email");
    expect(moveHref(item)).toContain("compose=");
    expect(moveOpensComposer(item)).toBe(true);
  });

  // THE distinction the wider vocabulary made necessary. One hardcoded label
  // was right while draft_reply was the only verb; over an opening outreach it
  // names a conversation nobody has had.
  it("names a first message as writing, not as replying", () => {
    expect(moveLabel(movingRow("draft_email"), t)).toBe("Draft the email");
    expect(moveLabel(movingRow("draft_reply"), t)).toBe("Draft the reply");
  });

  // And the label still follows the ROUTE. A deal has no composer, so the same
  // verb reaching only the record must not claim to draft anything.
  it("claims only to open where the composer cannot", () => {
    const onADeal = {
      ...(movingRow("draft_email") as unknown as { subject: unknown }),
      subject: { type: "deal", id: "d-1" },
    } as unknown as WorklistItem;
    expect(moveLabel(onADeal, t)).toBe("Open to write");
  });

  // These are PERFORMED, not navigated: one posts a task body, one leaves for a
  // provider's consent screen, and one names no step at all. A link drawn for
  // any of them would be pressable with nothing behind it.
  //
  // `open_meeting_brief` was on this list and no longer is. It reads as a
  // drawer, so it looked unaddressable — but the way IN is an address: the
  // brief opens as `?prep=<activity>` on a person's record. It is covered by
  // its own describe block below, including the case where the row names
  // nobody and correctly draws nothing.
  it("draws no link for a verb no address can perform", () => {
    for (const action of ["create_task", "reconnect", "none"]) {
      const item = movingRow(action);
      expect(moveHref(item)).toBeUndefined();
      expect(moveOpensComposer(item)).toBe(false);
    }
  });

  // A reply that names no message. Schema-VALID — `activity_id` is optional, for
  // the verbs that take no record — and undrawable all the same: there is
  // nothing to answer, and the contract says a client draws a control only where
  // the operand its verb needs is present.
  it("draws no reply where the move names no message", () => {
    const noMessage = {
      ...(movingRow("draft_email") as unknown as { move: unknown }),
      move: { action: "draft_reply" },
    } as unknown as WorklistItem;
    expect(moveHref(noMessage)).toBeUndefined();
    // And the LABEL agrees. A move refused a link must not still be described
    // as one that drafts, or the row says two things about one control.
    expect(moveOpensComposer(noMessage)).toBe(false);
  });

  // The sibling that must NOT be refused: an opening outreach legitimately names
  // no record, because there is no earlier message for it to name. Without this
  // case the completeness rule could be "require an id from every mail verb" and
  // the test above would still pass, while a first outreach lost its control.
  it("still draws a first message that names no record", () => {
    expect(moveHref(movingRow("draft_email"))).toContain("compose=");
    expect(moveOpensComposer(movingRow("draft_email"))).toBe(true);
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

describe("itemTitle — an incident names what broke, never an internal id", () => {
  // `cause` and `label` are two halves of one contract and the fixture supplies
  // both: `cause` is the identity the group was formed on, opaque and never
  // rendered, and `label` is what it identifies in words. A fixture carrying
  // only the field the code reads could not catch the code reading the other.
  const incident = (batch: { cause?: string; label?: string }): WorklistItem =>
    ({
      id: "i-1",
      source: "automation_run",
      batch: { key: "system_incident", count: 8, ...batch },
    }) as unknown as WorklistItem;

  const REF = "automation_run:01a065e8-617c-74ec-a4da-b41010b2a5b0";

  it("prints the label when the lane minted one", () => {
    expect(
      itemTitle(
        incident({ cause: REF, label: "Post-meeting recap draft" }),
        t,
        "en",
      ),
    ).toBe("Post-meeting recap draft failed 8 times");
  });

  it("never renders the cause, which is an identity and reads like one", () => {
    // What the Worklist's top row actually read against a seeded stack before
    // the contract grew `label`: "automation_run:01a065e8-… failed 8 times".
    // The contract now says `cause` is opaque, and that a client naming a group
    // by it is worse than a vaguer sentence — the reader can neither act on an
    // identity nor tell it from a bug. This is the client half of that promise,
    // and it is the only thing holding it.
    const title = itemTitle(incident({ cause: REF }), t, "en");
    expect(title).not.toContain("automation_run");
    expect(title).not.toContain("01a065e8");
    expect(title).toBe(itemTitle(incident({}), t, "en"));
  });

  it("says the cause is unnamed rather than borrowing the row's own source", () => {
    // The tempting fallback, and the wrong one: this item's `source` IS
    // `automation_run`, so a client reaching for something to say could print
    // the lane's key and look as though it had a name. A group whose lane
    // minted no label is a group nobody named, and the sentence says that.
    const title = itemTitle(incident({}), t, "en");
    expect(title).not.toContain("automation_run");
    expect(title).toContain("8 times");
  });
});

describe("an unavailable source", () => {
  // All three shipped locales, because the frame is a per-language decision and
  // a check that reads only English proves the rule for the one translator who
  // was in the room. The German and Vietnamese frames were rewritten in the
  // same change; nothing but this would notice if one drifted back.
  const LOCALES = ["en", "de", "vi"] as const;

  // Derived from the owner rather than listed here: a hand-kept copy of this
  // list goes short of the product the moment a source is added, and short is
  // the direction that passes silently.
  //
  // `batch` is left out on purpose, and it is the one exclusion: it names a
  // GROUP of decisions rather than a source the page reads, so it can never
  // appear in `sources_unavailable`.
  const SOURCES = Object.keys(KNOWN_SOURCES).filter(
    (source) => source !== "batch",
  );

  it("covers every source the product can name", () => {
    // The census, asserted. Without this the loop below could quietly narrow to
    // one source and still report a green run.
    expect(SOURCES.length).toBe(Object.keys(KNOWN_SOURCES).length - 1);
    expect(SOURCES).toContain("capture_health");
  });

  it("never reads its own title as the sentence's subject", () => {
    // What `sourceName` returns is a row TITLE, and most of them are whole
    // clauses — "A mailbox connection needs attention". Framed as a subject,
    // fourteen of these ran two sentences together, and each one read as a
    // rendering fault rather than as a source the page could not reach.
    //
    // The check is positional rather than a list of the bad wordings: the fact
    // comes first and the title follows it, so no title can be the subject
    // whatever the copy says next.
    for (const locale of LOCALES) {
      const t: Translator = (key, vars) => translate(locale, key, vars);
      for (const source of SOURCES) {
        const name = sourceName(source, t);
        for (const reason of ["withheld", "failed"] as const) {
          const where = `${locale}/${source}/${reason}`;
          const text = sourceUnavailableText({ source, reason }, t);
          expect(text, where).toContain(name);
          expect(text.startsWith(name), where).toBe(false);
        }
      }
    }
  });

  it("says which of the two reasons it is, because only one is anybody's to fix", () => {
    // A withheld source is the reader's own grants; a failed one is an outage.
    // One sentence for both would tell a reader to go asking for access to
    // something that is simply down.
    for (const locale of LOCALES) {
      const t: Translator = (key, vars) => translate(locale, key, vars);
      const withheld = sourceUnavailableText(
        { source: "dsr", reason: "withheld" },
        t,
      );
      const failed = sourceUnavailableText(
        { source: "dsr", reason: "failed" },
        t,
      );
      expect(withheld, locale).not.toBe(failed);
    }
  });
});

// The brief is not a page of its own: it opens as `?prep=<activity>` on a
// PERSON's record, so the address needs both ids and the row's subject — the
// meeting — carries only one.
describe("moveHref — the open_meeting_brief move", () => {
  it("opens the brief on the person the meeting names", () => {
    const href = moveHref(briefRow("p-9"));
    expect(href).toContain("#/contacts/p-9");
    expect(href).toContain("prep=a-7");
  });

  // A meeting naming nobody has no page to read the brief on. An internal
  // meeting and one whose attendees are all withheld arrive identically, and
  // both must draw NO way in rather than one that opens nothing.
  it("offers nothing where the meeting names nobody", () => {
    expect(moveHref(briefRow(undefined))).toBeUndefined();
  });

  // It is not the composer, and must not be described as one: the label reads
  // off the same predicate, so a brief announced as a draft would be a button
  // whose words and destination disagree.
  it("is not a draft", () => {
    expect(moveOpensComposer(briefRow("p-9"))).toBe(false);
  });
});

function briefRow(withPerson: string | undefined) {
  return {
    id: "m1",
    source: "meeting",
    category: "meetings",
    title: "Fleet retrofit review",
    because: [],
    actions: [],
    dispositions: [],
    overdue: false,
    subject: { type: "activity", id: "a-7" },
    with_person: withPerson,
    move: { action: "open_meeting_brief", activity_id: "a-7" },
  } as unknown as WorklistItem;
}
