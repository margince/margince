// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { DECISION_PROVIDERS, PROVIDERS } from "./ai-routing-fields";

type Provider =
  | (typeof PROVIDERS)[number]
  | (typeof DECISION_PROVIDERS)[number];

// Keyed by the routing form's mirrors of the server registries, so an adapter
// added there without a name here fails the typecheck.
const PROVIDER_NAMES = {
  anthropic: "aiProviders.name.anthropic",
  openai_compatible: "aiProviders.name.openaiCompatible",
  openai: "aiProviders.name.openai",
  gemini: "aiProviders.name.gemini",
  gemini_vertex: "aiProviders.name.geminiVertex",
  jev: "aiProviders.name.jev",
  jev_compatible: "aiProviders.name.jevCompatible",
  ollama: "aiProviders.name.ollama",
  vllm: "aiProviders.name.vllm",
  fake: "aiProviders.name.fake",
} as const satisfies Record<Provider, MessageKey>;

function isNamed(provider: string): provider is Provider {
  return Object.hasOwn(PROVIDER_NAMES, provider);
}

export function providerName(provider: string, t: Translator): string {
  return isNamed(provider) ? t(PROVIDER_NAMES[provider]) : provider;
}
