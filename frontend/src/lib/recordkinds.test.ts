import { describe, expect, it } from "vitest";

import { isExtensionEntityKind, recordRoute } from "./entity";
import { recordKindLabel } from "./recordkinds";

describe("isExtensionEntityKind", () => {
  it("accepts the ext:<unit>:<kind> spelling and nothing else", () => {
    expect(isExtensionEntityKind("ext:visit-log:visit")).toBe(true);
    expect(isExtensionEntityKind("ext:a:b_2")).toBe(true);
    for (const kind of [
      "contact",
      "ext:",
      "ext:a",
      "ext:a:",
      "ext::b",
      "ext:A:b",
      "ext:a:1b",
      "ext:a:b:c",
    ]) {
      expect(isExtensionEntityKind(kind)).toBe(false);
    }
  });
});

describe("recordRoute", () => {
  it("names no page for an extension kind, which has none yet", () => {
    expect(
      recordRoute(
        "ext:visit-log:visit",
        "0197a000-0000-7000-8000-000000000001",
      ),
    ).toBeUndefined();
  });
});

describe("recordKindLabel", () => {
  const directory = [{ name: "ext:acme:visit", display_name: "Site visit" }];

  it("prints the display name the extension declared", () => {
    expect(recordKindLabel("ext:acme:visit", directory)).toBe("Site visit");
  });

  it("falls back to the kind's own name for one nobody declares", () => {
    expect(recordKindLabel("ext:gone:ledger_account", directory)).toBe(
      "ledger account",
    );
  });
});
