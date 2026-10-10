// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { TIER_ORDER, tierLabel, tierRank } from "./ai-decision-labels";

const t = (key: keyof typeof en) => en[key];

describe("tierLabel", () => {
  it("names every ladder tier and the lanes beside it", () => {
    for (const tier of [
      ...TIER_ORDER,
      "embeddings",
      "embed",
      "decisions",
      "decide",
    ]) {
      expect(tierLabel(tier, t)).not.toBe(tier);
    }
    expect(tierLabel("cheap_cloud", t)).toBe("Everyday cloud");
    expect(tierLabel("embed", t)).toBe(tierLabel("embeddings", t));
    expect(tierLabel("decisions", t)).toBe(tierLabel("decide", t));
  });

  it("reads a tier nobody named as its own key", () => {
    expect(tierLabel("nightly_batch", t)).toBe("nightly_batch");
  });
});

describe("TIER_ORDER", () => {
  it("ranks every tier once, cheapest first, and a tier off it last", () => {
    expect(new Set(TIER_ORDER).size).toBe(TIER_ORDER.length);
    expect(TIER_ORDER.map(tierRank)).toEqual(TIER_ORDER.map((_, i) => i));
    expect(tierRank("embeddings")).toBe(TIER_ORDER.length);
  });
});
