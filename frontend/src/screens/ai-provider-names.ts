// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

// The name a reader knows each AI provider by. The routing name stays the
// wire's and the configuration's; a screen shows this one. A provider this
// table does not name shows its routing name rather than nothing.
const PROVIDER_NAMES: Readonly<Record<string, MessageKey>> = {
  anthropic: "aiProviders.name.anthropic",
  openai_compatible: "aiProviders.name.openaiCompatible",
  openai: "aiProviders.name.openai",
  gemini: "aiProviders.name.gemini",
  gemini_vertex: "aiProviders.name.geminiVertex",
  jev: "aiProviders.name.jev",
  jev_compatible: "aiProviders.name.jevCompatible",
  ollama: "aiProviders.name.ollama",
  vllm: "aiProviders.name.vllm",
};

export function providerName(provider: string, t: Translator): string {
  const key = PROVIDER_NAMES[provider];
  return key ? t(key) : provider;
}
