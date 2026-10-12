// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { providerUsage, type RoutingRead } from "./ai-routing-query";

const ROUTING: RoutingRead["routing"] = {
  profile: "cloud_frontier",
  tiers: {
    cheap_cloud: { provider: "anthropic", model: "claude-haiku" },
    premium: { provider: "anthropic", model: "claude-haiku" },
    frontier: { provider: "anthropic", model: "claude-opus" },
  },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
};

describe("providerUsage", () => {
  // The sheet warns once per unpriced model it lists here, so a model two tiers
  // share is one warning, not one per tier.
  it("lists a model two tiers share once, and both tiers it serves", () => {
    const anthropic = providerUsage(ROUTING)?.get("anthropic");
    expect(anthropic?.for).toEqual(["cheap_cloud", "premium", "frontier"]);
    expect(anthropic?.models).toEqual([
      { model: "claude-haiku", lane: "chat" },
      { model: "claude-opus", lane: "chat" },
    ]);
  });

  it("keeps one model bound in two lanes as two entries", () => {
    const usage = providerUsage({
      ...ROUTING,
      embeddings: { provider: "anthropic", model: "claude-haiku" },
    });
    expect(usage?.get("anthropic")?.models).toContainEqual({
      model: "claude-haiku",
      lane: "embeddings",
    });
    expect(usage?.get("anthropic")?.models).toHaveLength(3);
  });
});
