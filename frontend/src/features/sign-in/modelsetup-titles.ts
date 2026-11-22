// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";
import type { ModelSetupId } from "../../lib/modelsetups.gen";

/**
 * The catalog key that names each model setup to a reader. Typed against the
 * generated table, so a new preset does not build until it has a name in en,
 * de and vi.
 */
export const MODEL_SETUP_TITLES = {
  consumer_class_brokered: "firstRun.ai.setupTitle.consumer_class_brokered",
  gemini_cloud: "firstRun.ai.setupTitle.gemini_cloud",
  gemini_vertex_eu: "firstRun.ai.setupTitle.gemini_vertex_eu",
  gemma4_local_ollama: "firstRun.ai.setupTitle.gemma4_local_ollama",
  openrouter_cloud: "firstRun.ai.setupTitle.openrouter_cloud",
  openrouter_cloud_eu: "firstRun.ai.setupTitle.openrouter_cloud_eu",
  openrouter_openai: "firstRun.ai.setupTitle.openrouter_openai",
  qwen3_local_vllm: "firstRun.ai.setupTitle.qwen3_local_vllm",
} as const satisfies Record<ModelSetupId, MessageKey>;
