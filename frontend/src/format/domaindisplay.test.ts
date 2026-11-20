import { describe, expect, it } from "vitest";
import { displayDomain } from "./domaindisplay";

describe("displayDomain", () => {
  it("shows the Unicode form of a stored punycode domain", () => {
    expect(displayDomain("xn--mnchen-3ya.de")).toBe("münchen.de");
    expect(displayDomain("xn--bcher-kva.example")).toBe("bücher.example");
    expect(displayDomain("xn--e1afmkfd.xn--p1ai")).toBe("пример.рф");
  });

  it("leaves a plain domain and an unreadable label as stored", () => {
    expect(displayDomain("acme.example")).toBe("acme.example");
    expect(displayDomain("xn--!!.de")).toBe("xn--!!.de");
  });
});
