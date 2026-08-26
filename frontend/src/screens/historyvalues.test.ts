import { describe, expect, it } from "vitest";
import { MONEY_ABSENT } from "../format/format";
import { historyValue, isMinorUnitField } from "./historyvalues";

describe("historyValue", () => {
  it("renders a minor-unit amount as money, not as its stored integer", () => {
    const rendered = historyValue("amount_minor", "2500000", "EUR", "en");
    expect(rendered).toContain("25,000");
    expect(rendered).not.toBe("2500000");
  });

  // The scale is the currency's. A dong amount divided by a hundred would show
  // a reader a hundredth of the figure the record holds, which is exactly the
  // misreading this formatting exists to remove.
  it("keeps every digit of a currency that has no minor unit", () => {
    expect(historyValue("amount_minor", "2500000", "VND", "en")).toContain(
      "2,500,000",
    );
  });

  // Three digits, the other direction: a dinar's minor unit is a thousandth.
  it("scales a three-digit currency by a thousand", () => {
    expect(historyValue("amount_minor", "2500000", "KWD", "en")).toContain(
      "2,500.000",
    );
  });

  it("says nothing rather than guessing a currency it was not given", () => {
    expect(historyValue("amount_minor", "2500000", null, "en")).toBe(
      MONEY_ABSENT,
    );
  });

  it("leaves a value that is not an integer count as the spine recorded it", () => {
    expect(historyValue("amount_minor", "about 25k", "EUR", "en")).toBe(
      "about 25k",
    );
  });

  it("passes an ordinary field through untouched", () => {
    expect(historyValue("name", "Globex Renewal", "EUR", "en")).toBe(
      "Globex Renewal",
    );
  });

  // The diff draws its own wording for a value the record does not hold, so an
  // absent value has to stay distinguishable from empty text.
  it("keeps an absent value absent", () => {
    expect(historyValue("amount_minor", null, "EUR", "en")).toBeNull();
    expect(historyValue("name", undefined, "EUR", "en")).toBeNull();
    expect(historyValue("name", "", "EUR", "en")).toBe("");
  });
});

describe("isMinorUnitField", () => {
  it("recognises a money column by the suffix the contract spells it with", () => {
    expect(isMinorUnitField("amount_minor")).toBe(true);
    expect(isMinorUnitField("cf_budget_minor")).toBe(true);
    expect(isMinorUnitField("amount")).toBe(false);
    expect(isMinorUnitField("minor_reason")).toBe(false);
  });
});
