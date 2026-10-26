import { describe, expect, it } from "vitest";
import {
  closeDayRange,
  encodeRange,
  fiscalQuarter,
  parseDayRange,
  parseMinorRange,
  rangeQueryParams,
  THIS_QUARTER,
} from "./dealranges";
import { dealsQueryParams, totalsWithheldBecause } from "./deals.queries";

describe("a range held as one dial value", () => {
  it("joins two ends and leaves a blank end open", () => {
    expect(encodeRange("2026-10-01", "2026-12-31")).toBe(
      "2026-10-01..2026-12-31",
    );
    expect(encodeRange("", "2026-12-31")).toBe("..2026-12-31");
    expect(encodeRange("", "")).toBe("");
  });

  it("reads a day range back, open at either end", () => {
    expect(parseDayRange("2026-10-01..")).toEqual({
      from: "2026-10-01",
      to: undefined,
    });
    expect(parseDayRange("..2026-12-31")).toEqual({
      from: undefined,
      to: "2026-12-31",
    });
  });

  it("refuses a day that does not exist, a backwards range and no range", () => {
    expect(parseDayRange("2026-02-30..")).toBeUndefined();
    expect(parseDayRange("2026-12-31..2026-10-01")).toBeUndefined();
    expect(parseDayRange("..")).toBeUndefined();
    expect(parseDayRange(THIS_QUARTER)).toBeUndefined();
  });

  it("reads a minor-unit range and refuses anything but whole amounts", () => {
    expect(parseMinorRange("2000000..")).toEqual({
      min: 2000000,
      max: undefined,
    });
    expect(parseMinorRange("1.5..")).toBeUndefined();
    expect(parseMinorRange("-5..")).toBeUndefined();
    expect(parseMinorRange("500..100")).toBeUndefined();
  });
});

describe("this quarter", () => {
  it("is the calendar quarter when the fiscal year starts in January", () => {
    expect(fiscalQuarter("2026-10-26", 1)).toEqual({
      from: "2026-10-01",
      to: "2026-12-31",
    });
  });

  it("follows a fiscal year that starts in April", () => {
    expect(fiscalQuarter("2026-10-26", 4)).toEqual({
      from: "2026-10-01",
      to: "2026-12-31",
    });
    expect(fiscalQuarter("2026-06-15", 4)).toEqual({
      from: "2026-04-01",
      to: "2026-06-30",
    });
  });

  it("crosses the calendar year when the fiscal quarter does", () => {
    expect(fiscalQuarter("2027-01-10", 11)).toEqual({
      from: "2026-11-01",
      to: "2027-01-31",
    });
  });

  it("stays a word in the dial and becomes days only for the request", () => {
    expect(closeDayRange(THIS_QUARTER, "2026-02-14", 1)).toEqual({
      from: "2026-01-01",
      to: "2026-03-31",
    });
  });
});

describe("the request the dials send", () => {
  const base = {
    pipelineId: "",
    q: "",
    sort: "",
    includeArchived: false,
  };

  it("sends each bound as the wire names it", () => {
    const filters = {
      close_day: THIS_QUARTER,
      value: "2000000..5000000",
      currency: "EUR",
      country: "DE",
    };
    expect(
      dealsQueryParams({
        ...base,
        filters,
        closeDay: closeDayRange(THIS_QUARTER, "2026-10-26", 1),
      }),
    ).toMatchObject({
      close_from: "2026-10-01",
      close_to: "2026-12-31",
      value_min: 2000000,
      value_max: 5000000,
      currency: "EUR",
      country: "DE",
    });
  });

  it("sends no bound the dials do not hold", () => {
    expect(rangeQueryParams({}, undefined)).toEqual({
      close_from: undefined,
      close_to: undefined,
      value_min: undefined,
      value_max: undefined,
      currency: undefined,
      country: undefined,
    });
  });

  it("withholds the stage totals, which cannot narrow by these dials", () => {
    expect(totalsWithheldBecause({ ...base, filters: { country: "DE" } })).toBe(
      "deals.totalsNoRangeFilter",
    );
    expect(totalsWithheldBecause({ ...base, filters: {} })).toBeUndefined();
  });
});
