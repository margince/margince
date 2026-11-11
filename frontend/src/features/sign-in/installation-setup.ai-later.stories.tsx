// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AiSetupPanel } from "./installation-setup.ai-later";

// Settings → AI models on an installation whose admin skipped the first-run AI
// step: the same form, as a panel, with its verb in the panel's action band.
const meta: Meta<typeof AiSetupPanel> = {
  title: "Settings/AI/AI models/Set up AI",
  component: AiSetupPanel,
};
export default meta;

type Story = StoryObj<typeof AiSetupPanel>;

// The sheet a fresh installation is seeded with, so the plate shows prices.
function rate(model_id: string, lane: "chat" | "embeddings", input: string) {
  return {
    provider: "gemini",
    model_id,
    lane,
    input_per_mtok: input,
    output_per_mtok: lane === "chat" ? "0.40" : "0",
    cache_read_per_mtok: "0",
    cache_write_per_mtok: "0",
    effective_date: "2026-08-01",
  };
}
const SEEDED_SHEET = {
  data: [
    rate("gemini-3.1-flash-lite", "chat", "0.10"),
    rate("gemini-embedding-001", "embeddings", "0.15"),
  ],
};

export const NoModelYet: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ ai_routing: ["read", "update"] }),
      "GET /installation/setup": () =>
        jsonResponse({
          complete: false,
          steps: [
            { step: "ai_models", configured: false, blocking: true },
            { step: "oauth_app", configured: true, blocking: false },
          ],
        }),
      "GET /ai-model-rates": () => jsonResponse(SEEDED_SHEET),
    });
    return (
      <StoryProviders>
        <div className="settings-stack">
          <AiSetupPanel />
        </div>
      </StoryProviders>
    );
  },
};
