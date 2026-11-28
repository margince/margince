// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { pluralCategory } from "../format/plural";
import { pluralKey, translatePlural } from "./index";

describe("pluralCategory", () => {
  it("splits English and German at one", () => {
    expect(pluralCategory("en", 1)).toBe("one");
    expect(pluralCategory("en", 0)).toBe("other");
    expect(pluralCategory("en", 2)).toBe("other");
    expect(pluralCategory("de", 1)).toBe("one");
    expect(pluralCategory("de", 7)).toBe("other");
  });

  it("keeps Vietnamese wording stable across CLDR plural categories", () => {
    for (const count of [0, 1, 1.5, 5]) {
      expect(["one", "other"]).toContain(pluralCategory("vi", count));
      expect(
        translatePlural("vi", "share.teamMembers", count, { count: "5" }),
      ).toBe("Nhóm · 5 thành viên");
    }
  });

  it("refuses a count no reader has", () => {
    // `select` would answer for NaN, and the answer would be a category off a
    // number nothing rendered. Failing here names the caller; falling through
    // would put a plausible sentence on screen for a count that is not one.
    expect(() => pluralCategory("en", Number.NaN)).toThrow(/non-finite/);
    expect(() => pluralCategory("en", Number.POSITIVE_INFINITY)).toThrow(
      /non-finite/,
    );
  });
});

describe("translatePlural", () => {
  it("renders the singular for one and the plural for the rest", () => {
    expect(translatePlural("en", "share.teamMembers", 1, { count: "1" })).toBe(
      "Team · 1 member",
    );
    expect(translatePlural("en", "share.teamMembers", 4, { count: "4" })).toBe(
      "Team · 4 members",
    );
  });

  it("takes the count for the rule and the string for the reader", () => {
    // A formatted "1,204" is not a number the rule can select on, and a raw
    // 1204 is not what a reader should see — so the caller formats and this
    // selects. Passing only the formatted string is how a thousands separator
    // ends up deciding a plural form.
    expect(
      translatePlural("en", "share.teamMembers", 1204, { count: "1,204" }),
    ).toBe("Team · 1,204 members");
  });

  it("renders the reader's own language, not English with a count in it", () => {
    expect(translatePlural("de", "share.teamMembers", 1, { count: "1" })).toBe(
      "Team · 1 Mitglied",
    );
    expect(translatePlural("de", "share.teamMembers", 3, { count: "3" })).toBe(
      "Team · 3 Mitglieder",
    );
  });
});

describe("pluralKey", () => {
  it("names the catalogue entry a count resolves to", () => {
    expect(pluralKey("en", "ob.conv.review.requiredRemaining", 1)).toBe(
      "ob.conv.review.requiredRemaining_one",
    );
    expect(pluralKey("en", "ob.conv.review.requiredRemaining", 2)).toBe(
      "ob.conv.review.requiredRemaining_other",
    );
    expect(pluralKey("vi", "ob.conv.review.requiredRemaining", 5)).toBe(
      "ob.conv.review.requiredRemaining_other",
    );
  });
});
