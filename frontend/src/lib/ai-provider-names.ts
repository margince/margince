// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

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
} as const satisfies Record<string, MessageKey>;

export type NamedProvider = keyof typeof PROVIDER_NAMES;

function isNamed(provider: string): provider is NamedProvider {
  return Object.hasOwn(PROVIDER_NAMES, provider);
}

export function providerName(provider: string, t: Translator): string {
  return isNamed(provider) ? t(PROVIDER_NAMES[provider]) : provider;
}
