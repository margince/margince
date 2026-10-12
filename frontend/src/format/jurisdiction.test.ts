import { describe, expect, it } from "vitest";
import { jurisdictionName, parsePeriod } from "./jurisdiction";

describe("jurisdictionName", () => {
  it("names the country in the reader's language", () => {
    expect(jurisdictionName("de", "en")).toBe("Germany");
    expect(jurisdictionName("de", "de")).toBe("Deutschland");
  });

  it("falls back to the code for one the platform does not name", () => {
    expect(jurisdictionName("zz", "en")).toMatch(/ZZ|Unknown Region/);
  });
});

describe("parsePeriod", () => {
  it("reads the parts the server spells", () => {
    expect(parsePeriod("P6Y")).toEqual({ years: 6, months: 0, days: 0 });
    expect(parsePeriod("P1Y6M2D")).toEqual({ years: 1, months: 6, days: 2 });
    expect(parsePeriod("P0D")).toEqual({ years: 0, months: 0, days: 0 });
  });

  it("refuses any other shape", () => {
    expect(parsePeriod("PT72H")).toBeNull();
    expect(parsePeriod("P")).toBeNull();
    expect(parsePeriod("six years")).toBeNull();
  });
});
