import { describe, expect, it } from "vitest";
import { passportSnippet } from "./settings.passportuse";

describe("passportSnippet", () => {
  it("joins the API base and the path with one slash", () => {
    for (const base of [
      "https://crm.example.test/v1",
      "https://crm.example.test/v1/",
    ]) {
      expect(passportSnippet("curl", base)).toContain(
        '"https://crm.example.test/v1/companies?limit=5"',
      );
    }
  });
});
