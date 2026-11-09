// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";

import { byCodeUnit } from "./codeunitorder";

describe("byCodeUnit", () => {
  it("orders exactly as a bare sort does, case, accents and astral characters included", () => {
    const words = [
      "zeta",
      "Zeta",
      "äpfel",
      "apfel",
      "Ärger",
      "10",
      "9",
      "",
      "a\u{1F600}",
      "a￿",
      "_x",
      "-x",
    ];
    expect([...words].sort(byCodeUnit)).toEqual([...words].sort());
  });
});
