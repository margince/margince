// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { AiProviderKeysCard } from "./ai-provider-keys";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The Providers list and the sheet behind each row: whether a vendor is active,
// its credential, and the prices its models are billed at — with the form for a
// price swapping in inside the sheet rather than opening a second dialog.

const KEYS = {
  providers: [
    {
      provider: "gemini",
      configured: true,
      env_var: "GEMINI_API_KEY",
      usable: true,
      optional: false,
    },
    {
      provider: "openai_compatible",
      configured: true,
      env_var: "OPENAI_COMPATIBLE_API_KEY",
      usable: true,
      optional: false,
    },
    {
      provider: "anthropic",
      configured: true,
      env_var: "ANTHROPIC_API_KEY",
      usable: true,
      optional: false,
    },
    {
      provider: "openai",
      configured: false,
      env_var: "OPENAI_API_KEY",
      usable: false,
      optional: false,
    },
    {
      provider: "jev",
      configured: false,
      env_var: "TYPESAFE_API_KEY",
      usable: false,
      optional: false,
    },
  ],
};

const ROUTING = {
  profile: "cloud_ok",
  tiers: {
    local_small: {
      provider: "openai_compatible",
      model: "openai/gpt-oss-120b",
      base_url: "https://openrouter.ai/api/v1",
    },
    cheap_cloud: { provider: "gemini", model: "gemini-3.5-flash" },
    premium: { provider: "jev", model: "jev-1" },
    frontier: { provider: "anthropic", model: "claude-opus-4-1" },
    local_large: { provider: "anthropic", model: "claude-opus-4-1" },
  },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
};

const RATES = [
  ["gemini", "gemini-3.5-flash", "chat", "1.5", "9", "0.15", "0"],
  ["gemini", "gemini-2.5-pro", "chat", "1.25", "10", "0.31", "0"],
  ["gemini", "gemini-embedding-001", "embeddings", "0.15", "0", "0", "0"],
  [
    "openai_compatible",
    "openai/gpt-oss-120b",
    "chat",
    "0.04",
    "0.17",
    "0",
    "0",
  ],
].map(([provider, model_id, lane, i, o, cr, cw]) => ({
  provider,
  model_id,
  lane,
  input_per_mtok: i,
  output_per_mtok: o,
  cache_read_per_mtok: cr,
  cache_write_per_mtok: cw,
  effective_date: "2026-08-01",
}));

const WRITER: GrantSpec = {
  ai_routing: ["read", "update"],
  ai_model_rate: ["read", "create", "update"],
};

function stub(allow: GrantSpec = WRITER) {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({ allow })),
    "GET /ai/provider-keys": () => jsonResponse(KEYS),
    "GET /ai/routing": () => jsonResponse(ROUTING),
    "GET /ai-model-rates": () => jsonResponse({ data: RATES }),
    "GET /ai/available-models/{provider}": () =>
      jsonResponse({ provider: "gemini", models: [{ id: "gemini-4-pro" }] }),
    "POST /ai-model-rates": () => jsonResponse(RATES[0], 201),
    "DELETE /ai-model-rates": () => new Response(null, { status: 204 }),
    "POST /ai/provider-keys/gemini/test": () =>
      jsonResponse({ provider: "gemini", ok: true, model_count: 42 }),
  });
}

const meta: Meta<typeof AiProviderKeysCard> = {
  title: "Settings/AI/AI models/Providers and prices",
  component: AiProviderKeysCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof AiProviderKeysCard>;

const openSheet =
  (name: string): Story["play"] =>
  async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      within(await body.findByTestId(`ai-provider-row-${name}`)).getByRole(
        "button",
        {
          name: /^Edit/,
        },
      ),
    );
  };

/** Every vendor as a reading: active, ready, needs a key, or not active. */
export const List: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <AiProviderKeysCard />
      </StoryProviders>
    );
  },
};

/** A vendor's sheet: the connection, then its price table with the source link. */
export const Sheet: Story = { ...List, play: openSheet("gemini") };
export const SheetDark: Story = {
  ...List,
  play: openSheet("gemini"),
  globals: { theme: "dark" },
};

/** Adding a price swaps the table for the form, inside the same sheet. */
export const AddingAPrice: Story = {
  ...List,
  play: async (ctx) => {
    await openSheet("gemini")?.(ctx);
    const body = within(ctx.canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Add price" }),
    );
  },
};

/** Removing a price asks first: every date of the entry goes, and its calls read as unpriced. */
export const RemovingAPrice: Story = {
  ...List,
  play: async (ctx) => {
    await openSheet("gemini")?.(ctx);
    const body = within(ctx.canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Remove gemini-2.5-pro" }),
    );
  },
};

/** No prices on the sheet: the table says so, and a model two tiers share is named once. */
export const NoPrices: Story = { ...List, play: openSheet("anthropic") };

/** The phone: the sheet is a full-screen layer. */
export const SheetPhone: Story = {
  ...List,
  play: openSheet("gemini"),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
