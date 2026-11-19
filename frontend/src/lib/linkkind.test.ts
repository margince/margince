import { describe, expect, it } from "vitest";
import { linkKind } from "./linkkind";

describe("linkKind", () => {
  it.each([
    ["https://docs.google.com/document/d/abc/edit", "docs.kind.docs"],
    [
      "https://docs.google.com/spreadsheets/d/abc/edit#gid=0",
      "docs.kind.sheets",
    ],
    ["https://DOCS.google.com/presentation/d/abc/edit", "docs.kind.slides"],
    ["https://docs.google.com/forms/d/e/abc/viewform", "docs.kind.forms"],
    ["https://drive.google.com/file/d/abc/view", "docs.kind.drive"],
    ["https://docs.google.com/a/acme.example/", "docs.kind.drive"],
    ["https://docs.google.com/__proto__", "docs.kind.drive"],
    ["https://docs.google.com/constructor", "docs.kind.drive"],
    ["https://acme.example/pricing", "docs.kind.web"],
    ["not an address", "docs.kind.web"],
  ])("names %s as %s", (address, kind) => {
    expect(linkKind(address)).toBe(kind);
  });
});
