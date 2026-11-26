// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { stable as appStable } from "../../src/format/collate";
import { stable } from "./stable.mjs";

describe("the scripts' stable", () => {
  it("orders every pair as the app's stable does", () => {
    const words = [
      "zeta",
      "Zeta",
      "äpfel",
      "apfel",
      "10",
      "9",
      "",
      "a\u{1F600}",
      "a￿",
      "-x",
    ];
    for (const a of words) {
      for (const b of words) {
        expect(Math.sign(stable(a, b))).toBe(Math.sign(appStable(a, b)));
      }
    }
  });
});
