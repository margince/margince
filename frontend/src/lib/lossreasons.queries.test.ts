import { describe, expect, it } from "vitest";
import { type LossReason, lostReasonWords } from "./lossreasons.queries";

function reason(key: string, label: string, active = true): LossReason {
  return {
    id: `r-${key}`,
    key,
    label,
    active,
    sort_order: 10,
    system: true,
    version: 1,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T08:00:00Z",
  };
}

describe("how a lost deal says why", () => {
  const reasons = [
    reason("price", "Price"),
    reason("timing", "Bad timing", false),
  ];

  it("names a code-only loss by its label", () => {
    expect(lostReasonWords({ lost_reason_code: "price" }, reasons)).toBe(
      "Price",
    );
  });

  it("names a retired reason by its label, with the note", () => {
    expect(
      lostReasonWords(
        { lost_reason_code: "timing", lost_reason: " Back in spring " },
        reasons,
      ),
    ).toBe("Bad timing — Back in spring");
  });

  it("reads a code the list does not hold as the code, and a note alone as the note", () => {
    expect(lostReasonWords({ lost_reason_code: "gone" }, reasons)).toBe("gone");
    expect(lostReasonWords({ lost_reason: "Only a note" }, undefined)).toBe(
      "Only a note",
    );
    expect(lostReasonWords({}, reasons)).toBeUndefined();
  });
});
