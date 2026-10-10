import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import type { CreateField } from "./create";
import { groupValue } from "./recordfieldvalues";

const t = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);

const valueGroup: CreateField[] = [
  { key: "amount", label: "create.amount", type: "number" },
  { key: "expected_arr", label: "deal.expectedArr", type: "number" },
  { key: "currency", label: "create.currency", type: "text" },
];

describe("groupValue for a figure beside its currency", () => {
  it("writes the figure with separators and the code inside it, once", () => {
    const shown = groupValue(
      valueGroup,
      { amount: 150000000, currency: "JPY" },
      t,
      "en",
    );
    expect(shown).toBe("Value: JP¥150,000,000");
    expect(shown).not.toContain("JPY");
  });

  it("scales by the currency's own minor units", () => {
    const shown = groupValue(
      valueGroup,
      { amount: 1234.5, expected_arr: 12, currency: "EUR" },
      t,
      "en",
    );
    expect(shown).toBe("Value: €1,234.50 · Expected ARR: €12.00");
  });

  it("keeps the plain reading when no currency is known", () => {
    expect(groupValue(valueGroup, { amount: 5 }, t, "en")).toBe("Value: 5");
  });
});
