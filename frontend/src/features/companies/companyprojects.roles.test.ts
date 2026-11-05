import { describe, expect, it } from "vitest";

import { de } from "../../i18n/de";
import { en } from "../../i18n/en";
import { vi } from "../../i18n/vi";
import {
  COMPANY_ROLE_LABEL,
  COMPANY_ROLES,
  companyRoleLabel,
} from "./companyprojects";

describe("the roles a company holds on a project", () => {
  it("leads with partner, because the first role is the picker's default", () => {
    // A project already has its customer by the time anybody reaches the
    // section — creating one attaches its company as the customer — so a
    // company joining afterwards is a partner or a subcontractor. Defaulting to
    // customer hands the project two, which is what the reports group by and
    // what company_id resolves to.
    expect(COMPANY_ROLES[0]).toBe("partner");
    expect(COMPANY_ROLES).toContain("customer");
  });

  it("names every role in every language, each with its own words", () => {
    for (const catalog of [en, de, vi]) {
      const words = COMPANY_ROLES.map(
        (role) => catalog[COMPANY_ROLE_LABEL[role]],
      );
      expect(words.every(Boolean)).toBe(true);
      expect(new Set(words).size).toBe(COMPANY_ROLES.length);
    }
  });

  it("gives an unknown role no words rather than another role's", () => {
    expect(companyRoleLabel("customer")).toBe("projectRole.customer");
    expect(companyRoleLabel("reseller")).toBeUndefined();
  });
});
