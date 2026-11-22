// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";
import type { OtherHost } from "./ai-provider-settings.otherhost";

// The services a provider's sheet offers, and the host each one fills in.

export type Service = Readonly<{
  id: string;
  label: MessageKey;
  // The host it saves; empty means the adapter's compiled default.
  host: string;
  note?: MessageKey;
  noteLink?: string;
}>;

export type ProviderServices = Readonly<{
  services: readonly Service[];
  other: OtherHost & { label: MessageKey };
  // A host typed into one field with no service to pick. A local server has
  // no vendor, and a blank host is its compiled default.
  direct?: true;
}>;

export const OTHER = "other";

// Langdock serves each vendor's wire under its own path, once per region, on
// one key; `version` is the segment the adapter expects in its host.
function langdock(
  wire: string,
  version = "",
  note?: MessageKey,
): readonly Service[] {
  return [
    {
      id: "langdock-eu",
      label: "aiProviderSettings.service.langdockEu",
      host: `https://api.langdock.com/${wire}/eu${version}`,
      note,
    },
    {
      id: "langdock-us",
      label: "aiProviderSettings.service.langdockUs",
      host: `https://api.langdock.com/${wire}/us${version}`,
      note,
    },
  ];
}

export const SERVICES: ReadonlyMap<string, ProviderServices> = new Map([
  [
    "openai_compatible",
    {
      services: [
        {
          id: "openrouter",
          label: "aiProviderSettings.service.openrouter",
          host: "https://openrouter.ai/api",
        },
        {
          id: "openrouter-eu",
          label: "aiProviderSettings.service.openrouterEu",
          host: "https://eu.openrouter.ai/api",
          note: "aiProviderSettings.service.openrouterEu.note",
          noteLink: "https://openrouter.ai/docs/guides/features/sovereign-ai",
        },
        {
          id: "mistral",
          label: "aiProviderSettings.service.mistral",
          host: "https://api.mistral.ai",
        },
        {
          id: "together",
          label: "aiProviderSettings.service.together",
          host: "https://api.together.xyz",
        },
        {
          id: "groq",
          label: "aiProviderSettings.service.groq",
          host: "https://api.groq.com/openai",
        },
        {
          id: "deepseek",
          label: "aiProviderSettings.service.deepseek",
          host: "https://api.deepseek.com",
        },
        ...langdock("openai"),
      ],
      other: {
        label: "aiProviderSettings.service.otherChat",
        help: "aiRouting.baseUrl.help",
        placeholder: "aiRouting.baseUrl.placeholder",
      },
    },
  ],
  [
    "gemini",
    {
      services: [
        {
          id: "google-ai-studio",
          label: "aiProviderSettings.service.googleAiStudio",
          host: "",
        },
        // Its Gemini path serves no embedder, and the embeddings lane on
        // gemini follows this host.
        ...langdock(
          "google",
          "/v1beta",
          "aiProviderSettings.service.langdockGemini.note",
        ),
      ],
      other: {
        label: "aiProviderSettings.service.otherGemini",
        help: "aiRouting.baseUrl.help.gemini",
        placeholder: "aiRouting.baseUrl.placeholder.gemini",
      },
    },
  ],
  [
    "openai",
    {
      services: [
        { id: "openai", label: "aiProviderSettings.service.openai", host: "" },
        ...langdock("openai"),
      ],
      other: {
        label: "aiProviderSettings.service.otherOpenai",
        help: "aiRouting.baseUrl.help.openai",
        placeholder: "aiRouting.baseUrl.placeholder.openai",
      },
    },
  ],
  [
    "anthropic",
    {
      services: [
        {
          id: "anthropic",
          label: "aiProviderSettings.service.anthropic",
          host: "",
        },
        ...langdock("anthropic"),
      ],
      other: {
        label: "aiProviderSettings.service.otherAnthropic",
        help: "aiRouting.baseUrl.help.anthropic",
        placeholder: "aiRouting.baseUrl.placeholder.anthropic",
      },
    },
  ],
  [
    "jev_compatible",
    {
      services: [
        {
          id: "openrouter",
          label: "aiProviderSettings.service.openrouter",
          host: "https://openrouter.ai/api/alpha/decisions",
        },
        {
          id: "openrouter-eu",
          label: "aiProviderSettings.service.openrouterEu",
          host: "https://eu.openrouter.ai/api/alpha/decisions",
          note: "aiProviderSettings.service.openrouterEu.note",
          noteLink: "https://openrouter.ai/docs/guides/features/sovereign-ai",
        },
      ],
      other: {
        label: "aiProviderSettings.service.otherDecisions",
        help: "aiRouting.baseUrl.help.jevCompatible",
        placeholder: "aiRouting.baseUrl.placeholder.jevCompatible",
      },
    },
  ],
  [
    "jev",
    {
      services: [
        {
          id: "typesafe",
          label: "aiProviderSettings.service.typesafe",
          host: "",
        },
      ],
      other: {
        label: "aiProviderSettings.service.otherAddress",
        help: "aiRouting.baseUrl.help.jev",
        placeholder: "aiRouting.baseUrl.placeholder.jev",
      },
    },
  ],
  [
    "ollama",
    {
      services: [],
      direct: true,
      other: {
        label: "aiProviderSettings.service.otherAddress",
        help: "aiRouting.baseUrl.help.ollama",
        placeholder: "aiRouting.baseUrl.placeholder.ollama",
      },
    },
  ],
  [
    "vllm",
    {
      services: [],
      direct: true,
      other: {
        label: "aiProviderSettings.service.otherAddress",
        help: "aiRouting.baseUrl.help.vllm",
        placeholder: "aiRouting.baseUrl.placeholder.vllm",
      },
    },
  ],
]);
