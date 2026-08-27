import { describe, expect, it } from "vitest";
import { MONEY_ABSENT } from "../format/format";
import {
  type HistoryValueCtx,
  historyValue,
  isMinorUnitField,
} from "./historyvalues";

const ctx = (overrides: Partial<HistoryValueCtx> = {}): HistoryValueCtx => ({
  currency: "EUR",
  locale: "en",
  zone: "Asia/Ho_Chi_Minh",
  ...overrides,
});

describe("historyValue", () => {
  it("renders a minor-unit amount as money, not as its stored integer", () => {
    const rendered = historyValue("amount_minor", "2500000", ctx());
    expect(rendered).toContain("25,000");
    expect(rendered).not.toBe("2500000");
  });

  // The scale is the currency's. A dong amount divided by a hundred would show
  // a reader a hundredth of the figure the record holds, which is exactly the
  // misreading this formatting exists to remove.
  it("keeps every digit of a currency that has no minor unit", () => {
    expect(
      historyValue("amount_minor", "2500000", ctx({ currency: "VND" })),
    ).toContain("2,500,000");
  });

  // Three digits, the other direction: a dinar's minor unit is a thousandth.
  it("scales a three-digit currency by a thousand", () => {
    expect(
      historyValue("amount_minor", "2500000", ctx({ currency: "KWD" })),
    ).toContain("2,500.000");
  });

  it("says nothing rather than guessing a currency it was not given", () => {
    expect(
      historyValue("amount_minor", "2500000", ctx({ currency: null })),
    ).toBe(MONEY_ABSENT);
  });

  it("leaves a value that is not an integer count as the spine recorded it", () => {
    expect(historyValue("amount_minor", "about 25k", ctx())).toBe("about 25k");
  });

  it("passes an ordinary field through untouched", () => {
    expect(historyValue("name", "Globex Renewal", ctx())).toBe(
      "Globex Renewal",
    );
  });

  // The diff draws its own wording for a value the record does not hold, so an
  // absent value has to stay distinguishable from empty text.
  it("keeps an absent value absent", () => {
    expect(historyValue("amount_minor", null, ctx())).toBeNull();
    expect(historyValue("name", undefined, ctx())).toBeNull();
    expect(historyValue("name", "", ctx())).toBe("");
  });

  // Money is checked before every other rule: a `*_minor` column is money
  // whatever else its stored value happens to look like, so a non-numeric
  // shape (here, a JSON array) is left as the spine recorded it rather than
  // falling through to the array rule below.
  it("never lets the array rule claim a minor-unit column", () => {
    expect(historyValue("amount_minor", "[]", ctx())).toBe("[]");
  });

  it("renders an ISO timestamp through the shared formatter, in the given zone", () => {
    expect(
      historyValue("changed_at", "2026-08-26T02:12:07.551698Z", ctx()),
    ).toBe("26/08/2026, 09:12");
  });

  it("renders a JSON array as its items joined with a comma", () => {
    expect(historyValue("tags", '["customer"]', ctx())).toBe("customer");
    expect(historyValue("tags", '["customer","partner"]', ctx())).toBe(
      "customer, partner",
    );
  });

  it("renders an empty JSON array as the catalog's word for nothing set", () => {
    expect(historyValue("tags", "[]", ctx())).toBe("nothing set");
  });

  it("resolves a bare uuid to a name when the resolver names it", () => {
    const withResolver = ctx({
      nameOf: (id) =>
        id === "01a03bcb-0412-7034-8443-38ea77ed4a51" ? "Akeneo" : undefined,
    });
    expect(
      historyValue(
        "owner_id",
        "01a03bcb-0412-7034-8443-38ea77ed4a51",
        withResolver,
      ),
    ).toBe("Akeneo");
  });

  it("leaves a bare uuid EXACTLY as stored when the resolver cannot name it", () => {
    const id = "01a03bd4-9c9a-7987-b6c7-f5caaada25e3";
    const unresolved = ctx({ nameOf: () => undefined });
    expect(historyValue("owner_id", id, unresolved)).toBe(id);
    // And with no resolver wired at all.
    expect(historyValue("owner_id", id, ctx())).toBe(id);
  });

  it("renders a path or URL as its last segment", () => {
    expect(
      historyValue(
        "logo_path",
        "dataset:margince-demo-database/datasets/v1/logos/akeneo.com.png",
        ctx(),
      ),
    ).toBe("akeneo.com.png");
  });

  it("never mistakes a timestamp's zone offset for a path", () => {
    // A naive path check ("contains a slash") would also match nothing here,
    // but the ISO rule must win first regardless: this pins the ORDER.
    expect(
      historyValue("changed_at", "2026-08-26T09:12:02.503817+07:00", ctx()),
    ).toBe("26/08/2026, 09:12");
  });

  it("returns a plain string unchanged", () => {
    expect(historyValue("stage", "Enterprise", ctx())).toBe("Enterprise");
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
