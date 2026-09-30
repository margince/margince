// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  formatMoney,
  formatMoneyCompact,
  formatMoneyOrAbsent,
  MONEY_ABSENT,
} from "./format";
import { formatMoneyOrWord } from "./moneyword";

// A stat card's value is a READING, so the surfaces that draw one ask the same
// question and take a WORD for the absence. They used to ask it by comparing
// the dash, which made the glyph a protocol between files.
describe("money, or the word a slot gives for its absence", () => {
  it("answers with the caller's word rather than the dash", () => {
    expect(
      formatMoneyOrWord(null, "EUR", "en", "None won yet", formatMoney),
    ).toBe("None won yet");
    expect(
      formatMoneyOrWord(123_456, null, "en", "Not forecast", formatMoney),
    ).toBe("Not forecast");
    // The shape that reached Intl and threw mid-render.
    expect(
      formatMoneyOrWord(123_456, "", "en", "Not forecast", formatMoney),
    ).toBe("Not forecast");
    expect(
      formatMoneyOrWord(null, "EUR", "en", "None won yet", formatMoney),
    ).not.toContain(MONEY_ABSENT);
  });

  // A real zero is a figure the server DID send. The word is for a pair that
  // cannot be said as money at all, never for a small one.
  it("says a stored zero as money, not as the absent word", () => {
    expect(formatMoneyOrWord(0, "EUR", "en", "None won yet", formatMoney)).toBe(
      "€0.00",
    );
  });

  // The formatter is NAMED at every call site rather than defaulted: only the
  // site knows whether it holds a balance or an aggregate, and a default is a
  // place for it not to say.
  it("writes the figure the way the caller named", () => {
    expect(
      formatMoneyOrWord(1_234_567_00, "EUR", "en", "None", formatMoney),
    ).toBe(formatMoney(1_234_567_00, "EUR", "en"));
    expect(
      formatMoneyOrWord(1_234_567_00, "EUR", "en", "None", formatMoneyCompact),
    ).toBe(formatMoneyCompact(1_234_567_00, "EUR", "en"));
  });

  // The dash is this module's own answer and nothing outside it may depend on
  // the glyph, so the two spellings share ONE guard rather than agreeing twice.
  it("is the same guard the dash-returning spelling uses", () => {
    expect(formatMoneyOrAbsent(null, "EUR", "en")).toBe(
      formatMoneyOrWord(null, "EUR", "en", MONEY_ABSENT, formatMoney),
    );
    expect(formatMoneyOrAbsent(40, "EUR", "en")).toBe(
      formatMoneyOrWord(40, "EUR", "en", MONEY_ABSENT, formatMoney),
    );
  });
});
