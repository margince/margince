// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { feature } from "./ai-admin.testkit";
import { AiProviderKeysCard } from "./ai-provider-keys";
import type { SliceValue } from "./ai-routing-slice";
import { ServingSection } from "./ai-serving-editor";
import { TaskSheet } from "./ai-task-sheet";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The request settings an advanced admin tunes, each beside the figures it is
// decided by: the OpenRouter connection's privacy, a tier's serving JSON, and a
// task's thinking level and timeouts.

type Routing = components["schemas"]["AiRouting"];
type Feature = components["schemas"]["AiFeatureRoute"];

const ADMIN: GrantSpec = { ai_routing: ["read", "update"], ai_diagnostics: ["read"], ai_model_rate: ["read"] };
const READER: GrantSpec = { ai_routing: ["read"], ai_diagnostics: ["read"] };

const ROUTING: Routing = {
  profile: "cloud_frontier",
  tiers: {
    local_small: { provider: "openai_compatible", model: "openai/gpt-oss-120b", base_url: "https://openrouter.ai/api" },
    cheap_cloud: { provider: "openai_compatible", model: "openai/gpt-oss-120b", base_url: "https://openrouter.ai/api" },
    premium: { provider: "gemini", model: "gemini-3.1-flash-lite" },
  },
  embeddings: { provider: "openai_compatible", model: "mistralai/mistral-embed-2312" },
  providers: { openai_compatible: { base_url: "https://openrouter.ai/api", upstream: { zdr: true, only: ["mistral/eu", "cerebras"] } } },
};

const stat = (key: string, calls: number, failed: number, timeouts: number, p50: number, p95: number, cost: number) => ({
  key,
  calls,
  failed,
  timeouts,
  p50_ms: p50,
  p95_ms: p95,
  tokens_in: calls * 900,
  tokens_out: calls * 300,
  cost_microusd: cost,
  unpriced: failed,
});

const STATS = [
  stat("openai_compatible", 98, 11, 4, 1100, 2600, 20000),
  stat("Cerebras", 55, 0, 0, 1100, 2600, 20000),
  stat("", 11, 11, 4, 15000, 19500, 0),
  stat("gemini", 43, 0, 0, 1300, 2600, 10000),
  stat("decide", 239, 7, 7, 500, 1300, 40000),
];

const FLOW = {
  task: "capture_confidentiality_verdict",
  window: "7d",
  total: 360,
  unanswered: 4,
  steps: [
    { decision: true, tier: "decide", provider: "jev_compatible", model: "typesafe/jev-1.13", attempts: 360, answered: 349, p50_ms: 500, gave_up: { timeout: 11 } },
    { decision: false, tier: "local_small", provider: "openai_compatible", model: "openai/gpt-oss-120b", attempts: 11, answered: 7, p50_ms: 1100, gave_up: { provider_error: 4 } },
  ],
};

const SCHEMA = {
  openRouterProvider: {
    properties: {
      sort: { description: "Order candidate hosts by throughput, price or latency.", "x-doc-url": "https://openrouter.ai/docs/guides/routing/provider-selection", "x-placement": "tier", oneOf: [] },
      quantizations: { description: "Serving precisions a host may use.", type: "array", "x-placement": "tier" },
      zdr: { description: "Only hosts that keep no copy of the prompt.", type: "boolean", "x-placement": "connection" },
      only: { description: "Allowlist of upstream host slugs.", type: "array", "x-placement": "connection" },
    },
  },
  openRouterReasoning: { properties: { effort: { enum: ["minimal", "low", "medium", "high"], description: "The thinking budget as a level." } } },
};

const KEYS = {
  providers: [
    { provider: "openai_compatible", configured: true, env_var: "OPENAI_COMPATIBLE_API_KEY", usable: true, optional: false },
    { provider: "gemini", configured: true, env_var: "GEMINI_API_KEY", usable: true, optional: false },
  ],
};

function stub({
  allow = ADMIN,
  overrides = {},
  conflict = false,
  preview = { current_version: "v1", features: [], unused_tiers: [], effective: { tiers: { cheap_cloud: { provider: { sort: "latency", zdr: true, only: ["mistral/eu", "cerebras"] } } } } },
}: { allow?: GrantSpec; overrides?: unknown; conflict?: boolean; preview?: unknown } = {}) {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({ allow })),
    "GET /ai/provider-keys": () => jsonResponse(KEYS),
    "GET /ai/routing": () => jsonResponse(ROUTING),
    "GET /ai-model-rates": () => jsonResponse({ data: [] }),
    "GET /ai/routing/schema": () => jsonResponse(SCHEMA),
    "POST /ai/routing/preview": () => jsonResponse(preview),
    "GET /ai/call-stats": () => jsonResponse({ window: "7d", group: "provider", rows: STATS }),
    "GET /ai/call-stats/flow": () => jsonResponse(FLOW),
    "GET /ai/task-overrides": () => jsonResponse(overrides),
    "PUT /ai/task-overrides": () =>
      conflict ? jsonResponse({ title: "Conflict", status: 409, code: "version_skew" }, 409) : jsonResponse(overrides),
  });
}

