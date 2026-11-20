// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { winProbabilityFault } from "./wholenumber";

describe("a typed win probability", () => {
  // One reader for the stage editor and the deal field: Number() took "50.0",
  // "1e1" and "0x1F" as whole percents where the deal field refused them.
  it.each(["50.0", "1e1", "0x1F", "101", "-1", "12.5"])(
    "refuses %j",
    (typed) => {
      expect(winProbabilityFault(typed, "en", "Probability")).toBeDefined();
    },
  );

  it.each(["", "0", "40", "100", " 7 "])("takes %j", (typed) => {
    expect(winProbabilityFault(typed, "en", "Probability")).toBeUndefined();
  });
});
