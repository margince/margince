// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { pluralCategory } from "../format/plural";
import { pluralKey, translatePlural } from "./index";

// The plural rule itself: which form a count takes, and which catalogue entry
// that resolves to.
//
// The three shipped locales all split at exactly one, so none of these cases
// would fail against the fifteen hand-rolled `count === 1` ternaries this
// replaced. That is the point of the last suite: what a locale with a third
// category does is the behaviour those ternaries could not have, and it is
// asserted against a real CLDR rule rather than against a mock, because a mock
// would be this test agreeing with itself.

describe("pluralCategory", () => {
  it("splits English and German at one", () => {
    expect(pluralCategory("en", 1)).toBe("one");
    expect(pluralCategory("en", 0)).toBe("other");
    expect(pluralCategory("en", 2)).toBe("other");
    expect(pluralCategory("de", 1)).toBe("one");
    expect(pluralCategory("de", 7)).toBe("other");
  });

  it("gives Vietnamese one form for every count", () => {
    // vi does not inflect for number. Both catalogue arms read identically, and
    // the rule says so rather than the catalogue pretending to a distinction.
    expect(pluralCategory("vi", 1)).toBe("other");
    expect(pluralCategory("vi", 5)).toBe("other");
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
    // vi has one category, so both counts name the same entry — which is the
    // honest answer rather than a singular arm nothing ever reaches.
    expect(pluralKey("vi", "ob.conv.review.requiredRemaining", 1)).toBe(
      "ob.conv.review.requiredRemaining_other",
    );
  });
});

describe("a locale with more than two categories", () => {
  // The reason this layer exists is a locale whose rule has a `few` or a `many`
  // — Polish, Russian, Arabic — where no comparison with 1 can produce the right
  // form. That is deliberately NOT asserted by constructing
  // `new Intl.PluralRules("pl-PL")` here: a locale tag written into a source
  // file is exactly what `format/one-locale.test.ts` refuses, and it is right to,
  // because a pinned tag is a rendering in somebody else's language. The claim
  // about CLDR's categories is a fact about CLDR rather than about this code.
  //
  // What IS this code's behaviour, and is asserted, is the fallback such a
  // locale takes on the day it ships and before its `_few` arm is translated.
  it("falls back to the _other arm for a category the catalogue lacks", () => {
    // What a shipped locale gaining a category does before its translations
    // land: the reader gets the plural wording rather than a raw key. Asserted
    // through the vi rule, whose single category IS `other`, so this is the
    // fallback path taken by a real locale rather than a contrived one.
    expect(translatePlural("vi", "share.teamMembers", 1, { count: "1" })).toBe(
      translatePlural("vi", "share.teamMembers", 9, { count: "1" }),
    );
  });
});
