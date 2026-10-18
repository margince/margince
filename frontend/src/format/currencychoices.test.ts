// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { currencyCodes, currencyOptions } from "./currencychoices";

describe("currencyCodes", () => {
  it("offers the currencies the workspace prices in, not only four", () => {
    const codes = currencyCodes();
    expect(codes).toContain("JPY");
    expect(codes).toContain("NOK");
    expect(codes).toContain("VND");
  });

  it("leads with the four most used and lists each code once", () => {
    const codes = currencyCodes();
    expect(codes.slice(0, 4)).toEqual(["EUR", "USD", "GBP", "CHF"]);
    expect(new Set(codes).size).toBe(codes.length);
  });

  it("keeps the code a record already carries, even one the platform does not list", () => {
    expect(currencyCodes("ZZZ")[4]).toBe("ZZZ");
  });

  it("does not repeat the current code when it is already offered", () => {
    expect(currencyCodes("JPY").filter((code) => code === "JPY")).toHaveLength(
      1,
    );
  });
});

describe("currencyOptions", () => {
  it("uses the code as its own label", () => {
    expect(currencyOptions().find((option) => option.value === "NOK")).toEqual({
      value: "NOK",
      label: "NOK",
    });
  });
});
