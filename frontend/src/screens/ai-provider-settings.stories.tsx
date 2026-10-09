// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ProviderSettingsForm } from "./ai-provider-settings";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// One provider's own settings, under its key on the provider sheet. They are
// the host for a broker, Gemini, Anthropic or a decision server, OpenRouter's
// host pins when the host is OpenRouter, and the Vertex location.

type Routing = components["schemas"]["AiRouting"];

function routing(providers: Routing["providers"], profile = "cloud_frontier") {
  return {
    profile,
    tiers: {},
    embeddings: { provider: "gemini", model: "gemini-embedding-001" },
    providers,
  } as Routing;
}

function story(provider: string, value: Routing, canManage = true) {
  return function Render() {
    installFetchStub({
      "GET /ai/provider-locations/gemini_vertex": () =>
        jsonResponse({
          provider: "gemini_vertex",
          locations: [
            {
              id: "eu",
              display_name: "EU (multi-region)",
              jurisdiction: "eu",
              resident: true,
            },
            {
              id: "europe-west4",
              display_name: "Netherlands",
              jurisdiction: "eu",
              resident: true,
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: "560px" }}>
          <ProviderSettingsForm
            provider={provider}
            routing={value}
            canManage={canManage}
          />
        </div>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ProviderSettingsForm> = {
  title: "Settings/AI/AI models/Provider settings",
  component: ProviderSettingsForm,
};
export default meta;
type Story = StoryObj<typeof ProviderSettingsForm>;

// No host yet: the field is empty and the OpenRouter preset fills it.
export const BrokerUnset: Story = {
  render: story("openai_compatible", routing({})),
};

// Pointed at OpenRouter with pins set elsewhere: the sheet keeps them.
export const OpenRouterPinned: Story = {
  render: story(
    "openai_compatible",
    routing({
      openai_compatible: {
        base_url: "https://openrouter.ai/api",
        upstream: { only: ["mistral/eu"] },
      },
    }),
  ),
};

// OpenRouter's EU address, with what it requires.
export const OpenRouterEu: Story = {
  render: story(
    "openai_compatible",
    routing({
      openai_compatible: { base_url: "https://eu.openrouter.ai/api" },
    }),
  ),
};

// A host no known service has: the sheet opens on Other and asks for it.
export const Gateway: Story = {
  render: story(
    "openai_compatible",
    routing({ openai_compatible: { base_url: "https://gateway.example" } }),
  ),
};

// Gemini through Langdock's EU region: the host keeps Gemini's API version.
export const GeminiLangdock: Story = {
  render: story(
    "gemini",
    routing({
      gemini: { base_url: "https://api.langdock.com/google/eu/v1beta" },
    }),
  ),
};

export const AnthropicLangdock: Story = {
  render: story(
    "anthropic",
    routing({
      anthropic: { base_url: "https://api.langdock.com/anthropic/us" },
    }),
  ),
};

export const DecisionServer: Story = {
  render: story("jev_compatible", routing({})),
};

export const VertexLocation: Story = {
  render: story(
    "gemini_vertex",
    routing({ gemini_vertex: { location: "eu" } }, "eu_hosted"),
  ),
};

// A reader who may look but not change.
export const ReadOnly: Story = {
  render: story(
    "openai_compatible",
    routing({ openai_compatible: { base_url: "https://openrouter.ai/api" } }),
    false,
  ),
};

export const OpenRouterPinnedDark: Story = {
  globals: { theme: "dark" },
  render: story(
    "openai_compatible",
    routing({
      openai_compatible: {
        base_url: "https://openrouter.ai/api",
        upstream: { only: ["mistral/eu"] },
      },
    }),
  ),
};
