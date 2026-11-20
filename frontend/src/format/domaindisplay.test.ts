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

  it("keeps punycode for a label that mixes scripts", () => {
    // Cyrillic a followed by Latin pple
    expect(displayDomain("xn--pple-43d.com")).toBe("xn--pple-43d.com");
  });

  it("keeps punycode for a decoded bidi control", () => {
    // U+202E right-to-left override
    expect(displayDomain("xn--acme-yd7a.com")).toBe("xn--acme-yd7a.com");
  });

  it("keeps the stored form when the decoder overflows", () => {
    const stored = `xn--${"9".repeat(40)}.de`;
    expect(displayDomain(stored)).toBe(stored);
  });
});
