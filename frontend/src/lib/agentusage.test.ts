// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { tightestCounter, usedUp } from "./agentusage";

const counter = (used: number, limit: number) => ({ used, limit, released: 0 });

describe("tightestCounter", () => {
  it("names the allowance with the largest share used", () => {
    expect(
      tightestCounter({
        reads: counter(1_800, 2_000),
        writes: counter(190, 200),
        egress: counter(0, 20),
        calls: counter(600, 1_000),
      }),
    ).toBe("writes");
  });

  it("names the first allowance when nothing is used", () => {
    expect(
      tightestCounter({
        reads: counter(0, 2_000),
        writes: counter(0, 200),
        egress: counter(0, 20),
        calls: counter(0, 1_000),
      }),
    ).toBe("reads");
  });
});

describe("usedUp", () => {
  it("is true only once the allowance is reached", () => {
    expect(usedUp(counter(199, 200))).toBe(false);
    expect(usedUp(counter(200, 200))).toBe(true);
    expect(usedUp(counter(0, 0))).toBe(false);
  });
});
