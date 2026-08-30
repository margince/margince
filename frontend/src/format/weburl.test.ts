// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { linkedinUrl, webUrl } from "./weburl";

// The guard three surfaces share: a company link on a reading chip, a
// provider's source document in the person drawer, and a custom field holding
// whatever an import wrote. Each draws something different for a refused
// value; none of them may decide the schemes for itself, which is why this
// file is where the scheme list is proven.
describe("what may become a link", () => {
  it("admits the two schemes a web address can be, and normalizes it", () => {
    expect(webUrl("https://wiki.example.com/globex")?.href).toBe(
      "https://wiki.example.com/globex",
    );
    expect(webUrl("http://erp.internal/orders/44")?.href).toBe(
      "http://erp.internal/orders/44",
    );
    // A scheme is matched by what it IS, not how it was typed.
    expect(webUrl("HTTPS://example.com/a")?.protocol).toBe("https:");
  });

  it("refuses a clickable payload, whatever it is dressed as", () => {
    for (const value of [
      "javascript:alert(1)",
      "  javascript:alert(1)",
      "JavaScript:alert(1)",
      "data:text/html;base64,PHNjcmlwdD4=",
      "vbscript:msgbox",
      "file:///etc/passwd",
    ]) {
      expect(webUrl(value), value).toBeNull();
    }
  });

  it("refuses anything that is not an absolute destination", () => {
    // These would resolve against our own origin, which is never where a
    // record's link points — the reader would be sent to a page of ours.
    for (const value of [
      "example.com",
      "www.example.com/pricing",
      "/orders/4",
      "",
      "not a url at all",
    ]) {
      expect(webUrl(value), value).toBeNull();
    }
  });

  it("leaves a scheme that is neither web nor executable as text", () => {
    // `mailto:` is a real destination and still not a link this guard makes:
    // a surface that wants one asks for it deliberately.
    expect(webUrl("mailto:sofia@example.com")).toBeNull();
  });
});

// A link's LABEL is a claim about where it goes. These labels are fixed words
// ("LinkedIn", "Open profile") rather than the address, and the value behind
// them can be written by a crawl, a connector or a paste — so the host has to
// be checked before the word is allowed to stand over an anchor.
describe("what may be shown as a LinkedIn profile", () => {
  it("admits the profile hosts LinkedIn actually serves", () => {
    for (const address of [
      "https://linkedin.com/in/dana",
      "https://www.linkedin.com/in/dana",
      "https://de.linkedin.com/in/dana",
      "https://lnkd.in/abc123",
    ]) {
      expect(linkedinUrl(address)?.href).toBeTruthy();
    }
  });

  it("refuses another host under LinkedIn's name", () => {
    // The phishing shapes: a plain impostor, a lookalike that ENDS in the
    // right characters without being the right host, and one that merely
    // contains it.
    for (const address of [
      "https://attacker.example/login",
      "https://notlinkedin.com/in/dana",
      "https://linkedin.com.attacker.example/in/dana",
      "https://attacker.example/linkedin.com/in/dana",
    ]) {
      expect(linkedinUrl(address)).toBeNull();
    }
  });

  it("refuses what webUrl already refuses", () => {
    // The host check is added to the scheme check, never instead of it.
    for (const address of [
      "javascript:alert(1)",
      "data:text/html,<script>alert(1)</script>",
      "linkedin.com/in/dana",
      "",
    ]) {
      expect(linkedinUrl(address)).toBeNull();
    }
  });
});
