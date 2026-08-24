import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// The record pages read their 360s under keys the LINK sections have to
// invalidate after a write. Nothing in the type system connects the two: a page
// renaming its key leaves the section writing successfully and showing the
// state from before, which reads as a write that did not happen.
//
// So the keys are asserted against the pages that own them. A rename now fails
// here rather than in front of a user.
const KEY_OWNERS = [
  {
    page: "src/screens/project360.tsx",
    key: '["project", id, "360"]',
    writers: ["src/screens/projectcompanies.tsx"],
    invalidates: '["project", projectId, "360"]',
  },
  {
    page: "src/screens/company360.tsx",
    key: '["organization360", id]',
    writers: ["src/screens/companyprojects.tsx"],
    invalidates: '["organization360", organizationId]',
  },
  {
    page: "src/screens/person360.tsx",
    key: '["person360", id]',
    writers: ["src/screens/personprojects.tsx"],
    invalidates: '["person360", personId]',
  },
];

describe("the project-link sections invalidate the keys their pages read", () => {
  for (const owner of KEY_OWNERS) {
    it(`${owner.page} still reads under ${owner.key}`, () => {
      expect(readFileSync(owner.page, "utf8")).toContain(owner.key);
    });
    for (const writer of owner.writers) {
      it(`${writer} invalidates it`, () => {
        expect(readFileSync(writer, "utf8")).toContain(owner.invalidates);
      });
    }
  }
});
