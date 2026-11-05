// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { formatTokens } from "./tokens";

describe("formatTokens", () => {
  it("spells English markers K, M and B whatever ICU calls them", () => {
    expect(formatTokens(12_345, "en")).toBe("12.3K");
    expect(formatTokens(120_000_000, "en")).toBe("120M");
    expect(formatTokens(1_200_000_000, "en")).toBe("1.2B");
  });

  it("stays exact under ten thousand", () => {
    expect(formatTokens(9_999, "en")).toBe("9,999");
  });

  it("leaves other locales to their own abbreviations", () => {
    expect(formatTokens(120_000_000, "de")).toBe("120 Mio.");
    expect(formatTokens(1_200_000_000, "de")).toBe("1,2 Mrd.");
    expect(formatTokens(120_000_000, "vi")).toBe("120 Tr");
    expect(formatTokens(1_200_000_000, "vi")).toBe("1,2 T");
  });
});
