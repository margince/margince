import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { basisAddsARecord, momentIsARow, momentKicker } from "./moment";

// WHAT THE CARD MAY DROP. Both judgements here decide whether the reader is
// shown something — a row, a chip, a word — so each case names the shape of
// moment it is about rather than a rule in the abstract.

type ContactMoment = components["schemas"]["ContactMoment"];

const MOMENT: ContactMoment = {
  claim_key: "moment:1",
  evidence_fingerprint: "fp-1",
  rule: "open_promise",
  headline: "You owe them: send the renewal quote",
  why_now: "A commitment with a date on it, still open.",
  confidence: "observed_fact",
  evidence: [],
  recommended_action: {
    kind: "complete_task",
    label: "Open it",
    state: "available",
  },
};

const TASK = {
  type: "task",
  id: "a-9",
  label: "Send the renewal quote",
} as const;

describe("what the moment rests on", () => {
  it("drops the one chip that only repeats a promise's own ask", () => {
    expect(basisAddsARecord({ ...MOMENT, evidence: [TASK] })).toBe(false);
  });

  it("keeps a record the headline merely happens to contain a word of", () => {
    // The gone-quiet headline is written about the relationship, not composed
    // from the record, so a subject inside it is a different fact — and the
    // chip is the reader's only way into the message behind the claim.
    expect(
      basisAddsARecord({
        ...MOMENT,
        rule: "gone_quiet",
        headline: "No reply for 14 days",
        evidence: [{ type: "activity", id: "a-1", label: "Reply" }],
      }),
    ).toBe(true);
  });

  it("counts one record sent twice as one", () => {
    // Two entries, one record: counted before the duplicate goes, the card
    // draws a caption over a chip that says the ask back.
    expect(basisAddsARecord({ ...MOMENT, evidence: [TASK, { ...TASK }] })).toBe(
      false,
    );
  });

  it("keeps an item carrying the words the moment was read out of", () => {
    expect(
      basisAddsARecord({
        ...MOMENT,
        evidence: [{ ...TASK, snippet: "I'll send the quote by Friday." }],
      }),
    ).toBe(true);
  });

  it("draws nothing where the moment rests on nothing it can open", () => {
    expect(basisAddsARecord(MOMENT)).toBe(false);
  });
});

describe("the word for the rule", () => {
  it("names the rung a move fired on", () => {
    expect(momentKicker(MOMENT, (key) => key)).toBe(
      "contact.moment.rule.open_promise",
    );
  });

  it("says nothing beside an all-clear, which is a reading and not a find", () => {
    expect(
      momentKicker({ ...MOMENT, rule: "nothing_needed" }, (key) => key),
    ).toBeUndefined();
  });
});

describe("whether the moment is a row at all", () => {
  it("drops the all-clear where the list still asks for something", () => {
    expect(momentIsARow({ ...MOMENT, rule: "nothing_needed" }, true)).toBe(
      false,
    );
  });

  it("keeps it where it is the whole answer", () => {
    expect(momentIsARow({ ...MOMENT, rule: "nothing_needed" }, false)).toBe(
      true,
    );
  });

  it("keeps every rung that asks for something, whatever else is listed", () => {
    expect(momentIsARow(MOMENT, true)).toBe(true);
  });
});
