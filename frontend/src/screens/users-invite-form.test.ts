// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { nameFromEmail } from "./users-invite-form";

// The name a member starts under when only an address was asked for: it has
// to read as a name in the roster, not as the left half of an address.
describe("nameFromEmail", () => {
  it("reads the separators people write between their names as spaces", () => {
    expect(nameFromEmail("ada.byron@example.com")).toBe("Ada Byron");
    expect(nameFromEmail("ada_byron@example.com")).toBe("Ada Byron");
    expect(nameFromEmail("ada-byron@example.com")).toBe("Ada Byron");
  });

  it("capitalises a single word and keeps the rest as typed", () => {
    expect(nameFromEmail("ada@example.com")).toBe("Ada");
    expect(nameFromEmail("aDa@example.com")).toBe("ADa");
  });

  it("gives nothing for an address with nothing before the @", () => {
    expect(nameFromEmail("@example.com")).toBe("");
    expect(nameFromEmail("   ")).toBe("");
  });
});
