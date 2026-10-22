// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import {
  currencyCodes,
  currencyOptions,
  FALLBACK_LEADING_CURRENCIES,
} from "./currencychoices";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("currencyCodes", () => {
  it("leads with the codes it is given, in that order", () => {
    expect(currencyCodes(["NOK", "SEK", "JPY"]).slice(0, 3)).toEqual([
      "NOK",
      "SEK",
      "JPY",
    ]);
  });

  it("lists each code once, also the current one when it already leads", () => {
    const codes = currencyCodes(["NOK", "SEK"], "SEK");
    expect(codes.slice(0, 2)).toEqual(["NOK", "SEK"]);
    expect(new Set(codes).size).toBe(codes.length);
  });

  it("falls back to the leading codes where the platform has no currency list", () => {
    vi.stubGlobal("Intl", { ...Intl, supportedValuesOf: undefined });
    expect(currencyCodes(FALLBACK_LEADING_CURRENCIES, "NOK")).toEqual([
      "EUR",
      "USD",
      "GBP",
      "CHF",
      "NOK",
    ]);
  });

  it("offers the whole ISO list after the leading codes", () => {
    const codes = currencyCodes(["NOK"]);
    expect(codes).toContain("JPY");
    expect(codes).toContain("VND");
  });

  it("keeps the code a record already carries, even one the platform does not list", () => {
    expect(currencyCodes(["NOK", "SEK"], "ZZZ")[2]).toBe("ZZZ");
  });
});

describe("currencyOptions", () => {
  it("uses the code as its own label", () => {
    expect(
      currencyOptions(["NOK"]).find((option) => option.value === "JPY"),
    ).toEqual({ value: "JPY", label: "JPY" });
  });
});
