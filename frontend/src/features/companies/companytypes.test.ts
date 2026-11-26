import { describe, expect, it } from "vitest";

import { companyTypeLabel } from "./companytypes";

describe("companyTypeLabel", () => {
  const directory = [{ value: "ext:acme:reseller", label: "Reseller" }];

  it("prints the label the extension declared", () => {
    expect(companyTypeLabel("ext:acme:reseller", directory)).toBe("Reseller");
  });

  it("falls back to the type's own name for a value nobody declares", () => {
    expect(companyTypeLabel("ext:gone:portfolio_holding", directory)).toBe(
      "portfolio holding",
    );
  });
});
