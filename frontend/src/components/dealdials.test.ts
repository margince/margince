import { describe, expect, it } from "vitest";
import { THIS_QUARTER } from "../lib/dealranges";
import { admittedDealFilters } from "./dealdials";

// A range dial from an address or a saved view is sent only in a shape the
// wire takes. The row a reader sees and the request then agree.
describe("admittedDealFilters over the range dials", () => {
  it("drops a stored value the wire would refuse, and keeps a sound one", () => {
    expect(
      admittedDealFilters({
        close_day: "2026-12-31..2026-10-01",
        value: "abc..",
        currency: "eur",
        country: "DE",
      }),
    ).toEqual({ country: "DE" });
    const sound = { close_day: THIS_QUARTER, value: "100..", currency: "USD" };
    expect(admittedDealFilters(sound)).toBe(sound);
  });
});
