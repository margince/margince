import { describe, expect, it } from "vitest";
import { stable } from "../../format/collate";
import { MODEL_SETUPS } from "../../lib/modelsetups.gen";
import { SETUP_PROVIDERS } from "../../lib/setup-providers";
import { chatTiers, taskLadders } from "./installation-setup.ai-key";

// Onboarding binds the graded preset's own model on each tier, so a new
// installation runs the setup the certification measured.

describe("chatTiers", () => {
  it("binds each tier to the graded preset's model for that tier", () => {
    const gemini = SETUP_PROVIDERS.gemini;
    const tiers = chatTiers(
      {
        provider: gemini.provider,
        setup: gemini.setup,
        chatModel: gemini.chatModel,
      },
      {},
    );
    expect(tiers).toEqual(MODEL_SETUPS.gemini_cloud.routing.tiers);
    const models = new Set(Object.values(tiers).map((b) => b.model));
    expect(models.size).toBeGreaterThan(1);
  });

  it("binds a graded Vertex preset at the location the reader chose", () => {
    const vertex = SETUP_PROVIDERS.gemini_vertex;
    const tiers = chatTiers(
      {
        provider: vertex.provider,
        location: "europe-west4",
        setup: vertex.setup,
        chatModel: vertex.chatModel,
      },
      {},
    );
    const preset = new Map(
      Object.entries(MODEL_SETUPS.gemini_vertex_eu.routing.tiers),
    );
    for (const [tier, binding] of Object.entries(tiers)) {
      expect(binding.location, tier).toBe("europe-west4");
      expect(binding.model, tier).toBe(preset.get(tier)?.model);
    }
  });

  it("puts the one model on every tier when the reader asks for that", () => {
    const tiers = chatTiers(
      { provider: "openai_compatible", chatModel: "vendor/one" },
      { base_url: "https://openrouter.ai/api" },
    );
    expect(Object.keys(tiers).sort(stable)).toEqual([
      "cheap_cloud",
      "frontier",
      "local_small",
      "premium",
    ]);
    for (const binding of Object.values(tiers)) {
      expect(binding).toEqual({
        provider: "openai_compatible",
        model: "vendor/one",
        base_url: "https://openrouter.ai/api",
      });
    }
  });
});

describe("taskLadders", () => {
  it("carries the graded preset's task order, and none for one model on every tier", () => {
    const gemini = SETUP_PROVIDERS.gemini;
    expect(taskLadders({ setup: gemini.setup })).toEqual({
      task_ladders: MODEL_SETUPS.gemini_cloud.routing.task_ladders,
    });
    expect(
      MODEL_SETUPS.gemini_cloud.routing.task_ladders?.agent_loop?.[0],
    ).toBe("premium");
    expect(taskLadders({})).toEqual({});
  });
});
