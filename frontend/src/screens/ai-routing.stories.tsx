// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { AiRoutingCard } from "./ai-routing";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The installation's tier→model binding: which vendor serves each cost rung,
// and the egress posture that constrains what a rung may be bound to.
//
// /me is not optional furniture here, it selects the shape. `ai_routing:read`
// decides whether the binding is this reader's to see at all, and `update`
// decides whether the form is theirs to change — so the three seats below are
// three different cards, not one card with a disabled attribute.
const MANAGER: GrantSpec = { ai_routing: ["read", "update"] };
const READER: GrantSpec = { ai_routing: ["read"] };
// Reaches the AI tab on another grant and holds no ai_routing at all.
const NO_GRANT: GrantSpec = { automation: ["read"] };

const BOUND = {
  profile: "eu_hosted",
  tiers: {
    local_small: { provider: "ollama", model: "gemma3" },
    cheap_cloud: { provider: "gemini", model: "gemini-3.1-flash-lite" },
    premium: { provider: "gemini", model: "gemini-3.5-flash" },
    frontier: { provider: "gemini", model: "gemini-3.1-pro-preview" },
  },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
};

// What the price sheet can cost a call on, and what each VENDOR says it serves.
// The two are different questions and the card shows both: the sheet is a table
// somebody maintains, the vendor list is asked live, and a model on the second
// but not the first is bindable and reads as unpriced.
const SHEET = [
  rate("gemini", "gemini-3.1-flash-lite", "chat", "0.25", "1.50"),
  rate("gemini", "gemini-3.5-flash", "chat", "1.50", "9.00"),
  rate("gemini", "gemini-embedding-001", "embeddings", "0.15", "0"),
  rate("ollama", "gemma3", "chat", "0", "0"),
];

function rate(
  provider: string,
  model_id: string,
  lane: "chat" | "embeddings",
  input_per_mtok: string,
  output_per_mtok: string,
) {
  return {
    provider,
    model_id,
    lane,
    input_per_mtok,
    output_per_mtok,
    cache_read_per_mtok: "0",
    cache_write_per_mtok: "0",
    effective_date: "2026-08-12",
  };
}

// Newer than anything the sheet carries — the case the live list exists for.
const VENDOR_LIST: Record<string, unknown> = {
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
      { id: "gemini-embedding-001", lane: "embeddings" },
    ],
  },
  ollama: { provider: "ollama", models: [{ id: "gemma3:latest" }] },
  anthropic: { provider: "anthropic", models: [], unavailable: "no_key" },
};

function story(
  routing: unknown,
  allow: GrantSpec = MANAGER,
  vendors: Record<string, unknown> = VENDOR_LIST,
) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow })),
      "GET /ai/routing": () => jsonResponse(routing),
      "GET /ai-model-rates": () => jsonResponse({ data: SHEET }),
      "GET /ai/provider-keys": () =>
        jsonResponse({
          providers: [
            { provider: "gemini", configured: true, env_var: "GEMINI_API_KEY" },
            {
              provider: "anthropic",
              configured: false,
              env_var: "ANTHROPIC_API_KEY",
            },
          ],
        }),
      ...Object.fromEntries(
        Object.entries(vendors).map(([provider, body]) => [
          `GET /ai/available-models/${provider}`,
          () => jsonResponse(body),
        ]),
      ),
    });
    return (
      <StoryProviders>
        <AiRoutingCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof AiRoutingCard> = {
  title: "Settings/Admin settings/AI/Model routing",
  component: AiRoutingCard,
};
export default meta;
type Story = StoryObj<typeof AiRoutingCard>;

// Every rung bound, mixing a local provider on the cheapest one — the shape a
// working installation is in, and the one where tier ORDER matters: the ladder
// reads cheapest to most capable, not alphabetically.
export const Bound: Story = { render: story(BOUND) };

// An installation that has bound nothing. Its AI lanes are absent, which is a
// state rather than a fault — nothing guesses which vendor an installation's
// text goes to — so this must not read as an error.
export const Unbound: Story = {
  render: story({
    profile: "eu_hosted",
    tiers: {},
    embeddings: { provider: "", model: "" },
  }),
};

// The sovereign posture, which is the one that CONSTRAINS: it admits no cloud
// provider at all, so a reader has to be able to tell at a glance that the
// binding below it is limited by the choice above it.
export const SovereignProfile: Story = {
  render: story({
    ...BOUND,
    profile: "sovereign",
    tiers: { local_small: { provider: "ollama", model: "gemma3" } },
  }),
};

// Read but not change. The binding stays fully legible — an operator who must
// ask somebody else to re-point a lane still needs to see where it points.
export const ReadOnlySeat: Story = { render: story(BOUND, READER) };

// No read grant. The card keeps its place and says the binding is withheld,
// because an absent card would claim this installation binds no models — a
// statement about the data where the truth is only about who may see it. It
// asks the server for nothing, so there is no 403 error box either.
export const Withheld: Story = { render: story(BOUND, NO_GRANT) };

// A vendor that cannot be asked. The list falls back to the price sheet and the
// field's own hint says which state it is in — a vendor being unreachable must
// not empty the picker or fail the form, because the box still binds anything
// typed into it.
export const VendorCannotBeAsked: Story = {
  render: story(BOUND, MANAGER, {
    gemini: { provider: "gemini", models: [], unavailable: "unreachable" },
  }),
};

// Dark. The profile and each tier's provider are carried by text on the panel
// surface; a token that flattens here costs the reader the one thing the card
// says.
export const BoundDark: Story = {
  globals: { theme: "dark" },
  render: story(BOUND),
};
