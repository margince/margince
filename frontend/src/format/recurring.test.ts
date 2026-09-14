import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

import { monthlyEquivalent } from "./recurring";

/**
 * The browser's monthly reading answers the SAME corpus the Go kernel does.
 *
 * The file is read from the backend tree on purpose. A copy of the cases here
 * would prove the two lists agreed the day somebody typed them and nothing
 * about the next edit, which is the drift the minor-unit vocabulary gate was
 * written after.
 */
type MonthlyCase = Readonly<{
  name: string;
  currency: string;
  annual_minor: number;
  monthly_minor: number;
  approximate: boolean;
}>;

// Relative to THIS file, so the reference survives the frontend being run from
// anywhere. It points into the backend tree deliberately: one corpus, two
// readers.
const CORPUS_PATH =
  "../../../backend/internal/shared/kernel/values/testdata/monthly_equivalent.json";

const cases: MonthlyCase[] = JSON.parse(
  readFileSync(new URL(CORPUS_PATH, import.meta.url), "utf8"),
).cases;

describe("monthlyEquivalent", () => {
  // A corpus that read as empty agrees with every implementation, including a
  // wrong one, so the floor runs before the cases do.
  it("reads the shared corpus the Go side reads", () => {
    expect(cases.length).toBeGreaterThanOrEqual(8);
  });

  it.each(cases)("$name", (c) => {
    const reading = monthlyEquivalent(c.annual_minor);
    expect(reading).not.toBeNull();
    expect(reading?.monthlyMinor).toBe(c.monthly_minor);
    expect(reading?.approximate).toBe(c.approximate);
  });

  // The property the approximate flag claims, checked rather than restated as
  // more corpus rows.
  it("reports exact only when twelve months add back to the year", () => {
    for (let annual = 0; annual < 500; annual += 1) {
      const reading = monthlyEquivalent(annual);
      expect(reading).not.toBeNull();
      expect((reading?.monthlyMinor ?? 0) * 12 === annual).toBe(
        !reading?.approximate,
      );
    }
  });

  // A figure this cannot divide exactly answers with nothing rather than a
  // guess. It returns null rather than throwing because the callers render it
  // beside an annual figure that displays correctly on its own, and a throw
  // would take the whole record page down over the derived line.
  it("answers nothing for a figure it cannot divide exactly", () => {
    expect(monthlyEquivalent(10.5)).toBeNull();
    expect(monthlyEquivalent(Number.NaN)).toBeNull();
    // Past the safe-integer bound the value arrived having already lost
    // precision in JSON, so there is no exact figure left to divide.
    expect(monthlyEquivalent(Number.MAX_SAFE_INTEGER + 2)).toBeNull();
  });
});
