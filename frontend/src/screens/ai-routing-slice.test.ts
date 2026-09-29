import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { sameSlice, sliceOf, withSlice } from "./ai-routing-slice";

type Routing = components["schemas"]["AiRouting"];

const DOC: Routing = {
  profile: "cloud_frontier",
  tiers: {
    premium: { provider: "gemini", model: "gemini-3.5-flash" },
    cheap_cloud: { provider: "gemini", model: "gemini-3.1-flash-lite" },
  },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
  decisions: { provider: "jev", model: "jev-1.13.0" },
};

describe("a routing slice", () => {
  it("replaces only the lane it names", () => {
    const next = withSlice(DOC, {
      kind: "tier",
      tier: "premium",
      binding: { provider: "anthropic", model: "claude-opus-4-8" },
    });
    expect(next.tiers.premium.model).toBe("claude-opus-4-8");
    expect(next.tiers.cheap_cloud).toEqual(DOC.tiers.cheap_cloud);
    expect(next.embeddings).toEqual(DOC.embeddings);
    expect(next.decisions).toEqual(DOC.decisions);
  });

  it("replaces the embedder and keeps its width", () => {
    const next = withSlice(DOC, {
      kind: "embeddings",
      binding: { provider: "openai", model: "e3", dimensions: 1536 },
    });
    expect(next.embeddings).toEqual({
      provider: "openai",
      model: "e3",
      dimensions: 1536,
    });
  });

  // Absent is what the server reads as "no decision model"; a key holding
  // undefined serializes the same but is not the same document.
  it("deletes the decision key rather than setting it to undefined", () => {
    const next = withSlice(DOC, { kind: "decisions", binding: undefined });
    expect("decisions" in next).toBe(false);
  });

  it("reads a tier the document does not bind as an empty binding", () => {
    expect(sliceOf(DOC, { kind: "tier", tier: "frontier" }).binding).toEqual({
      provider: "",
      model: "",
    });
  });

  // The server answers fields in its own order and omits what is unset, so
  // neither is a change of binding.
  it("compares bindings by content, not by spelling", () => {
    const a = {
      kind: "embeddings" as const,
      binding: { provider: "g", model: "m" },
    };
    const b = {
      kind: "embeddings" as const,
      binding: { model: "m", dimensions: undefined, provider: "g" },
    };
    expect(sameSlice(a, b)).toBe(true);
    expect(
      sameSlice(a, {
        kind: "embeddings",
        binding: { provider: "g", model: "n" },
      }),
    ).toBe(false);
    expect(
      sameSlice(
        { kind: "decisions", binding: undefined },
        { kind: "decisions", binding: { provider: "jev", model: "x" } },
      ),
    ).toBe(false);
  });
});
