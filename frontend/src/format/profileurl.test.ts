// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { normalizeProfileUrl, profileUrlLabel } from "./profileurl";

// The product's position is that a profile is read by the PERSON, in their own
// browser, and that the software only stores what they bring back. So the one
// thing proven here is that this file changes a string's shape and never
// reaches the network — every case below is a pure input/output pair, which is
// the only shape a fetch could not hide in.
describe("normalizing a pasted profile address", () => {
  it("prepends https to the bare host a browser shows", () => {
    expect(normalizeProfileUrl("linkedin.com/in/jdoe")).toBe(
      "https://linkedin.com/in/jdoe",
    );
    expect(normalizeProfileUrl("www.linkedin.com/in/jdoe")).toBe(
      "https://www.linkedin.com/in/jdoe",
    );
  });

  it("keeps a scheme the person typed rather than upgrading it", () => {
    // Silently promoting http to https would claim a certificate nobody saw.
    expect(normalizeProfileUrl("http://example.com/team/jdoe")).toBe(
      "http://example.com/team/jdoe",
    );
  });

  it("drops the query and fragment a copied address carries", () => {
    expect(
      normalizeProfileUrl(
        "https://linkedin.com/in/jdoe?originalSubdomain=de&trk=nav#about",
      ),
    ).toBe("https://linkedin.com/in/jdoe");
  });

  it("drops the trailing slash that separates a copied address from a typed one", () => {
    expect(normalizeProfileUrl("https://linkedin.com/in/jdoe/")).toBe(
      "https://linkedin.com/in/jdoe",
    );
    // The root's own slash is the path, not a trailing one.
    expect(normalizeProfileUrl("https://linkedin.com/")).toBe(
      "https://linkedin.com/",
    );
  });

  it("trims the whitespace a paste brings with it", () => {
    expect(normalizeProfileUrl("  https://linkedin.com/in/jdoe  ")).toBe(
      "https://linkedin.com/in/jdoe",
    );
    expect(normalizeProfileUrl("   ")).toBe("");
  });

  it("hands back what was typed when the value is not an address", () => {
    // The person has to be able to see and correct their own mistake, so a
    // refused value is never mangled into something else.
    expect(normalizeProfileUrl("jdoe")).toBe("jdoe");
    expect(normalizeProfileUrl("javascript:alert(1)")).toBe(
      "javascript:alert(1)",
    );
  });
});

describe("the address as a person reads it", () => {
  it("drops the scheme and www a reader already knows", () => {
    expect(profileUrlLabel("https://www.linkedin.com/in/jdoe")).toBe(
      "linkedin.com/in/jdoe",
    );
    expect(profileUrlLabel("linkedin.com/in/jdoe/")).toBe(
      "linkedin.com/in/jdoe",
    );
  });

  it("shows a host with no path as the host alone", () => {
    expect(profileUrlLabel("https://linkedin.com")).toBe("linkedin.com");
  });

  it("shows an unusable value as its own text", () => {
    expect(profileUrlLabel("  not-a-url  ")).toBe("not-a-url");
  });
});
