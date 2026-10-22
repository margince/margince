import { describe, expect, it } from "vitest";
import { mapDealCreate } from "./deals";

describe("mapDealCreate currency", () => {
  // A currency with no figure is half a money value. The API refuses it in
  // its own field names, so a name-only deal carries no currency.
  it("sends no currency when neither figure was typed", () => {
    const body = mapDealCreate(
      { name: "x", stage_id: "s-1", currency: "EUR" },
      "p-1",
      "EUR",
    );
    expect(body.currency).toBeNull();
    expect(body.amount_minor).toBeNull();
  });

  it("keeps the currency beside a typed recurring figure", () => {
    const body = mapDealCreate(
      { name: "x", stage_id: "s-1", expected_arr: "12", currency: "USD" },
      "p-1",
      "EUR",
    );
    expect(body.currency).toBe("USD");
    expect(body.expected_arr_minor).toBe(1_200);
  });

  // The default is the first code the picker offered, so what is sent is
  // what the reader was shown.
  it("prices an empty currency field in the default it is given", () => {
    const body = mapDealCreate(
      { name: "x", stage_id: "s-1", amount: "5" },
      "p-1",
      "NOK",
    );
    expect(body.currency).toBe("NOK");
    expect(body.amount_minor).toBe(500);
  });
});
