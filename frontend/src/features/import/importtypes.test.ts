// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { isImportObject } from "./importtypes";

describe("isImportObject", () => {
  it("accepts every type the contract names and nothing else", () => {
    for (const object of ["company", "contact", "lead", "deal", "activity"]) {
      expect(isImportObject(object)).toBe(true);
    }
    for (const other of ["note", "", "toString", "constructor", 1, null]) {
      expect(isImportObject(other)).toBe(false);
    }
  });
});
