import { describe, expect, it } from "vitest";
import { mapDealCreate } from "./deals";

describe("mapDealCreate currency", () => {
  // A currency with no figure is half a money value, which the API refuses in
  // its own field names; a deal born with only a name carries no currency.
  it("sends no currency when neither figure was typed", () => {
    const body = mapDealCreate(
      { name: "x", stage_id: "s-1", currency: "EUR" },
      "p-1",
    );
    expect(body.currency).toBeNull();
    expect(body.amount_minor).toBeNull();
  });

  it("keeps the currency beside a typed recurring figure", () => {
    const body = mapDealCreate(
      { name: "x", stage_id: "s-1", expected_arr: "12", currency: "USD" },
      "p-1",
    );
    expect(body.currency).toBe("USD");
    expect(body.expected_arr_minor).toBe(1_200);
  });
});
