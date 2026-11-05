import { describe, expect, it } from "vitest";
import { currencyBeside } from "./dealcurrency";

describe("currencyBeside", () => {
  it("travels with any typed figure", () => {
    expect(currencyBeside("USD", "", "12")).toBe("USD");
    expect(currencyBeside("USD", "5", undefined)).toBe("USD");
  });

  it("is null when no figure was typed", () => {
    expect(currencyBeside("USD", "", undefined)).toBeNull();
    expect(currencyBeside("USD")).toBeNull();
  });
});
