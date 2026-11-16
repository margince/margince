// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { bootsOnEssentialCopy } from "./essentialscreens";

describe("the screens a cold load paints from the essential catalog", () => {
  it("are Home, under every address it answers, and Analytics", () => {
    for (const hash of ["", "#/", "#/home", "#/brief", "#/analytics/forecast"])
      expect(bootsOnEssentialCopy(hash), hash).toBe(true);
  });

  it("leave every other screen on the complete catalog", () => {
    for (const hash of ["#/deals", "#/settings", "#/contacts/abc"])
      expect(bootsOnEssentialCopy(hash), hash).toBe(false);
  });
});
