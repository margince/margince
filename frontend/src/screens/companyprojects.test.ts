import { describe, expect, it } from "vitest";

import { COMPANY_ROLES, roleKey } from "./companyprojects";

describe("the roles a company holds on a project", () => {
  it("leads with partner, because the first role is the picker's default", () => {
    // A project already has its customer by the time anybody reaches the
    // section — creating one attaches its company as the customer — so a
    // company joining afterwards is a partner or a subcontractor. Defaulting to
    // customer hands the project two, which is what the reports group by and
    // what organization_id resolves to.
    expect(COMPANY_ROLES[0]).toBe("partner");
    expect(COMPANY_ROLES).toContain("customer");
  });

  it("names every role it offers", () => {
    // A role with no label renders as a blank option, which is a choice a
    // reader cannot make.
    for (const role of COMPANY_ROLES) {
      expect(roleKey(role)).toMatch(/^projectRole\./);
    }
  });

  it("does not silently call an unknown role a partner", () => {
    // roleKey falls back, and the fallback is only safe while every role it can
    // be handed is in the list above.
    expect(new Set(COMPANY_ROLES.map(roleKey)).size).toBe(COMPANY_ROLES.length);
  });
});
