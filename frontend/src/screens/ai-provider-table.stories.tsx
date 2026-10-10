// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { meFixture } from "../app/mefixture";
import { Panel } from "../design-system/panel";
import type { ProviderUsage } from "./ai-provider-sheet";
import { ProviderTable } from "./ai-provider-table";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const key = (
  provider: string,
  configured: boolean,
  usable = configured,
  optional = false,
) => ({
  provider,
  configured,
  usable,
  optional,
  env_var: `${provider.toUpperCase()}_API_KEY`,
  credential_kind: "api_key" as const,
});

const PROVIDERS = [
  key("ollama", false),
  key("gemini", true),
  key("jev_compatible", false, true, true),
  key("anthropic", false),
  key("openai", true),
  key("fake", false, true),
];

const use = (tiers: string[]): ProviderUsage => ({
  for: tiers,
  baseUrls: [],
  models: [],
});

const USAGE = new Map([
  ["gemini", use(["cheap_cloud", "premium", "frontier", "embeddings"])],
  ["anthropic", use(["local_large"])],
  ["openai", use(["local_small"])],
]);

function Table({ canManage }: Readonly<{ canManage: boolean }>) {
  installFetchStub({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { ai_diagnostics: ["read"] } })),
    "GET /ai/call-stats": () =>
      jsonResponse({
        window: "7d",
        group: "provider",
        rows: [
          {
            key: "gemini",
            calls: 412,
            failed: 3,
            timeouts: 1,
            p50_ms: 900,
            p95_ms: 2600,
            tokens_in: 1000,
            tokens_out: 500,
            cost_microusd: 20000,
            unpriced: 0,
          },
        ],
      }),
  });
  return (
    <StoryProviders>
      <Panel title="Providers">
        <ProviderTable
          providers={PROVIDERS}
          usage={USAGE}
          health={[
            {
              provider: "openai",
              health: "unauthorized",
              since: "2026-10-05T10:00:00Z",
            },
          ]}
          canManage={canManage}
          onOpen={() => {}}
        />
      </Panel>
    </StoryProviders>
  );
}

const meta: Meta<typeof Table> = {
  title: "Settings/AI/AI models/Provider table",
  component: Table,
  args: { canManage: true },
};
export default meta;
type Story = StoryObj<typeof Table>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const rows = await canvas.findAllByTestId(/^ai-provider-row-/);
    await expect(
      rows.slice(0, 3).map((row) => row.getAttribute("data-testid")),
    ).toEqual([
      "ai-provider-row-anthropic",
      "ai-provider-row-openai",
      "ai-provider-row-gemini",
    ]);
    await canvas.findByText(/412 calls/);
  },
};

export const Dark: Story = { globals: { theme: "dark" } };

// A seat without the routing update grant opens a read-only sheet.
export const ReadOnlySeat: Story = {
  args: { canManage: false },
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("button", {
      name: "Open Google Gemini",
    });
  },
};

export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("table");
  },
};
