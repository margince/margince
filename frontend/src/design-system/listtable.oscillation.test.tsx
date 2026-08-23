import { describe, expect, it } from "vitest";
import { widthWorthAdopting } from "./listtable";

/**
 * The scroller's width is not a single number on a platform with classic
 * scrollbars: showing a vertical bar takes width, and the widths this table
 * computes are what decide whether the bar shows. These are the readings that
 * arrive, and which of them the table is allowed to act on.
 */
describe("widthWorthAdopting", () => {
  it("adopts the first real reading", () => {
    expect(widthWorthAdopting(900, 0, 0)).toBe(true);
  });

  it("adopts a genuine resize", () => {
    expect(widthWorthAdopting(1200, 900, 885)).toBe(true);
  });

  it("refuses a reading the table already holds", () => {
    expect(widthWorthAdopting(900, 900, 885)).toBe(false);
  });

  it("refuses a reading that returns to the width of a render ago", () => {
    // The oscillation: 900 → 885 → 900 → … Left unrefused this is the loop
    // React aborts with "Maximum update depth exceeded", taking the list with
    // it.
    expect(widthWorthAdopting(900, 885, 900)).toBe(false);
  });
});
