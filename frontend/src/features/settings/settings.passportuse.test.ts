import { describe, expect, it } from "vitest";
import { examplePathFor, passportSnippet } from "./settings.passportuse";

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

describe("examplePathFor", () => {
  it("shows a Chat Bridge passport a call it is allowed to make", () => {
    expect(
      passportSnippet(
        "curl",
        "https://crm.example.test/v1",
        examplePathFor(["personal_channels"]),
      ),
    ).toContain('"https://crm.example.test/v1/personal-channels/connection"');
  });
  it("keeps the records read for a passport that may read", () => {
    expect(examplePathFor(["read", "personal_channels"])).toBe(
      "/companies?limit=5",
    );
  });
});
