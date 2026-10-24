// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";

import { contactFirstName } from "./contactfirstname";

describe("contactFirstName", () => {
  it("uses the stored first name of a family-name-first contact", () => {
    expect(
      contactFirstName({ full_name: "Lê Thị Mai", first_name: "Mai" }),
    ).toBe("Mai");
  });

  it("uses the whole display name when no first name is stored", () => {
    expect(
      contactFirstName({ full_name: "Lê Thị Mai", first_name: null }),
    ).toBe("Lê Thị Mai");
    expect(contactFirstName({ full_name: "Lê Thị Mai" })).toBe("Lê Thị Mai");
  });
});
