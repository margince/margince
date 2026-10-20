// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { daysInStage } from "./stagetime";

const NOW = Date.parse("2026-08-13T09:00:00Z");

describe("the days a deal has stood in its stage", () => {
  it("count whole calendar days from the day it entered", () => {
    expect(daysInStage("2026-08-03T09:00:00Z", NOW)).toBe(10);
  });

  it("are unknown, not zero, when no entry day was sent or it cannot be read", () => {
    expect(daysInStage(undefined, NOW)).toBeNull();
    expect(daysInStage("not a date", NOW)).toBeNull();
  });

  it("never count below zero for an entry stamped after the reader's clock", () => {
    expect(daysInStage("2026-08-20T09:00:00Z", NOW)).toBe(0);
  });
});