const DECIDING: Feature = {
  ...feature,
  task: "capture_confidentiality_verdict",
  display_name: "Thread confidentiality check",
  execution_mode: "background",
  decides: true,
  decision_first: true,
  leading_tier: "local_small",
};

const meta: Meta = {
  title: "Settings/AI/Request settings",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

/** The Providers card: every row says what its calls did this week. */
export const ProvidersWithFigures: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <AiProviderKeysCard />
      </StoryProviders>
    );
  },
};

const openConnection: Story["play"] = async ({ canvasElement }) => {
  const body = within(canvasElement.ownerDocument.body);
  await userEvent.click(within(await body.findByTestId("ai-provider-row-openai_compatible")).getByRole("button", { name: /^Edit/ }));
};

/** The OpenRouter connection: its privacy rules, then its calls by host. */
export const OpenRouterConnection: Story = { ...ProvidersWithFigures, play: openConnection };
export const OpenRouterConnectionDark: Story = { ...ProvidersWithFigures, play: openConnection, globals: { theme: "dark" } };

const TIER: SliceValue = { kind: "tier", tier: "cheap_cloud", binding: ROUTING.tiers.cheap_cloud };

function Serving({ value }: Readonly<{ value: SliceValue }>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 960 }}>
        <ServingSection value={value} routing={ROUTING} disabled={false} onChange={() => {}} onValid={() => {}} />
      </div>
    </StoryProviders>
  );
}

/** A tier's serving JSON the server accepted, with the request it will send. */
export const ServingValid: Story = {
  render: () => {
    stub();
    return <Serving value={{ ...TIER, binding: { ...TIER.binding, routing: { provider: { sort: "latency" } } } }} />;
  },
};
export const ServingValidDark: Story = { ...ServingValid, globals: { theme: "dark" } };

/** Two keys the server refused, each on its line. */
export const ServingRefused: Story = {
  render: () => {
    stub({
      preview: {
        current_version: "v1",
        features: [],
        unused_tiers: [],
        errors: [
          { field: "tiers.cheap_cloud.routing.provider.sort.by", code: "setting_invalid", message: "must be one of price, throughput, latency." },
          { field: "tiers.cheap_cloud.routing.provider.zdr", code: "moved_to_provider", message: "is set on the connection, under OpenRouter settings, and applies to every tier. Remove it here." },
        ],
      },
    });
    return <Serving value={{ ...TIER, binding: { ...TIER.binding, routing: { provider: { sort: { by: "price", partition: "model" }, zdr: true } } } }} />;
  },
};

/** A tier a vendor serves itself: no editor, and why. */
export const ServingNotApplicable: Story = {
  render: () => {
    stub();
    return <Serving value={{ kind: "tier", tier: "premium", binding: ROUTING.tiers.premium }} />;
  },
};

function Sheet({ route, canManage = true }: Readonly<{ route: Feature; canManage?: boolean }>) {
  return (
    <StoryProviders>
      <TaskSheet route={route} canManage={canManage} canSeeCalls onClose={() => {}} />
    </StoryProviders>
  );
}

/** A deciding task at its defaults: the outcome, the latency against the timeout, three settings. */
export const TaskDecisionDefault: Story = {
  render: () => {
    stub();
    return <Sheet route={DECIDING} />;
  },
};
export const TaskDecisionDefaultDark: Story = { ...TaskDecisionDefault, globals: { theme: "dark" } };

/** A deciding task an admin customised. */
export const TaskDecisionOverridden: Story = {
  render: () => {
    stub({ overrides: { capture_confidentiality_verdict: { thinking: "low", decision_timeout_ms: 30000 } } });
    return <Sheet route={DECIDING} />;
  },
};

/** A one-shot task: no decision timeout. */
export const TaskOneShot: Story = {
  render: () => {
    stub();
    return <Sheet route={feature} />;
  },
};

/** A reader who may look but not change. */
export const TaskReadOnly: Story = {
  render: () => {
    stub({ allow: READER });
    return <Sheet route={DECIDING} canManage={false} />;
  },
};

/** A colleague saved first: the save says so rather than overwriting them. */
export const TaskConflict: Story = {
  render: () => {
    stub({ conflict: true });
    return <Sheet route={feature} />;
  },
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(await body.findByRole("combobox", { name: /Thinking level/ }));
    await userEvent.click(await body.findByRole("option", { name: "low" }));
    await userEvent.click(body.getByRole("button", { name: "Save settings" }));
  },
};
