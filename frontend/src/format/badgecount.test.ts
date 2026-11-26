// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { formatBadgeCount } from "./badgecount";

describe("formatBadgeCount", () => {
  it("draws a count up to two digits as it is", () => {
    expect(formatBadgeCount(7, "en")).toBe("7");
    expect(formatBadgeCount(99, "en")).toBe("99");
  });

  it("stops a larger count at 99+", () => {
    expect(formatBadgeCount(100, "en")).toBe("99+");
    expect(formatBadgeCount(1340, "de")).toBe("99+");
  });
});
