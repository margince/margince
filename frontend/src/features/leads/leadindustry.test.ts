import { describe, expect, it } from "vitest";
import { leadIndustry } from "./leadindustry";
import { lead } from "./leads.testkit";

describe("a lead's industry", () => {
  it("is the linked company's while the company has one", () => {
    expect(
      leadIndustry({
        ...lead,
        industry: "software",
        company_facts: { industry: "manufacturing" },
      }),
    ).toEqual({ code: "manufacturing", from: "company" });
  });

  it("is the lead's own while the company has none", () => {
    expect(
      leadIndustry({
        ...lead,
        industry: "software",
        company_facts: { industry: null },
      }),
    ).toEqual({ code: "software", from: "lead" });
  });

  it("is unknown when neither holds one", () => {
    expect(leadIndustry(lead)).toBeNull();
  });
});
