// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import { BindingEditor } from "./ai-binding-editor";
import type { RoutingRead } from "./ai-routing-query";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

type Routing = components["schemas"]["AiRouting"];

const ROUTING: Routing = {
  profile: "eu_hosted",
  tiers: {
    local_small: { provider: "ollama", model: "gemma3" },
    premium: { provider: "gemini", model: "gemini-3.5-flash" },
    frontier: { provider: "anthropic", model: "claude-opus-4-5" },
  },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
};

const OPENED: RoutingRead = { routing: ROUTING, version: '"routing-v1"' };

// Anthropic takes a key and holds none; Ollama has no entry, so it takes none.
const KEYS = [
  {
    provider: "gemini",
    configured: true,
    env_var: "GEMINI_API_KEY",
    optional: false,
  },
  {
    provider: "anthropic",
    configured: false,
    env_var: "ANTHROPIC_API_KEY",
    optional: false,
  },
];

const SHEET = [
  {
    provider: "gemini",
    model_id: "gemini-3.5-flash",
    lane: "chat" as const,
    input_per_mtok: "1.50",
    output_per_mtok: "9.00",
    cache_read_per_mtok: "0",
    cache_write_per_mtok: "0",
    effective_date: "2026-08-12",
  },
];

const VENDORS: Record<string, unknown> = {
  gemini: {
    provider: "gemini",
    models: [
      {
        id: "gemini-4.0-flash",
        display_name: "Gemini 4.0 Flash",
        lane: "chat",
      },
      {
        id: "gemini-3.5-flash",
        display_name: "Gemini 3.5 Flash",
        lane: "chat",
      },
    ],
  },
  anthropic: { provider: "anthropic", models: [], unavailable: "no_key" },
  ollama: { provider: "ollama", models: [{ id: "gemma3" }] },
};

const REFUSAL = {
  type: "https://errors.gradion.com/validation_error",
  title: "Validation failed",
  status: 422,
  code: "validation_error",
  detail: "Provider gemini cannot serve tier premium under this profile.",
};

function routingRead(): Response {
  const response = jsonResponse(ROUTING);
  response.headers.set("ETag", OPENED.version);
  return response;
}

function editor(put: RouteMap[string] = routingRead) {
  return (args: ComponentProps<typeof BindingEditor>) => {
    installFetchStub({
      "GET /ai/routing": routingRead,
      "PUT /ai/routing": put,
      ...Object.fromEntries(
        Object.entries(VENDORS).map(([provider, body]) => [
          `GET /ai/available-models/${provider}`,
          () => jsonResponse(body),
        ]),
      ),
    });
    return (
      <StoryProviders>
        <BindingEditor {...args} />
      </StoryProviders>
    );
  };
}

async function pressSave(canvasElement: HTMLElement) {
  const body = within(canvasElement.ownerDocument.body);
  await userEvent.click(
    await body.findByRole("button", { name: en["aiRouting.saveBinding"] }),
  );
  return body;
}

const meta = {
  title: "Settings/AI/AI models/Binding editor",
  component: BindingEditor,
  args: {
    opened: OPENED,
    initial: {
      kind: "tier",
      tier: "premium",
      binding: { provider: "gemini", model: "gemini-3.5-flash" },
    },
    label: "premium",
    keys: KEYS,
    catalogue: SHEET,
    canManage: true,
    onClose: () => {},
  },
  render: editor(),
} satisfies Meta<typeof BindingEditor>;
export default meta;

type Story = StoryObj<typeof meta>;

export const EditingATier: Story = {};

export const KeyMissing: Story = {
  args: {
    initial: {
      kind: "tier",
      tier: "frontier",
      binding: { provider: "anthropic", model: "claude-opus-4-5" },
    },
    label: "frontier",
  },
};

export const ReadOnlySeat: Story = { args: { canManage: false } };

export const SaveRefused: Story = {
  render: editor(() => jsonResponse(REFUSAL, 422)),
  play: async ({ canvasElement }) => {
    const body = await pressSave(canvasElement);
    await body.findByText(REFUSAL.detail);
  },
};

export const ChangedWhileEditing: Story = {
  render: editor(() =>
    jsonResponse({ title: "Conflict", status: 409, code: "version_skew" }, 409),
  ),
  play: async ({ canvasElement }) => {
    const body = await pressSave(canvasElement);
    await body.findByText(en["aiAdmin.routingStale"]);
  },
};

export const EditingATierDark: Story = { globals: { theme: "dark" } };

const DECISIONS = {
  provider: "jev_compatible",
  model: "typesafe/jev-router",
  base_url: "https://openrouter.ai/api/alpha/decisions",
};

// The decision model bound, so the footer offers to remove it, with the
// OpenRouter preset beside its provider.
export const EditingTheDecisionModel: Story = {
  args: {
    opened: {
      routing: { ...ROUTING, decisions: DECISIONS },
      version: OPENED.version,
    },
    initial: { kind: "decisions", binding: DECISIONS },
    label: "decisions",
  },
};

export const EditingTheDecisionModelDark: Story = {
  ...EditingTheDecisionModel,
  globals: { theme: "dark" },
};
