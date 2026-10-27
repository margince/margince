import { describe, expect, it } from "vitest";
import { count, day, money, percent } from "./values";

describe("the value helpers refuse to render nonsense", () => {
  it("renders a non-finite proportion as an em dash rather than NaN", () => {
    expect(percent(0.42)).toBe("42%");
    expect(percent(undefined)).toBe("—");
    expect(percent(Number.POSITIVE_INFINITY)).toBe("—");
    expect(count(Number.NaN)).toBe("—");
    expect(count(7)).toBe("7");
  });
});

/** inCurrency renders the expectation with money's own formatter, so a test
 *  checks the scale and not how the machine groups digits. */
function inCurrency(major: number, currency: string): string {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency,
  }).format(major);
}

const euros = (major: number) => inCurrency(major, "EUR");

describe("money is scaled by the currency's own minor units", () => {
  it("scales a two-decimal currency by a hundred", () => {
    // 24_000_000 minor EUR is 240,000 euros, not 24,000,000.
    expect(money(24_000_000, "EUR")).toBe(euros(240_000));
    expect(money(24_000_000, "EUR")).not.toBe(euros(24_000_000));
  });

  it("does not scale a zero-decimal currency at all", () => {
    // JPY stores 1234 minor units and means ¥1,234. A hard-coded /100 renders
    // ¥12.34 here and would pass every euro test in the suite.
    expect(money(1234, "JPY")).toBe(inCurrency(1234, "JPY"));
    expect(money(1234, "JPY")).not.toBe(inCurrency(12.34, "JPY"));
  });

  it("renders an absent amount or currency as an em dash, never as zero", () => {
    expect(money(undefined, "EUR")).toBe("—");
    expect(money(1000, undefined)).toBe("—");
    expect(money(Number.NaN, "EUR")).toBe("—");
  });

  it("renders a currency Intl does not know as an em dash rather than throwing", () => {
    // Intl throws a RangeError on an unknown code, and a view that threw
    // mid-render would leave the reader a blank panel with nothing saying why.
    expect(() => money(1000, "not-a-currency")).not.toThrow();
    expect(money(1000, "not-a-currency")).toBe("—");
  });
});

describe("day renders the calendar day the server judged in", () => {
  it("renders an instant as its UTC day", () => {
    expect(day("2026-06-10T12:00:00Z")).toBe("2026-06-10");
  });

  it("does not shift the day into the reader's own zone", () => {
    // 23:30 UTC is already the next day in much of the world. Rendering it
    // there would put "due 2026-06-11" beside the word "overdue", which the
    // server judged against 2026-06-10.
    expect(day("2026-06-10T23:30:00Z")).toBe("2026-06-10");
  });

  it("renders an absent or unparseable instant as an em dash", () => {
    expect(day(undefined)).toBe("—");
    expect(day("")).toBe("—");
    expect(day("not a date")).toBe("—");
  });
});
