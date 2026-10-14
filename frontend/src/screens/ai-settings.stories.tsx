// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { Panel } from "../design-system/panel";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { ProvidersStatus } from "./ai-settings";

// The status line at the top of the Providers panel on AI models. The key count
// rides `ai_routing:read`. The last call and the stand-in ride
// `ai_diagnostics:read`, so a seat holding one grant sees half a line.
const OPERATOR: GrantSpec = {
  ai_routing: ["read", "update"],
  ai_diagnostics: ["read"],
};

const ROUTING = {
  profile: "eu_hosted" as const,
  tiers: {
    cheap_cloud: { provider: "gemini", model: "gemini-3.1-flash-lite" },
    premium: { provider: "anthropic", model: "claude-opus-4-8" },
  },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
};

const PROVIDERS = [
  {
    provider: "gemini",
    configured: true,
    env_var: "GEMINI_API_KEY",
    usable: true,
    optional: false,
    credential_kind: "api_key" as const,
  },
  {
    provider: "openai",
    configured: true,
    env_var: "OPENAI_API_KEY",
    usable: true,
    optional: false,
    credential_kind: "api_key" as const,
  },
  // Bound by the premium lane and holding nothing.
  {
    provider: "anthropic",
    configured: false,
    env_var: "ANTHROPIC_API_KEY",
    usable: false,
    optional: false,
    credential_kind: "api_key" as const,
  },
];

const CALL = {
  id: "01a0-0000-7000-8000-000000000001",
  occurred_at: new Date(Date.now() - 20 * 60_000).toISOString(),
  kind: "completion",
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
  decision_attempted: false,
};

function Status({
  allow,
  calls,
  served,
}: Readonly<{ allow: GrantSpec; calls: unknown[]; served: string[] }>) {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({ allow })),
    "GET /ai/provider-health": () =>
      jsonResponse({ providers: [], configured: served }),
    "GET /ai/calls": () =>
      jsonResponse({
        data: calls,
        page: { next_cursor: null, has_more: false },
        tasks: [],
        task_options: [],
        payload_capture_enabled: false,
      }),
  });
  return (
    <StoryProviders>
      <Panel title="Providers">
        <ProvidersStatus providers={PROVIDERS} routing={ROUTING} />
      </Panel>
    </StoryProviders>
  );
}

const meta = {
  title: "Settings/AI/AI models/Providers status",
  component: Status,
  args: { allow: OPERATOR, calls: [CALL], served: ["anthropic", "gemini"] },
} satisfies Meta<typeof Status>;
export default meta;
type Story = StoryObj<typeof meta>;

// Two of three keyed, the bound one without a key in the danger ink, and when
// a vendor last answered.
export const Operator: Story = {
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText(/^last call/);
  },
};

export const OperatorDark: Story = { globals: { theme: "dark" } };

export const OperatorPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// A fresh installation: no call yet, said in words rather than an empty tail.
export const NeverCalled: Story = { args: { calls: [] } };

// The keys are readable and the trace is not, so the line stops at the count.
export const TraceWithheld: Story = {
  args: { allow: { ai_routing: ["read"] } },
};

// The process serves the offline fake: answers come from the test provider.
export const AnsweringFromTestProvider: Story = {
  args: { served: ["fake"] },
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText(
      "answering from the built-in test provider",
    );
  },
};
