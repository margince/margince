// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { formatMoneyCompactRange } from "./analytics.moneyrange";

// The compact suffix's letter case is ICU's call ("k" or "K"), so figures are
// folded to lower case; the digits and the sign stay exact.
describe("a money range abbreviated for a KPI slot", () => {
  it("adds digits until the two ends read apart", () => {
    expect(
      formatMoneyCompactRange(
        2_453_456_600,
        2_454_776_600,
        "EUR",
        "en",
      ).toLowerCase(),
    ).toBe("€24.53m–24.54m");
  });

  it("keeps three digits where they already differ", () => {
    expect(
      formatMoneyCompactRange(
        61_000_000,
        98_000_000,
        "EUR",
        "en",
      ).toLowerCase(),
    ).toBe("€610k–980k");
  });

  it("puts a trailing currency sign once, after the high end", () => {
    expect(
      formatMoneyCompactRange(61_000_000, 98_000_000, "EUR", "de").replace(
        /\s/g,
        " ",
      ),
    ).toMatch(/^[^€]+–[^€]+ €$/);
  });

  it("says one amount once", () => {
    expect(
      formatMoneyCompactRange(
        42_000_000,
        42_000_000,
        "EUR",
        "en",
      ).toLowerCase(),
    ).toBe("€420k");
  });
});
