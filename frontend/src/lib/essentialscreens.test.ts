// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { paintsFromEssentialCopy } from "./essentialscreens";

describe("the screens that draw from the essential catalog alone", () => {
  it("are Home, under every address it answers, and Analytics", () => {
    for (const hash of ["", "#/", "#/home", "#/brief", "#/analytics/forecast"])
      expect(paintsFromEssentialCopy(hash), hash).toBe(true);
  });

  it("leave every other screen waiting for the complete catalog", () => {
    for (const hash of ["#/deals", "#/settings", "#/contacts/abc"])
      expect(paintsFromEssentialCopy(hash), hash).toBe(false);
  });
});
