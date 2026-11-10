// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { READ_BY_AN_OUTSIDER } from "../../scripts/lib/german-catalogs";

// The catalogs are held by census/rules/address-register.census.ts.

describe("German copy speaks to its reader as du", () => {
  it("every exempt prefix says who reads it", () => {
    const silent = Object.entries(READ_BY_AN_OUTSIDER)
      .filter(([, why]) => why.trim() === "")
      .map(([prefix]) => prefix);
    expect(silent).toEqual([]);
  });
});
