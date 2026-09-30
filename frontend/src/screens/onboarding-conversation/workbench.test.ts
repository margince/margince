// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { translate } from "../../i18n";
import { configuredModelLabel } from "./workbench";

type AiProfile = components["schemas"]["AiProfile"];

const en = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);

function profile(overrides: Partial<AiProfile>): AiProfile {
  return {
    name: "Margince",
    kind: "ai",
    state: "configured",
    inference_mode: "cloud",
    providers: ["gemini"],
    configured_models: [
      { tier: "cheap_cloud", provider: "gemini", model: "gemini-3.5-flash" },
    ],
    ...overrides,
  };
}

describe("configuredModelLabel", () => {
  it("names every configured model by its exact identifier and tier", () => {
    const three = profile({
      configured_models: [
        {
          tier: "cheap_cloud",
          provider: "gemini",
          model: "gemini-3.1-flash-lite",
        },
        { tier: "local_small", provider: "ollama", model: "qwen3-32b" },
        { tier: "premium", provider: "anthropic", model: "claude-opus-4" },
      ],
    });
    expect(configuredModelLabel(three, "unavailable", en)).toBe(
      "gemini/gemini-3.1-flash-lite · cloud, efficient + " +
        "ollama/qwen3-32b · local, fast + " +
        "anthropic/claude-opus-4 · premium reasoning",
    );
  });

  it("names the providers when no per-model bindings are reported", () => {
    const providersOnly = profile({
      configured_models: [],
      providers: ["anthropic", "gemini"],
    });
    expect(configuredModelLabel(providersOnly, "unavailable", en)).toBe(
      "anthropic + gemini",
    );
  });

  it("hands back the unavailable label while the profile has not loaded", () => {
    expect(configuredModelLabel(undefined, "unavailable", en)).toBe(
      "unavailable",
    );
  });
});
