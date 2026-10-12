// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { meFixture } from "../app/mefixture";
import { ProviderCallsLine } from "./ai-call-figures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

function Lines() {
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
            calls: 98,
            failed: 11,
            timeouts: 4,
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
      <dl>
        <dt>Google Gemini</dt>
        <dd>
          <ProviderCallsLine provider="gemini" />
        </dd>
        <dt>Anthropic</dt>
        <dd>
          <ProviderCallsLine provider="anthropic" />
        </dd>
      </dl>
    </StoryProviders>
  );
}

const meta: Meta<typeof Lines> = {
  title: "Settings/AI/AI models/Provider calls line",
  component: Lines,
};
export default meta;
type Story = StoryObj<typeof Lines>;

export const CalledAndNot: Story = {
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText(/98 calls/);
  },
};
