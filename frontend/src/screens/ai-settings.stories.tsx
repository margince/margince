// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { ProvidersStat, SpendStat } from "./ai-settings";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

/** The pair, composed here because no single page carries both any more. */
const AiReadings = () => (
  <>
    <SpendStat />
    <ProvidersStat />
  </>
);

// The two AI readings, side by side.
//
// They used to sit in a header above a five-tab strip; each of those tabs is its
// own page now, so the readings ride the two pages they belong to. The pair is
// still worth one story because of what follows.
//
// The two header readings are the point of the shape and they follow DIFFERENT
// grants — spend on `automation:update`, the vendor keys on `ai_routing:read` —
// so a seat holding one and not the other sees one figure and one withheld
// notice side by side. That pair is what these stories are for; each tab's own
// card has its own story.
const OPERATOR: GrantSpec = {
  ai_routing: ["read", "update"],
  automation: ["read", "update"],
  ai_model_rate: ["read"],
};
// Reaches the page on the automations read alone: no spend, no vendor keys.
const AUTOMATIONS_ONLY: GrantSpec = { automation: ["read"] };

const ROUTING = {
  profile: "eu_hosted",
  tiers: {
    local_small: { provider: "ollama", model: "gemma3" },
    cheap_cloud: { provider: "gemini", model: "gemini-3.1-flash-lite" },
    premium: { provider: "anthropic", model: "claude-opus-4-8" },
  },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
};

// A month with the budget a fifth spent, so the meter has something to draw and
// the estimate has something to price.
const USAGE = {
  days: [
    {
      date: "2026-09-01",
      tasks: [
        {
          task: "company.enrich",
          tier: "cheap_cloud",
          calls: 640,
          cached_hits: 96,
          tokens_in: 190000,
          tokens_out: 24000,
          cost_est_minor: 412,
        },
      ],
    },
  ],
  budget: {
    monthly_tokens: 1000000,
    spent_tokens: 214000,
    band: "normal",
    currency: "USD",
  },
};

function story(allow: GrantSpec) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow })),
      "GET /ai/routing": () => jsonResponse(ROUTING),
      "GET /ai/usage": () => jsonResponse(USAGE),
      "GET /ai-model-rates": () => jsonResponse({ data: [] }),
      "GET /ai/provider-keys": () =>
        jsonResponse({
          providers: [
            { provider: "gemini", configured: true, env_var: "GEMINI_API_KEY" },
            { provider: "openai", configured: true, env_var: "OPENAI_API_KEY" },
            // Bound by the premium lane and holding nothing — the join the
            // header's second line reports and the lane row's pill repeats.
            {
              provider: "anthropic",
              configured: false,
              env_var: "ANTHROPIC_API_KEY",
            },
          ],
        }),
      "GET /ai/calls": () =>
        jsonResponse({
          data: [
            {
              id: "01a0-0000-7000-8000-000000000001",
              occurred_at: "2026-09-01T14:22:09Z",
              task: "company.enrich",
              tier: "cheap_cloud",
              provider: "gemini",
              model_id: "gemini-3.1-flash-lite",
              served_model: "gemini-3.1-flash-lite",
              calls_attempted: 1,
              tokens_in: 3102,
              tokens_out: 480,
              reasoning_tokens: 0,
              cached_tokens: 0,
              latency_ms: 804,
              cache_hit: false,
              degraded: false,
              has_payload: false,
            },
          ],
          page: { next_cursor: null, has_more: false },
          tasks: ["company.enrich"],
          payload_capture_enabled: false,
        }),
    });
    return (
      <StoryProviders>
        <AiReadings />
      </StoryProviders>
    );
  };
}

// Across pages, not on one: `ProvidersStat` renders on Models & routing and
// `SpendStat` on AI usage. The pair is the point — the two readings an operator
// opens AI settings for — so the title says across rather than picking one and
// misplacing the other.
const meta: Meta<typeof AiReadings> = {
  title: "Settings/Across pages/AI readings",
  component: AiReadings,
};
export default meta;
type Story = StoryObj<typeof AiReadings>;

// The shape an operator holding everything sees: both readings answered, the
// strip open on Routing.
export const Operator: Story = { render: story(OPERATOR) };

// The seat the page's self-gating exists for. Both header readings say they are
// withheld rather than vanishing — an absent spend card would claim this
// installation had spent nothing, which is a statement about the DATA where the
// truth is only about who may read it.
export const WithheldReadings: Story = { render: story(AUTOMATIONS_ONLY) };

// Dark. The header's figures and the strip's current-tab underline are the two
// things a flattened token costs the reader here.
export const OperatorDark: Story = {
  globals: { theme: "dark" },
  render: story(OPERATOR),
};
